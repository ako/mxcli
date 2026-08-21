// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// dropCleanupCtx builds an ExecContext over a Bank.Account entity whose
// validation rules and access-rule members reference their attribute the way a
// backend actually hands them back: by QUALIFIED NAME, not by element ID.
//
// Both engines do this — sdk/mpr's parseValidationRule stores the "Attribute"
// string verbatim, and mdl/backend/modelsdk/domainmodel.go says so in a comment
// ("qualified name; ruleInfoToGen handles it"). The indexes deliberately use
// element IDs, because AttributePointer really is one: they are the control
// showing the drop path itself works when the reference form matches.
func dropCleanupCtx(t *testing.T) (*ExecContext, **domainmodel.Entity) {
	t.Helper()
	mod := mkModule("Bank")
	acct := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		ContainerID: nextID("dm"),
		Name:        "Account",
		Persistable: true,
	}

	qn := func(attr string) model.ID { return model.ID(fmt.Sprintf("Bank.Account.%s", attr)) }
	for _, name := range []string{"AccountNumber", "Balance"} {
		acct.Attributes = append(acct.Attributes, &domainmodel.Attribute{
			BaseElement: model.BaseElement{ID: nextID("attr")},
			Name:        name,
		})
	}
	numberID := acct.Attributes[0].ID

	acct.ValidationRules = []*domainmodel.ValidationRule{
		{BaseElement: model.BaseElement{ID: nextID("vr")}, AttributeID: qn("AccountNumber"), Type: "Required"},
		{BaseElement: model.BaseElement{ID: nextID("vr")}, AttributeID: qn("AccountNumber"), Type: "Unique"},
		{BaseElement: model.BaseElement{ID: nextID("vr")}, AttributeID: qn("Balance"), Type: "Required"},
	}
	// AttributeName is QUALIFIED in both engines, and which field is populated
	// differs between them: the legacy backend fills AttributeID and
	// AttributeName, while modelsdk (memberAccessFromGen) fills only
	// AttributeName. One member of each shape, so the cleanup cannot pass by
	// looking at one field.
	acct.AccessRules = []*domainmodel.AccessRule{{
		BaseElement: model.BaseElement{ID: nextID("ar")},
		MemberAccesses: []*domainmodel.MemberAccess{
			{AttributeName: string(qn("AccountNumber"))},                       // modelsdk shape
			{AttributeName: string(qn("Balance")), AttributeID: qn("Balance")}, // legacy shape
		},
	}}
	acct.Indexes = []*domainmodel.Index{{
		BaseElement:  model.BaseElement{ID: nextID("idx")},
		Name:         "Idx_AccountNumber",
		Attributes:   []*domainmodel.IndexAttribute{{AttributeID: numberID}},
		AttributeIDs: []model.ID{numberID},
	}}

	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{acct},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	withContainer(h, acct.ContainerID, dm.ID)

	var saved *domainmodel.Entity
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		UpdateEntityFunc:     func(dmID model.ID, e *domainmodel.Entity) error { saved = e; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, &saved
}

func dropAccountNumber(t *testing.T, ctx *ExecContext) {
	t.Helper()
	err := execAlterEntity(ctx, &ast.AlterEntityStmt{
		Name:          ast.QualifiedName{Module: "Bank", Name: "Account"},
		Operation:     ast.AlterEntityDropAttribute,
		AttributeName: "AccountNumber",
	})
	assertNoError(t, err)
}

// TestDropAttributeRemovesValidationRulesReferencedByQualifiedName is the
// regression for the CE1613 this fixes:
//
//	[error] [CE1613] "The selected attribute 'Bank.Account.AccountNumber' no
//	longer exists." at Validation rule of entity 'Bank.Account'
//
// The rule outlived its attribute because the cleanup compared a qualified name
// against an element ID, so it never matched anything and quietly kept every
// rule. Measured on a real 11.13.0 project before the fix.
func TestDropAttributeRemovesValidationRulesReferencedByQualifiedName(t *testing.T) {
	ctx, saved := dropCleanupCtx(t)
	dropAccountNumber(t, ctx)

	if *saved == nil {
		t.Fatal("expected the drop to write the entity")
	}
	for _, vr := range (*saved).ValidationRules {
		if strings.HasSuffix(string(vr.AttributeID), ".AccountNumber") {
			t.Errorf("validation rule %q outlived its attribute — this is CE1613", vr.AttributeID)
		}
	}
	// The surviving rule must be exactly Balance's: a cleanup that drops
	// everything would pass the check above while corrupting the entity.
	if got := len((*saved).ValidationRules); got != 1 {
		t.Fatalf("kept %d validation rules, want 1 (Balance's)", got)
	}
	if got := (*saved).ValidationRules[0].AttributeID; got != "Bank.Account.Balance" {
		t.Errorf("kept the wrong rule: %q", got)
	}
}

// TestDropAttributeRemovesMemberAccessesReferencedByQualifiedName covers the
// same mismatch in the access-rule cleanup. A post-execution pass
// (ReconcileMemberAccesses) already repairs this downstream, so it is not a
// user-visible defect on its own — but the cleanup here reports a count it
// could never produce, and the reconcile does not run on every path.
func TestDropAttributeRemovesMemberAccessesReferencedByQualifiedName(t *testing.T) {
	ctx, saved := dropCleanupCtx(t)
	dropAccountNumber(t, ctx)

	members := (*saved).AccessRules[0].MemberAccesses
	for _, ma := range members {
		if strings.HasSuffix(string(ma.AttributeID), ".AccountNumber") || strings.HasSuffix(ma.AttributeName, ".AccountNumber") {
			t.Errorf("member access %q/%q outlived its attribute", ma.AttributeID, ma.AttributeName)
		}
	}
	if got := len(members); got != 1 {
		t.Fatalf("kept %d member accesses, want 1 (Balance's)", got)
	}

	out := ctx.Output.(interface{ String() string }).String()
	if !strings.Contains(out, "access rule member reference") {
		t.Errorf("the cleanup removed a member but did not report it; output was:\n%s", out)
	}
}

// TestDropAttributeStillCleansIndexes is the positive control. Index
// references are element IDs, so this path always worked — it proves the test
// harness drives a real drop, and that the fix did not trade one reference form
// for the other.
func TestDropAttributeStillCleansIndexes(t *testing.T) {
	ctx, saved := dropCleanupCtx(t)
	dropAccountNumber(t, ctx)

	if got := len((*saved).Indexes); got != 0 {
		t.Errorf("kept %d indexes, want 0 (the only column was dropped)", got)
	}
	out := ctx.Output.(interface{ String() string }).String()
	if !strings.Contains(out, "Removed 1 index(es)") {
		t.Errorf("expected the index removal to be reported; output was:\n%s", out)
	}
}
