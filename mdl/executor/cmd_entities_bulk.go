// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// execAlterEntities handles the bulk form:
//
//	ALTER ENTITIES [IN <module>] ADD ATTRIBUTE ... [WHERE PERSISTENT|NON-PERSISTENT]
//
// It resolves the target set and then runs each action through execAlterEntity
// unchanged. That delegation is the whole design: the single-entity path
// already carries the reserved-word refusals, the access-rule reconciliation,
// the IF NOT EXISTS skip and the write elision, and a second implementation
// would have to be kept in step with all of it. One statement per entity is
// what this replaces, not the code that executes one.
func execAlterEntities(ctx *ExecContext, s *ast.AlterEntitiesStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	targets, err := resolveBulkEntityTargets(ctx, s)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		// Not an error: a module whose entities all match already, or a filter
		// that excludes everything, is a legitimate no-op in a re-run script.
		fmt.Fprintf(ctx.Output, "No entities matched%s\n", bulkScopeSuffix(s))
		return nil
	}

	for _, qn := range targets {
		for _, action := range s.Actions {
			// Copy per entity: execAlterEntity reads Name, and the parsed
			// action is shared across every target.
			perEntity := *action
			perEntity.Name = qn
			if err := execAlterEntity(ctx, &perEntity); err != nil {
				return fmt.Errorf("%s: %w", qn, err)
			}
		}
	}

	fmt.Fprintf(ctx.Output, "Altered %d entit%s%s\n",
		len(targets), plural(len(targets), "y", "ies"), bulkScopeSuffix(s))
	return nil
}

// resolveBulkEntityTargets returns the qualified names the statement applies
// to, sorted, so a run is deterministic and its output diffable.
func resolveBulkEntityTargets(ctx *ExecContext, s *ast.AlterEntitiesStmt) ([]ast.QualifiedName, error) {
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return nil, mdlerrors.NewBackend("list modules", err)
	}

	var out []ast.QualifiedName
	var skipped []string
	// qualified name -> its generalization's qualified name ("" at the root)
	parentOf := map[string]string{}
	matchedModule := false
	for _, m := range modules {
		if s.Module != "" && !strings.EqualFold(m.Name, s.Module) {
			continue
		}
		matchedModule = true

		// Sweeping the whole project must not edit System or a Marketplace
		// module: an upgrade replaces the module and the attribute goes with
		// it, so the write is silently undone later rather than refused now.
		// Naming the module explicitly is taken as meaning it -- the same
		// division `mxcli layout` makes.
		if s.Module == "" && isUneditableModule(m) {
			skipped = append(skipped, m.Name)
			continue
		}

		dm, err := ctx.Backend.GetDomainModel(m.ID)
		if err != nil {
			return nil, mdlerrors.NewBackend("get domain model", err)
		}
		if dm == nil {
			continue
		}
		for _, e := range dm.Entities {
			if !entityMatchesFilter(e, s.Filter) {
				continue
			}
			out = append(out, ast.QualifiedName{Module: m.Name, Name: e.Name})
			parentOf[m.Name+"."+e.Name] = e.GeneralizationRef
		}
	}

	if s.Module != "" && !matchedModule {
		return nil, mdlerrors.NewNotFoundMsg("module", s.Module,
			fmt.Sprintf("module not found: %s", s.Module))
	}

	if len(skipped) > 0 {
		sort.Strings(skipped)
		fmt.Fprintf(ctx.Output, "Skipped %d System/Marketplace module(s): %s\n",
			len(skipped), strings.Join(skipped, ", "))
	}

	out = dropInheritedTargets(out, parentOf)

	sort.Slice(out, func(i, j int) bool {
		if out[i].Module != out[j].Module {
			return out[i].Module < out[j].Module
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// entityMatchesFilter applies the WHERE clause. A view entity is neither
// persistent nor non-persistent in the sense this filter means -- its rows come
// from an OQL query -- so it is excluded from both, rather than silently
// treated as one of them.
func entityMatchesFilter(e *domainmodel.Entity, f ast.EntityPersistenceFilter) bool {
	if e == nil {
		return false
	}
	// A view entity is never a target, with or without a filter: its columns
	// are defined by its OQL select list, and adding an attribute gives
	// CE6770 "View Entity is out of sync with the OQL Query".
	if e.Source != "" || e.OqlQuery != "" {
		return false
	}
	switch f {
	case ast.EntityFilterPersistent:
		return e.Persistable
	case ast.EntityFilterNonPersistent:
		return !e.Persistable
	default:
		return true
	}
}

func bulkScopeSuffix(s *ast.AlterEntitiesStmt) string {
	scope := ""
	if s.Module != "" {
		scope = " in " + s.Module
	}
	switch s.Filter {
	case ast.EntityFilterPersistent:
		scope += " (persistent only)"
	case ast.EntityFilterNonPersistent:
		scope += " (non-persistent only)"
	}
	return scope
}

// isUneditableModule reports a module whose contents an upgrade replaces.
func isUneditableModule(m *model.Module) bool {
	if m == nil {
		return false
	}
	if m.Name == "System" {
		return true
	}
	return m.FromAppStore || strings.TrimSpace(m.AppStoreGuid) != ""
}

// dropInheritedTargets removes an entity whose ANCESTOR is also a target.
//
// Mendix refuses an attribute name that appears on both a generalization and
// its specialization -- CE0069 "Duplicate member name" -- and adding it to the
// parent is what the author wants anyway, since the child inherits it. Without
// this, ALTER ENTITIES over a module with any inheritance produced a project
// that would not build, which is how DmTest.Vehicle/Truck/PassengerCar caught
// it. The child is dropped rather than the parent: dropping the parent would
// leave the attribute off entities that have no specialization.
func dropInheritedTargets(targets []ast.QualifiedName, parentOf map[string]string) []ast.QualifiedName {
	inSet := make(map[string]bool, len(targets))
	for _, t := range targets {
		inSet[t.Module+"."+t.Name] = true
	}

	kept := targets[:0]
	for _, t := range targets {
		covered := false
		// Walk up the chain; a cycle is impossible in a loadable model, but
		// bound the walk anyway rather than trust that.
		for p, hops := parentOf[t.Module+"."+t.Name], 0; p != "" && hops < 64; hops++ {
			if inSet[p] {
				covered = true
				break
			}
			p = parentOf[p]
		}
		if !covered {
			kept = append(kept, t)
		}
	}
	return kept
}
