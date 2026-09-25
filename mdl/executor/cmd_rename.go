// SPDX-License-Identifier: Apache-2.0

// Package executor — RENAME commands (entity, module)
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// execRename handles RENAME statements for all document types.
func execRename(ctx *ExecContext, s *ast.RenameStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	switch s.ObjectType {
	case "entity":
		return execRenameEntity(ctx, s)
	case "microflow":
		return execRenameDocument(ctx, s, "microflow")
	case "nanoflow":
		return execRenameDocument(ctx, s, "nanoflow")
	case "page":
		return execRenameDocument(ctx, s, "page")
	case "enumeration":
		return execRenameEnumeration(ctx, s)
	case "association":
		return execRenameAssociation(ctx, s)
	case "constant":
		return execRenameDocument(ctx, s, "constant")
	case "workflow":
		return execRenameDocument(ctx, s, "workflow")
	case "javaaction":
		return execRenameJavaAction(ctx, s)
	case "module":
		return execRenameModule(ctx, s)
	default:
		return mdlerrors.NewUnsupported(fmt.Sprintf("rename not supported for %s", s.ObjectType))
	}
}

// execRenameEntity renames an entity and updates all BY_NAME references.
func execRenameEntity(ctx *ExecContext, s *ast.RenameStmt) error {
	// Find the entity
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	found := false
	collision := false
	for _, ent := range dm.Entities {
		if ent.Name == s.Name.Name {
			found = true
		} else if ent.Name == s.NewName {
			collision = true
		}
	}
	if !found {
		return mdlerrors.NewNotFound("entity", s.Name.String())
	}
	if collision {
		return mdlerrors.NewValidationf("entity %s already exists in module %s", s.NewName, s.Name.Module)
	}

	oldQualifiedName := s.Name.Module + "." + s.Name.Name
	newQualifiedName := s.Name.Module + "." + s.NewName

	// Scan for references
	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	// Update the entity name in the domain model
	for _, ent := range dm.Entities {
		if ent.Name == s.Name.Name {
			ent.Name = s.NewName
			repointEntitySelfRefs(ent, oldQualifiedName, newQualifiedName)
			break
		}
	}
	if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
		return mdlerrors.NewBackend("update entity name", err)
	}

	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)

	fmt.Fprintf(ctx.Output, "Renamed entity: %s → %s\n", oldQualifiedName, newQualifiedName)
	if len(hits) > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", totalRefCount(hits), len(hits))
	}
	return nil
}

// repointEntitySelfRefs rewrites the qualified names the entity uses to refer to
// its OWN members, which embed the entity name that has just changed.
//
// It is needed because of a clobber, not because the sweep missed them: the
// project-wide RenameReferences pass above does rewrite these names in the raw
// unit, and then UpdateDomainModel persists the semantic model read BEFORE it and
// puts the stale ones back. Measured on a real 11.13 app, the report and the errors
// line up exactly — "Updated 3 reference(s) in 1 document(s)" followed by three
// CE1613s at "Access rule of entity" and "Validation rule of entity" for the entity
// just renamed. Re-pointing here makes the persist agree with the sweep instead of
// undoing it.
//
// Leaving them stale is worse than a dangling string: entityToGen's
// syncMemberAccesses matches existing entries BY qualified name, so a stale
// `Module.Old.Attr` never equals the rebuilt `Module.New.Attr` and it appends the
// new entry while keeping the old — the entity ends up carrying every member twice,
// half of the entries dangling. DESCRIBE renders members bare, so the duplication is
// the only visible trace. This is the RENAME sibling of the MOVE ENTITY defect in
// ako/mxcli#605, where the same names went stale on the module prefix instead.
//
// Only names qualified by the ENTITY are rewritten. An association member is named
// `Module.Association` — it carries no entity name — so a blanket prefix swap would
// corrupt a reference that is still correct.
func repointEntitySelfRefs(ent *domainmodel.Entity, oldQualifiedName, newQualifiedName string) {
	oldPrefix, newPrefix := oldQualifiedName+".", newQualifiedName+"."
	repoint := func(name string) string {
		if strings.HasPrefix(name, oldPrefix) {
			return newPrefix + name[len(oldPrefix):]
		}
		return name
	}
	for _, ar := range ent.AccessRules {
		for _, ma := range ar.MemberAccesses {
			ma.AttributeName = repoint(ma.AttributeName)
		}
	}
	for _, vr := range ent.ValidationRules {
		vr.AttributeID = model.ID(repoint(string(vr.AttributeID)))
	}
}

// execRenameModule renames a module and updates all BY_NAME references with the module prefix.
func execRenameModule(ctx *ExecContext, s *ast.RenameStmt) error {
	oldModuleName := s.Name.Module
	newModuleName := s.NewName

	module, err := findModule(ctx, oldModuleName)
	if err != nil {
		return err
	}

	// Scan for all references with the old module prefix
	// Module rename replaces "OldModule." with "NewModule." in all qualified names
	hits, err := ctx.Backend.RenameReferences(oldModuleName+".", newModuleName+".", s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	// Also scan for exact module name matches (e.g., in navigation, security role refs)
	exactHits, err := ctx.Backend.RenameReferences(oldModuleName, newModuleName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan exact module references", err)
	}

	// Merge hit lists (deduplicate by unit ID)
	allHits := mergeHits(hits, exactHits)

	if s.DryRun {
		printRenameReport(ctx, oldModuleName, newModuleName, allHits)
		return nil
	}

	// Update the module name
	module.Name = newModuleName
	if err := ctx.Backend.UpdateModule(module); err != nil {
		return mdlerrors.NewBackend("update module name", err)
	}

	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)

	fmt.Fprintf(ctx.Output, "Renamed module: %s → %s\n", oldModuleName, newModuleName)
	if len(allHits) > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", totalRefCount(allHits), len(allHits))
	}
	return nil
}

// execRenameDocument handles RENAME MICROFLOW/NANOFLOW/PAGE/CONSTANT.
// These are standalone documents where the Name field is in the document BSON itself.
// The reference scanner handles updating all BY_NAME references, and then we update
// the document's own Name field via a raw BSON rewrite.
func execRenameDocument(ctx *ExecContext, s *ast.RenameStmt, docType string) error {
	oldQualifiedName := s.Name.Module + "." + s.Name.Name
	newQualifiedName := s.Name.Module + "." + s.NewName

	// Verify the document exists
	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	found := false
	collision := false
	switch docType {
	case "microflow":
		mfs, _ := ctx.Backend.ListMicroflows()
		for _, mf := range mfs {
			modID := h.FindModuleID(mf.ContainerID)
			if h.GetModuleName(modID) != s.Name.Module {
				continue
			}
			if mf.Name == s.Name.Name {
				found = true
			} else if mf.Name == s.NewName {
				collision = true
			}
		}
	case "nanoflow":
		nfs, _ := ctx.Backend.ListNanoflows()
		for _, nf := range nfs {
			modID := h.FindModuleID(nf.ContainerID)
			if h.GetModuleName(modID) != s.Name.Module {
				continue
			}
			if nf.Name == s.Name.Name {
				found = true
			} else if nf.Name == s.NewName {
				collision = true
			}
		}
	case "page":
		pgs, _ := ctx.Backend.ListPages()
		for _, pg := range pgs {
			modID := h.FindModuleID(pg.ContainerID)
			if h.GetModuleName(modID) != s.Name.Module {
				continue
			}
			if pg.Name == s.Name.Name {
				found = true
			} else if pg.Name == s.NewName {
				collision = true
			}
		}
	case "constant":
		cs, _ := ctx.Backend.ListConstants()
		for _, c := range cs {
			modID := h.FindModuleID(c.ContainerID)
			if h.GetModuleName(modID) != s.Name.Module {
				continue
			}
			if c.Name == s.Name.Name {
				found = true
			} else if c.Name == s.NewName {
				collision = true
			}
		}
	case "workflow":
		wfs, _ := ctx.Backend.ListWorkflows()
		for _, wf := range wfs {
			modID := h.FindModuleID(wf.ContainerID)
			if h.GetModuleName(modID) != s.Name.Module {
				continue
			}
			if wf.Name == s.Name.Name {
				found = true
			} else if wf.Name == s.NewName {
				collision = true
			}
		}
	}

	if !found {
		return mdlerrors.NewNotFound(s.ObjectType, oldQualifiedName)
	}
	if collision {
		return mdlerrors.NewValidationf("%s %s already exists in module %s", docType, s.NewName, s.Name.Module)
	}

	// The reference scanner will also update the document's own Name field
	// when it matches the old qualified name. But the Name field is just the
	// simple name (e.g., "OldName"), not the qualified name. So we need to
	// handle it separately — the scanner updates cross-references, and we
	// update the Name field directly.
	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	// Update the document's own Name field via the raw BSON name updater
	if err := ctx.Backend.RenameDocumentByName(s.Name.Module, s.Name.Name, s.NewName); err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("rename %s", docType), err)
	}

	invalidateHierarchy(ctx)

	fmt.Fprintf(ctx.Output, "Renamed %s: %s → %s\n", docType, oldQualifiedName, newQualifiedName)
	if len(hits) > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", totalRefCount(hits), len(hits))
	}
	return nil
}

// execRenameEnumeration renames an enumeration and updates all references.
func execRenameEnumeration(ctx *ExecContext, s *ast.RenameStmt) error {
	// Platform built-in, no stored unit to rename (#1102).
	if err := refuseSystemEnumerationWrite("rename enumeration", s.Name); err != nil {
		return err
	}

	oldQualifiedName := s.Name.Module + "." + s.Name.Name
	newQualifiedName := s.Name.Module + "." + s.NewName

	// Verify it exists
	enums, err := ctx.Backend.ListEnumerations()
	if err != nil {
		return mdlerrors.NewBackend("list enumerations", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	found := false
	collision := false
	for _, en := range enums {
		modID := h.FindModuleID(en.ContainerID)
		if h.GetModuleName(modID) != s.Name.Module {
			continue
		}
		if en.Name == s.Name.Name {
			found = true
		} else if en.Name == s.NewName {
			collision = true
		}
	}
	if !found {
		return mdlerrors.NewNotFound("enumeration", oldQualifiedName)
	}
	if collision {
		return mdlerrors.NewValidationf("enumeration %s already exists in module %s", s.NewName, s.Name.Module)
	}

	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	// Update enumeration name via raw BSON
	if err := ctx.Backend.RenameDocumentByName(s.Name.Module, s.Name.Name, s.NewName); err != nil {
		return mdlerrors.NewBackend("rename enumeration", err)
	}

	// Also update enumeration refs in domain models (attribute types store qualified enum names)
	if err := ctx.Backend.UpdateEnumerationRefsInAllDomainModels(oldQualifiedName, newQualifiedName); err != nil {
		fmt.Fprintf(ctx.Output, "Warning: failed to update enumeration references in domain models: %v\n", err)
	}

	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)

	fmt.Fprintf(ctx.Output, "Renamed enumeration: %s → %s\n", oldQualifiedName, newQualifiedName)
	if len(hits) > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", totalRefCount(hits), len(hits))
	}
	return nil
}

// execRenameAssociation renames an association and updates all references.
func execRenameAssociation(ctx *ExecContext, s *ast.RenameStmt) error {
	oldQualifiedName := s.Name.Module + "." + s.Name.Name
	newQualifiedName := s.Name.Module + "." + s.NewName

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	found := false
	collision := false
	for _, assoc := range dm.Associations {
		if assoc.Name == s.Name.Name {
			found = true
		} else if assoc.Name == s.NewName {
			collision = true
		}
	}
	if !found {
		return mdlerrors.NewNotFound("association", oldQualifiedName)
	}
	if collision {
		return mdlerrors.NewValidationf("association %s already exists in module %s", s.NewName, s.Name.Module)
	}

	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	// Update association name in domain model
	for _, assoc := range dm.Associations {
		if assoc.Name == s.Name.Name {
			assoc.Name = s.NewName
			break
		}
	}
	repointAssociationMemberRefs(dm, oldQualifiedName, newQualifiedName)
	if err := ctx.Backend.UpdateDomainModel(dm); err != nil {
		return mdlerrors.NewBackend("update association name", err)
	}

	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)

	fmt.Fprintf(ctx.Output, "Renamed association: %s → %s\n", oldQualifiedName, newQualifiedName)
	if len(hits) > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", totalRefCount(hits), len(hits))
	}
	return nil
}

// repointAssociationMemberRefs rewrites the entity access rules in this unit that
// name the renamed association, which they do by qualified name.
//
// Same clobber as repointEntitySelfRefs, one document type over: RenameReferences
// rewrites these in the raw unit and the UpdateDomainModel that follows puts the
// stale name back. It was the last of the four CE1613s a RENAME ASSOCIATION followed
// by a RENAME ENTITY left on a real 11.13 app, and the one that survives longest,
// because the stale name is re-read by every later statement that loads the unit.
//
// The match is EXACT rather than a prefix: an association member is named
// `Module.Association` with nothing after it, and a prefix match would also rewrite
// a differently-named association that happens to start with the same text.
//
// Every entity is walked, not just the association's FROM side. Mendix stores a
// MemberAccess for an association only on the FROM entity (adding one to the TO
// entity is CE0066), so in a well-formed model this finds them all on one entity —
// but a sweep that assumed the storage rule would silently skip anything a model
// carries in spite of it.
func repointAssociationMemberRefs(dm *domainmodel.DomainModel, oldQualifiedName, newQualifiedName string) {
	if dm == nil {
		return
	}
	for _, ent := range dm.Entities {
		for _, ar := range ent.AccessRules {
			for _, ma := range ar.MemberAccesses {
				if ma.AssociationName == oldQualifiedName {
					ma.AssociationName = newQualifiedName
				}
			}
		}
	}
}

// execRenameJavaAction renames a Java action and its .java source file.
func execRenameJavaAction(ctx *ExecContext, s *ast.RenameStmt) error {
	oldQualifiedName := s.Name.Module + "." + s.Name.Name
	newQualifiedName := s.Name.Module + "." + s.NewName

	// Verify the Java action exists
	jas, err := ctx.Backend.ListJavaActions()
	if err != nil {
		return mdlerrors.NewBackend("list java actions", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	found := false
	collision := false
	for _, ja := range jas {
		modID := h.FindModuleID(ja.ContainerID)
		if h.GetModuleName(modID) != s.Name.Module {
			continue
		}
		if ja.Name == s.Name.Name {
			found = true
		} else if ja.Name == s.NewName {
			collision = true
		}
	}
	if !found {
		return mdlerrors.NewNotFound("java action", oldQualifiedName)
	}
	if collision {
		return mdlerrors.NewValidationf("java action %s already exists in module %s", s.NewName, s.Name.Module)
	}

	hits, err := ctx.Backend.RenameReferences(oldQualifiedName, newQualifiedName, s.DryRun)
	if err != nil {
		return mdlerrors.NewBackend("scan references", err)
	}

	if s.DryRun {
		printRenameReport(ctx, oldQualifiedName, newQualifiedName, hits)
		return nil
	}

	if err := ctx.Backend.RenameDocumentByName(s.Name.Module, s.Name.Name, s.NewName); err != nil {
		return mdlerrors.NewBackend("rename java action", err)
	}
	if err := ctx.Backend.RenameJavaSourceFile(s.Name.Module, s.Name.Name, s.NewName); err != nil {
		return mdlerrors.NewBackend("rename java source file", err)
	}

	invalidateHierarchy(ctx)

	fmt.Fprintf(ctx.Output, "Renamed java action: %s → %s\n", oldQualifiedName, newQualifiedName)
	if len(hits) > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", totalRefCount(hits), len(hits))
	}
	return nil
}

// printRenameReport outputs a dry-run report of what would change.
func printRenameReport(ctx *ExecContext, oldName, newName string, hits []types.RenameHit) {
	fmt.Fprintf(ctx.Output, "Would rename: %s → %s\n", oldName, newName)
	fmt.Fprintf(ctx.Output, "References found: %d in %d document(s)\n", totalRefCount(hits), len(hits))

	for _, h := range hits {
		label := h.Name
		if label == "" {
			label = h.UnitID
		}
		typeName := h.UnitType
		if idx := strings.Index(typeName, "$"); idx >= 0 {
			typeName = typeName[idx+1:]
		}
		fmt.Fprintf(ctx.Output, "  %s (%s) — %d reference(s)\n", label, typeName, h.Count)
	}
}

func totalRefCount(hits []types.RenameHit) int {
	total := 0
	for _, h := range hits {
		total += h.Count
	}
	return total
}

func mergeHits(a, b []types.RenameHit) []types.RenameHit {
	seen := make(map[string]int) // unitID → index in result
	result := make([]types.RenameHit, len(a))
	copy(result, a)
	for i := range result {
		seen[result[i].UnitID] = i
	}
	for _, h := range b {
		if idx, ok := seen[h.UnitID]; ok {
			result[idx].Count += h.Count
		} else {
			seen[h.UnitID] = len(result)
			result = append(result, h)
		}
	}
	return result
}
