// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#524: `grant write *` on an entity carrying an autonumber wrote
// ReadWrite on it and the build failed CE6592, so the user had to narrow the
// grant with a hand-written REVOKE.
//
// The GRANT path is only half of it. ReconcileMemberAccesses runs on the
// executor's finalize step after EVERY program, so a grant corrected by hand
// was re-broken by the next write touching the module. Both halves shared one
// defect: the downgrade asked whether an attribute was CALCULATED, which an
// autonumber is not — it carries no DomainModels$CalculatedValue, because its
// value comes from the database rather than from a microflow.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// memberRights maps each member reference in an entity's first access rule to
// the rights it carries. memberRefs (reconcile_inherited_assoc_test.go) reports
// presence only, and presence is not the question here.
func memberRights(t *testing.T, b *Backend, modID model.ID, entityName string) map[string]string {
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
			t.Fatalf("entity %s has no access rule", entityName)
		}
		out := map[string]string{}
		for _, ma := range e.AccessRules[0].MemberAccesses {
			if ma.AttributeName != "" {
				out[ma.AttributeName] = string(ma.AccessRights)
			}
		}
		return out
	}
	t.Fatalf("entity %s not found", entityName)
	return nil
}

// autoNumberFixture builds one entity with the three attribute shapes that
// matter to CE6592: a plain string, an autonumber, and a calculated decimal.
func autoNumberFixture(t *testing.T) (*Backend, *model.Module, *domainmodel.DomainModel) {
	t.Helper()
	b, mod, dm := inheritanceFixture(t)

	ent := &domainmodel.Entity{Name: "ZzAutoNum", Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{Name: "Description", Type: &domainmodel.StringAttributeType{Length: 200}},
			{Name: "RequestNumber", Type: &domainmodel.AutoNumberAttributeType{},
				Value: &domainmodel.AttributeValue{Type: "StoredValue", DefaultValue: "1001"}},
			{Name: "TotalCost", Type: &domainmodel.DecimalAttributeType{},
				Value: &domainmodel.AttributeValue{
					Type: "CalculatedValue", MicroflowName: "MyFirstModule.CalcTotal"}},
		}}
	if err := b.CreateEntity(dm.ID, ent); err != nil {
		t.Fatalf("CreateEntity ZzAutoNum: %v", err)
	}
	return b, mod, dm
}

// A rule that already carries ReadWrite on the autonumber — the state the GRANT
// used to produce — must be downgraded, not preserved.
func TestReconcile_DowngradesWriteOnAnExistingAutoNumberEntry(t *testing.T) {
	b, mod, dm := autoNumberFixture(t)

	grantAll(t, b, dm.ID, "ZzAutoNum", []types.EntityMemberAccess{
		{AttributeRef: "MyFirstModule.ZzAutoNum.Description", AccessRights: "ReadWrite"},
		{AttributeRef: "MyFirstModule.ZzAutoNum.RequestNumber", AccessRights: "ReadWrite"},
		{AttributeRef: "MyFirstModule.ZzAutoNum.TotalCost", AccessRights: "ReadWrite"},
	})

	if _, err := b.ReconcileMemberAccesses(dm.ID, mod.Name); err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}

	rights := memberRights(t, b, mod.ID, "ZzAutoNum")
	if got := rights["MyFirstModule.ZzAutoNum.RequestNumber"]; got != "ReadOnly" {
		t.Errorf("autonumber rights = %q, want ReadOnly — write on an autonumber is CE6592 (all: %v)", got, rights)
	}
	// Control: the calculated attribute was already downgraded before the fix,
	// and the plain one must keep its write. Without the second, a blanket
	// "downgrade everything" passes this test while stripping the model.
	if got := rights["MyFirstModule.ZzAutoNum.TotalCost"]; got != "ReadOnly" {
		t.Errorf("calculated rights = %q, want ReadOnly", got)
	}
	if got := rights["MyFirstModule.ZzAutoNum.Description"]; got != "ReadWrite" {
		t.Errorf("plain attribute rights = %q, want ReadWrite — reconcile must not strip granted write", got)
	}
}

// The other direction: an autonumber MISSING from the rule is added by
// reconcile, and must arrive as ReadOnly rather than inheriting the rule's
// ReadWrite default. This is the path that re-broke a hand-corrected grant.
func TestReconcile_AddsAMissingAutoNumberAsReadOnly(t *testing.T) {
	b, mod, dm := autoNumberFixture(t)

	// The shape a user reached by hand: write granted, then REVOKEd off the
	// autonumber, so the rule simply has no entry for it.
	grantAll(t, b, dm.ID, "ZzAutoNum", []types.EntityMemberAccess{
		{AttributeRef: "MyFirstModule.ZzAutoNum.Description", AccessRights: "ReadWrite"},
	})

	if _, err := b.ReconcileMemberAccesses(dm.ID, mod.Name); err != nil {
		t.Fatalf("ReconcileMemberAccesses: %v", err)
	}

	rights := memberRights(t, b, mod.ID, "ZzAutoNum")
	got, ok := rights["MyFirstModule.ZzAutoNum.RequestNumber"]
	if !ok {
		t.Fatalf("reconcile did not add the autonumber at all (CE0066): %v", rights)
	}
	if got != "ReadOnly" {
		t.Errorf("added autonumber rights = %q, want ReadOnly — the rule default is ReadWrite, "+
			"so an added entry that inherits it is CE6592", got)
	}
}
