// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// unquoteIdentifier strips surrounding double-quotes or backticks from a quoted identifier.
func unquoteIdentifier(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '`' && s[len(s)-1] == '`') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// unquoteQualifiedName strips quotes from each segment of a dotted qualified name.
func unquoteQualifiedName(s string) string {
	parts := strings.Split(s, ".")
	for i, p := range parts {
		parts[i] = unquoteIdentifier(p)
	}
	return strings.Join(parts, ".")
}

// resolveAttributePath resolves a short attribute name to a fully qualified name
// using the current entity context. If the attribute already has dots or no entity
// context is available, the attribute is returned as-is.
func (pb *pageBuilder) resolveAttributePath(attr string) string {
	if attr == "" {
		return ""
	}
	attr = storedSystemMemberName(attr)
	// If the attribute already contains a dot, it's already qualified
	if strings.Contains(attr, ".") {
		return attr
	}
	// If we have an entity context, prefix the attribute with it — but with the
	// entity that actually DECLARES it, which for an inherited attribute is an
	// ancestor rather than the context entity itself.
	if pb.entityContext != "" {
		if declaring, ok := pb.declaringEntityFor(pb.entityContext, attr); ok {
			return declaring + "." + attr
		}
		return pb.entityContext + "." + attr
	}
	return attr
}

// declaringEntityFor returns the entity in entityQN's generalization chain that
// declares attrName — entityQN itself when the attribute is its own.
//
// Mendix stores a page's attribute reference against the declaring entity. A
// reference qualified with a specialization that merely inherits the attribute
// is dangling, and the build fails with
//
//	[CE1613] "The selected attribute 'Module.Sub.Attr' no longer exists."
//
// which reads as if the attribute had been deleted; it never existed there.
// Entity access rules resolve inherited members correctly, so this was the page
// layer alone. (mxcli-todo findings #12)
//
// ok is false when nothing in the chain declares the name — an unknown
// attribute, or a domain model we cannot read. The caller then keeps today's
// behaviour rather than inventing a qualification.
func (pb *pageBuilder) declaringEntityFor(entityQN, attrName string) (string, bool) {
	if entityQN == "" || attrName == "" {
		return "", false
	}
	// Resolution is best-effort: without a model to consult (no backend and
	// nothing cached — e.g. a unit test building widgets in isolation) keep the
	// caller's plain context qualification rather than panicking on the lookup.
	if pb.backend == nil && (pb.execCache == nil || pb.execCache.domainModels == nil) {
		return "", false
	}
	owners, parents, err := pb.entityAttributeOwners()
	if err != nil {
		return "", false
	}
	lower := strings.ToLower(attrName)
	seen := map[string]bool{}
	for cur := entityQN; cur != "" && !seen[cur]; cur = parents[cur] {
		seen[cur] = true
		if attrs, ok := owners[cur]; ok && attrs[lower] {
			return cur, true
		}
	}
	return "", false
}

// entityAttributeOwners indexes, for every entity in the project, the set of
// attribute names it declares itself, plus each entity's direct parent.
func (pb *pageBuilder) entityAttributeOwners() (owners map[string]map[string]bool, parents map[string]string, err error) {
	dms, err := pb.getDomainModels()
	if err != nil {
		return nil, nil, err
	}
	h, err := pb.getHierarchy()
	if err != nil {
		return nil, nil, err
	}
	owners = make(map[string]map[string]bool, len(dms)*8)
	parents = make(map[string]string)
	for _, dm := range dms {
		mod := h.GetModuleName(dm.ContainerID)
		for _, e := range dm.Entities {
			qn := mod + "." + e.Name
			attrs := make(map[string]bool, len(e.Attributes))
			for _, a := range e.Attributes {
				attrs[strings.ToLower(a.Name)] = true
			}
			owners[qn] = attrs
			if e.GeneralizationRef != "" {
				parents[qn] = e.GeneralizationRef
			}
		}
	}
	return owners, parents, nil
}

// rejectAssociationAsAttribute refuses a widget property that is attribute-typed
// but was given the name of an ASSOCIATION — `column c (attribute: Order_Customer)`.
//
// The reference cannot be represented. mxcli qualified the name like an attribute
// and wrote `DomainModels$AttributeRef{Attribute: "Mod.Order.Order_Customer"}`, so
// the build failed CE1613 "The selected attribute … no longer exists" — and there
// is no correct BSON to write instead: `CustomWidgets$WidgetValue.AttributeRef` is
// typed `AttributeRef`, not the polymorphic `MemberRef`. Storing an
// `AssociationRef` there makes the project UNLOADABLE ("Object of type
// 'AssociationRef' cannot be converted to type 'AttributeRef'"), and the WidgetValue
// has no association-valued property at all — `Mendix.Modeler.WebUI.dll`, which
// defines the type, carries no `AssociationRef` member.
//
// The DataGrid column's `<associationTypes>Reference/ReferenceSet</associationTypes>`
// is what makes this look supported. It is not permission to bind the reference; it
// is permission for the attribute PATH to traverse one — `attribute: Assoc/Attr`,
// which mxcli already writes as an AttributeRef with association steps. (issue #830)
//
// where names the binding for the message (e.g. "column `colCustomer`").
func (pb *pageBuilder) rejectAssociationAsAttribute(name, entityContext, where string) error {
	// A path (`Assoc/Attr`) is the SUPPORTED form; a `$var` reference and an
	// empty binding are not ours to judge here.
	if name == "" || strings.ContainsAny(name, "/$") || entityContext == "" {
		return nil
	}
	// Deciding attribute-vs-association needs the model; without one (no backend
	// and nothing cached — a unit test building widgets in isolation) let the
	// binding through rather than guessing, exactly as declaringEntityFor does.
	if pb.backend == nil && (pb.execCache == nil || pb.execCache.domainModels == nil) {
		return nil
	}
	name = storedSystemMemberName(name)
	// A three-part name is already an explicit Module.Entity.Attribute.
	if strings.Count(name, ".") >= 2 {
		return nil
	}
	// An attribute wins: entity members share one namespace, so a name that
	// resolves as an attribute anywhere in the generalization chain is not an
	// association.
	if _, ok := pb.declaringEntityFor(entityContext, name); ok {
		return nil
	}
	assocQN := pb.resolveAssociationPathIn(name, entityContext)
	fromEntity, toEntity, ok := pb.associationEndpoints(assocQN)
	if !ok || !pb.entityInChain(entityContext, fromEntity) {
		return nil
	}
	leaf := toEntity
	if idx := strings.LastIndex(leaf, "."); idx >= 0 {
		leaf = leaf[idx+1:]
	}
	return mdlerrors.NewValidationf(
		"%s binds `%s`, which is an association (%s → %s), not an attribute — "+
			"Mendix cannot store a reference in an attribute-typed widget property and the build fails "+
			"CE1613 \"The selected attribute '%s.%s' no longer exists\". "+
			"To SHOW a value from the associated object, traverse the reference: `attribute: %s/<Attr>` "+
			"(e.g. `%s/Name`). To FILTER the grid by the reference, use the drop-down filter's association mode: "+
			"`dropdownfilter f (Association: %s, datasource: database %s, CaptionAttribute: <Attr>)`",
		where, name, fromEntity, toEntity, entityContext, name,
		name, name, assocQN, toEntity)
}

// entityInChain reports whether want is entityQN or one of its ancestors, so a
// member declared on a generalization still counts as reachable from the
// specialization the widget is bound to.
func (pb *pageBuilder) entityInChain(entityQN, want string) bool {
	if want == "" {
		return false
	}
	if entityQN == want {
		return true
	}
	_, parents, err := pb.entityAttributeOwners()
	if err != nil {
		return false
	}
	seen := map[string]bool{}
	for cur := entityQN; cur != "" && !seen[cur]; cur = parents[cur] {
		seen[cur] = true
		if cur == want {
			return true
		}
	}
	return false
}

// systemMemberBindingNames maps the name an audit member is DECLARED under to
// the name Mendix actually stores it as.
//
// `alter entity … add attribute CreatedDate: AutoCreatedDate` is the spelling
// mxcli requires (it rejects any other declared name, telling you to use this
// one), but the member is stored as `createdDate` — so binding a widget to
// `CreatedDate`, the same name you just declared, failed the build with CE1613
// "The selected attribute … no longer exists" while the undocumented lowercase
// form worked. Accept the declared spelling and write the stored one.
// (issuetracker #19)
var systemMemberBindingNames = map[string]string{
	"createddate": "createdDate",
	"changeddate": "changedDate",
	"changedby":   "changedBy",
	"owner":       "owner",
}

// storedSystemMemberName maps a bare audit-member name to its stored spelling,
// leaving every other name (and any qualified or association path) untouched.
func storedSystemMemberName(attr string) string {
	if strings.ContainsAny(attr, "./$") {
		return attr
	}
	if stored, ok := systemMemberBindingNames[strings.ToLower(attr)]; ok {
		return stored
	}
	return attr
}

// resolveAssociationPath resolves a short association name to a fully qualified name.
// Associations are module-level objects, so the path is Module.AssociationName (2-part).
// If the name already contains a dot, it's returned as-is.
func (pb *pageBuilder) resolveAssociationPath(assocName string) string {
	return pb.resolveAssociationPathIn(assocName, pb.entityContext)
}

// resolveAssociationPathIn is resolveAssociationPath against an explicit entity
// context. Callers that bind a member of a *containing* entity — while
// pb.entityContext already points at their own data source — must pass that
// containing entity, or a bare name is qualified with the wrong module.
func (pb *pageBuilder) resolveAssociationPathIn(assocName, entityContext string) string {
	if assocName == "" {
		return ""
	}
	// If already qualified (contains a dot), return as-is
	if strings.Contains(assocName, ".") {
		return assocName
	}
	// Extract module name from entity context (e.g., "PgTest.Order" → "PgTest")
	if entityContext != "" {
		parts := strings.SplitN(entityContext, ".", 2)
		if len(parts) >= 1 {
			return parts[0] + "." + assocName
		}
	}
	return assocName
}

// resolveSnippetRef resolves a snippet qualified name to its ID.
func (pb *pageBuilder) resolveSnippetRef(snippetRef string) (model.ID, error) {
	if snippetRef == "" {
		return "", mdlerrors.NewValidation("empty snippet reference")
	}

	snippetRef = unquoteQualifiedName(snippetRef)
	parts := strings.Split(snippetRef, ".")
	var moduleName, snippetName string
	if len(parts) >= 2 {
		moduleName = parts[0]
		snippetName = parts[len(parts)-1]
	} else {
		snippetName = snippetRef
	}

	// First, check if the snippet was created during this session
	// (not yet visible via reader)
	if pb.execCache != nil && pb.execCache.createdSnippets != nil {
		if info, ok := pb.execCache.createdSnippets[snippetRef]; ok {
			return info.ID, nil
		}
		if moduleName != "" {
			if info, ok := pb.execCache.createdSnippets[moduleName+"."+snippetName]; ok {
				return info.ID, nil
			}
		}
	}

	snippets, err := pb.backend.ListSnippets()
	if err != nil {
		return "", err
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, s := range snippets {
		modID := h.FindModuleID(s.ContainerID)
		modName := h.GetModuleName(modID)
		if s.Name == snippetName && (moduleName == "" || modName == moduleName) {
			return s.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("snippet", snippetRef)
}

func (pb *pageBuilder) resolveMicroflow(qualifiedName string) (model.ID, error) {
	qualifiedName = unquoteQualifiedName(qualifiedName)
	// Parse qualified name
	parts := strings.Split(qualifiedName, ".")
	if len(parts) < 2 {
		return "", mdlerrors.NewValidationf("invalid microflow name: %s", qualifiedName)
	}
	moduleName := parts[0]
	mfName := strings.Join(parts[1:], ".")

	// First, check if the microflow was created during this session
	// (not yet visible via reader)
	if pb.execCache != nil && pb.execCache.createdMicroflows != nil {
		if info, ok := pb.execCache.createdMicroflows[qualifiedName]; ok {
			return info.ID, nil
		}
	}

	// Get microflows from backend
	mfs, err := pb.getMicroflows()
	if err != nil {
		return "", mdlerrors.NewBackend("list microflows", err)
	}

	// Use hierarchy to resolve module names (handles microflows in folders)
	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find matching microflow
	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == moduleName && mf.Name == mfName {
			return mf.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("microflow", qualifiedName)
}

func (pb *pageBuilder) resolvePageRef(pageRef string) (model.ID, error) {
	if pageRef == "" {
		return "", mdlerrors.NewValidation("empty page reference")
	}

	pageRef = unquoteQualifiedName(pageRef)
	parts := strings.Split(pageRef, ".")
	var moduleName, pageName string
	if len(parts) >= 2 {
		moduleName = parts[0]
		pageName = parts[len(parts)-1]
	} else {
		pageName = pageRef
	}

	// First, check if the page was created during this session
	// (not yet visible via reader)
	if pb.execCache != nil && pb.execCache.createdPages != nil {
		if info, ok := pb.execCache.createdPages[pageRef]; ok {
			return info.ID, nil
		}
		// Also check with module prefix if not found
		if moduleName != "" {
			if info, ok := pb.execCache.createdPages[moduleName+"."+pageName]; ok {
				return info.ID, nil
			}
		}
	}

	pgs, err := pb.getPages()
	if err != nil {
		return "", err
	}

	h, err := pb.getHierarchy()
	if err != nil {
		return "", mdlerrors.NewBackend("build hierarchy", err)
	}

	for _, p := range pgs {
		modID := h.FindModuleID(p.ContainerID)
		modName := h.GetModuleName(modID)
		if p.Name == pageName && (moduleName == "" || modName == moduleName) {
			return p.ID, nil
		}
	}

	return "", mdlerrors.NewNotFound("page", pageRef)
}
