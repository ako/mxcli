// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func init() {
	// applyDefaults for domain-model elements: the fields Studio Pro adds
	// internally on create that genDm.NewEntity() does not yet set (confirmed
	// against real Studio-Pro BSON in mx-test-projects/test7-app). The encoder
	// emits these for fresh elements of the registered $Type.
	codec.RegisterTypeDefaults("DomainModels$EntityImpl", codec.TypeDefaults{
		EmitGUID:       true,
		MandatoryLists: []string{"Attributes", "AccessRules", "ValidationRules", "Indexes", "Events"},
	})
}

// CreateEntity is the Phase-2 write slice: add an entity to a domain model
// through the codec engine. Entities are children of the DomainModel unit, so
// this loads the DM element, adds the new entity child (marking it dirty), and
// re-encodes the whole unit — the roundtrip encoder passes existing sibling
// entities through verbatim and freshly encodes only the new one.
func (b *Backend) CreateEntity(domainModelID model.ID, entity *domainmodel.Entity) error {
	if entity == nil {
		return fmt.Errorf("CreateEntity: nil entity")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateEntity: not connected for writing")
	}
	dm, err := b.loadDomainModelGen(domainModelID)
	if err != nil {
		return err
	}
	ge := entityToGen(entity)
	assignEntityIDs(ge)
	dm.AddEntities(ge)

	enc := &codec.Encoder{}
	contents, err := enc.Encode(dm)
	if err != nil {
		return fmt.Errorf("CreateEntity: encode domain model: %w", err)
	}
	if err := b.writer.UpdateRawUnit(string(domainModelID), contents); err != nil {
		return fmt.Errorf("CreateEntity: persist domain model %s: %w", domainModelID, err)
	}
	return nil
}

// loadDomainModelGen decodes a DomainModel unit into its gen element so it can
// be mutated and re-encoded.
func (b *Backend) loadDomainModelGen(id model.ID) (*genDm.DomainModel, error) {
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		return nil, fmt.Errorf("load domain model %s: %w", id, err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("domain model %s not found", id)
	}
	dec := codec.NewDecoder(codec.DefaultRegistry)
	el, err := dec.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("decode domain model %s: %w", id, err)
	}
	dm, ok := el.(*genDm.DomainModel)
	if !ok {
		return nil, fmt.Errorf("unit %s is not a domain model (got %T)", id, el)
	}
	return dm, nil
}

// entityToGen is the write-direction adapter (domainmodel.Entity → genDm.Entity).
// Phase-2 slice scope: the entity header — name, documentation, location, and
// generalization (NoGeneralization with persistability + system-attribute flags,
// or a Generalization parent ref). Attributes/indexes/access rules come with the
// domain-model breadth step.
func entityToGen(e *domainmodel.Entity) *genDm.Entity {
	out := genDm.NewEntity()
	out.SetName(e.Name)
	out.SetDocumentation(e.Documentation)
	out.SetLocation(fmt.Sprintf("%d;%d", e.Location.X, e.Location.Y))
	// ExportLevel is a mandatory scalar NewEntity's pending applyDefaults does not
	// yet set (engalar tech-debt Fix 4). The entity GUID and the empty member
	// arrays (Attributes/AccessRules/…) legacy also emits are NOT settable on the
	// gen Entity / not emitted for unset PartLists — that residual is the
	// documented applyDefaults gap (see the write-parity known-gap test).
	out.SetExportLevel("Hidden")

	if e.GeneralizationRef != "" {
		g := genDm.NewGeneralization()
		g.SetGeneralizationQualifiedName(e.GeneralizationRef)
		out.SetGeneralization(g)
	} else {
		ng := genDm.NewNoGeneralization()
		ng.SetPersistable(e.Persistable)
		// Legacy omits the system-attribute flags when false, so only set the
		// true ones to keep the BSON in parity.
		if e.HasOwner {
			ng.SetHasOwner(true)
		}
		if e.HasChangedBy {
			ng.SetHasChangedBy(true)
		}
		if e.HasCreatedDate {
			ng.SetHasCreatedDate(true)
		}
		if e.HasChangedDate {
			ng.SetHasChangedDate(true)
		}
		out.SetGeneralization(ng)
	}
	return out
}

// assignEntityIDs gives the entity and its generalization fresh IDs (mirrors
// engalar's assignEntityIDsGen; extended to attributes/etc. with breadth).
func assignEntityIDs(e *genDm.Entity) {
	assignID(e)
	assignID(e.Generalization())
}

func assignID(elem element.Element) {
	ider, ok := elem.(interface {
		ID() element.ID
		SetID(element.ID)
	})
	if !ok || ider == nil || ider.ID() != "" {
		return
	}
	ider.SetID(element.ID(mmpr.GenerateID()))
}
