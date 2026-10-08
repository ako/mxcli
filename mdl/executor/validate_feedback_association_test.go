// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// upstream #1322: a validation feedback aimed at an association written by its
// BARE name was stored as an attribute reference.
//
//	create association "G45"."Input_Person" from "G45"."Input" to "G45"."Person" type Reference;
//	…
//	validation feedback $Input/Input_Person message 'Pick a person.';
//
//	[CE1613] "The selected attribute 'G45.Input.Input_Person' no longer exists."
//	         at Validation feedback activity 'Show validation message on member Input_Person of Input'
//
// `check --references` and `exec` pass; the qualified spelling
// (`$Input/G45.Input_Person`) builds clean. The builder decided attribute vs
// association from the dot count alone, so a dotless name was always an
// attribute — the change-object writer (resolveMemberChange) already looks the
// bare name up in the domain model.

// feedbackAssocFixture is G45.Input → G45.Person (the report), plus
// M.Base → M.Target inherited by M.Child, and a System.User specialization
// inheriting the cross-module System.UserRoles.
func feedbackAssocFixture(t *testing.T) *flowBuilder {
	t.Helper()
	g45 := mkModule("G45")
	m := mkModule("M")
	sys := &model.Module{Name: "System"}
	sys.ID = model.ID("m-system-1322")

	person := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e-person"}, Name: "Person",
		Attributes: []*domainmodel.Attribute{{Name: "Name"}}}
	input := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e-input"}, Name: "Input",
		Attributes: []*domainmodel.Attribute{{Name: "Note"}}}
	g45DM := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-g45"},
		ContainerID: g45.ID,
		Entities:    []*domainmodel.Entity{person, input},
		Associations: []*domainmodel.Association{{
			Name: "Input_Person", ParentID: input.ID, ChildID: person.ID, Type: domainmodel.AssociationTypeReference,
		}},
	}

	base := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e-base"}, Name: "Base"}
	child := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e-child"}, Name: "Child", GeneralizationRef: "M.Base"}
	target := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e-target"}, Name: "Target"}
	account := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e-account"}, Name: "Account", GeneralizationRef: "System.User"}
	mDM := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-m"},
		ContainerID: m.ID,
		Entities:    []*domainmodel.Entity{base, child, target, account},
		Associations: []*domainmodel.Association{{
			Name: "Base_Target", ParentID: base.ID, ChildID: target.ID, Type: domainmodel.AssociationTypeReference,
		}},
	}

	user := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e-user"}, Name: "User"}
	userRole := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e-userrole"}, Name: "UserRole"}
	sysDM := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-sys"},
		ContainerID: sys.ID,
		Entities:    []*domainmodel.Entity{user, userRole},
		Associations: []*domainmodel.Association{{
			Name: "UserRoles", ParentID: user.ID, ChildID: userRole.ID, Type: domainmodel.AssociationTypeReferenceSet,
		}},
	}

	modules := map[string]*model.Module{"G45": g45, "M": m, "System": sys}
	dms := map[model.ID]*domainmodel.DomainModel{g45.ID: g45DM, m.ID: mDM, sys.ID: sysDM}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{g45, m, sys}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if mod, ok := modules[name]; ok {
				return mod, nil
			}
			return nil, fmt.Errorf("module not found: %s", name)
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if dm, ok := dms[id]; ok {
				return dm, nil
			}
			return nil, fmt.Errorf("domain model not found: %s", id)
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{g45DM, mDM, sysDM}, nil
		},
	}

	return &flowBuilder{
		backend: mb,
		varTypes: map[string]string{
			"Input":   "G45.Input",
			"Person":  "G45.Person",
			"Child":   "M.Child",
			"Account": "M.Account",
		},
	}
}

// feedbackTargetFor runs the builder and returns the stored Attribute and
// Association references.
func feedbackTargetFor(t *testing.T, fb *flowBuilder, variable, member string) (attr, assoc string) {
	t.Helper()
	fb.addValidationFeedbackAction(&ast.ValidationFeedbackStmt{
		AttributePath: &ast.AttributePathExpr{
			Variable: variable,
			Segments: []ast.PathSegment{{Name: member}},
		},
		Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "Pick a person."},
	})
	activity, ok := fb.objects[len(fb.objects)-1].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("got %T, want *microflows.ActionActivity", fb.objects[len(fb.objects)-1])
	}
	action, ok := activity.Action.(*microflows.ValidationFeedbackAction)
	if !ok {
		t.Fatalf("got %T, want *microflows.ValidationFeedbackAction", activity.Action)
	}
	return action.AttributeName, action.AssociationName
}

func TestValidationFeedback_BareAssociationNameIsStoredAsAnAssociation(t *testing.T) {
	fb := feedbackAssocFixture(t)

	attr, assoc := feedbackTargetFor(t, fb, "Input", "Input_Person")
	if attr != "" || assoc != "G45.Input_Person" {
		t.Errorf("bare association stored as Attribute=%q Association=%q, want Association \"G45.Input_Person\" — "+
			"the attribute spelling is CE1613 \"The selected attribute 'G45.Input.Input_Person' no longer exists.\"", attr, assoc)
	}
}

// An association declared on an ancestor is a member of the specialization, and
// it is qualified by the module that holds it — including System.
func TestValidationFeedback_BareInheritedAssociationNameIsStoredAsAnAssociation(t *testing.T) {
	fb := feedbackAssocFixture(t)

	if attr, assoc := feedbackTargetFor(t, fb, "Child", "Base_Target"); attr != "" || assoc != "M.Base_Target" {
		t.Errorf("inherited association stored as Attribute=%q Association=%q, want Association \"M.Base_Target\"", attr, assoc)
	}
	if attr, assoc := feedbackTargetFor(t, fb, "Account", "UserRoles"); attr != "" || assoc != "System.UserRoles" {
		t.Errorf("association inherited from System.User stored as Attribute=%q Association=%q, want Association \"System.UserRoles\"", attr, assoc)
	}
}

// The controls. An attribute stays an attribute; the qualified association
// keeps working; a name that is neither keeps the old attribute spelling, which
// mxbuild names; and an association the entity is NOT an end of is not picked
// up just because it shares the module.
func TestValidationFeedback_BareNameControls(t *testing.T) {
	fb := feedbackAssocFixture(t)

	if attr, assoc := feedbackTargetFor(t, fb, "Input", "Note"); attr != "G45.Input.Note" || assoc != "" {
		t.Errorf("attribute stored as Attribute=%q Association=%q, want Attribute \"G45.Input.Note\"", attr, assoc)
	}
	if attr, assoc := feedbackTargetFor(t, fb, "Input", "G45.Input_Person"); attr != "" || assoc != "G45.Input_Person" {
		t.Errorf("qualified association stored as Attribute=%q Association=%q", attr, assoc)
	}
	if attr, assoc := feedbackTargetFor(t, fb, "Input", "NoSuchMember"); attr != "G45.Input.NoSuchMember" || assoc != "" {
		t.Errorf("unresolvable member stored as Attribute=%q Association=%q, want the old attribute spelling", attr, assoc)
	}
	if attr, assoc := feedbackTargetFor(t, fb, "Account", "Base_Target"); attr != "M.Account.Base_Target" || assoc != "" {
		t.Errorf("unrelated association stored as Attribute=%q Association=%q, want the old attribute spelling", attr, assoc)
	}
}
