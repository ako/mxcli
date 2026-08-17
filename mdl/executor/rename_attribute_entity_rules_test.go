// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestRenameAttributeInEntityRules pins that an entity's own by-name references
// to one of its attributes move with it.
//
// This is not tidiness. UpdateEntity re-derives an access rule's members from the
// attributes it can match, so a member left pointing at the old name is not
// merely stale — a READ * rule comes back carrying a member for the new name AND
// the orphan, and mxbuild reports CE0066 "Entity access is out of date". Measured
// on Mendix 11.13.0 with mdl-examples/bug-tests/910-rename-attribute-xpath-constraints.mdl:
// without this pass the rename leaves 1 error and three members for a two-attribute
// entity; with it, 0 errors and two members.
func TestRenameAttributeInEntityRules(t *testing.T) {
	minAttr := "Sales.Person.FirstName"
	entity := &domainmodel.Entity{
		Name: "Person",
		AccessRules: []*domainmodel.AccessRule{{
			MemberAccesses: []*domainmodel.MemberAccess{
				{AttributeName: "Sales.Person.FirstName"},
				{AttributeName: "Sales.Person.LastName"},
				{AssociationName: "Sales.Order_Person"},
				nil,
			},
		}, nil},
		ValidationRules: []*domainmodel.ValidationRule{
			{AttributeID: model.ID("Sales.Person.FirstName"), Type: "Required"},
			{AttributeID: model.ID("Sales.Person.LastName"), Type: "Required"},
			{
				AttributeID: model.ID("Sales.Person.LastName"),
				Type:        "Range",
				Rule:        &domainmodel.RangeValidationRuleInfo{MinAttributeQualifiedName: minAttr},
			},
			nil,
		},
	}

	renameAttributeInEntityRules(entity, "Sales.Person", "FirstName", "GivenName")

	ma := entity.AccessRules[0].MemberAccesses
	if ma[0].AttributeName != "Sales.Person.GivenName" {
		t.Errorf("the access rule member still points at %q", ma[0].AttributeName)
	}
	if ma[1].AttributeName != "Sales.Person.LastName" {
		t.Errorf("another attribute's member moved: %q", ma[1].AttributeName)
	}
	if ma[2].AssociationName != "Sales.Order_Person" {
		t.Errorf("an association member was touched: %q", ma[2].AssociationName)
	}

	vr := entity.ValidationRules
	if string(vr[0].AttributeID) != "Sales.Person.GivenName" {
		t.Errorf("the validation rule still points at %q", vr[0].AttributeID)
	}
	if string(vr[1].AttributeID) != "Sales.Person.LastName" {
		t.Errorf("another attribute's validation rule moved: %q", vr[1].AttributeID)
	}
	rng, ok := vr[2].Rule.(*domainmodel.RangeValidationRuleInfo)
	if !ok {
		t.Fatalf("the range rule lost its payload: %T", vr[2].Rule)
	}
	if rng.MinAttributeQualifiedName != "Sales.Person.GivenName" {
		t.Errorf("an attribute-bounded range still points at %q", rng.MinAttributeQualifiedName)
	}
}

// TestRenameAttributeInEntityRulesLeavesUUIDsAlone pins that a rule whose
// AttributeID is a UUID — an entity built in this run, where the serializer looks
// the name up — is not mangled into a qualified name.
func TestRenameAttributeInEntityRulesLeavesUUIDsAlone(t *testing.T) {
	const id = "0f6a2c14-8f6f-4a1e-8a1b-2f4d9b0c1e33"
	entity := &domainmodel.Entity{
		Name:            "Person",
		ValidationRules: []*domainmodel.ValidationRule{{AttributeID: model.ID(id)}},
	}

	renameAttributeInEntityRules(entity, "Sales.Person", "FirstName", "GivenName")

	if string(entity.ValidationRules[0].AttributeID) != id {
		t.Errorf("a UUID attribute reference was rewritten to %q", entity.ValidationRules[0].AttributeID)
	}
}
