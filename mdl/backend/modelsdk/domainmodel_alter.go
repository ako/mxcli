// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
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
	found := false
	for _, el := range order {
		if string(el.ID()) == string(entity.ID) {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("entity not found: %s", entity.ID)
	}

	ge := entityToGen(entity, b.moduleNameFor(domainModelID))
	ge.SetID(element.ID(entity.ID))
	assignEntityIDs(ge)

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
	dm.RemoveEntities(eidx)
	removeAssocsReferencing(dm, entityID)
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
		odm, err := b.loadDomainModelGen(other.ID)
		if err != nil {
			return fmt.Errorf("DeleteEntity: cascade cleanup: load %s: %w", other.ID, err)
		}
		if removeAssocsReferencing(odm, entityID) {
			if err := b.persistDM(other.ID, odm); err != nil {
				return fmt.Errorf("DeleteEntity: cascade cleanup: update %s: %w", other.ID, err)
			}
		}
	}
	return nil
}
