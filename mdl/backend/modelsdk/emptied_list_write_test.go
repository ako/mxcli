// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// emptiedListEntity creates an entity carrying exactly ONE attribute and ONE
// validation rule on it, then hands back a fresh read of it.
//
// One of each is the whole point. A child list that still has members after an
// update is dirty (entityToGen appended to it), so the codec re-emits it and
// everything looks fine; the raw-passthrough bug only shows when the update
// removes the LAST member. A fixture with two of anything passes against the
// broken writer — measured, not assumed.
func emptiedListEntity(t *testing.T, name string) (*Backend, string, model.ID, *domainmodel.Entity) {
	// modID, not the domain-model ID: GetDomainModel is keyed by MODULE, while
	// UpdateEntity takes the domain model's own ID. Carrying the module ID and
	// re-reading the domain model keeps that straight across a reconnect.
	t.Helper()
	proj := copyFixture(t)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName(MyFirstModule) = %v, %v", mod, err)
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		t.Fatalf("GetDomainModel = %v, %v", dm, err)
	}

	ent := &domainmodel.Entity{Name: name, Persistable: true}
	ent.Attributes = []*domainmodel.Attribute{{Name: "Solo", Type: &domainmodel.StringAttributeType{}}}
	if err := b.CreateEntity(dm.ID, ent); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}

	stored := reread(t, b, mod.ID, name)
	if len(stored.Attributes) != 1 {
		t.Fatalf("fixture entity has %d attributes, want exactly 1", len(stored.Attributes))
	}
	// Add the validation rule now that the attribute has a persisted identity.
	stored.ValidationRules = []*domainmodel.ValidationRule{{
		BaseElement: model.BaseElement{ID: model.ID(stored.Attributes[0].ID)},
		AttributeID: model.ID("MyFirstModule." + name + ".Solo"),
		Type:        "Required",
	}}
	if err := b.UpdateEntity(dm.ID, stored); err != nil {
		t.Fatalf("UpdateEntity (seed rule): %v", err)
	}

	stored = reread(t, b, mod.ID, name)
	if len(stored.ValidationRules) != 1 {
		t.Fatalf("seeded %d validation rules, want exactly 1", len(stored.ValidationRules))
	}
	return b, proj, mod.ID, stored
}

// dmIDFor returns the domain model's own ID, which UpdateEntity requires.
func dmIDFor(t *testing.T, b *Backend, modID model.ID) model.ID {
	t.Helper()
	dm, err := b.GetDomainModel(modID)
	if err != nil || dm == nil {
		t.Fatalf("GetDomainModel = %v, %v", dm, err)
	}
	return dm.ID
}

func reread(t *testing.T, b *Backend, modID model.ID, name string) *domainmodel.Entity {
	t.Helper()
	dm, err := b.GetDomainModel(modID)
	if err != nil || dm == nil {
		t.Fatalf("GetDomainModel = %v, %v", dm, err)
	}
	for _, e := range dm.Entities {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("entity %q not found", name)
	return nil
}

// reopen closes the backend and returns a fresh one, so assertions are made
// against what reached disk rather than against in-memory state.
func reopen(t *testing.T, b *Backend, proj string, modID model.ID, name string) *domainmodel.Entity {
	t.Helper()
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })
	return reread(t, b2, modID, name)
}

// TestUpdateEntityPersistsRemovalOfLastValidationRule is the regression for the
// CE1613 that DROP ATTRIBUTE produced:
//
//	[error] [CE1613] "The selected attribute 'DropAttr.Account.AccountNumber'
//	no longer exists." at Validation rule of entity 'DropAttr.Account'
//
// The executor removed the rule correctly; the write dropped the removal on the
// floor, because an emptied child list is "clean" and the codec then keeps the
// stored raw bytes.
func TestUpdateEntityPersistsRemovalOfLastValidationRule(t *testing.T) {
	b, proj, modID, ent := emptiedListEntity(t, "EmptiedRules")

	ent.ValidationRules = nil
	if err := b.UpdateEntity(dmIDFor(t, b, modID), ent); err != nil {
		t.Fatalf("UpdateEntity: %v", err)
	}

	got := reopen(t, b, proj, modID, "EmptiedRules")
	if len(got.ValidationRules) != 0 {
		t.Errorf("%d validation rule(s) survived after removing the last one — this is CE1613", len(got.ValidationRules))
	}
	// The rest of the entity must be intact: a writer that clears the list by
	// clearing the entity would pass the check above.
	if len(got.Attributes) != 1 {
		t.Errorf("attributes = %d, want 1 (untouched)", len(got.Attributes))
	}
}

// TestUpdateEntityPersistsRemovalOfLastAttribute covers the same mechanism on
// the Attributes list, where it is worse than a dangling reference: DROP
// ATTRIBUTE reported success and wrote nothing at all.
func TestUpdateEntityPersistsRemovalOfLastAttribute(t *testing.T) {
	b, proj, modID, ent := emptiedListEntity(t, "EmptiedAttrs")

	// The rule has to go with it — a rule referencing a removed attribute is
	// the CE1613 above, not what this test is about.
	ent.ValidationRules = nil
	ent.Attributes = nil
	if err := b.UpdateEntity(dmIDFor(t, b, modID), ent); err != nil {
		t.Fatalf("UpdateEntity: %v", err)
	}

	got := reopen(t, b, proj, modID, "EmptiedAttrs")
	if len(got.Attributes) != 0 {
		t.Errorf("%d attribute(s) survived after removing the last one — the drop silently did nothing", len(got.Attributes))
	}
}

// TestUpdateEntityKeepsRemainingListMembers is the positive control. Removing
// one of TWO rules always worked, because the surviving member dirties the
// list — so this is the case that would have hidden the bug. It has to keep
// passing, or the fix has traded a stale list for a cleared one.
func TestUpdateEntityKeepsRemainingListMembers(t *testing.T) {
	b, proj, modID, ent := emptiedListEntity(t, "KeptRules")

	ent.Attributes = append(ent.Attributes, &domainmodel.Attribute{Name: "Second", Type: &domainmodel.StringAttributeType{}})
	if err := b.UpdateEntity(dmIDFor(t, b, modID), ent); err != nil {
		t.Fatalf("UpdateEntity (add second attribute): %v", err)
	}

	got := reopen(t, b, proj, modID, "KeptRules")
	if len(got.Attributes) != 2 {
		t.Fatalf("attributes = %d, want 2", len(got.Attributes))
	}
	if len(got.ValidationRules) != 1 {
		t.Errorf("validation rules = %d, want 1 (the seeded rule must survive an unrelated update)", len(got.ValidationRules))
	}
}
