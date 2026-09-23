// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// persistDM re-encodes a (mutated) domain model and writes it back to its unit.
// The codec passes unchanged children through their original raw bytes, so only
// the elements actually mutated are rebuilt — the rest stay byte-faithful to
// what Studio Pro wrote.
func (b *Backend) persistDM(domainModelID model.ID, dm *genDm.DomainModel) error {
	enc := &codec.Encoder{}
	contents, err := enc.Encode(dm)
	if err != nil {
		return fmt.Errorf("encode domain model %s: %w", domainModelID, err)
	}
	if err := b.writer.UpdateRawUnit(string(domainModelID), contents); err != nil {
		return fmt.Errorf("persist domain model %s: %w", domainModelID, err)
	}
	return nil
}

// findGenEntity returns the gen entity with the given ID, or nil.
func findGenEntity(dm *genDm.DomainModel, entityID model.ID) *genDm.Entity {
	for _, el := range dm.EntitiesItems() {
		if string(el.ID()) == string(entityID) {
			if e, ok := el.(*genDm.Entity); ok {
				return e
			}
		}
	}
	return nil
}

// removeAssocsReferencing drops every regular association in dm whose FROM
// (ParentPointer) or TO (ChildPointer) endpoint is entityID. Returns whether
// anything was removed. Iterates back-to-front so removal indices stay valid.
func removeAssocsReferencing(dm *genDm.DomainModel, entityID model.ID) bool {
	changed := false
	items := dm.AssociationsItems()
	for i := len(items) - 1; i >= 0; i-- {
		a, ok := items[i].(*genDm.Association)
		if !ok {
			continue
		}
		if string(a.ParentRefID()) == string(entityID) || string(a.ChildRefID()) == string(entityID) {
			dm.RemoveAssociations(i)
			changed = true
		}
	}
	return changed
}

// removeCrossAssocsReferencing drops every CROSS-MODULE association in dm that
// ends at the deleted entity. Returns whether anything was removed.
//
// A cross-module association is stored in the FROM entity's module, in a separate
// collection from the regular ones, and its two ends are addressed differently:
// the FROM end by element id (local to this domain model) and the TO end by
// qualified name (it lives in another module). So both have to be matched, and
// they fail differently when they are not (ako/mxcli#553):
//
//   - a dangling BY-ID end is a 16-byte pointer to an element that is gone, and
//     mxbuild cannot LOAD the project — KeyNotFoundException at
//     StreamingBsonUnitReader.ResolvePostponedProperties(), with no CE code and
//     no document named.
//   - a dangling BY-NAME end is an ordinary model error, CE1613 at the
//     cross-module association.
//
// qualifiedName may be empty when the module name could not be established; the
// by-name sweep is then skipped rather than guessed at, since an empty name would
// match nothing at best and everything at worst.
func removeCrossAssocsReferencing(dm *genDm.DomainModel, entityID model.ID, qualifiedName string) bool {
	changed := false
	items := dm.CrossAssociationsItems()
	for i := len(items) - 1; i >= 0; i-- {
		ca, ok := items[i].(*genDm.CrossAssociation)
		if !ok {
			continue
		}
		byID := string(ca.ParentRefID()) == string(entityID)
		byName := qualifiedName != "" && strings.EqualFold(ca.ChildQualifiedName(), qualifiedName)
		if byID || byName {
			dm.RemoveCrossAssociations(i)
			changed = true
		}
	}
	return changed
}

// DeleteAttribute removes an attribute from an entity. The remaining attributes
// pass through the codec unchanged; only the Attributes list is rebuilt. Mirrors
// legacy semantics (no cascade — dangling index/validation refs are left as-is,
// same as the legacy writer).
func (b *Backend) DeleteAttribute(domainModelID, entityID, attrID model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteAttribute: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	ent := findGenEntity(dm, entityID)
	if ent == nil {
		return fmt.Errorf("entity not found: %s", entityID)
	}
	idx := -1
	for i, el := range ent.AttributesItems() {
		if string(el.ID()) == string(attrID) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("attribute not found: %s", attrID)
	}
	ent.RemoveAttributes(idx)
	return b.persistDM(domainModelID, dm)
}

// UpdateEntity replaces an entity with the fully-modified domainmodel.Entity the
// executor passes (the executor routes every ALTER ENTITY op — rename, doc, add/
// modify/drop attribute, generalization, index — through here). The entity keeps
// its position: the entities list is rebuilt in order with the target swapped for
// a freshly-converted gen entity, while every other entity passes through its
// original raw bytes. Mirrors legacy UpdateEntity (full re-serialize of the
// replaced entity, siblings untouched).
func (b *Backend) UpdateEntity(domainModelID model.ID, entity *domainmodel.Entity) error {
	// Refuse rather than downgrade: a validation rule type this writer cannot
	// reproduce (RegEx, Range) used to come back as Required, silently dropping
	// the pattern reference — and mxbuild reports nothing, because a Required
	// rule is perfectly valid (guard-don't-drop, ADR-0005).
	if ruleType, ok := validationRulesAreReproducible(entity); !ok {
		return fmt.Errorf(
			"entity %s has a %s validation rule, which mxcli cannot rewrite without losing it — "+
				"change this entity in Studio Pro, or remove the rule first.\n"+
				"  (Rewriting would silently turn it into a Required rule: the constraint would be gone "+
				"and the build would still pass.)",
			entity.Name, ruleType)
	}
	if entity == nil {
		return fmt.Errorf("UpdateEntity: nil entity")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateEntity: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	order := dm.EntitiesItems()
	orig := findGenEntity(dm, entity.ID)
	if orig == nil {
		return fmt.Errorf("entity not found: %s", entity.ID)
	}

	ge := entityToGen(entity, b.moduleNameFor(domainModelID), b.majorVersion())
	ge.SetID(element.ID(entity.ID))
	assignEntityIDs(ge)

	// Carry the original raw bytes onto the rebuilt target so the codec treats it
	// as an EXISTING element: dirty (re-encoded) properties come from ge, while
	// unmodeled fields — notably the entity GUID — pass through verbatim from raw.
	// Without this, entityToGen produces a fresh (raw==nil) element and the codec
	// emits GUID = $ID (EmitGUID default), discarding the on-disk GUID. That GUID
	// is the entity's stable cross-reference identity: pages/grids and inheriting
	// entities in other modules resolve members through it, so changing it dangles
	// those references (CE1613 — GitHub issue #657). Siblings are untouched (raw
	// passthrough in the list rebuild below), so they already keep their GUID; this
	// closes the same gap for the ALTER target itself.
	if raw := orig.Raw(); raw != nil {
		ge.SetRaw(raw)
	}

	// The same carry, one level down, for the entity's GUID-bearing CHILDREN.
	// #657 closed this for the entity element and noted that siblings survive via
	// the list-rebuild raw passthrough — but the target's own children do not:
	// entityToGen rebuilds every attribute and index from the semantic model, so
	// each arrives raw==nil and the codec's EmitGUID default writes GUID = $ID.
	//
	// For an attribute that GUID is the database's identity, not a cross-reference:
	// the runtime keys mendixsystem$attribute.id on it, so re-minting it makes the
	// synchroniser treat every column as deleted-and-re-added and DROP it on the
	// next deploy. Measured on a real project: one ALTER, 0 of 28 GUIDs surviving,
	// all 607 rows' attribute values gone (issue #1119). Nothing caught it — the
	// model stays valid, mx check is clean, and because the new GUID is derived
	// from a now-stable $ID the damage is idempotent, so a second run is elided
	// and reports "Unchanged".
	carryChildIdentity(ge, orig, entity)

	// When an update empties a child list, the fresh (empty) list on ge is "clean"
	// — entityToGen appended nothing to it — so the codec passes the STORED raw
	// bytes through unchanged and the removal silently does not happen. Touching
	// the list (append + remove) marks it dirty, so the codec re-emits it as empty
	// and clears the raw. A list that still has members is already dirty from
	// entityToGen's appends, which is why this is only needed for the last one out.
	//
	// That "last one out" is the whole trap: any test that removes one of two
	// members passes against the broken code. Measured on 11.13.0, one member each:
	//
	//   Indexes          orphaned index → `mx check` dies with an unhandled
	//                    AggregateException (ledger #39)
	//   ValidationRules  rule outlives the attribute it constrains → CE1613
	//   Attributes       DROP ATTRIBUTE reports success and writes nothing at all
	//
	// AccessRules and EventHandlers share the mechanism; they are covered here
	// rather than left to be rediscovered one error code at a time.
	for _, l := range []struct {
		newLen, storedLen int
		touch             func()
	}{
		{len(entity.Attributes), len(orig.AttributesItems()), func() {
			ge.AddAttributes(genDm.NewAttribute())
			ge.RemoveAttributes(0)
		}},
		{len(entity.ValidationRules), len(orig.ValidationRulesItems()), func() {
			ge.AddValidationRules(genDm.NewValidationRule())
			ge.RemoveValidationRules(0)
		}},
		{len(entity.Indexes), len(orig.IndexesItems()), func() {
			ge.AddIndexes(genDm.NewIndex())
			ge.RemoveIndexes(0)
		}},
		{len(entity.AccessRules), len(orig.AccessRulesItems()), func() {
			ge.AddAccessRules(genDm.NewAccessRule())
			ge.RemoveAccessRules(0)
		}},
		{len(entity.EventHandlers), len(orig.EventHandlersItems()), func() {
			ge.AddEventHandlers(genDm.NewEventHandler())
			ge.RemoveEventHandlers(0)
		}},
	} {
		if l.newLen == 0 && l.storedLen > 0 {
			l.touch()
		}
	}

	// Rebuild the list in place: drop all, re-add in original order swapping the
	// target. Re-added existing elements stay clean (only the list is dirtied),
	// so the codec re-emits them byte-faithfully; only ge is built fresh.
	for i := len(order) - 1; i >= 0; i-- {
		dm.RemoveEntities(i)
	}
	for _, el := range order {
		if string(el.ID()) == string(entity.ID) {
			dm.AddEntities(ge)
		} else {
			dm.AddEntities(el)
		}
	}
	return b.persistDM(domainModelID, dm)
}

// UpdateDomainModel persists a whole mutated domain model (the executor's
// read-modify-write path for ALTER ASSOCIATION, CREATE OR MODIFY ASSOCIATION,
// and RENAME). It rebuilds the Entities and Associations lists from the semantic
// model via the byte-faithful converters, carrying each element's stored identity
// — both its $ID and its storage GUID — onto the rebuild.
// CrossAssociations and Annotations are NOT represented in domainmodel.DomainModel,
// so they are left as gen passthrough rather than dropped (ADR-0005: guard
// fidelity — the existing raw bytes carry forward unchanged).
//
// The GUID half of that was missing until ako/mxcli#1169. Unlike UpdateEntity,
// which swaps one entity into an otherwise raw-passthrough list, this rebuilds
// EVERY entity and EVERY association — so every element arrived raw==nil and the
// codec's EmitGUID default wrote GUID = $ID across the whole unit, including
// elements the statement never named. The reporter measured 282 moved GUIDs in one
// module. What that costs is in the carry comments below; the short version is
// that the runtime keys the database on the GUID, so re-minting one drops a column
// (an attribute) or a whole table (an entity, whose name is the table name — which
// is why RENAME ENTITY routing through here was the worst of the affected
// statements).
func (b *Backend) UpdateDomainModel(dm *domainmodel.DomainModel) error {
	if dm == nil {
		return fmt.Errorf("UpdateDomainModel: nil domain model")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateDomainModel: not connected for writing")
	}
	gdm, err := b.loadDomainModelGen(dm.ID)
	if err != nil {
		return err
	}
	moduleName := b.moduleNameFor(dm.ID)
	major := b.majorVersion()

	// Index the stored elements by $ID BEFORE the removal loops empty the lists.
	// The $ID is the right key because the read path round-trips it into the
	// semantic model, so it is exact rather than structural: a RENAME keeps the ID
	// and carries the identity forward (Studio Pro renames the table or column and
	// keeps the data), while a genuinely new element arrives with an empty ID,
	// matches nothing, and correctly gets a fresh GUID.
	storedEntities := make(map[string]*genDm.Entity, len(gdm.EntitiesItems()))
	for _, el := range gdm.EntitiesItems() {
		if ge, ok := el.(*genDm.Entity); ok {
			storedEntities[string(ge.ID())] = ge
		}
	}
	storedAssociations := make(map[string]*genDm.Association, len(gdm.AssociationsItems()))
	for _, el := range gdm.AssociationsItems() {
		if ga, ok := el.(*genDm.Association); ok {
			storedAssociations[string(ga.ID())] = ga
		}
	}

	for i := len(gdm.EntitiesItems()) - 1; i >= 0; i-- {
		gdm.RemoveEntities(i)
	}
	for _, e := range dm.Entities {
		ge := entityToGen(e, moduleName, major)
		ge.SetID(element.ID(e.ID))
		assignEntityIDs(ge)
		// Carry the stored raw bytes so the codec treats the rebuild as an EXISTING
		// element: the properties entityToGen set re-encode, while the ones the
		// semantic model does not carry — the GUID above all — pass through verbatim.
		// This is #657's carry (for the entity) and #1119's (for its attributes and
		// indexes) applied to a path that had neither, because it rebuilds the whole
		// list instead of swapping one member into it (#1169).
		if orig := storedEntities[string(e.ID)]; orig != nil {
			if raw := orig.Raw(); raw != nil {
				ge.SetRaw(raw)
			}
			carryChildIdentity(ge, orig, e)
		}
		gdm.AddEntities(ge)
	}

	for i := len(gdm.AssociationsItems()) - 1; i >= 0; i-- {
		gdm.RemoveAssociations(i)
	}
	for _, a := range dm.Associations {
		ga := assocToGen(a)
		if a.ID != "" {
			ga.SetID(element.ID(a.ID))
		}
		assignAssociationIDs(ga)
		// The same carry for associations. A direct SetRaw is enough here, where the
		// cross-module MOVE needed a raw transform (crossAssocRawFromAssoc, #503):
		// assocToGen is Association -> Association, so the stored $Type and the
		// rebuilt one agree and nothing has to be rewritten on the way.
		//
		// It also subsumes the property-by-property patch #872 made for the line
		// anchors: those were lost to this same "the rebuild only carries what the
		// semantic model models" mechanism, and raw passthrough covers the whole
		// class rather than one property of it.
		if orig := storedAssociations[string(a.ID)]; orig != nil {
			if raw := orig.Raw(); raw != nil {
				ga.SetRaw(raw)
			}
		}
		gdm.AddAssociations(ga)
	}

	return b.persistDM(dm.ID, gdm)
}

// DeleteAssociation removes an association from a domain model by ID. Used by
// DROP ASSOCIATION and by the executor's CREATE OR MODIFY ASSOCIATION (delete +
// recreate) path.
func (b *Backend) DeleteAssociation(domainModelID, assocID model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteAssociation: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	for i, el := range dm.AssociationsItems() {
		if string(el.ID()) == string(assocID) {
			dm.RemoveAssociations(i)
			return b.persistDM(domainModelID, dm)
		}
	}
	return fmt.Errorf("association not found: %s", assocID)
}

// DeleteCrossAssociation removes a cross-module association from a domain model by ID.
func (b *Backend) DeleteCrossAssociation(domainModelID, assocID model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteCrossAssociation: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	for i, el := range dm.CrossAssociationsItems() {
		if string(el.ID()) == string(assocID) {
			dm.RemoveCrossAssociations(i)
			return b.persistDM(domainModelID, dm)
		}
	}
	return fmt.Errorf("cross association not found: %s", assocID)
}

// DeleteEntity removes an entity and cascades association cleanup: associations
// in the same DM and in every other DM that reference the entity (by
// ParentPointer = FROM or ChildPointer = TO) are removed. Mirrors legacy
// DeleteEntity, including the cross-module cascade.
func (b *Backend) DeleteEntity(domainModelID, entityID model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteEntity: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	eidx := -1
	for i, el := range dm.EntitiesItems() {
		if string(el.ID()) == string(entityID) {
			eidx = i
			break
		}
	}
	if eidx < 0 {
		return fmt.Errorf("entity not found: %s", entityID)
	}
	// The qualified name the OTHER modules' cross-module associations know this
	// entity by. Read before the entity is removed, for obvious reasons.
	qualifiedName := ""
	if ge := findGenEntity(dm, entityID); ge != nil {
		if moduleName := b.moduleNameFor(domainModelID); moduleName != "" {
			qualifiedName = moduleName + "." + ge.Name()
		}
	}

	dm.RemoveEntities(eidx)
	removeAssocsReferencing(dm, entityID)
	removeCrossAssocsReferencing(dm, entityID, qualifiedName)
	if err := b.persistDM(domainModelID, dm); err != nil {
		return err
	}

	// Cascade: remove associations referencing this entity from all other DMs.
	allDMs, err := b.ListDomainModels()
	if err != nil {
		return fmt.Errorf("DeleteEntity: cascade cleanup: list domain models: %w", err)
	}
	for _, other := range allDMs {
		if other.ID == domainModelID {
			continue
		}
		// The virtual System domain model has no on-disk unit and is immutable —
		// skip it (ListDomainModels injects it for entity resolution).
		if string(other.ID) == meta.SystemDomainModelID {
			continue
		}
		odm, err := b.loadDomainModelGen(other.ID)
		if err != nil {
			return fmt.Errorf("DeleteEntity: cascade cleanup: load %s: %w", other.ID, err)
		}
		// Two sweeps, not one: a regular association here can only reference the
		// entity by id, while a cross-module one in ANOTHER module reaches it by
		// qualified name — which is the half that leaves CE1613 behind.
		removed := removeAssocsReferencing(odm, entityID)
		if removeCrossAssocsReferencing(odm, entityID, qualifiedName) {
			removed = true
		}
		if removed {
			if err := b.persistDM(other.ID, odm); err != nil {
				return fmt.Errorf("DeleteEntity: cascade cleanup: update %s: %w", other.ID, err)
			}
		}
	}
	return nil
}

// SetDomainModelAnnotations replaces the canvas notes on a domain model.
//
// Annotations are mutated in place on the gen document rather than rebuilt from
// the semantic type: domainmodel.Annotation carries Caption, Location and Width,
// while the stored element also has ExportLevel — and anything else a future
// Mendix adds. Matching each note to the one already there and setting only the
// three fields MDL owns means a property nobody has modelled yet rides along
// untouched, which is the whole reason UpdateDomainModel leaves this collection
// as passthrough (ADR-0005).
//
// A note with no stored counterpart is appended with the defaults Studio Pro
// writes; a stored one with no counterpart in the new list is removed, which is
// what makes DROP ANNOTATION work.
func (b *Backend) SetDomainModelAnnotations(domainModelID model.ID, annotations []*domainmodel.Annotation) error {
	if b.writer == nil {
		return fmt.Errorf("SetDomainModelAnnotations: not connected for writing")
	}
	gdm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}

	// Index what is stored by element ID so an edit keeps its identity: minting a
	// fresh one would make an otherwise-unchanged domain model differ (ADR-0008).
	stored := map[model.ID]*genDm.Annotation{}
	for _, el := range gdm.AnnotationsItems() {
		if ga, ok := el.(*genDm.Annotation); ok {
			stored[model.ID(ga.ID())] = ga
		}
	}

	for i := len(gdm.AnnotationsItems()) - 1; i >= 0; i-- {
		gdm.RemoveAnnotations(i)
	}
	for _, a := range annotations {
		ga, ok := stored[a.ID]
		if !ok {
			ga = genDm.NewAnnotation()
			if a.ID != "" {
				ga.SetID(element.ID(a.ID))
			} else {
				ga.SetID(element.ID(mmpr.GenerateID()))
			}
			ga.SetExportLevel("Hidden")
		}
		ga.SetCaption(a.Caption)
		ga.SetLocation(fmt.Sprintf("%d;%d", a.Location.X, a.Location.Y))
		ga.SetWidth(int32(a.Width))
		gdm.AddAnnotations(ga)
	}
	return b.persistDM(domainModelID, gdm)
}

// entityIsExternal reports whether a stored entity is an external (OData) one,
// whose attributes carry mapped remote values rather than plain Mendix types.
//
// CreateEntity derives this from the semantic model it is given; here the entity
// already exists, so it is read back off the stored document. Getting it wrong
// would write a plain attribute into an external entity, which mxbuild accepts
// and Studio Pro then shows with an empty remote mapping.
func entityIsExternal(ent *genDm.Entity) bool {
	src := ent.Source()
	if src == nil {
		return false
	}
	return src.TypeName() == "Rest$ODataRemoteEntitySource" ||
		src.TypeName() == "DatabaseConnector$DatabaseRemoteEntitySource"
}

// AddAttribute appends an attribute to an existing entity.
//
// Implemented for the api/ package, which is its only caller: ALTER ENTITY
// reaches the same result through the mutator. Until api/ was routed through the
// backend abstraction this method was on FullBackend with no caller that used a
// backend value, which is why it sat unimplemented here (see
// unimplemented_reachability_test.go).
func (b *Backend) AddAttribute(domainModelID, entityID model.ID, attr *domainmodel.Attribute) error {
	if b.writer == nil {
		return fmt.Errorf("AddAttribute: not connected for writing")
	}
	if attr == nil {
		return fmt.Errorf("AddAttribute: nil attribute")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	ent := findGenEntity(dm, entityID)
	if ent == nil {
		return fmt.Errorf("entity not found: %s", entityID)
	}
	for _, el := range ent.AttributesItems() {
		if a, ok := el.(*genDm.Attribute); ok && a.Name() == attr.Name {
			return fmt.Errorf("attribute %q already exists on entity %s", attr.Name, entityID)
		}
	}
	ent.AddAttributes(attributeToGen(attr, entityIsExternal(ent)))
	return b.persistDM(domainModelID, dm)
}

// UpdateAttribute replaces an existing attribute in place.
//
// Replaces rather than merges: the caller hands a whole Attribute, so a field it
// leaves zero is a field it means to clear. Merging would make "set no
// documentation" indistinguishable from "leave the documentation alone", and the
// legacy writer this replaces did not merge either.
//
// The attribute keeps its stored $ID. Minting a fresh one would make the runtime
// treat it as a different attribute — see CLAUDE.md on GUIDs and identity — and
// would churn every reference to it in the same document.
func (b *Backend) UpdateAttribute(domainModelID, entityID model.ID, attr *domainmodel.Attribute) error {
	if b.writer == nil {
		return fmt.Errorf("UpdateAttribute: not connected for writing")
	}
	if attr == nil {
		return fmt.Errorf("UpdateAttribute: nil attribute")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	ent := findGenEntity(dm, entityID)
	if ent == nil {
		return fmt.Errorf("entity not found: %s", entityID)
	}
	items := ent.AttributesItems()
	idx := -1
	for i, el := range items {
		if string(el.ID()) == string(attr.ID) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("attribute not found: %s", attr.ID)
	}

	next := attributeToGen(attr, entityIsExternal(ent))
	next.SetID(items[idx].ID())

	// The generated list offers only Append and Remove, so an in-place replace
	// means rebuilding it. Order is worth the rebuild: it is the order Studio
	// Pro shows the attributes in, and appending instead would move the edited
	// one to the bottom of every entity anyone touches.
	replacement := make([]element.Element, len(items))
	copy(replacement, items)
	replacement[idx] = next
	for i := len(items) - 1; i >= 0; i-- {
		ent.RemoveAttributes(i)
	}
	for _, el := range replacement {
		ent.AddAttributes(el)
	}
	return b.persistDM(domainModelID, dm)
}
