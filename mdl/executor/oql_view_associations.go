// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Selecting a persistent entity's id under an alias does not give a view entity
// an ATTRIBUTE. It gives it an ASSOCIATION to that entity, whose name is the
// alias:
//
//	create view entity Sales.OrdersVE (
//	  order_date: DateTime              -- one attribute…
//	) as (
//	  from Sales."Order" as o
//	  select o.ID        as persistent_order   -- …but two columns
//	       , o.OrderDate as order_date
//	);
//
// Studio Pro creates the association member when the column is added, and there
// is no separate declaration anywhere — the column IS the declaration. mxcli
// therefore derives it the same way, which is also what makes describe → exec
// round-trip: the OQL carries the column, so re-executing a describe rebuilds
// the association without a second statement to keep in step.
//
// Three consequences, each of which was a defect before
// (ako/view-entity-examples FINDINGS §1):
//
//   - the id column has no declared attribute, so it must be skipped when
//     columns are aligned with attributes — otherwise every attribute after it
//     is checked against the wrong column;
//   - the association must actually be created, or mxbuild reports CE6770
//     "View Entity is out of sync with the OQL Query";
//   - it must carry an OqlViewAssociationSource, or mxbuild reports CE6771
//     "It is not possible to create associations to/from View Entities".

// viewAssociationColumn is one `<alias>.ID as <name>` select column.
type viewAssociationColumn struct {
	// Name is the select alias. It becomes BOTH the association's name and the
	// OqlViewAssociationSource's Reference — Studio Pro writes the same string
	// in both places.
	Name string
	// Entity is the qualified name of the entity whose id is selected, resolved
	// from the FROM/JOIN clause. Empty when the source alias could not be
	// resolved, in which case this is not treated as an association column at
	// all: guessing an entity would write a dangling reference.
	Entity string
	// Expr is the column as written, for diagnostics.
	Expr string
}

// oqlIDColumnRe matches a bare `<alias>.ID` select expression. Bare on purpose:
// `cast(m.ID as string) as MeterId` is a perfectly good STRING ATTRIBUTE
// carrying the object id as text — a different and often better design (one
// statement instead of two, no objects materialised in the client) — and it
// must not be mistaken for an association.
var oqlIDColumnRe = regexp.MustCompile(`(?i)^([A-Za-z_]\w*)\s*\.\s*id$`)

// viewAssociationColumns returns the select columns of oql that declare an
// association, in select order.
func viewAssociationColumns(oql string) []viewAssociationColumn {
	selectClause := extractSelectClause(oql)
	if selectClause == "" {
		return nil
	}
	aliases := extractAliasMap(oql)
	var out []viewAssociationColumn
	for _, expr := range parseSelectColumns(selectClause) {
		col, ok := viewAssociationColumnOf(expr, aliases)
		if ok {
			out = append(out, col)
		}
	}
	return out
}

// viewAssociationColumnOf decides whether one select column declares an
// association, given the query's alias → entity map.
func viewAssociationColumnOf(expr string, aliases map[string]string) (viewAssociationColumn, bool) {
	name := ""
	if m := oqlAliasSuffixRe.FindStringSubmatch(expr); m != nil {
		name = unquoteOQLIdent(m[1])
		expr = strings.TrimSpace(strings.TrimSuffix(expr, m[0]))
	}
	// No alias means no association: the alias is the association's name, and
	// MDL030 already reports a select column without one.
	if name == "" {
		return viewAssociationColumn{}, false
	}
	m := oqlIDColumnRe.FindStringSubmatch(strings.TrimSpace(expr))
	if m == nil {
		return viewAssociationColumn{}, false
	}
	entity := aliases[m[1]]
	if entity == "" {
		return viewAssociationColumn{}, false
	}
	return viewAssociationColumn{Name: name, Entity: entity, Expr: expr}, true
}

// isViewAssociationColumn reports whether one select column (as written, alias
// included) declares an association rather than an attribute.
func isViewAssociationColumn(expr string, aliases map[string]string) bool {
	_, ok := viewAssociationColumnOf(expr, aliases)
	return ok
}

// attributeSelectColumns drops the association columns from a select list, so
// what remains lines up ONE-TO-ONE with the declared attributes.
//
// This is the whole of the reported symptom 1. The alignment is positional, so
// an id column does not merely add an unchecked entry — it SHIFTS every column
// after it, and each remaining attribute is then reported against its
// neighbour's expression. With the id first, three correct attributes produced
// "attribute 'TotalKwh': declared as Decimal but OQL expression returns
// Integer"; with it last, "OQL select has 4 columns but 3 attributes declared".
// Neither describes the script.
func attributeSelectColumns(oql string, columns []string) []string {
	aliases := extractAliasMap(oql)
	out := make([]string, 0, len(columns))
	for _, c := range columns {
		if isViewAssociationColumn(c, aliases) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// syncViewEntityAssociations makes the view entity's associations match its OQL:
// one association per `<alias>.ID as <name>` column, and none left over from a
// column that has since been removed.
//
// It runs as part of CREATE VIEW ENTITY rather than being a statement of its
// own, because in Mendix the column IS the declaration — there is nothing else
// for a separate statement to say, and a second statement could disagree with
// the OQL, which is precisely the state (CE6770) this exists to prevent.
//
// Only associations mxcli can prove it owns are removed: one carrying an
// OqlViewAssociationSource, whose FROM end is this view entity. A hand-written
// association between other entities is never touched.
func syncViewEntityAssociations(ctx *ExecContext, moduleID model.ID, moduleName string,
	viewEntity *domainmodel.Entity, oql string) error {

	dm, err := ctx.Backend.GetDomainModel(moduleID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	wanted := viewAssociationColumns(oql)
	keep := make(map[string]bool, len(wanted))
	for _, c := range wanted {
		keep[c.Name] = true
	}

	// Drop the ones this view entity used to have and no longer declares.
	var dropped []string
	for _, a := range dm.Associations {
		if a.Source == domainmodel.OqlViewAssociationSource && a.ParentID == viewEntity.ID && !keep[a.Name] {
			if err := ctx.Backend.DeleteAssociation(dm.ID, a.ID); err != nil {
				return mdlerrors.NewBackend("remove stale view association", err)
			}
			dropped = append(dropped, a.Name)
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.Source == domainmodel.OqlViewAssociationSource && ca.ParentID == viewEntity.ID && !keep[ca.Name] {
			if err := ctx.Backend.DeleteCrossAssociation(dm.ID, ca.ID); err != nil {
				return mdlerrors.NewBackend("remove stale view association", err)
			}
			dropped = append(dropped, ca.Name)
		}
	}
	for _, name := range dropped {
		fmt.Fprintf(ctx.Output, "Removed view association: %s.%s (its OQL column is gone)\n", moduleName, name)
	}

	for _, col := range wanted {
		if err := upsertViewAssociation(ctx, moduleID, moduleName, viewEntity, col); err != nil {
			return err
		}
	}
	return nil
}

// upsertViewAssociation creates or updates the one association a column declares.
func upsertViewAssociation(ctx *ExecContext, moduleID model.ID, moduleName string,
	viewEntity *domainmodel.Entity, col viewAssociationColumn) error {

	parts := strings.Split(col.Entity, ".")
	if len(parts) != 2 {
		return mdlerrors.NewValidationf(
			"select column '%s as %s' names an association to %q, which is not a qualified entity name",
			col.Expr, col.Name, col.Entity)
	}
	targetModule, targetName := parts[0], parts[1]
	target, err := findEntity(ctx, targetModule, targetName)
	if err != nil {
		return mdlerrors.NewNotFoundMsg("entity", col.Entity, fmt.Sprintf(
			"select column '%s as %s' gives the view entity an association to %s, which does not exist",
			col.Expr, col.Name, col.Entity))
	}

	dm, err := ctx.Backend.GetDomainModel(moduleID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// An association is stored in the module of its FROM entity, which here is
	// always the view entity — so this domain model is always the right one, and
	// cross-module only ever refers to where the TARGET lives.
	crossModule := targetModule != moduleName

	for _, a := range dm.Associations {
		if a.Name != col.Name {
			continue
		}
		if a.ParentID != viewEntity.ID {
			return viewAssociationNameClash(moduleName, col.Name)
		}
		if crossModule {
			// The target moved to another module: the stored form has to change
			// type, which is a delete + create rather than an update.
			if err := ctx.Backend.DeleteAssociation(dm.ID, a.ID); err != nil {
				return mdlerrors.NewBackend("replace view association", err)
			}
			break
		}
		a.ChildID = target.ID
		a.Source = domainmodel.OqlViewAssociationSource
		a.ViewSourceReference = col.Name
		if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
			return mdlerrors.NewBackend("update view association", err)
		}
		return nil
	}
	for _, ca := range dm.CrossAssociations {
		if ca.Name != col.Name {
			continue
		}
		if ca.ParentID != viewEntity.ID {
			return viewAssociationNameClash(moduleName, col.Name)
		}
		if !crossModule {
			if err := ctx.Backend.DeleteCrossAssociation(dm.ID, ca.ID); err != nil {
				return mdlerrors.NewBackend("replace view association", err)
			}
			break
		}
		ca.ChildRef = col.Entity
		ca.Source = domainmodel.OqlViewAssociationSource
		ca.ViewSourceReference = col.Name
		if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
			return mdlerrors.NewBackend("update view association", err)
		}
		return nil
	}

	keep := &domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences}
	if crossModule {
		ca := &domainmodel.CrossModuleAssociation{
			Name:                col.Name,
			Type:                domainmodel.AssociationTypeReference,
			Owner:               domainmodel.AssociationOwnerDefault,
			StorageFormat:       domainmodel.StorageFormatColumn,
			ParentID:            viewEntity.ID,
			ChildRef:            col.Entity,
			ChildDeleteBehavior: keep,
			Source:              domainmodel.OqlViewAssociationSource,
			ViewSourceReference: col.Name,
		}
		if err := ctx.Backend.CreateCrossAssociation(dm.ID, ca); err != nil {
			return mdlerrors.NewBackend("create view association", err)
		}
	} else {
		a := &domainmodel.Association{
			Name:                col.Name,
			Type:                domainmodel.AssociationTypeReference,
			Owner:               domainmodel.AssociationOwnerDefault,
			StorageFormat:       domainmodel.StorageFormatColumn,
			ParentID:            viewEntity.ID,
			ChildID:             target.ID,
			ChildDeleteBehavior: keep,
			Source:              domainmodel.OqlViewAssociationSource,
			ViewSourceReference: col.Name,
		}
		if err := ctx.Backend.CreateAssociation(dm.ID, a); err != nil {
			return mdlerrors.NewBackend("create view association", err)
		}
	}
	fmt.Fprintf(ctx.Output, "Created view association: %s.%s -> %s\n", moduleName, col.Name, col.Entity)
	return nil
}

// viewAssociationNameClash reports an alias colliding with something else in the
// module. Mendix's own message is "Duplicate name '<x>' in module '<m>'.
// Entities, associations and enumerations cannot share names." — and it is
// CASE-INSENSITIVE, so `as meter` beside an entity called `Meter` collides
// (ako/view-entity-examples FINDINGS §1).
func viewAssociationNameClash(moduleName, name string) error {
	return mdlerrors.NewValidationf(
		"select alias '%s' cannot name this view entity's association: module '%s' already has a "+
			"different association of that name.\n    Rename the OQL alias — the alias IS the "+
			"association's name, and Mendix requires it to be unique among the module's entities, "+
			"associations and enumerations (case-insensitively)",
		name, moduleName)
}

// viewEntityAssociationRefusal is the CE6771 message. Mendix rejects an
// association with a view entity at either end, and there is nothing to write
// instead — a plain association there is the thing the platform refuses.
//
// The advice used to be "use a non-persistent entity with a real reference to
// the target instead", which was a workaround for a capability mxcli did not
// have. It has it now: selecting the target's id under an alias gives the view
// entity exactly this association, built the way Studio Pro builds it. Pointing
// at that is the difference between refusing the statement and refusing the
// goal.
func viewEntityAssociationRefusal(assocName, endpoint string) error {
	return mdlerrors.NewValidationf(
		"cannot create association %s: %s is a view entity — Mendix does not allow a plain "+
			"association to or from one (CE6771).\n"+
			"    A view entity gets an association by SELECTING THE TARGET'S ID under an alias, "+
			"which is also the association's name:\n"+
			"        select t.ID as %s, …    -- t is the target entity's alias in the FROM/JOIN\n"+
			"    mxcli creates the association from that column, with the "+
			"OqlViewAssociationSource the platform requires. There is no separate statement for it, "+
			"because in Mendix the column IS the declaration",
		assocName, endpoint, shortAssociationName(assocName))
}

// shortAssociationName is the bare name of a possibly-qualified association, for
// use inside an example.
func shortAssociationName(qualified string) string {
	if i := strings.LastIndex(qualified, "."); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

// validateViewAssociationNames reports select aliases that cannot become
// association names.
//
// The alias is the association's name, and Mendix requires that name to be
// unique among the module's entities, associations and enumerations —
// CASE-INSENSITIVELY. `select m.ID as meter` beside an entity called `Meter`
// fails with "Duplicate name 'Meter' in module 'Trends'. Entities, associations
// and enumerations cannot share names." (FINDINGS §1, "found the hard way").
//
// Reported here rather than left to mxbuild because the fix is a rename, and a
// rename is much cheaper before the name has spread through pages and
// microflows. Shared by `check` and `exec` so the two cannot disagree.
func validateViewAssociationNames(ctx *ExecContext, moduleName, viewEntityName, oql string,
	scriptEntities map[string]bool) []string {

	cols := viewAssociationColumns(oql)
	if len(cols) == 0 {
		return nil
	}
	taken := map[string]string{} // lower-case name -> what holds it

	if dms, err := ctx.Backend.ListDomainModels(); err == nil {
		if h, herr := getHierarchy(ctx); herr == nil {
			for _, dm := range dms {
				if h.GetModuleName(dm.ContainerID) != moduleName {
					continue
				}
				for _, e := range dm.Entities {
					if e.Name != viewEntityName {
						taken[strings.ToLower(e.Name)] = "an entity"
					}
				}
				for _, a := range dm.Associations {
					if a.Source != domainmodel.OqlViewAssociationSource {
						taken[strings.ToLower(a.Name)] = "an association"
					}
				}
				for _, ca := range dm.CrossAssociations {
					if ca.Source != domainmodel.OqlViewAssociationSource {
						taken[strings.ToLower(ca.Name)] = "an association"
					}
				}
			}
		}
	}
	// Entities the same script creates are not in the project yet.
	for qn := range scriptEntities {
		parts := strings.SplitN(qn, ".", 2)
		if len(parts) == 2 && parts[0] == moduleName && parts[1] != viewEntityName {
			taken[strings.ToLower(parts[1])] = "an entity"
		}
	}

	var out []string
	seen := map[string]bool{}
	for _, c := range cols {
		lower := strings.ToLower(c.Name)
		if what, clash := taken[lower]; clash {
			out = append(out, fmt.Sprintf(
				"select alias '%s' cannot be used: module '%s' already has %s of that name, and "+
					"Mendix reports \"Duplicate name\" for entities, associations and enumerations "+
					"sharing one (case-insensitively). The alias IS the association's name, so rename "+
					"the alias — e.g. '%sRef'",
				c.Name, moduleName, what, c.Name))
			continue
		}
		if seen[lower] {
			out = append(out, fmt.Sprintf(
				"select alias '%s' is used by two association columns — each one names a separate "+
					"association, so the names have to differ", c.Name))
		}
		seen[lower] = true
	}
	return out
}
