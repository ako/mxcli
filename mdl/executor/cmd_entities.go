// SPDX-License-Identifier: Apache-2.0

// Package executor - Entity commands (SHOW/DESCRIBE/CREATE/DROP ENTITY)
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/dmlayout"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// execCreateEntity handles CREATE ENTITY statements.
// buildEventHandlers converts a list of AST EventHandlerDef to domain model EventHandler.
// Resolves the microflow qualified name to a microflow ID, but stores the qualified name
// for BY_NAME serialization.
func buildEventHandlers(ctx *ExecContext, defs []ast.EventHandlerDef) ([]*domainmodel.EventHandler, error) {
	if len(defs) == 0 {
		return nil, nil
	}
	var handlers []*domainmodel.EventHandler
	for _, d := range defs {
		mfQN := d.Microflow.String()
		mfID, err := resolveMicroflowByName(ctx, mfQN)
		if err != nil {
			return nil, mdlerrors.NewNotFound("microflow", mfQN)
		}
		if err := checkBeforeCreateHandlerHasNoParameters(ctx, d, mfQN, mfID); err != nil {
			return nil, err
		}
		handlers = append(handlers, &domainmodel.EventHandler{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "DomainModels$EventHandler",
			},
			Moment:            domainmodel.EventMoment(d.Moment),
			Event:             domainmodel.EventType(d.Event),
			MicroflowID:       mfID,
			MicroflowName:     mfQN,
			RaiseErrorOnFalse: d.RaiseErrorOnFalse,
			PassEventObject:   d.PassEventObject,
		})
	}
	return handlers, nil
}

// checkBeforeCreateHandlerHasNoParameters refuses to wire a microflow that takes
// parameters to a BEFORE CREATE event handler.
//
// Mendix passes no object to a before-create handler — the object does not exist
// yet — so the build fails with
//
//	[CE7247] "Microflow should not have parameters" at Event handler of entity …
//
// which `mxcli check` had no way to see. The handler a defaulting microflow
// actually wants is AFTER CREATE, which does receive the object.
// (mxcli-todo findings #14a)
func checkBeforeCreateHandlerHasNoParameters(ctx *ExecContext, d ast.EventHandlerDef, mfQN string, mfID model.ID) error {
	if !strings.EqualFold(d.Moment, "Before") || !strings.EqualFold(d.Event, "Create") {
		return nil
	}
	mf, err := ctx.Backend.GetMicroflow(mfID)
	// A microflow created earlier in this same script is not readable back yet;
	// skip rather than guess. mxbuild still catches it, and refusing on a failed
	// read would break a legitimate script.
	if err != nil || mf == nil {
		return nil
	}
	if len(mf.Parameters) == 0 {
		return nil
	}
	names := make([]string, 0, len(mf.Parameters))
	for _, p := range mf.Parameters {
		names = append(names, "$"+p.Name)
	}
	return mdlerrors.NewValidationf(
		"microflow %s takes %d parameter(s) (%s), but a BEFORE CREATE event handler is called with none — "+
			"Mendix has no object to pass yet, so this builds as CE7247 \"Microflow should not have parameters\"\n"+
			"hint: use ON AFTER CREATE, which does receive the object, or remove the parameters",
		mfQN, len(mf.Parameters), strings.Join(names, ", "))
}

func execCreateEntity(ctx *ExecContext, s *ast.CreateEntityStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// An unqualified EXTENDS target is refused here as well as at check time:
	// `exec --no-check` skips the pre-flight, and this is the case that writes a
	// file Mendix cannot open rather than one it merely reports on. It needs no
	// project state, so it runs before anything is looked up or created — the
	// module below is AUTO-CREATED, and a statement that can never be valid must
	// not leave one behind.
	//
	// A qualified target that resolves to nothing is left to check: the
	// generalization is resolved lazily, so a parent created later in the same
	// script is legitimate and a per-statement guard would refuse it.
	if s.Generalization != nil {
		if err := checkGeneralizationQualified(s.Name, *s.Generalization); err != nil {
			return err
		}
	}

	// Find or auto-create module
	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Get domain model
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Check if entity already exists
	var existingEntity *domainmodel.Entity
	for _, entity := range dm.Entities {
		if entity.Name == s.Name.Name {
			existingEntity = entity
			break
		}
	}

	// If entity exists and not using CREATE OR MODIFY, return error. Note the
	// suggested fixes: `alter entity` for an incremental change (safe — it edits
	// the members it names and leaves the rest alone), and `create or modify` only
	// when the intent is to replace the whole definition — that path rebuilds the
	// entity from the statement and DROPS any attribute the statement omits, so it
	// is destructive if used for a partial update. (findings #24)
	if existingEntity != nil && s.IfNotExists {
		// IF NOT EXISTS is the safe half of re-runnability: it leaves the stored
		// definition alone entirely, where CREATE OR MODIFY rebuilds it from the
		// statement. (sudoku findings #10)
		fmt.Fprintf(ctx.Output, "Entity %s.%s already exists — skipped\n", s.Name.Module, s.Name.Name)
		return nil
	}
	if existingEntity != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExistsMsg("entity", s.Name.Module+"."+s.Name.Name,
			fmt.Sprintf("entity already exists: %s.%s — to add or change a member use 'alter entity %s.%s add attribute ...' (leaves the rest intact); "+
				"to make the script re-runnable use 'create entity if not exists' (leaves an existing entity untouched); "+
				"use 'create or modify entity' only to replace the whole definition (it drops any attribute this statement omits)",
				s.Name.Module, s.Name.Name, s.Name.Module, s.Name.Name))
	}

	// Calculate position
	var location model.Point
	if s.Position != nil {
		location = model.Point{X: s.Position.X, Y: s.Position.Y}
	} else if existingEntity != nil {
		location = existingEntity.Location
	} else {
		// No @Position and no stored entity: take the next grid slot.
		//
		// This used to be `X: 100 + len(dm.Entities)*150, Y: 100` — one row, for
		// every entity ever created. A 40-entity domain model came out as a
		// 6,000px line, and 150px is narrower than an entity box, so the boxes
		// touched as well. The grid is not a good layout, only a defensible
		// default; `mxcli layout` computes one from the association graph.
		location = dmlayout.GridSlot(len(dm.Entities))
	}

	// Determine persistable based on entity kind
	persistable := s.Kind != ast.EntityNonPersistent

	// Auto-default Boolean attributes to false if no DEFAULT specified
	for i := range s.Attributes {
		if s.Attributes[i].Type.Kind == ast.TypeBoolean && !s.Attributes[i].HasDefault {
			s.Attributes[i].HasDefault = true
			s.Attributes[i].DefaultValue = false
		}
	}

	// Reject reserved attribute names (CE7247) before writing anything.
	// Mendix rejects names like Owner/Type/Context/Id at runtime regardless of
	// entity kind — catching here avoids writing a project that Studio Pro will
	// refuse to open. Issue #552.
	// Only ERROR-severity violations block the write. ValidateEntity also emits
	// warnings (MDL022 AutoX rename, MDL071 OQL reserved name) and those must not
	// refuse a script that `mxcli check` reports as passing.
	if v := firstBlockingViolation(ValidateEntity(s)); v != nil {
		return mdlerrors.NewValidationf("%s — %s", v.Message, v.Suggestion)
	}

	// Validate TypeEnumeration attribute refs before writing anything.
	for _, a := range s.Attributes {
		if err := validateAttributeTypeRef(ctx, a.Name, a.Type); err != nil {
			return err
		}
	}

	// Create attributes and build name-to-ID map for validation rules and indexes
	// Also detect pseudo-types (AutoOwner, AutoChangedBy, etc.) and set entity flags
	var storeOwner, storeChangedBy, storeCreatedDate, storeChangedDate bool

	// On CREATE OR MODIFY of an existing entity, reuse each retained attribute's
	// existing ID rather than minting a fresh one. Attribute identity is what
	// Mendix's DB synchronizer keys off: a new ID for an unchanged attribute reads
	// as "old attribute departed, new attribute added", so it DROPS and re-adds the
	// column — wiping the stored data. Reusing the ID by name makes an identical
	// re-run truly idempotent (no spurious sync, no data loss). Genuinely new
	// attributes still get a fresh ID. (findings #13)
	existingAttrIDByName := make(map[string]model.ID)
	if s.CreateOrModify && existingEntity != nil {
		for _, ea := range existingEntity.Attributes {
			existingAttrIDByName[ea.Name] = ea.ID
		}
	}

	var attrs []*domainmodel.Attribute
	attrNameToID := make(map[string]model.ID)
	for _, a := range s.Attributes {
		// Pseudo-types: set entity flags instead of creating attributes
		switch a.Type.Kind {
		case ast.TypeAutoOwner:
			storeOwner = true
			continue
		case ast.TypeAutoChangedBy:
			storeChangedBy = true
			continue
		case ast.TypeAutoCreatedDate:
			storeCreatedDate = true
			continue
		case ast.TypeAutoChangedDate:
			storeChangedDate = true
			continue
		}

		// CALCULATED attributes are only supported on persistent entities
		if a.Calculated && !persistable {
			return mdlerrors.NewValidationf("attribute '%s': calculated attributes are only supported on persistent entities", a.Name)
		}

		// Use Documentation if available, fall back to Comment
		doc := a.Documentation
		if doc == "" {
			doc = a.Comment
		}

		// Generate ID for the attribute so we can reference it in validation rules/indexes.
		// On CREATE OR MODIFY, reuse the existing attribute's ID (by name) to preserve
		// identity and avoid a destructive DB column drop/re-add. (findings #13)
		attrID := model.ID(types.GenerateID())
		if id, ok := existingAttrIDByName[a.Name]; ok {
			attrID = id
		}
		attrNameToID[a.Name] = attrID

		attr := &domainmodel.Attribute{
			Name:          a.Name,
			Documentation: doc,
			Type:          convertDataType(a.Type),
		}
		attr.ID = attrID

		// Value type: CALCULATED or DEFAULT
		if a.Calculated {
			attrValue, err := resolveCalculatedValue(ctx, a.CalculatedMicroflow, s.Name.String(), a.Name, a.Type)
			if err != nil {
				return err
			}
			attr.Value = attrValue
		} else if a.HasDefault {
			defaultStr := fmt.Sprintf("%v", a.DefaultValue)
			// For enum attributes, Mendix stores just the value name (e.g., "Open"),
			// not the fully qualified name. The EnumerationRef already provides context.
			// Strip the enum prefix if the default is fully qualified.
			if a.Type.Kind == ast.TypeEnumeration && a.Type.EnumRef != nil {
				enumPrefix := a.Type.EnumRef.String() + "."
				if strings.HasPrefix(defaultStr, enumPrefix) {
					defaultStr = strings.TrimPrefix(defaultStr, enumPrefix)
				}
			}
			attr.Value = &domainmodel.AttributeValue{
				DefaultValue: defaultStr,
			}
		}

		attrs = append(attrs, attr)
	}

	// Create validation rules for NOT NULL and UNIQUE constraints
	var validationRules []*domainmodel.ValidationRule
	for _, a := range s.Attributes {
		attrID := attrNameToID[a.Name]

		// NOT NULL -> Required validation rule
		if a.NotNull {
			vr := &domainmodel.ValidationRule{
				AttributeID: attrID,
				Type:        "Required",
			}
			vr.ID = model.ID(types.GenerateID())
			if a.NotNullError != "" {
				vr.ErrorMessage = &model.Text{
					Translations: map[string]string{authoringLanguage(ctx): a.NotNullError},
				}
				vr.ErrorMessage.ID = model.ID(types.GenerateID())
			}
			validationRules = append(validationRules, vr)
		}

		// UNIQUE -> Unique validation rule
		if a.Unique {
			vr := &domainmodel.ValidationRule{
				AttributeID: attrID,
				Type:        "Unique",
			}
			vr.ID = model.ID(types.GenerateID())
			if a.UniqueError != "" {
				vr.ErrorMessage = &model.Text{
					Translations: map[string]string{authoringLanguage(ctx): a.UniqueError},
				}
				vr.ErrorMessage.ID = model.ID(types.GenerateID())
			}
			validationRules = append(validationRules, vr)
		}
	}

	// Create indexes
	var indexes []*domainmodel.Index
	for _, idx := range s.Indexes {
		idxID := model.ID(types.GenerateID())
		var indexAttrs []*domainmodel.IndexAttribute
		for _, col := range idx.Columns {
			if attrID, ok := attrNameToID[col.Name]; ok {
				iaID := model.ID(types.GenerateID())
				ia := &domainmodel.IndexAttribute{
					AttributeID: attrID,
					Ascending:   !col.Descending,
				}
				ia.ID = iaID
				indexAttrs = append(indexAttrs, ia)
			}
		}
		if len(indexAttrs) > 0 {
			index := &domainmodel.Index{
				Attributes: indexAttrs,
			}
			index.ID = idxID
			indexes = append(indexes, index)
		}
	}

	// Create entity
	// Build event handlers
	eventHandlers, err := buildEventHandlers(ctx, s.EventHandlers)
	if err != nil {
		return err
	}

	entity := &domainmodel.Entity{
		Name:            s.Name.Name,
		Documentation:   s.Documentation,
		Location:        location,
		Persistable:     persistable,
		Attributes:      attrs,
		ValidationRules: validationRules,
		Indexes:         indexes,
		EventHandlers:   eventHandlers,
		HasOwner:        storeOwner,
		HasChangedBy:    storeChangedBy,
		HasCreatedDate:  storeCreatedDate,
		HasChangedDate:  storeChangedDate,
	}

	// Set generalization (inheritance) if specified. A bare, unqualified target
	// was already refused at the top of this function — it is stored as-is and
	// produces a project Mendix cannot open.
	if s.Generalization != nil {
		entity.GeneralizationRef = s.Generalization.String()
	}

	if s.CreateOrModify && existingEntity != nil {
		// If the entity being replaced was a view entity, its OQL source document is
		// now orphaned — execCreateEntity only ever produces persistent/non-persistent
		// entities, so converting a view entity here leaves the ViewEntitySourceDocument
		// dangling (CE6786 "view entity document no longer linked to any view entity").
		if existingEntity.Source == "DomainModels$OqlViewEntitySource" {
			if err := ctx.Backend.DeleteViewEntitySourceDocumentByName(s.Name.Module, s.Name.Name); err != nil {
				return mdlerrors.NewBackend("delete orphaned view entity source document", err)
			}
		}
		// Warn (non-blocking) about members this CREATE OR MODIFY drops. The
		// path rebuilds the entity from the statement alone and replaces the
		// stored one, so any existing attribute the statement omits is silently
		// deleted — which, when a widget or microflow still binds it, surfaces
		// much later as CE1613. The user asked to modify, so we still apply it,
		// but list what is being removed so accidental data loss is visible
		// rather than silent. (findings #24)
		if dropped := droppedEntityMembers(existingEntity, entity); len(dropped) > 0 {
			fmt.Fprintf(ctx.Output,
				"⚠ create or modify entity %s drops %d existing member(s) not listed in this statement: %s\n",
				s.Name, len(dropped), strings.Join(dropped, ", "))
			fmt.Fprintf(ctx.Output,
				"  They are removed from the entity; anything still bound to them (widgets, microflows) will fail the build with CE1613.\n")
			fmt.Fprintf(ctx.Output,
				"  To add attributes without disturbing the rest, use: alter entity %s add attribute <name>: <type>;\n", s.Name)
		}
		// Carry forward (and prune) indexes so a dropped indexed attribute doesn't
		// leave an orphaned index that crashes `mx check` (finding #39).
		// MODIFY IN PLACE rather than replace. `create or modify` declares the
		// entity's ATTRIBUTE SET — omitting one removes it, warned about above —
		// but that is a statement about attributes and not a licence to discard
		// everything else the entity holds. The code used to rebuild the whole
		// entity from the AST and swap the object in, so every field the syntax
		// has no words for went with it.
		//
		// That is how the access rules went: `create or modify persistent entity`
		// on an existing entity deleted every grant on it, with `Modified entity:`
		// as the only output (ako/mxcli-rest). Measured on a real project, 174
		// access rules to 132 by re-running one domain-model script over eight
		// entities — and a byte-identical re-run lost them just as completely,
		// because the rebuild genuinely differs and so is not elided.
		//
		// Preserving by construction rather than by a carry list is the point.
		// The list was already at documentation and indexes, each added after its
		// own defect report; access rules would have been the third, and the
		// external-source and OData fields the next. Starting from what is stored
		// and overwriting only what the statement declares has no such tail.
		entity = mergeDeclaredOntoStoredEntity(existingEntity, entity, s)
		if droppedIdx := reconcileDroppedIndexes(entity, existingEntity); droppedIdx > 0 {
			fmt.Fprintf(ctx.Output,
				"  Dropped %d index(es) that referenced removed attribute(s).\n", droppedIdx)
		}
		// An attribute the statement dropped must lose its member access too, or
		// the carried rule dangles — the same class reconcileDroppedIndexes
		// exists to prevent, one member over.
		pruneMemberAccessesForDroppedAttributes(entity, existingEntity)
		// Update existing entity
		entity.ID = existingEntity.ID
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("update entity", err)
		}
		// Invalidate caches so updated entity is visible
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		ctx.ReportMutation("Modified", "entity: %s", s.Name)
	} else {
		// Create new entity
		if err := ctx.Backend.CreateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("create entity", err)
		}
		// Invalidate caches so new entity is visible
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Created entity: %s\n", s.Name)
	}

	ctx.trackModifiedDomainModel(module.ID, module.Name)
	return nil
}

// mergeDeclaredOntoStoredEntity returns the stored entity with the fields this
// statement actually declares overwritten from the rebuilt one.
//
// The split is what the MDL syntax can express. Everything in the first group
// below has a spelling in `create or modify entity`, so the statement is
// authoritative about it — including the attribute set, where an omission is a
// removal (warned about by droppedEntityMembers). Everything else — access
// rules, the view/external source fields, the OData remote properties — has no
// spelling at all, so an omission carries no meaning and the stored value stands.
//
// Documentation is declared-but-optional and is distinguished by its own AST
// flag rather than by an empty value: a rewrite that says nothing about it
// preserves what is stored, while an explicitly empty `/** */` clears it
// (mendixlabs/mxcli#1018).
//
// entityFieldsDeclaredByStatement lists the first group by name, and
// TestMergeDeclaredOntoStoredEntity_EveryFieldHasADecision fails on any Entity
// field missing from it — so adding one to the struct forces the choice rather
// than defaulting to whichever branch happens to be the safe one here.
func mergeDeclaredOntoStoredEntity(stored, declared *domainmodel.Entity, s *ast.CreateEntityStmt) *domainmodel.Entity {
	if stored == nil {
		return declared
	}
	merged := *stored
	merged.Name = declared.Name
	merged.Persistable = declared.Persistable
	merged.Location = declared.Location
	merged.Attributes = declared.Attributes
	merged.ValidationRules = declared.ValidationRules
	merged.Indexes = declared.Indexes
	merged.EventHandlers = declared.EventHandlers
	merged.HasOwner = declared.HasOwner
	merged.HasChangedBy = declared.HasChangedBy
	merged.HasCreatedDate = declared.HasCreatedDate
	merged.HasChangedDate = declared.HasChangedDate
	// An omitted EXTENDS still removes the generalization — there is no "extends
	// nothing" spelling, so preserving it would make an inheritance impossible to
	// remove. It is reported by droppedEntityMembers instead, which is the same
	// treatment an omitted attribute gets.
	//
	// Generalization and GeneralizationID are the READ-side spellings of the same
	// thing, which the executor never fills. Both writers key off
	// GeneralizationRef alone, so carrying the stored pair would be inert today —
	// they are overwritten anyway, because a stale parent left beside a changed
	// ref is a trap for whichever writer reads them first.
	merged.GeneralizationRef = declared.GeneralizationRef
	merged.Generalization = declared.Generalization
	merged.GeneralizationID = declared.GeneralizationID
	if s != nil && !s.DocumentationSet {
		merged.Documentation = stored.Documentation
	} else {
		merged.Documentation = declared.Documentation
	}
	return &merged
}

// entityFieldsDeclaredByStatement names the domainmodel.Entity fields that
// `create or modify entity` is authoritative about — the ones
// mergeDeclaredOntoStoredEntity takes from the rebuilt entity. Every other field
// is preserved from what is stored. It is a package-level var rather than a
// literal in the test because the test's job is to fail when the struct grows a
// field this list does not mention.
var entityFieldsDeclaredByStatement = map[string]bool{
	"Name":              true,
	"Documentation":     true, // via s.DocumentationSet; see above
	"Location":          true,
	"Persistable":       true,
	"Attributes":        true,
	"ValidationRules":   true,
	"Indexes":           true,
	"EventHandlers":     true,
	"HasOwner":          true,
	"HasChangedBy":      true,
	"HasCreatedDate":    true,
	"HasChangedDate":    true,
	"GeneralizationRef": true,
	"Generalization":    true,
	"GeneralizationID":  true,
	// ContainerID and the embedded BaseElement identify the stored element, so
	// they are preserved. `entity.ID = existingEntity.ID` at the call site says
	// the same thing about the ID; the merge is where it now comes from.
}

// pruneMemberAccessesForDroppedAttributes removes the member entries of the
// preserved access rules whose attribute this rewrite removed.
//
// Attributes the rewrite ADDS need nothing here: each engine extends a rule with
// the missing members at the rule's default rights on write (syncMemberAccesses,
// ReconcileMemberAccesses). Only removal has to be handled, and only here.
func pruneMemberAccessesForDroppedAttributes(entity, stored *domainmodel.Entity) {
	if stored == nil || len(entity.AccessRules) == 0 {
		return
	}
	// A member is pruned only on POSITIVE evidence that this rewrite removed its
	// attribute: the STORED entity owned it and the rebuilt one does not. The
	// obvious predicate — "not among the rebuilt entity's attributes" — is wrong,
	// and wrong in the direction that breaks security. An entity's access rules
	// also govern its INHERITED members, and those never appear in its own
	// Attributes list: CapTrack.ExportDocument extends System.FileDocument and
	// owns no attributes at all, so that predicate emptied all five of its rules
	// and mxbuild reported CE0066 "Entity access is out of date" — a regression
	// an earlier version of this fix introduced, and which only a specialisation
	// exposes.
	kept := make(map[string]bool, len(entity.Attributes))
	for _, a := range entity.Attributes {
		kept[strings.ToLower(a.Name)] = true
	}
	removed := make(map[string]bool)
	for _, a := range stored.Attributes {
		if !kept[strings.ToLower(a.Name)] {
			removed[strings.ToLower(a.Name)] = true
			if a.ID != "" {
				removed[string(a.ID)] = true
			}
		}
	}
	if len(removed) == 0 {
		return
	}
	survives := func(ma *domainmodel.MemberAccess) bool {
		// Both fields are consulted because the two engines fill different ones:
		// the modelsdk reader populates only AttributeName (memberAccessFromGen),
		// the legacy one an AttributeID. The DROP ATTRIBUTE cleanup this mirrors
		// checks both for the same reason.
		if ma.AttributeID != "" && removed[string(ma.AttributeID)] {
			return false
		}
		if ma.AttributeName == "" {
			return true
		}
		name := ma.AttributeName
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		return !removed[strings.ToLower(name)]
	}
	for _, rule := range entity.AccessRules {
		if rule == nil {
			continue
		}
		var members []*domainmodel.MemberAccess
		for _, ma := range rule.MemberAccesses {
			if survives(ma) {
				members = append(members, ma)
			}
		}
		rule.MemberAccesses = members
	}
}

// reconcileDroppedIndexes fixes ledger finding #39: when CREATE OR MODIFY drops
// an indexed attribute, the entity's index for it is left orphaned (its column
// references a GUID that no longer exists), which crashes `mx check` with an
// unhandled AggregateException. When the statement lists NO index clause of its
// own (so `entity.Indexes` is empty), carry the existing entity's indexes forward
// but prune any column referencing an attribute the new entity no longer has, and
// drop indexes left with no columns. A statement that DOES list indexes replaces
// wholesale (unchanged). Attribute IDs survive by name across CREATE OR MODIFY
// (findings #13), so a surviving attribute keeps the ID its index references.
// Returns the number of indexes dropped, for the drop warning.
func reconcileDroppedIndexes(entity, existing *domainmodel.Entity) int {
	if entity == nil || existing == nil || len(entity.Indexes) > 0 {
		return 0
	}
	valid := make(map[model.ID]bool, len(entity.Attributes))
	for _, a := range entity.Attributes {
		valid[a.ID] = true
	}
	var kept []*domainmodel.Index
	dropped := 0
	for _, idx := range existing.Indexes {
		var attrs []*domainmodel.IndexAttribute
		for _, ia := range idx.Attributes {
			if valid[ia.AttributeID] {
				attrs = append(attrs, ia)
			}
		}
		var ids []model.ID
		for _, id := range idx.AttributeIDs {
			if valid[id] {
				ids = append(ids, id)
			}
		}
		if len(attrs) > 0 || len(ids) > 0 {
			idx.Attributes = attrs
			idx.AttributeIDs = ids
			kept = append(kept, idx)
		} else {
			dropped++
		}
	}
	entity.Indexes = kept
	return dropped
}

// isViewEntity reports whether an entity is an OQL view entity. View entities
// cannot participate in associations (CE6771) and behave differently from
// persistent entities in several checks. The canonical marker is the
// DomainModels$OqlViewEntitySource source type; OqlQuery / SourceDocumentRef are
// belt-and-suspenders fallbacks for read paths that populate one but not the other.
func isViewEntity(e *domainmodel.Entity) bool {
	if e == nil {
		return false
	}
	return e.Source == "DomainModels$OqlViewEntitySource" ||
		e.OqlQuery != "" || e.SourceDocumentRef != ""
}

// droppedEntityMembers reports the members present on existing but absent from
// replacement — i.e. what a CREATE OR MODIFY replace would delete. Named
// attributes are compared case-insensitively; the four audit system fields are
// reported when their flag is on in existing but off in replacement. Used to
// surface accidental data loss (findings #24).
func droppedEntityMembers(existing, replacement *domainmodel.Entity) []string {
	keep := make(map[string]bool, len(replacement.Attributes))
	for _, a := range replacement.Attributes {
		keep[strings.ToLower(a.Name)] = true
	}
	var dropped []string
	for _, a := range existing.Attributes {
		if !keep[strings.ToLower(a.Name)] {
			dropped = append(dropped, a.Name)
		}
	}
	// Audit system fields that were enabled and are no longer requested are also
	// removed by the replace.
	if existing.HasOwner && !replacement.HasOwner {
		dropped = append(dropped, "owner (system field)")
	}
	if existing.HasChangedBy && !replacement.HasChangedBy {
		dropped = append(dropped, "changedBy (system field)")
	}
	if existing.HasCreatedDate && !replacement.HasCreatedDate {
		dropped = append(dropped, "createdDate (system field)")
	}
	if existing.HasChangedDate && !replacement.HasChangedDate {
		dropped = append(dropped, "changedDate (system field)")
	}
	// An omitted EXTENDS un-inherits the entity, which is a bigger change than a
	// dropped attribute and was the only one of these that happened in silence.
	// It is reported rather than preserved because there is no "extends nothing"
	// spelling, so preserving it would make an inheritance impossible to remove.
	if existing.GeneralizationRef != "" && replacement.GeneralizationRef == "" {
		dropped = append(dropped, "extends "+existing.GeneralizationRef+" (generalization)")
	}
	return dropped
}

// execCreateViewEntity handles CREATE VIEW ENTITY statements.
func execCreateViewEntity(ctx *ExecContext, s *ast.CreateViewEntityStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Version pre-check
	if err := checkFeature(ctx, "domain_model", "view_entities",
		"create view entity",
		"upgrade your project to 10.18+ or use a regular entity with a microflow data source"); err != nil {
		return err
	}

	// Validate OQL syntax before creating the view entity
	// This prevents creating view entities that will crash Studio Pro
	if s.Query.RawQuery != "" {
		if oqlViolations := ValidateOQLSyntax(s.Query.RawQuery); len(oqlViolations) > 0 {
			var msgs []string
			for _, v := range oqlViolations {
				msgs = append(msgs, v.Message)
			}
			return mdlerrors.NewValidationf("invalid OQL in view entity '%s':\n  - %s",
				s.Name.String(), strings.Join(msgs, "\n  - "))
		}
	}

	// Find module
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Get domain model
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Check if entity already exists
	var existingEntity *domainmodel.Entity
	for _, entity := range dm.Entities {
		if entity.Name == s.Name.Name {
			existingEntity = entity
			break
		}
	}

	// If entity exists: REPLACE drops and recreates, MODIFY updates in place
	if existingEntity != nil && !s.CreateOrModify && !s.CreateOrReplace {
		return mdlerrors.NewAlreadyExistsMsg("entity", s.Name.Module+"."+s.Name.Name, fmt.Sprintf("entity already exists: %s.%s (use create or modify to update, or create or replace to drop and recreate)", s.Name.Module, s.Name.Name))
	}

	// CREATE OR REPLACE: delete existing entity and source doc, then recreate
	if existingEntity != nil && s.CreateOrReplace {
		// Preserve location from old entity if no explicit position
		if s.Position == nil {
			s.Position = &ast.Position{X: existingEntity.Location.X, Y: existingEntity.Location.Y}
		}
		// Delete ViewEntitySourceDocument
		if err := ctx.Backend.DeleteViewEntitySourceDocumentByName(s.Name.Module, s.Name.Name); err != nil {
			return mdlerrors.NewBackend("delete existing ViewEntitySourceDocument", err)
		}
		// Delete the entity itself
		if err := ctx.Backend.DeleteEntity(dm.ID, existingEntity.ID); err != nil {
			return mdlerrors.NewBackend("delete existing entity for replace", err)
		}
		existingEntity = nil
		// Re-fetch domain model after deletion so entity count is correct for positioning
		dm, err = ctx.Backend.GetDomainModel(module.ID)
		if err != nil {
			return mdlerrors.NewBackend("get domain model after delete", err)
		}
	}

	// Calculate position
	var location model.Point
	if s.Position != nil {
		location = model.Point{X: s.Position.X, Y: s.Position.Y}
	} else if existingEntity != nil {
		location = existingEntity.Location
	} else {
		location = model.Point{X: 100 + len(dm.Entities)*150, Y: 100}
	}

	// Create or update ViewEntitySourceDocument (separate document for OQL query)
	sourceDocRef := s.Name.Module + "." + s.Name.Name
	// Always delete any existing ViewEntitySourceDocument before creating a new one.
	// This prevents duplicate OQL documents from accumulating (e.g., from re-running
	// scripts or after a previous DROP that didn't clean up properly).
	if err := ctx.Backend.DeleteViewEntitySourceDocumentByName(s.Name.Module, s.Name.Name); err != nil {
		return mdlerrors.NewBackend("delete existing ViewEntitySourceDocument", err)
	}
	_, err = ctx.Backend.CreateViewEntitySourceDocument(
		module.ID,
		s.Name.Module,
		s.Name.Name,
		s.Query.RawQuery,
		s.Documentation,
	)
	if err != nil {
		return mdlerrors.NewBackend("create ViewEntitySourceDocument", err)
	}

	// Create view attributes with OqlViewValue references.
	// Preserve existing attribute and value IDs on modify to avoid CE-6770.
	existingAttrsByName := make(map[string]*domainmodel.Attribute)
	if existingEntity != nil {
		for _, ea := range existingEntity.Attributes {
			existingAttrsByName[ea.Name] = ea
		}
	}
	var attrs []*domainmodel.Attribute
	for _, a := range s.Attributes {
		attr := &domainmodel.Attribute{
			Name: a.Name,
			Type: convertDataType(a.Type),
			Value: &domainmodel.AttributeValue{
				ViewReference: a.Name, // OQL column alias matches attribute name
			},
		}
		// Preserve IDs from existing attribute with same name
		if ea, ok := existingAttrsByName[a.Name]; ok {
			attr.ID = ea.ID
			if ea.Value != nil {
				attr.Value.ID = ea.Value.ID
			}
		}
		attrs = append(attrs, attr)
	}

	// Create view entity with source document reference.
	// View entities use Persistable=true because they are retrievable from database (via OQL).
	// Studio Pro treats view entities as persistable for database retrieval purposes.
	entity := &domainmodel.Entity{
		Name:              s.Name.Name,
		Documentation:     s.Documentation,
		Location:          location,
		Persistable:       true,
		Attributes:        attrs,
		Source:            "DomainModels$OqlViewEntitySource",
		SourceDocumentRef: sourceDocRef,
		OqlQuery:          s.Query.RawQuery,
	}

	if s.CreateOrModify && existingEntity != nil {
		// Update existing entity — preserve Source object ID to avoid CE-6770
		entity.ID = existingEntity.ID
		entity.SourceObjectID = existingEntity.SourceObjectID
		// A rewrite that carried no doc comment keeps the stored one (#1018).
		entity.Documentation = carriedDocumentation(
			s.DocumentationSet, s.Documentation, existingEntity.Documentation)
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("update view entity", err)
		}
		// Invalidate caches so updated entity is visible
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		ctx.ReportMutation("Modified", "view entity: %s", s.Name)
	} else {
		// Create new entity
		if err := ctx.Backend.CreateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("create view entity", err)
		}
		// Invalidate caches so new entity is visible
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Created view entity: %s\n", s.Name)
	}

	return nil
}

// execAlterEntity handles ALTER ENTITY statements.
func execAlterEntity(ctx *ExecContext, s *ast.AlterEntityStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find module
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	// Get domain model
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Find entity
	var entity *domainmodel.Entity
	for _, ent := range dm.Entities {
		if ent.Name == s.Name.Name {
			entity = ent
			break
		}
	}
	if entity == nil {
		return mdlerrors.NewNotFoundMsg("entity", fmt.Sprint(s.Name), fmt.Sprintf("entity not found: %s", s.Name))
	}

	switch s.Operation {
	case ast.AlterEntityAddAttribute:
		a := s.Attribute
		if a == nil {
			return mdlerrors.NewValidation("no attribute definition provided")
		}
		// Pseudo-types: set entity flags instead of adding real attributes
		switch a.Type.Kind {
		case ast.TypeAutoOwner:
			entity.HasOwner = true
			return ctx.Backend.UpdateEntity(dm.ID, entity)
		case ast.TypeAutoChangedBy:
			entity.HasChangedBy = true
			return ctx.Backend.UpdateEntity(dm.ID, entity)
		case ast.TypeAutoCreatedDate:
			entity.HasCreatedDate = true
			return ctx.Backend.UpdateEntity(dm.ID, entity)
		case ast.TypeAutoChangedDate:
			entity.HasChangedDate = true
			return ctx.Backend.UpdateEntity(dm.ID, entity)
		}
		// CALCULATED attributes are only supported on persistent entities
		if a.Calculated && !entity.Persistable {
			return mdlerrors.NewValidationf("attribute '%s': calculated attributes are only supported on persistent entities", a.Name)
		}
		// Auto-default Boolean attributes to false if no DEFAULT specified
		if a.Type.Kind == ast.TypeBoolean && !a.HasDefault {
			a.HasDefault = true
			a.DefaultValue = false
		}
		// Check for duplicate attribute name. With IF NOT EXISTS, an existing
		// attribute is a no-op (with a notice) rather than an error, so a domain
		// script re-runs cleanly (findings #10).
		for _, existing := range entity.Attributes {
			if existing.Name == a.Name {
				if s.IfNotExists {
					fmt.Fprintf(ctx.Output, "Attribute '%s' already exists on entity %s — skipped\n", a.Name, s.Name)
					return nil
				}
				return mdlerrors.NewAlreadyExistsMsg("attribute", a.Name, fmt.Sprintf("attribute '%s' already exists on entity %s", a.Name, s.Name))
			}
		}

		attrID := model.ID(types.GenerateID())
		attr := &domainmodel.Attribute{
			Name:          a.Name,
			Documentation: a.Documentation,
			Type:          convertDataType(a.Type),
		}
		attr.ID = attrID
		if a.Calculated {
			attrValue, err := resolveCalculatedValue(ctx, a.CalculatedMicroflow, s.Name.String(), a.Name, a.Type)
			if err != nil {
				return err
			}
			attr.Value = attrValue
		} else if a.HasDefault {
			defaultStr := fmt.Sprintf("%v", a.DefaultValue)
			if a.Type.Kind == ast.TypeEnumeration && a.Type.EnumRef != nil {
				enumPrefix := a.Type.EnumRef.String() + "."
				if strings.HasPrefix(defaultStr, enumPrefix) {
					defaultStr = strings.TrimPrefix(defaultStr, enumPrefix)
				}
			}
			attr.Value = &domainmodel.AttributeValue{
				DefaultValue: defaultStr,
			}
		}
		entity.Attributes = append(entity.Attributes, attr)

		// Add validation rules for NOT NULL and UNIQUE
		if a.NotNull {
			vr := &domainmodel.ValidationRule{
				AttributeID: attrID,
				Type:        "Required",
			}
			vr.ID = model.ID(types.GenerateID())
			if a.NotNullError != "" {
				vr.ErrorMessage = &model.Text{
					Translations: map[string]string{authoringLanguage(ctx): a.NotNullError},
				}
				vr.ErrorMessage.ID = model.ID(types.GenerateID())
			}
			entity.ValidationRules = append(entity.ValidationRules, vr)
		}
		if a.Unique {
			vr := &domainmodel.ValidationRule{
				AttributeID: attrID,
				Type:        "Unique",
			}
			vr.ID = model.ID(types.GenerateID())
			if a.UniqueError != "" {
				vr.ErrorMessage = &model.Text{
					Translations: map[string]string{authoringLanguage(ctx): a.UniqueError},
				}
				vr.ErrorMessage.ID = model.ID(types.GenerateID())
			}
			entity.ValidationRules = append(entity.ValidationRules, vr)
		}

		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("add attribute", err)
		}
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Added attribute '%s' to entity %s\n", a.Name, s.Name)

	case ast.AlterEntityRenameAttribute:
		var target *domainmodel.Attribute
		for _, attr := range entity.Attributes {
			switch attr.Name {
			case s.AttributeName:
				target = attr
			case s.NewName:
				return mdlerrors.NewValidationf("attribute '%s' already exists on entity %s", s.NewName, s.Name)
			}
		}
		if target == nil {
			return mdlerrors.NewNotFoundMsg("attribute", s.AttributeName, fmt.Sprintf("attribute '%s' not found on entity %s", s.AttributeName, s.Name))
		}
		target.Name = s.NewName
		// The entity's own access and validation rules hold the attribute's
		// qualified name as a string, so they have to move with it *in the model*
		// — not merely in the BSON the reference scan rewrites afterwards.
		// Leaving them to the scan produced a duplicate member: UpdateEntity saw
		// an attribute with no matching MemberAccess and added one, and the scan
		// then renamed the stale entry into a second copy of it. mxbuild caught
		// that as CE0066 "Entity access is out of date".
		renameAttributeInEntityRules(entity, s.Name.String(), s.AttributeName, s.NewName)
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("rename attribute", err)
		}

		// Everything that points at an attribute — a create/change activity's
		// member, a page's attribute widget, the entity's own validation and
		// access rules — stores the fully qualified name as a string. Renaming
		// only the domain model leaves every one of them dangling, which mxbuild
		// reports as CE1613 "The selected attribute 'Mod.Entity.Old' no longer
		// exists." (#910). The scan runs *after* UpdateEntity on purpose: it is a
		// raw-BSON pass over every unit including the domain model, and writing
		// the model afterwards would re-serialize it from the parsed entity and
		// undo the scan's edits there.
		hits, err := ctx.Backend.RenameReferences(
			s.Name.String()+"."+s.AttributeName,
			s.Name.String()+"."+s.NewName,
			false,
		)
		if err != nil {
			return mdlerrors.NewBackend("update attribute references", err)
		}

		// XPath constraints name the attribute as a bare step, so the scan above
		// cannot see them. They are resolvable without any type inference — a
		// constraint's target entity is known structurally and every further hop
		// is named in the path — so they are rewritten here rather than left for
		// the user. See mdl/xpathrefs for why the edit is textual and what it
		// refuses to touch.
		xres, err := renameAttributeInXPath(ctx, s.Name.String(), string(dm.ID), s.Name.Name, s.AttributeName, s.NewName)
		if err != nil {
			return mdlerrors.NewBackend("update attribute references in XPath constraints", err)
		}

		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Renamed attribute '%s' to '%s' on entity %s\n", s.AttributeName, s.NewName, s.Name)
		if n := totalRefCount(hits); n > 0 {
			fmt.Fprintf(ctx.Output, "Updated %d reference(s) in %d document(s)\n", n, len(hits))
		}
		reportXPathRename(ctx, xres)
		// Microflow expressions ($obj/Attr) are the one place left. A bare name
		// there is only resolvable from the type of what precedes it, which needs
		// the resolver in PROPOSAL_expression_type_checking; mxbuild reports the
		// leftovers as CE0117, so say so rather than let a half-done rename look
		// finished.
		fmt.Fprintf(ctx.Output,
			"Note: uses in microflow expressions ($obj/%s) are stored as text and were "+
				"not rewritten — run 'mxcli docker check' to find them.\n",
			s.AttributeName)

	case ast.AlterEntityDropDefault:
		// Clearing a default is its own operation because MODIFY ATTRIBUTE cannot
		// express it: that form always takes a type, and its type slot accepts a
		// bare qualified name, so `MODIFY ATTRIBUTE X SET DEFAULT NULL` read SET
		// as the type and wrote an unloadable project (#910).
		found := false
		for _, attr := range entity.Attributes {
			if attr.Name != s.AttributeName {
				continue
			}
			// Only the stored default goes. A CalculatedValue is not a default —
			// dropping it would silently turn a calculated attribute into a plain
			// one, which is a different operation the user did not ask for.
			if attr.Value != nil && attr.Value.Type == "CalculatedValue" {
				return mdlerrors.NewValidationf(
					"attribute '%s' is calculated, not defaulted — DROP DEFAULT does not apply "+
						"(use MODIFY ATTRIBUTE to change how it is computed)", s.AttributeName)
			}
			attr.Value = nil
			found = true
			break
		}
		if !found {
			return mdlerrors.NewNotFoundMsg("attribute", s.AttributeName,
				fmt.Sprintf("attribute '%s' not found on entity %s", s.AttributeName, s.Name))
		}
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("drop default", err)
		}
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Dropped default value on attribute '%s' of entity %s\n", s.AttributeName, s.Name)

	case ast.AlterEntityModifyAttribute:
		// CALCULATED attributes are only supported on persistent entities
		if s.Calculated && !entity.Persistable {
			return mdlerrors.NewValidationf("attribute '%s': calculated attributes are only supported on persistent entities", s.AttributeName)
		}
		// Reject a type that resolves to nothing BEFORE touching the attribute.
		// Writing one produces a .mpr Mendix cannot load at all (#910).
		if err := validateModifyAttributeTypeRef(ctx, s.AttributeName, s.DataType); err != nil {
			return err
		}
		found := false
		for _, attr := range entity.Attributes {
			if attr.Name == s.AttributeName {
				attr.Type = convertDataType(s.DataType)
				if s.Calculated {
					attrValue, err := resolveCalculatedValue(ctx, s.CalculatedMicroflow, s.Name.String(), s.AttributeName, s.DataType)
					if err != nil {
						return err
					}
					attr.Value = attrValue
				}
				// Bug 12a: apply the constraints the user specified and preserve
				// the ones they didn't. NOT NULL / UNIQUE are stored as entity
				// ValidationRules, not attribute flags, so NULLABLE removes the
				// "Required" rule and NOT NULL (re)adds it.
				attrQN := s.Name.String() + "." + attr.Name
				if s.ModifyNotNull != nil {
					setAttributeValidationRule(entity, attr, attrQN, "Required", *s.ModifyNotNull, s.ModifyNotNullError, authoringLanguage(ctx))
				}
				if s.ModifyUnique != nil {
					setAttributeValidationRule(entity, attr, attrQN, "Unique", *s.ModifyUnique, s.ModifyUniqueError, authoringLanguage(ctx))
				}
				if s.ModifyHasDefault {
					defaultStr := fmt.Sprintf("%v", s.ModifyDefaultValue)
					if s.DataType.Kind == ast.TypeEnumeration && s.DataType.EnumRef != nil {
						defaultStr = strings.TrimPrefix(defaultStr, s.DataType.EnumRef.String()+".")
					}
					attr.Value = &domainmodel.AttributeValue{DefaultValue: defaultStr}
				}
				found = true
				break
			}
		}
		if !found {
			return mdlerrors.NewNotFoundMsg("attribute", s.AttributeName, fmt.Sprintf("attribute '%s' not found on entity %s", s.AttributeName, s.Name))
		}
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("modify attribute", err)
		}
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		ctx.ReportMutation("Modified", "attribute '%s' on entity %s", s.AttributeName, s.Name)

	case ast.AlterEntityDropAttribute:
		// System attribute pseudo-names: drop by clearing entity flags
		switch strings.ToLower(s.AttributeName) {
		case "owner":
			if entity.HasOwner {
				entity.HasOwner = false
				return ctx.Backend.UpdateEntity(dm.ID, entity)
			}
		case "changedby":
			if entity.HasChangedBy {
				entity.HasChangedBy = false
				return ctx.Backend.UpdateEntity(dm.ID, entity)
			}
		case "createddate":
			if entity.HasCreatedDate {
				entity.HasCreatedDate = false
				return ctx.Backend.UpdateEntity(dm.ID, entity)
			}
		case "changeddate":
			if entity.HasChangedDate {
				entity.HasChangedDate = false
				return ctx.Backend.UpdateEntity(dm.ID, entity)
			}
		}

		idx := -1
		for i, attr := range entity.Attributes {
			if attr.Name == s.AttributeName {
				idx = i
				break
			}
		}
		if idx < 0 {
			// With IF EXISTS, an already-absent attribute is a no-op (with a
			// notice) rather than an error, so a domain script re-runs cleanly
			// (findings #10).
			if s.IfExists {
				fmt.Fprintf(ctx.Output, "Attribute '%s' not found on entity %s — skipped\n", s.AttributeName, s.Name)
				return nil
			}
			return mdlerrors.NewNotFoundMsg("attribute", s.AttributeName, fmt.Sprintf("attribute '%s' not found on entity %s", s.AttributeName, s.Name))
		}
		// Clean up entity-level references to the dropped attribute.
		//
		// Two reference forms have to be matched, not one. Mendix stores an
		// index's column as an element ID (AttributePointer) but a validation
		// rule's Attribute and an access rule's MemberAccess as a BY_NAME
		// qualified name string. Both come back in the same model.ID field, so
		// comparing against the element ID alone matched nothing and left the
		// rules behind — CE1613 "The selected attribute ... no longer exists."
		// serializeValidationRule documents the same duality on the way out.
		droppedID := entity.Attributes[idx].ID
		droppedQName := fmt.Sprintf("%s.%s.%s", module.Name, entity.Name, entity.Attributes[idx].Name)
		// Which FIELD carries the reference also varies by engine: the legacy
		// backend fills a MemberAccess's AttributeID and AttributeName both, while
		// the modelsdk one leaves AttributeID empty and fills only AttributeName
		// (domainmodel.go: memberAccessFromGen). Both spell it qualified, so check
		// every field that could hold it rather than picking one.
		refersToDropped := func(refs ...string) bool {
			for _, r := range refs {
				if r != "" && (r == string(droppedID) || r == droppedQName) {
					return true
				}
			}
			return false
		}

		// Track what gets cleaned up for reporting
		origValidationCount := len(entity.ValidationRules)
		origIndexCount := len(entity.Indexes)

		// Remove validation rules that reference this attribute
		var keepRules []*domainmodel.ValidationRule
		for _, vr := range entity.ValidationRules {
			if !refersToDropped(string(vr.AttributeID)) {
				keepRules = append(keepRules, vr)
			}
		}
		entity.ValidationRules = keepRules

		// Remove MemberAccess entries from access rules that reference this attribute
		removedMemberAccess := 0
		for _, rule := range entity.AccessRules {
			var keepMembers []*domainmodel.MemberAccess
			for _, ma := range rule.MemberAccesses {
				if !refersToDropped(string(ma.AttributeID), ma.AttributeName) {
					keepMembers = append(keepMembers, ma)
				} else {
					removedMemberAccess++
				}
			}
			rule.MemberAccesses = keepMembers
		}

		// Remove index attributes that reference this attribute, and drop empty indexes
		var keepIndexes []*domainmodel.Index
		for _, idx := range entity.Indexes {
			var keepAttrs []*domainmodel.IndexAttribute
			for _, ia := range idx.Attributes {
				if !refersToDropped(string(ia.AttributeID)) {
					keepAttrs = append(keepAttrs, ia)
				}
			}
			idx.Attributes = keepAttrs

			var keepIDs []model.ID
			for _, id := range idx.AttributeIDs {
				if !refersToDropped(string(id)) {
					keepIDs = append(keepIDs, id)
				}
			}
			idx.AttributeIDs = keepIDs

			if len(idx.Attributes) > 0 || len(idx.AttributeIDs) > 0 {
				keepIndexes = append(keepIndexes, idx)
			}
		}
		entity.Indexes = keepIndexes

		// Remove the attribute
		entity.Attributes = append(entity.Attributes[:idx], entity.Attributes[idx+1:]...)
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("drop attribute", err)
		}
		invalidateHierarchy(ctx)
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Dropped attribute '%s' from entity %s\n", s.AttributeName, s.Name)

		// Report what was cleaned up on the entity itself
		if n := origValidationCount - len(keepRules); n > 0 {
			fmt.Fprintf(ctx.Output, "  Removed %d validation rule(s)\n", n)
		}
		if removedMemberAccess > 0 {
			fmt.Fprintf(ctx.Output, "  Removed %d access rule member reference(s)\n", removedMemberAccess)
		}
		if n := origIndexCount - len(keepIndexes); n > 0 {
			fmt.Fprintf(ctx.Output, "  Removed %d index(es)\n", n)
		}

		// Warn about references in other documents that are NOT auto-cleaned
		entityQName := s.Name.String()
		fmt.Fprintf(ctx.Output, "  Warning: pages, microflows, and other documents may still reference '%s'. Update them manually.\n", s.AttributeName)
		fmt.Fprintf(ctx.Output, "  Use show references to %s to find usages (requires refresh catalog full).\n", entityQName)

	case ast.AlterEntitySetDocumentation:
		entity.Documentation = s.Documentation
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("set documentation", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Set documentation on entity %s\n", s.Name)

	case ast.AlterEntitySetComment:
		// Comments are stored as documentation in the Mendix model
		entity.Documentation = s.Comment
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("set comment", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Set comment on entity %s\n", s.Name)

	case ast.AlterEntitySetPosition:
		if s.Position == nil {
			return mdlerrors.NewValidation("no position provided")
		}
		entity.Location = model.Point{X: s.Position.X, Y: s.Position.Y}
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("set position", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Set position of entity %s to (%d, %d)\n", s.Name, s.Position.X, s.Position.Y)

	case ast.AlterEntityAddIndex:
		if s.Index == nil {
			return mdlerrors.NewValidation("no index definition provided")
		}
		// Build name-to-ID map for attribute lookup
		attrNameToID := make(map[string]model.ID)
		for _, attr := range entity.Attributes {
			attrNameToID[attr.Name] = attr.ID
		}
		// An index already covering these columns, in this order, with these
		// sort directions IS this index — a Mendix index has no name to tell two
		// apart. Adding it again used to append a silent duplicate that mxbuild
		// rejects with CE0072 "Duplicate indexes", so a domain script run twice
		// produced a model that no longer builds, with nothing in mxcli's output
		// to say so. Now the bare form errors like ADD ATTRIBUTE does, and
		// IF NOT EXISTS skips. (sudoku findings #10)
		if existing := findIndexByColumns(entity, s.Index.Columns); existing != nil {
			if s.IfNotExists {
				fmt.Fprintf(ctx.Output, "Index %s already exists on entity %s — skipped\n", indexColumnLabel(s.Index.Columns), s.Name)
				return nil
			}
			return mdlerrors.NewAlreadyExistsMsg("index", indexColumnLabel(s.Index.Columns),
				fmt.Sprintf("index %s already exists on entity %s — re-running this statement would create a duplicate, which fails the build with CE0072; use 'add index if not exists' to make the script re-runnable",
					indexColumnLabel(s.Index.Columns), s.Name))
		}

		idxID := model.ID(types.GenerateID())
		var indexAttrs []*domainmodel.IndexAttribute
		for _, col := range s.Index.Columns {
			if attrID, ok := attrNameToID[col.Name]; ok {
				ia := &domainmodel.IndexAttribute{
					AttributeID: attrID,
					Ascending:   !col.Descending,
				}
				ia.ID = model.ID(types.GenerateID())
				indexAttrs = append(indexAttrs, ia)
			} else {
				return mdlerrors.NewNotFoundMsg("attribute", col.Name, fmt.Sprintf("attribute '%s' not found for index on entity %s", col.Name, s.Name))
			}
		}
		index := &domainmodel.Index{
			Attributes: indexAttrs,
		}
		index.ID = idxID
		entity.Indexes = append(entity.Indexes, index)
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("add index", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Added index %s to entity %s\n", indexColumnLabel(s.Index.Columns), s.Name)

	case ast.AlterEntityDropIndex:
		// Two selectors. The column list is the index's real identity — it is
		// what `describe entity` prints, and a Mendix index stores no name — so
		// it is the only form that round-trips. The ordinal name ("idx1") is
		// kept for compatibility, but it names a POSITION that shifts as soon as
		// an earlier index is dropped, which makes a multi-drop script depend on
		// its own statement order.
		label := s.IndexName
		if s.Index != nil {
			label = indexColumnLabel(s.Index.Columns)
		}
		idx := -1
		switch {
		case s.Index != nil:
			for i := range entity.Indexes {
				if indexMatchesColumns(entity, entity.Indexes[i], s.Index.Columns) {
					idx = i
					break
				}
			}
		case s.IndexName != "":
			for i := range entity.Indexes {
				if fmt.Sprintf("idx%d", i+1) == s.IndexName {
					idx = i
					break
				}
			}
		default:
			return mdlerrors.NewValidation("no index specified")
		}
		if idx < 0 {
			// IF EXISTS makes a drop re-runnable: the second run finds nothing
			// and says so instead of halting the script. (sudoku findings #10)
			if s.IfExists {
				fmt.Fprintf(ctx.Output, "Index %s not found on entity %s — skipped\n", label, s.Name)
				return nil
			}
			hint := ""
			if s.Index == nil {
				hint = " — a Mendix index stores no name, so 'idx1' is a position, not an identity; select it by its columns instead, e.g. drop index (Row, Col)"
			}
			return mdlerrors.NewNotFoundMsg("index", label,
				fmt.Sprintf("index %s not found on entity %s%s", label, s.Name, hint))
		}
		entity.Indexes = append(entity.Indexes[:idx], entity.Indexes[idx+1:]...)
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("drop index", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Dropped index %s from entity %s\n", label, s.Name)

	case ast.AlterEntityAddEventHandler:
		if s.EventHandler == nil {
			return mdlerrors.NewValidation("missing event handler definition")
		}
		ehs, err := buildEventHandlers(ctx, []ast.EventHandlerDef{*s.EventHandler})
		if err != nil {
			return err
		}
		// Reject duplicate (same Moment + Event)
		for _, existing := range entity.EventHandlers {
			if existing.Moment == ehs[0].Moment && existing.Event == ehs[0].Event {
				// An event handler has no other re-run route: a defensive
				// drop-then-add fails on the drop when it is absent and on the
				// add when it is present. IF NOT EXISTS makes the script
				// idempotent, like ADD ATTRIBUTE. (mxcli-todo findings #18)
				if s.IfNotExists {
					fmt.Fprintf(ctx.Output, "Event handler %s %s already exists on %s, skipping\n",
						s.EventHandler.Moment, s.EventHandler.Event, s.Name)
					return nil
				}
				return mdlerrors.NewAlreadyExistsMsg("event handler",
					fmt.Sprintf("%s %s", s.EventHandler.Moment, s.EventHandler.Event),
					fmt.Sprintf("event handler already exists for %s %s on %s (use ADD EVENT HANDLER IF NOT EXISTS to make the script re-runnable)", s.EventHandler.Moment, s.EventHandler.Event, s.Name))
			}
		}
		entity.EventHandlers = append(entity.EventHandlers, ehs[0])
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("add event handler", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Added event handler %s %s on %s\n",
			s.EventHandler.Moment, s.EventHandler.Event, s.Name)

	case ast.AlterEntityDropEventHandler:
		if s.EventHandler == nil {
			return mdlerrors.NewValidation("missing event handler reference")
		}
		moment := domainmodel.EventMoment(s.EventHandler.Moment)
		event := domainmodel.EventType(s.EventHandler.Event)
		idx := -1
		for i, eh := range entity.EventHandlers {
			if eh.Moment == moment && eh.Event == event {
				idx = i
				break
			}
		}
		if idx < 0 {
			if s.IfExists {
				fmt.Fprintf(ctx.Output, "Event handler %s %s not present on %s, skipping\n",
					s.EventHandler.Moment, s.EventHandler.Event, s.Name)
				return nil
			}
			return mdlerrors.NewNotFoundMsg("event handler",
				fmt.Sprintf("%s %s", s.EventHandler.Moment, s.EventHandler.Event),
				fmt.Sprintf("event handler %s %s not found on %s (use DROP EVENT HANDLER IF EXISTS to make the script re-runnable)", s.EventHandler.Moment, s.EventHandler.Event, s.Name))
		}
		entity.EventHandlers = append(entity.EventHandlers[:idx], entity.EventHandlers[idx+1:]...)
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("drop event handler", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Dropped event handler %s %s from %s\n",
			s.EventHandler.Moment, s.EventHandler.Event, s.Name)

	case ast.AlterEntitySetAllowCreateChangeLocally:
		entity.CreateChangeLocally = s.BoolValue
		if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
			return mdlerrors.NewBackend("set allow create change locally", err)
		}
		invalidateDomainModelsCache(ctx)
		fmt.Fprintf(ctx.Output, "Set AllowCreateChangeLocally = %v on entity %s\n", s.BoolValue, s.Name)

	default:
		return mdlerrors.NewUnsupported("unsupported alter entity operation")
	}

	ctx.trackModifiedDomainModel(module.ID, module.Name)
	return nil
}

// setAttributeValidationRule sets or clears an attribute's "Required" (NOT NULL)
// or "Unique" validation rule on the entity. NOT NULL and UNIQUE are entity-level
// ValidationRule entries, not attribute flags — so MODIFY ATTRIBUTE … NULLABLE
// has to drop the rule and NOT NULL has to (re)add it (Bug 12a). Any existing
// rule of the same type for the attribute is removed first, so this is
// idempotent and preserves rules of other types / attributes.
//
// The rule's AttributeID is representation-dependent: the read path stores the
// fully-qualified attribute name (Module.Entity.Attr), while a freshly-created
// rule may carry the bare UUID — ruleTargetsAttribute handles both. A rule we
// add is stored as the qualified name, which validationRuleToGen writes verbatim.
func setAttributeValidationRule(entity *domainmodel.Entity, attr *domainmodel.Attribute, attrQualifiedName, ruleType string, want bool, errMsg, lang string) {
	kept := entity.ValidationRules[:0]
	for _, vr := range entity.ValidationRules {
		if vr != nil && vr.Type == ruleType && ruleTargetsAttribute(string(vr.AttributeID), attr) {
			continue // drop the existing rule of this type for this attribute
		}
		kept = append(kept, vr)
	}
	entity.ValidationRules = kept
	if !want {
		return
	}
	vr := &domainmodel.ValidationRule{AttributeID: model.ID(attrQualifiedName), Type: ruleType}
	vr.ID = model.ID(types.GenerateID())
	if errMsg != "" {
		vr.ErrorMessage = &model.Text{Translations: map[string]string{lang: errMsg}}
		vr.ErrorMessage.ID = model.ID(types.GenerateID())
	}
	entity.ValidationRules = append(entity.ValidationRules, vr)
}

// ruleTargetsAttribute reports whether a validation rule's AttributeID refers to
// attr. The ID is either the bare attribute UUID (freshly-created rules) or the
// fully-qualified attribute name Module.Entity.Attr (read-back rules) — for the
// latter the final dot-segment is the attribute name. Validation rules are
// entity-scoped, so a last-segment name match is unambiguous within the entity.
func ruleTargetsAttribute(ruleAttrID string, attr *domainmodel.Attribute) bool {
	if ruleAttrID == string(attr.ID) {
		return true
	}
	if i := strings.LastIndex(ruleAttrID, "."); i >= 0 {
		return ruleAttrID[i+1:] == attr.Name
	}
	return ruleAttrID == attr.Name
}

// execDropEntity handles DROP ENTITY statements.
func execDropEntity(ctx *ExecContext, s *ast.DropEntityStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	// Find module and entity
	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	for _, entity := range dm.Entities {
		if entity.Name == s.Name.Name {
			// Warn about references before deleting (best-effort)
			warnEntityReferences(ctx, s.Name.String())

			// If this is a view entity, also delete the associated ViewEntitySourceDocument
			if entity.Source == "DomainModels$OqlViewEntitySource" {
				if err := ctx.Backend.DeleteViewEntitySourceDocumentByName(s.Name.Module, s.Name.Name); err != nil {
					return mdlerrors.NewBackend("delete view entity source document", err)
				}
			}
			if err := ctx.Backend.DeleteEntity(dm.ID, entity.ID); err != nil {
				return mdlerrors.NewBackend("delete entity", err)
			}
			invalidateDomainModelsCache(ctx)
			fmt.Fprintf(ctx.Output, "Dropped entity: %s\n", s.Name)
			return nil
		}
	}

	return mdlerrors.NewNotFoundMsg("entity", fmt.Sprint(s.Name), fmt.Sprintf("entity not found: %s", s.Name))
}

// warnEntityReferences prints a warning if the entity is referenced by other elements.
// Uses the catalog if available; silently skips if catalog is not built.
func warnEntityReferences(ctx *ExecContext, entityName string) {
	if ctx.Catalog == nil || !ctx.Catalog.IsBuilt() {
		return
	}

	query := fmt.Sprintf(
		"select SourceType, SourceName, RefKind from refs where TargetName = '%s'",
		strings.ReplaceAll(entityName, "'", "''"),
	)
	result, err := ctx.Catalog.Query(query)
	if err != nil || result.Count == 0 {
		return
	}

	fmt.Fprintf(ctx.Output, "warning: %s is referenced by %d element(s):\n", entityName, result.Count)
	for _, row := range result.Rows {
		sourceType, _ := row[0].(string)
		sourceName, _ := row[1].(string)
		refKind, _ := row[2].(string)
		fmt.Fprintf(ctx.Output, "  - %s %s (%s)\n", sourceType, sourceName, refKind)
	}
}

// --- Executor method wrappers for callers not yet migrated ---

// indexColumnLabel renders an index's columns the way `describe entity` does,
// so an error names the index in the spelling the author can act on.
func indexColumnLabel(cols []ast.IndexColumn) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = c.Name
		if c.Descending {
			parts[i] += " desc"
		}
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// indexMatchesColumns reports whether a stored index covers exactly these
// columns, in this order, with these sort directions.
//
// Order and direction are part of the identity, not decoration: Mendix builds a
// composite database index, and (Row, Col) serves different queries from
// (Col, Row). Treating them as the same index would make ADD refuse a distinct
// index and DROP remove the wrong one.
func indexMatchesColumns(entity *domainmodel.Entity, index *domainmodel.Index, cols []ast.IndexColumn) bool {
	if len(index.Attributes) != len(cols) {
		return false
	}
	nameByID := make(map[model.ID]string, len(entity.Attributes))
	for _, attr := range entity.Attributes {
		nameByID[attr.ID] = attr.Name
	}
	for i, ia := range index.Attributes {
		if !strings.EqualFold(nameByID[ia.AttributeID], cols[i].Name) {
			return false
		}
		if ia.Ascending == cols[i].Descending {
			return false
		}
	}
	return true
}

// findIndexByColumns returns the stored index with exactly these columns, or nil.
func findIndexByColumns(entity *domainmodel.Entity, cols []ast.IndexColumn) *domainmodel.Index {
	for _, index := range entity.Indexes {
		if indexMatchesColumns(entity, index, cols) {
			return index
		}
	}
	return nil
}
