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

// upstream #974: a validation feedback activity aimed at an INHERITED attribute
// wrote a reference Mendix cannot resolve.
//
//	create persistent entity M.Base ("Title": String(200));
//	create persistent entity M.Child extends M.Base ("Own": String(20));
//	…
//	validation feedback $Child/Title message 'Title is required.';
//
//	[CE1613] "The selected attribute 'M.Child.Title' no longer exists."
//	         at Validation feedback activity
//
// Measured on mxbuild 11.13.0, on BOTH engines. `mxcli check --references`
// passes and describe renders the statement back unchanged, so the damage is
// visible only to Mendix.
//
// Mendix qualifies a member reference against the entity that DECLARES it, so an
// inherited attribute carries the ancestor's name. The builder used the
// VARIABLE's declared type instead, with no generalization walk — the same rule
// the access-rule writer learned in #758 and the mapping writer in #703, both of
// which already go through the walk in entity_hierarchy.go.

// feedbackFixture is Base ← Child, plus a System.User specialization, and
// captures the member reference the builder produced.
func feedbackFixture(t *testing.T) *flowBuilder {
	t.Helper()
	mod := mkModule("M")
	sys := &model.Module{Name: "System"}
	sys.ID = model.ID("m-system-974")

	base := &domainmodel.Entity{
		Name:       "Base",
		Attributes: []*domainmodel.Attribute{{Name: "Title"}},
	}
	child := &domainmodel.Entity{
		Name:              "Child",
		Attributes:        []*domainmodel.Attribute{{Name: "Own"}},
		GeneralizationRef: "M.Base",
	}
	account := &domainmodel.Entity{
		Name:              "Account",
		Attributes:        []*domainmodel.Attribute{{Name: "FullName"}},
		GeneralizationRef: "System.User",
	}
	user := &domainmodel.Entity{
		Name:       "User",
		Attributes: []*domainmodel.Attribute{{Name: "Name"}},
	}

	modDM := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-m"},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{base, child, account},
	}
	sysDM := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-sys"},
		ContainerID: sys.ID,
		Entities:    []*domainmodel.Entity{user},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod, sys}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			switch name {
			case "M":
				return mod, nil
			case "System":
				return sys, nil
			}
			return nil, fmt.Errorf("module not found: %s", name)
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if id == sys.ID {
				return sysDM, nil
			}
			return modDM, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{modDM, sysDM}, nil
		},
	}

	return &flowBuilder{
		backend: mb,
		varTypes: map[string]string{
			"Child":   "M.Child",
			"Base":    "M.Base",
			"Account": "M.Account",
		},
	}
}

// feedbackRefFor runs the builder and returns the stored Attribute reference.
func feedbackRefFor(t *testing.T, fb *flowBuilder, variable, member string) string {
	t.Helper()
	before := len(fb.objects)
	fb.addValidationFeedbackAction(&ast.ValidationFeedbackStmt{
		AttributePath: &ast.AttributePathExpr{
			Variable: variable,
			Segments: []ast.PathSegment{{Name: member}},
		},
		Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "required"},
	})
	if len(fb.objects) != before+1 {
		t.Fatalf("expected one new activity, got %d", len(fb.objects)-before)
	}
	activity, ok := fb.objects[len(fb.objects)-1].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("got %T, want *microflows.ActionActivity", fb.objects[len(fb.objects)-1])
	}
	action, ok := activity.Action.(*microflows.ValidationFeedbackAction)
	if !ok {
		t.Fatalf("got %T, want *microflows.ValidationFeedbackAction", activity.Action)
	}
	return action.AttributeName
}

func TestValidationFeedback_QualifiesAnInheritedAttributeAgainstItsDeclaringEntity(t *testing.T) {
	fb := feedbackFixture(t)

	if got, want := feedbackRefFor(t, fb, "Child", "Title"), "M.Base.Title"; got != want {
		t.Errorf("inherited attribute stored as %q, want %q — mxbuild rejects the child's name with CE1613", got, want)
	}
}

// A member inherited from System.User resolves the same way. The access-rule
// walk deliberately STOPS at System.User (its members are Mendix's, and listing
// them in a rule is CE0066), so reusing that walk here would silently fall back
// to the child's name — the very thing being fixed.
func TestValidationFeedback_QualifiesAcrossASystemAncestor(t *testing.T) {
	fb := feedbackFixture(t)

	if got, want := feedbackRefFor(t, fb, "Account", "Name"), "System.User.Name"; got != want {
		t.Errorf("attribute inherited from System.User stored as %q, want %q", got, want)
	}
}

// The controls. An attribute the entity declares itself must keep its own name,
// or the walk is returning an ancestor for everything.
func TestValidationFeedback_KeepsAnOwnAttributeOnTheEntityItself(t *testing.T) {
	fb := feedbackFixture(t)

	if got, want := feedbackRefFor(t, fb, "Child", "Own"), "M.Child.Own"; got != want {
		t.Errorf("own attribute stored as %q, want %q", got, want)
	}
	if got, want := feedbackRefFor(t, fb, "Base", "Title"), "M.Base.Title"; got != want {
		t.Errorf("attribute on the declaring entity itself stored as %q, want %q", got, want)
	}
	if got, want := feedbackRefFor(t, fb, "Account", "FullName"), "M.Account.FullName"; got != want {
		t.Errorf("own attribute of a user entity stored as %q, want %q", got, want)
	}
}

// A member that resolves nowhere keeps the old spelling rather than becoming
// empty or bare: the reference is wrong either way, and mxbuild names it. This
// is also the path taken when there is no backend at all.
func TestValidationFeedback_FallsBackWhenNothingResolves(t *testing.T) {
	fb := feedbackFixture(t)
	if got, want := feedbackRefFor(t, fb, "Child", "NoSuchAttribute"), "M.Child.NoSuchAttribute"; got != want {
		t.Errorf("unresolvable member stored as %q, want %q", got, want)
	}

	bare := &flowBuilder{varTypes: map[string]string{"Child": "M.Child"}}
	if got, want := feedbackRefFor(t, bare, "Child", "Title"), "M.Child.Title"; got != want {
		t.Errorf("with no backend, stored as %q, want %q", got, want)
	}
}
