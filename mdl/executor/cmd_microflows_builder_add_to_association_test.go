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
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1288: `add $X to $Obj/Assoc` is a Change object activity
// with one Add member — what Studio Pro writes for the Add button — not a
// Change list activity on a variable named "Obj".
func TestAddToAssociationBuildsChangeObjectAdd(t *testing.T) {
	fb := &flowBuilder{varTypes: map[string]string{"Order": "Sales.Order"}}

	fb.addAddToListAction(&ast.AddToListStmt{
		Item:        "Line",
		Value:       &ast.VariableExpr{Name: "Line"},
		List:        "Order",
		Association: "Sales.Order_Line",
		Commit:      ast.CommitYes,
	})

	action := lastChangeObjectAction(t, fb)
	if action.ChangeVariable != "Order" || action.Commit != microflows.CommitTypeYes || action.RefreshInClient {
		t.Fatalf("action = var %q commit %q refresh %v", action.ChangeVariable, action.Commit, action.RefreshInClient)
	}
	if len(action.Changes) != 1 {
		t.Fatalf("Changes = %d, want 1", len(action.Changes))
	}
	mc := action.Changes[0]
	if mc.Type != microflows.MemberChangeTypeAdd {
		t.Errorf("Type = %q, want Add", mc.Type)
	}
	if mc.AssociationQualifiedName != "Sales.Order_Line" || mc.AttributeQualifiedName != "" {
		t.Errorf("member = assoc %q attr %q, want association Sales.Order_Line", mc.AssociationQualifiedName, mc.AttributeQualifiedName)
	}
	if mc.Value != "$Line" {
		t.Errorf("Value = %q, want $Line", mc.Value)
	}
}

func TestRemoveFromAssociationBuildsChangeObjectRemove(t *testing.T) {
	fb := &flowBuilder{varTypes: map[string]string{"Order": "Sales.Order"}}

	fb.addRemoveFromListAction(&ast.RemoveFromListStmt{
		Item:            "Line",
		List:            "Order",
		Association:     "Sales.Order_Line",
		RefreshInClient: true,
	})

	action := lastChangeObjectAction(t, fb)
	if !action.RefreshInClient {
		t.Error("refresh was dropped")
	}
	mc := action.Changes[0]
	if mc.Type != microflows.MemberChangeTypeRemove || mc.AssociationQualifiedName != "Sales.Order_Line" || mc.Value != "$Line" {
		t.Fatalf("member = %+v, want Remove Sales.Order_Line $Line", mc)
	}
}

// Add/Remove exist for association members only; an attribute target would be
// written as an Add on an attribute, which no Mendix dialog can produce.
func TestAddToAttributePathIsRefused(t *testing.T) {
	fb := &flowBuilder{varTypes: map[string]string{"Order": "Sales.Order"}}

	fb.addAddToListAction(&ast.AddToListStmt{
		Item:        "Line",
		Value:       &ast.VariableExpr{Name: "Line"},
		List:        "Order",
		Association: "Sales.Order.Number",
	})

	if len(fb.errors) == 0 {
		t.Fatal("add to an attribute path built without an error")
	}
	if !strings.Contains(strings.Join(fb.errors, "\n"), "association") {
		t.Errorf("error does not say an association is required: %v", fb.errors)
	}
}

// The association form takes the object variable, not a list: the retrieve
// that bound $Order must not be typed as a list because of it, and the value
// may be a list ($Lines) as well as an object, so it says nothing either way.
func TestAssociationTargetIsAnObjectInput(t *testing.T) {
	stmts := []ast.MicroflowStatement{
		&ast.AddToListStmt{Value: &ast.VariableExpr{Name: "Lines"}, Item: "Lines", List: "Order", Association: "Sales.Order_Line"},
		&ast.RemoveFromListStmt{Item: "Line", List: "Invoice", Association: "Sales.Invoice_Line"},
	}
	lists := collectListInputVariables(stmts)
	if lists["Order"] || lists["Invoice"] {
		t.Errorf("association target collected as a list input: %v", lists)
	}
	objects := collectObjectInputVariables(stmts)
	if !objects["Order"] || !objects["Invoice"] {
		t.Errorf("association target not collected as an object input: %v", objects)
	}
	if objects["Lines"] {
		t.Errorf("value of an association add collected as an object input: %v", objects)
	}
}

// Add/Remove is a reference-SET operation. Measured on Mendix 11.12.2: an Add
// member on a plain Reference fails `mx check` with CE0033 "The 'Type' property
// of this member cannot be 'Add'." — as it does on an attribute — so the
// builder refuses it instead of writing a model mxbuild will reject. The
// ReferenceSet case is the control: the same statement must build cleanly.
func TestAddRemoveOnReferenceIsRefused(t *testing.T) {
	const (
		modID   = model.ID("mod-sales")
		orderID = model.ID("e-order")
		lineID  = model.ID("e-line")
	)
	newBuilder := func() *flowBuilder {
		mb := &mock.MockBackend{
			GetModuleByNameFunc: func(name string) (*model.Module, error) {
				if name == "Sales" {
					return &model.Module{BaseElement: model.BaseElement{ID: modID}, Name: "Sales"}, nil
				}
				return nil, fmt.Errorf("no module %q", name)
			},
			GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
				return &domainmodel.DomainModel{
					ContainerID: modID,
					Entities: []*domainmodel.Entity{
						{BaseElement: model.BaseElement{ID: orderID}, Name: "Order", Persistable: true},
						{BaseElement: model.BaseElement{ID: lineID}, Name: "Line", Persistable: true},
					},
					Associations: []*domainmodel.Association{
						{Name: "Order_Lines", ParentID: orderID, ChildID: lineID, Type: domainmodel.AssociationTypeReferenceSet},
						{Name: "Order_MainLine", ParentID: orderID, ChildID: lineID, Type: domainmodel.AssociationTypeReference},
					},
				}, nil
			},
		}
		return &flowBuilder{backend: mb, varTypes: map[string]string{"Order": "Sales.Order"}}
	}

	set := newBuilder()
	set.addAddToListAction(&ast.AddToListStmt{Item: "Line", List: "Order", Association: "Sales.Order_Lines"})
	set.addRemoveFromListAction(&ast.RemoveFromListStmt{Item: "Line", List: "Order", Association: "Sales.Order_Lines"})
	if len(set.errors) != 0 {
		t.Fatalf("control: add/remove on a ReferenceSet refused: %v", set.errors)
	}

	for name, build := range map[string]func(fb *flowBuilder){
		"add": func(fb *flowBuilder) {
			fb.addAddToListAction(&ast.AddToListStmt{Item: "Line", List: "Order", Association: "Sales.Order_MainLine"})
		},
		"remove": func(fb *flowBuilder) {
			fb.addRemoveFromListAction(&ast.RemoveFromListStmt{Item: "Line", List: "Order", Association: "Sales.Order_MainLine"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			fb := newBuilder()
			build(fb)
			if len(fb.errors) == 0 {
				t.Fatal("add/remove on a Reference built without an error; mxbuild answers CE0033")
			}
			msg := strings.Join(fb.errors, "\n")
			for _, want := range []string{"Sales.Order_MainLine", "Reference", "CE0033", "change $Order"} {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q does not mention %q", msg, want)
				}
			}
		})
	}
}

// mendixlabs/mxcli#1473: `change $Obj (Attr = …, Assoc += $X, Assoc -= $Y)`
// builds one Change object activity whose members carry Set, Add and Remove —
// the activity Studio Pro writes when one dialog mixes the three. The `+=` on
// a Reference is refused for the same CE0033 the `add … to` form is.
func TestChangeObjectMemberOperators(t *testing.T) {
	newBuilder := func() *flowBuilder {
		const (
			modID   = model.ID("mod-sales")
			orderID = model.ID("e-order")
			lineID  = model.ID("e-line")
		)
		mb := &mock.MockBackend{
			GetModuleByNameFunc: func(name string) (*model.Module, error) {
				if name == "Sales" {
					return &model.Module{BaseElement: model.BaseElement{ID: modID}, Name: "Sales"}, nil
				}
				return nil, fmt.Errorf("no module %q", name)
			},
			GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
				return &domainmodel.DomainModel{
					ContainerID: modID,
					Entities: []*domainmodel.Entity{
						{BaseElement: model.BaseElement{ID: orderID}, Name: "Order", Persistable: true},
						{BaseElement: model.BaseElement{ID: lineID}, Name: "Line", Persistable: true},
					},
					Associations: []*domainmodel.Association{
						{Name: "Order_Lines", ParentID: orderID, ChildID: lineID, Type: domainmodel.AssociationTypeReferenceSet},
						{Name: "Order_MainLine", ParentID: orderID, ChildID: lineID, Type: domainmodel.AssociationTypeReference},
					},
				}, nil
			},
		}
		return &flowBuilder{backend: mb, varTypes: map[string]string{"Order": "Sales.Order"}}
	}

	fb := newBuilder()
	fb.addChangeObjectAction(&ast.ChangeObjectStmt{
		Variable: "Order",
		Changes: []ast.ChangeItem{
			{Attribute: "Sales.Order_MainLine", Value: &ast.VariableExpr{Name: "Line"}},
			{Attribute: "Sales.Order_Lines", Op: ast.MemberAdd, Value: &ast.VariableExpr{Name: "Line"}},
			{Attribute: "Sales.Order_Lines", Op: ast.MemberRemove, Value: &ast.VariableExpr{Name: "Old"}},
		},
	})
	if len(fb.errors) != 0 {
		t.Fatalf("mixed change refused: %v", fb.errors)
	}
	action := lastChangeObjectAction(t, fb)
	wantTypes := []microflows.MemberChangeType{microflows.MemberChangeTypeSet, microflows.MemberChangeTypeAdd, microflows.MemberChangeTypeRemove}
	if len(action.Changes) != len(wantTypes) {
		t.Fatalf("Changes = %d, want %d", len(action.Changes), len(wantTypes))
	}
	for i, want := range wantTypes {
		if got := action.Changes[i].Type; got != want {
			t.Errorf("Changes[%d].Type = %q, want %q", i, got, want)
		}
	}

	for name, item := range map[string]ast.ChangeItem{
		"reference": {Attribute: "Sales.Order_MainLine", Op: ast.MemberAdd, Value: &ast.VariableExpr{Name: "Line"}},
		"attribute": {Attribute: "Number", Op: ast.MemberRemove, Value: &ast.LiteralExpr{Kind: ast.LiteralInteger, Value: int64(1)}},
	} {
		t.Run(name, func(t *testing.T) {
			fb := newBuilder()
			fb.addChangeObjectAction(&ast.ChangeObjectStmt{Variable: "Order", Changes: []ast.ChangeItem{item}})
			if len(fb.errors) == 0 {
				t.Fatal("+=/-= on a non-reference-set member built without an error; mxbuild answers CE0033")
			}
			if msg := strings.Join(fb.errors, "\n"); !strings.Contains(msg, "CE0033") {
				t.Errorf("error does not name CE0033: %q", msg)
			}
		})
	}
}
