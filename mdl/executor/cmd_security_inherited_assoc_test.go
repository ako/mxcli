// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// mxcli-chat FINDINGS §26: a GRANT naming an association declared on the
// generalization was refused —
//
//	entity OpenAIConnector.OpenAIDeployedModel has no member(s)
//	DeployedModel_InputModality; grant only names members of the entity or of an
//	entity it inherits from
//
// — which contradicts its own rule: OpenAIDeployedModel extends
// GenAICommons.DeployedModel, and that association is declared on the parent.
// Inherited *attributes* resolved; inherited *associations* did not, which made
// the rule the module itself ships impossible to express in MDL.
func inheritedAssocFixture(t *testing.T) (*ExecContext, **backend.EntityAccessRuleParams) {
	t.Helper()
	const (
		baseID    = model.ID("e-base")
		derivedID = model.ID("e-derived")
		otherID   = model.ID("e-other")
	)
	mod := mkModule("IT")
	h := mkHierarchy(mod)

	base := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: baseID},
		Name:        "Base",
		Attributes:  []*domainmodel.Attribute{{Name: "Code"}},
	}
	derived := &domainmodel.Entity{
		BaseElement:       model.BaseElement{ID: derivedID},
		Name:              "Derived",
		Attributes:        []*domainmodel.Attribute{{Name: "Extra"}},
		GeneralizationRef: "IT.Base",
	}
	other := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: otherID},
		Name:        "Other",
		Attributes:  []*domainmodel.Attribute{{Name: "Label"}},
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-it"},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{base, derived, other},
		Associations: []*domainmodel.Association{{
			Name:     "Base_Other",
			ParentID: baseID, // declared on the GENERALIZATION
			ChildID:  otherID,
			Owner:    domainmodel.AssociationOwnerDefault,
		}},
	}

	var captured *backend.EntityAccessRuleParams
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc:  func(string) (*model.Module, error) { return mod, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{ModuleRoles: []*security.ModuleRole{{Name: "Admin"}}}, nil
		},
		AddEntityAccessRuleFunc: func(p backend.EntityAccessRuleParams) error {
			cp := p
			captured = &cp
			return nil
		},
		ReconcileMemberAccessesFunc: func(model.ID, string) (int, error) { return 0, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, &captured
}

func TestGrantEntityAccess_NamingAnInheritedAssociationIsAccepted(t *testing.T) {
	ctx, captured := inheritedAssocFixture(t)

	err := execGrantEntityAccess(ctx, &ast.GrantEntityAccessStmt{
		Entity: ast.QualifiedName{Module: "IT", Name: "Derived"},
		Roles:  []ast.QualifiedName{{Module: "IT", Name: "Admin"}},
		Rights: []ast.EntityAccessRight{{
			Type:    ast.EntityAccessReadMembers,
			Members: []string{"Extra", "Code", "Base_Other"},
		}},
	})
	if err != nil {
		t.Fatalf("grant naming an inherited association was refused: %v", err)
	}
	if *captured == nil {
		t.Fatal("no rule was written")
	}
	var found bool
	for _, ma := range (*captured).MemberAccesses {
		if ma.AssociationRef == "IT.Base_Other" {
			found = true
		}
	}
	if !found {
		t.Errorf("no MemberAccess for the inherited association: %+v", (*captured).MemberAccesses)
	}
}

// READ * covers inherited members too, associations included.
func TestGrantEntityAccess_ReadAllCoversAnInheritedAssociation(t *testing.T) {
	ctx, captured := inheritedAssocFixture(t)

	if err := execGrantEntityAccess(ctx, &ast.GrantEntityAccessStmt{
		Entity: ast.QualifiedName{Module: "IT", Name: "Derived"},
		Roles:  []ast.QualifiedName{{Module: "IT", Name: "Admin"}},
		Rights: []ast.EntityAccessRight{{Type: ast.EntityAccessReadAll}},
	}); err != nil {
		t.Fatalf("grant failed: %v", err)
	}
	for _, ma := range (*captured).MemberAccesses {
		if ma.AssociationRef == "IT.Base_Other" {
			return
		}
	}
	t.Errorf("READ * left the inherited association out: %+v", (*captured).MemberAccesses)
}

// The negative half: the association must NOT appear on an unrelated entity's
// rule. A walk that collected every association in the module would pass the
// tests above and put entries where Mendix reports CE0066 for having them.
func TestGrantEntityAccess_InheritedAssociationOnlyOnSpecializations(t *testing.T) {
	ctx, captured := inheritedAssocFixture(t)

	if err := execGrantEntityAccess(ctx, &ast.GrantEntityAccessStmt{
		Entity: ast.QualifiedName{Module: "IT", Name: "Other"},
		Roles:  []ast.QualifiedName{{Module: "IT", Name: "Admin"}},
		Rights: []ast.EntityAccessRight{{Type: ast.EntityAccessReadAll}},
	}); err != nil {
		t.Fatalf("grant failed: %v", err)
	}
	for _, ma := range (*captured).MemberAccesses {
		if ma.AssociationRef == "IT.Base_Other" {
			t.Errorf("the TO entity of a Default-owner association got a MemberAccess for it (CE0066): %+v",
				(*captured).MemberAccesses)
		}
	}
}
