// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1047: adding an attribute to a GENERALIZATION left the
// project at CE0066 "Entity access is out of date", and `UPDATE SECURITY` —
// the command that exists to repair exactly that — reported "All entity access
// rules are up to date" and changed nothing. Reported against 0.21.0 on a
// confirmed MPR v2 project; reproduced on both engines.
//
// ReconcileMemberAccesses computes the ancestor set (ownerIDs, via
// sameModuleAncestors) and then uses it ONLY for associations. The attribute
// pass walked ent.AttributesItems() — the entity's own attributes — so a
// specialization's expected member set never contained the members it inherits.
// Nothing looked missing, nothing was added, and `modified` stayed 0, which is
// what the command turns into "up to date".
//
// The sibling case (a rule that HAS the inherited entry, which reconcile must
// not delete) is covered by reconcile_inherited_assoc_test.go; this is the
// other direction, the one #1047 reports.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// shadowFixture builds ZzShadowParent <- ZzShadowChild where BOTH declare an
// attribute called Code, so the child's shadows the ancestor's.
func shadowFixture(t *testing.T) (*Backend, *model.Module, *domainmodel.DomainModel) {
	t.Helper()
	b, mod, dm := inheritanceFixture(t)

	parent := &domainmodel.Entity{Name: "ZzShadowParent", Persistable: true,
		Attributes: []*domainmodel.Attribute{{Name: "Code", Type: &domainmodel.StringAttributeType{}}}}
	if err := b.CreateEntity(dm.ID, parent); err != nil {
		t.Fatalf("CreateEntity ZzShadowParent: %v", err)
	}
	child := &domainmodel.Entity{Name: "ZzShadowChild", Persistable: true,
		GeneralizationRef: "MyFirstModule.ZzShadowParent",
		Attributes:        []*domainmodel.Attribute{{Name: "Code", Type: &domainmodel.StringAttributeType{}}}}
	if err := b.CreateEntity(dm.ID, child); err != nil {
		t.Fatalf("CreateEntity ZzShadowChild: %v", err)
	}
	return b, mod, dm
}

// The reported repro, at this layer: the ancestor gains an attribute after the
// specialization's rule was written, so the rule is missing an inherited member
// and Mendix reports CE0066. Reconcile has to add it — qualified against the
// entity that DECLARES it, which is what Mendix stores and what the reporter's
// own tool added ("+ Spec: ProbeSecond.Gen.AfterSpec (ReadWrite)").
func TestReconcile_AddsAncestorAttributeToASpecializationsRule(t *testing.T) {
	b, mod, dm := inheritanceFixture(t)

	// A rule holding only the specialization's own attribute — the state a
	// project reaches when the generalization gains an attribute afterwards.
	grantAll(t, b, dm.ID, "ZzDerived", []types.EntityMemberAccess{
		{AttributeRef: "MyFirstModule.ZzDerived.Extra", AccessRights: "ReadWrite"},
	})

	modified, err := b.ReconcileMemberAccesses(dm.ID, mod.Name)
	if err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}

	refs := memberRefs(t, b, mod.ID, "ZzDerived")
	if !hasRef(refs, "attr:MyFirstModule.ZzBase.Code") {
		t.Fatalf("reconcile did not add the inherited attribute: %v\n"+
			"this is the CE0066 in mendixlabs/mxcli#1047, and the rule Mendix rejects", refs)
	}
	// The count is what `update security` prints. Reporting 0 here is how the
	// command came to say "All entity access rules are up to date" over a
	// project that mx check rejects — a false success, worse than an error.
	if modified == 0 {
		t.Error("reconcile added the member but reported 0 modified; `update security` would still say 'up to date'")
	}

	// The specialization's own attribute must survive the change.
	if !hasRef(refs, "attr:MyFirstModule.ZzDerived.Extra") {
		t.Errorf("reconcile dropped the entity's own attribute: %v", refs)
	}
}

// The control that keeps the test above honest: reconcile must not invent
// members for an entity with no generalization. If it reported a change here,
// the test above would pass against a fix that simply marks everything dirty.
func TestReconcile_LeavesAnUnrelatedEntitysRuleAlone(t *testing.T) {
	b, mod, dm := inheritanceFixture(t)

	grantAll(t, b, dm.ID, "ZzOther", []types.EntityMemberAccess{
		{AttributeRef: "MyFirstModule.ZzOther.Label", AccessRights: "ReadWrite"},
	})

	before := memberRefs(t, b, mod.ID, "ZzOther")
	if _, err := b.ReconcileMemberAccesses(dm.ID, mod.Name); err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}
	after := memberRefs(t, b, mod.ID, "ZzOther")

	if len(before) != len(after) {
		t.Errorf("an entity with no generalization changed: %v -> %v", before, after)
	}
	for _, r := range after {
		if r != "attr:MyFirstModule.ZzOther.Label" {
			t.Errorf("reconcile invented a member on an unrelated entity: %v", after)
		}
	}
}

// An attribute the specialization declares under a name its ancestor also uses
// SHADOWS the ancestor's, exactly as the executor's own member walk
// (EntityMembersFor) treats it. Emitting both would put two entries in the rule
// for one member the modeller sees.
func TestReconcile_ChildAttributeShadowsTheAncestorsOfTheSameName(t *testing.T) {
	b, mod, dm := shadowFixture(t)

	grantAll(t, b, dm.ID, "ZzShadowChild", []types.EntityMemberAccess{
		{AttributeRef: "MyFirstModule.ZzShadowChild.Code", AccessRights: "ReadWrite"},
	})

	if _, err := b.ReconcileMemberAccesses(dm.ID, mod.Name); err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}

	refs := memberRefs(t, b, mod.ID, "ZzShadowChild")
	if hasRef(refs, "attr:MyFirstModule.ZzShadowParent.Code") {
		t.Errorf("the ancestor's shadowed attribute was added alongside the child's: %v", refs)
	}
	if !hasRef(refs, "attr:MyFirstModule.ZzShadowChild.Code") {
		t.Errorf("the child's own attribute was lost: %v", refs)
	}
}
