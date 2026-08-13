// SPDX-License-Identifier: Apache-2.0

// mxcli-chat FINDINGS §25: `GRANT <role> ON Module.Specialization (READ *, WRITE *)`
// produced a model Mendix rejects with CE0066 "Entity access is out of date",
// while the same grant on the generalization checked clean.
//
// A specialization has every member of its generalization, associations
// included. The executor's grant walk learned to collect them (#26), but
// ReconcileMemberAccesses — which runs after every program and after every new
// association — recomputed the expected member set from the entity's OWN
// attributes and the module's FROM-side associations only. An inherited
// association was therefore "stale" and deleted, immediately after the grant
// that had just written it. Measured: the executor passed 3 entries, storage
// held 2.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// inheritanceFixture builds Base <- Derived plus an unrelated Other, with
// Base_Other declared FROM Base, and returns the connected backend + module.
func inheritanceFixture(t *testing.T) (*Backend, *model.Module, *domainmodel.DomainModel) {
	t.Helper()
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}

	base := &domainmodel.Entity{Name: "ZzBase", Persistable: true,
		Attributes: []*domainmodel.Attribute{{Name: "Code", Type: &domainmodel.StringAttributeType{}}}}
	other := &domainmodel.Entity{Name: "ZzOther", Persistable: true,
		Attributes: []*domainmodel.Attribute{{Name: "Label", Type: &domainmodel.StringAttributeType{}}}}
	for _, e := range []*domainmodel.Entity{base, other} {
		if err := b.CreateEntity(dm.ID, e); err != nil {
			t.Fatalf("CreateEntity %s: %v", e.Name, err)
		}
	}
	derived := &domainmodel.Entity{Name: "ZzDerived", Persistable: true,
		GeneralizationRef: "MyFirstModule.ZzBase",
		Attributes:        []*domainmodel.Attribute{{Name: "Extra", Type: &domainmodel.StringAttributeType{}}}}
	if err := b.CreateEntity(dm.ID, derived); err != nil {
		t.Fatalf("CreateEntity ZzDerived: %v", err)
	}
	// CreateEntity does not write the minted ID back onto the struct, so the
	// association's endpoints have to be read out of the stored model.
	ids := entityIDs(t, b, mod.ID)
	if err := b.CreateAssociation(dm.ID, &domainmodel.Association{
		Name: "ZzBase_ZzOther", ParentID: ids["ZzBase"], ChildID: ids["ZzOther"],
		Type: "Reference", Owner: "Default",
	}); err != nil {
		t.Fatalf("CreateAssociation: %v", err)
	}
	return b, mod, dm
}

// entityIDs maps entity name -> stored ID for a module's domain model.
func entityIDs(t *testing.T, b *Backend, modID model.ID) map[string]model.ID {
	t.Helper()
	dm, err := b.GetDomainModel(modID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	out := map[string]model.ID{}
	for _, e := range dm.Entities {
		out[e.Name] = e.ID
	}
	return out
}

// memberRefs returns an entity's first access rule's member references, in
// storage order, as "attr:X" / "assoc:X".
func memberRefs(t *testing.T, b *Backend, modID model.ID, entityName string) []string {
	t.Helper()
	dm, err := b.GetDomainModel(modID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	for _, e := range dm.Entities {
		if e.Name != entityName {
			continue
		}
		if len(e.AccessRules) == 0 {
			return nil
		}
		var out []string
		for _, ma := range e.AccessRules[0].MemberAccesses {
			switch {
			case ma.AttributeName != "":
				out = append(out, "attr:"+ma.AttributeName)
			case ma.AssociationName != "":
				out = append(out, "assoc:"+ma.AssociationName)
			}
		}
		return out
	}
	t.Fatalf("entity %s not found", entityName)
	return nil
}

func hasRef(refs []string, want string) bool {
	for _, r := range refs {
		if r == want {
			return true
		}
	}
	return false
}

func grantAll(t *testing.T, b *Backend, dmID model.ID, entityName string, members []types.EntityMemberAccess) {
	t.Helper()
	if err := b.AddEntityAccessRule(backend.EntityAccessRuleParams{
		UnitID: dmID, EntityName: entityName,
		RoleNames:           []string{"MyFirstModule.User"},
		DefaultMemberAccess: "ReadWrite",
		MemberAccesses:      members,
	}); err != nil {
		t.Fatalf("AddEntityAccessRule %s: %v", entityName, err)
	}
}

func TestReconcile_KeepsAssociationInheritedFromAGeneralization(t *testing.T) {
	b, mod, dm := inheritanceFixture(t)

	// What the executor's grant walk writes for `READ *, WRITE *` on the
	// specialization: own attribute, inherited attribute, inherited association.
	grantAll(t, b, dm.ID, "ZzDerived", []types.EntityMemberAccess{
		{AttributeRef: "MyFirstModule.ZzDerived.Extra", AccessRights: "ReadWrite"},
		{AttributeRef: "MyFirstModule.ZzBase.Code", AccessRights: "ReadWrite"},
		{AssociationRef: "MyFirstModule.ZzBase_ZzOther", AccessRights: "ReadWrite"},
	})

	if _, err := b.ReconcileMemberAccesses(dm.ID, mod.Name); err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}

	refs := memberRefs(t, b, mod.ID, "ZzDerived")
	if !hasRef(refs, "assoc:MyFirstModule.ZzBase_ZzOther") {
		t.Fatalf("reconcile dropped the inherited association: %v — this is the CE0066", refs)
	}
	if !hasRef(refs, "attr:MyFirstModule.ZzBase.Code") {
		t.Errorf("reconcile dropped the inherited attribute (#758 regression): %v", refs)
	}
}

// Reconcile also has to ADD it: a rule written before the ancestor gained the
// association must pick it up, which is what Studio Pro's "Update security"
// button does and what `create association` triggers.
func TestReconcile_AddsAncestorAssociationToASpecializationsRule(t *testing.T) {
	b, mod, dm := inheritanceFixture(t)

	grantAll(t, b, dm.ID, "ZzDerived", []types.EntityMemberAccess{
		{AttributeRef: "MyFirstModule.ZzDerived.Extra", AccessRights: "ReadWrite"},
	})

	if _, err := b.ReconcileMemberAccesses(dm.ID, mod.Name); err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}

	if refs := memberRefs(t, b, mod.ID, "ZzDerived"); !hasRef(refs, "assoc:MyFirstModule.ZzBase_ZzOther") {
		t.Fatalf("reconcile did not add the ancestor's association: %v", refs)
	}
}

// The control that a walk collecting every association in the module would fail:
// ZzOther is the TO side of an `OWNER Default` association and is not related to
// ZzBase, so it must NOT gain an entry. Mendix reports CE0066 for having one.
func TestReconcile_DoesNotGiveAnUnrelatedEntityTheAssociation(t *testing.T) {
	b, mod, dm := inheritanceFixture(t)

	grantAll(t, b, dm.ID, "ZzOther", []types.EntityMemberAccess{
		{AttributeRef: "MyFirstModule.ZzOther.Label", AccessRights: "ReadWrite"},
	})

	if _, err := b.ReconcileMemberAccesses(dm.ID, mod.Name); err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}

	if refs := memberRefs(t, b, mod.ID, "ZzOther"); hasRef(refs, "assoc:MyFirstModule.ZzBase_ZzOther") {
		t.Fatalf("the TO side of an OWNER Default association got a member entry: %v", refs)
	}
}

// Preserving inherited references must not blunt stale detection: an association
// of THIS module that no longer exists is still removed, on the specialization
// as well as on the entity that declared it (Mendix: CE1613).
func TestReconcile_StillDropsAnAssociationThatNoLongerExists(t *testing.T) {
	b, mod, dm := inheritanceFixture(t)

	grantAll(t, b, dm.ID, "ZzDerived", []types.EntityMemberAccess{
		{AttributeRef: "MyFirstModule.ZzDerived.Extra", AccessRights: "ReadWrite"},
		{AssociationRef: "MyFirstModule.ZzBase_ZzGone", AccessRights: "ReadWrite"},
	})

	if _, err := b.ReconcileMemberAccesses(dm.ID, mod.Name); err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}

	if refs := memberRefs(t, b, mod.ID, "ZzDerived"); hasRef(refs, "assoc:MyFirstModule.ZzBase_ZzGone") {
		t.Fatalf("a deleted association was preserved: %v", refs)
	}
}

// An ancestor in another module cannot be resolved from this domain model, so
// its associations are preserved rather than validated — the same "preserve what
// cannot be checked" rule the attribute branch uses (#758).
func TestReconcile_PreservesAnAssociationFromAnotherModule(t *testing.T) {
	b, mod, dm := inheritanceFixture(t)

	grantAll(t, b, dm.ID, "ZzDerived", []types.EntityMemberAccess{
		{AttributeRef: "MyFirstModule.ZzDerived.Extra", AccessRights: "ReadWrite"},
		{AssociationRef: "GenAICommons.DeployedModel_Provider", AccessRights: "ReadWrite"},
	})

	if _, err := b.ReconcileMemberAccesses(dm.ID, mod.Name); err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}

	if refs := memberRefs(t, b, mod.ID, "ZzDerived"); !hasRef(refs, "assoc:GenAICommons.DeployedModel_Provider") {
		t.Fatalf("a reference this domain model cannot validate was deleted: %v", refs)
	}
}
