// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// tripPinMetadata is a minimal OData 4 $metadata with the two shapes the repo
// had no fixture for and that mendixlabs/mxcli#1020 is about: an action
// returning an ENTITY and one returning a COLLECTION of entities. GetNearestAirport
// and its parameters are modelled on the public TripPin service.
const tripPinMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="Trippin" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Airport">
        <Key><PropertyRef Name="IcaoCode"/></Key>
        <Property Name="IcaoCode" Type="Edm.String" Nullable="false"/>
        <Property Name="Name" Type="Edm.String"/>
      </EntityType>
      <EntityType Name="Person">
        <Key><PropertyRef Name="UserName"/></Key>
        <Property Name="UserName" Type="Edm.String" Nullable="false"/>
      </EntityType>
      <Action Name="GetNearestAirport">
        <Parameter Name="lat" Type="Edm.Double" Nullable="false"/>
        <Parameter Name="lon" Type="Edm.Double" Nullable="false"/>
        <ReturnType Type="Trippin.Airport"/>
      </Action>
      <Action Name="GetFriends">
        <Parameter Name="userName" Type="Edm.String" Nullable="false"/>
        <ReturnType Type="Collection(Trippin.Person)"/>
      </Action>
      <Action Name="ResetDataSource"/>
      <Action Name="GetCount">
        <ReturnType Type="Edm.Int32"/>
      </Action>
      <EntityContainer Name="Container">
        <EntitySet Name="People" EntityType="Trippin.Person"/>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

func parseTripPin(t *testing.T) *types.EdmxDocument {
	t.Helper()
	doc, err := types.ParseEdmx(tripPinMetadata)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return doc
}

func actionNamed(t *testing.T, doc *types.EdmxDocument, name string) *types.EdmAction {
	t.Helper()
	for _, a := range doc.Actions {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("fixture has no action %q", name)
	return nil
}

// TestEdmBareTypeName pins the type-name normalisation the entity lookup rests
// on: the contract says `Trippin.Airport`, the imported entity records `Airport`.
func TestEdmBareTypeName(t *testing.T) {
	tests := []struct {
		in       string
		wantName string
		wantList bool
	}{
		{"Trippin.Airport", "Airport", false},
		{"Collection(Trippin.Person)", "Person", true},
		// A namespace with dots in it still yields the last segment.
		{"Some.Deep.Namespace.Thing", "Thing", false},
		{"Collection(Some.Deep.Ns.Thing)", "Thing", true},
		// Primitives never reach the entity lookup.
		{"Edm.String", "", false},
		{"Collection(Edm.String)", "", true},
		{"", "", false},
		// Unqualified type name (no namespace) is still a name.
		{"Airport", "Airport", false},
	}
	for _, tt := range tests {
		gotName, gotList := edmBareTypeName(tt.in)
		if gotName != tt.wantName || gotList != tt.wantList {
			t.Errorf("edmBareTypeName(%q) = (%q, %v), want (%q, %v)",
				tt.in, gotName, gotList, tt.wantName, tt.wantList)
		}
	}
}

// TestEdmReturnTypeToKindLeavesEntitiesToTheEntityLookup documents the split:
// primitives resolve here, entity-typed returns deliberately do not, because
// they need the project to find the imported entity.
func TestEdmReturnTypeToKindLeavesEntitiesToTheEntityLookup(t *testing.T) {
	if got := edmReturnTypeToKind("Edm.Int32"); got != "Integer" {
		t.Errorf("Edm.Int32 = %q, want Integer", got)
	}
	if got := edmReturnTypeToKind(""); got != "Void" {
		t.Errorf("empty = %q, want Void", got)
	}
	for _, entityReturn := range []string{"Trippin.Airport", "Collection(Trippin.Person)"} {
		if got := edmReturnTypeToKind(entityReturn); got != "" {
			t.Errorf("edmReturnTypeToKind(%q) = %q, want \"\" — entity returns are resolved against the project",
				entityReturn, got)
		}
	}
}

// TestCheckExternalActionParameters covers CE7252. The arguments a statement
// writes must match the action's declared parameter list.
func TestCheckExternalActionParameters(t *testing.T) {
	doc := parseTripPin(t)
	action := actionNamed(t, doc, "GetNearestAirport")

	call := func(args ...string) externalCall {
		var as []ast.CallArgument
		for _, a := range args {
			as = append(as, ast.CallArgument{Name: a})
		}
		return externalCall{flow: "M.Flow", stmt: &ast.CallExternalActionStmt{
			ActionName: "GetNearestAirport",
			Arguments:  as,
		}}
	}

	if err := checkExternalActionParameters(call("lat", "lon"), action); err != nil {
		t.Errorf("both parameters supplied should pass: %v", err)
	}
	// Order is not part of the contract — these are named mappings.
	if err := checkExternalActionParameters(call("lon", "lat"), action); err != nil {
		t.Errorf("order must not matter: %v", err)
	}

	err := checkExternalActionParameters(call("lat"), action)
	if err == nil {
		t.Fatal("a missing parameter must be reported")
	}
	for _, want := range []string{"declared but not supplied", "lon", "CE7252"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}

	err = checkExternalActionParameters(call("lat", "lon", "altitude"), action)
	if err == nil {
		t.Fatal("an undeclared argument must be reported")
	}
	if !strings.Contains(err.Error(), "not declared by the action") || !strings.Contains(err.Error(), "altitude") {
		t.Errorf("error %q should name the undeclared argument", err.Error())
	}

	// An action with no parameters and no arguments is fine.
	if err := checkExternalActionParameters(
		externalCall{flow: "M.Flow", stmt: &ast.CallExternalActionStmt{ActionName: "ResetDataSource"}},
		actionNamed(t, doc, "ResetDataSource"),
	); err != nil {
		t.Errorf("a parameterless action should pass: %v", err)
	}
}

// TestCheckExternalActionParametersBoundAction covers the binding parameter: a
// bound action's first parameter is supplied by Mendix from the object the
// action is called on, so a statement that does not name it is correct.
func TestCheckExternalActionParametersBoundAction(t *testing.T) {
	bound := &types.EdmAction{
		Name:    "Rate",
		IsBound: true,
		Parameters: []*types.EdmActionParameter{
			{Name: "bindingParameter", Type: "Trippin.Person"},
			{Name: "rating", Type: "Edm.Int32"},
		},
	}
	c := externalCall{flow: "M.Flow", stmt: &ast.CallExternalActionStmt{
		ActionName: "Rate",
		Arguments:  []ast.CallArgument{{Name: "rating"}},
	}}
	if err := checkExternalActionParameters(c, bound); err != nil {
		t.Errorf("the binding parameter must not be demanded from the statement: %v", err)
	}
}

// TestExternalActionCallsIn pins collection, including from nested bodies — a
// call the walker misses is a call nothing checks.
func TestExternalActionCallsIn(t *testing.T) {
	mk := func(action string) *ast.CallExternalActionStmt {
		return &ast.CallExternalActionStmt{ActionName: action}
	}

	tests := []struct {
		name string
		stmt ast.Statement
		want []string
	}{
		{
			name: "top-level call",
			stmt: &ast.CreateMicroflowStmt{
				Name: ast.QualifiedName{Module: "M", Name: "F"},
				Body: []ast.MicroflowStatement{mk("A")},
			},
			want: []string{"A"},
		},
		{
			name: "inside a loop and an if",
			stmt: &ast.CreateMicroflowStmt{
				Name: ast.QualifiedName{Module: "M", Name: "F"},
				Body: []ast.MicroflowStatement{
					&ast.LoopStmt{Body: []ast.MicroflowStatement{mk("A")}},
					&ast.IfStmt{
						ThenBody: []ast.MicroflowStatement{mk("B")},
						ElseBody: []ast.MicroflowStatement{mk("C")},
					},
				},
			},
			want: []string{"A", "B", "C"},
		},
		{
			name: "nanoflows carry them too",
			stmt: &ast.CreateNanoflowStmt{
				Name: ast.QualifiedName{Module: "M", Name: "N"},
				Body: []ast.MicroflowStatement{mk("A")},
			},
			want: []string{"A"},
		},
		{
			name: "an unrelated statement contributes nothing",
			stmt: &ast.CreateUserRoleStmt{Name: "Administrator"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := externalActionCallsIn(tt.stmt)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d calls, want %d", len(got), len(tt.want))
			}
			for i, w := range tt.want {
				if got[i].stmt.ActionName != w {
					t.Errorf("call[%d] = %q, want %q", i, got[i].stmt.ActionName, w)
				}
				if got[i].flow == "" {
					t.Errorf("call[%d] has no flow name for the message", i)
				}
			}
		})
	}
}
