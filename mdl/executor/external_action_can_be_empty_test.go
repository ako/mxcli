// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Mendix stores each external-action parameter's nullability on the mapping as
// CanBeEmpty, and compares it against the contract on every build. mxcli left it
// at Go's zero value, so a call on a NULLABLE parameter was CE7252 "the
// parameters for remote action '<x>' have changed" — with no way to clear it
// from MDL, because nothing in the language reaches the field.
//
// The three parameter shapes below are the ones that behave differently.
// `note` is the subtle one: CSDL makes Nullable optional on <Parameter> and
// defaults it to TRUE, so an absent attribute is the opposite of Go's zero
// value. A contract that spells every Nullable out cannot tell the two apart.
const canBeEmptyMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="Probe" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <Action Name="RunCommand" IsBound="false">
        <Parameter Name="command"    Type="Edm.String" Nullable="false"/>
        <Parameter Name="additional" Type="Edm.String" Nullable="true"/>
        <Parameter Name="note"       Type="Edm.String"/>
      </Action>
      <EntityContainer Name="Container">
        <ActionImport Name="RunCommand" Action="Probe.RunCommand"/>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

// paramCanBeEmpty is where the default lives, so it is pinned on its own: the
// end-to-end test below would still pass if `nil` and `false` were conflated,
// as long as no contract in the suite omitted the attribute.
func TestParamCanBeEmpty_AbsentNullableMeansNullable(t *testing.T) {
	f, tr := false, true
	for _, tc := range []struct {
		name     string
		nullable *bool
		want     bool
	}{
		{"explicit false", &f, false},
		{"explicit true", &tr, true},
		{"absent", nil, true}, // CSDL default, confirmed against mxbuild 11.14
	} {
		if got := paramCanBeEmpty(&types.EdmActionParameter{Nullable: tc.nullable}); got != tc.want {
			t.Errorf("%s: CanBeEmpty = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// buildCanBeEmptyCall runs the real builder against the contract above and
// returns the mappings it wrote.
func buildCanBeEmptyCall(t *testing.T) []*microflows.ExternalActionParameterMapping {
	t.Helper()

	svcID := model.ID("svc-1")
	modID := model.ID("mod-1")
	mb := &mock.MockBackend{
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{{
				BaseElement: model.BaseElement{ID: svcID},
				ContainerID: modID,
				Name:        "Bug1073",
				Metadata:    canBeEmptyMetadata,
			}}, nil
		},
	}
	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing, backend: mb,
		hierarchy:    &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "Odata"}},
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
	}

	// Parsed rather than hand-built: the argument expressions have to be the
	// ones the real front end produces, or the Argument assertion below is
	// testing the fixture instead of the builder.
	prog, errs := visitor.Build(
		"create microflow Odata.ACT_Probe()\nbegin\n" +
			"  call external action Odata.Bug1073.RunCommand(" +
			"command = empty, additional = empty, note = empty);\nend;")
	if len(errs) > 0 {
		t.Fatalf("parsing the probe script: %v", errs)
	}
	call := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.CallExternalActionStmt)
	fb.addCallExternalActionAction(call)

	for _, obj := range fb.objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		if call, ok := act.Action.(*microflows.CallExternalAction); ok {
			return call.ParameterMappings
		}
	}
	t.Fatal("the builder produced no CallExternalAction")
	return nil
}

// The measured shape of a Studio Pro document: CanBeEmpty tracks the contract's
// Nullable per parameter, and Argument is the expression `empty` — NOT an empty
// string. Pinned against Odata.UnboundedActionMF in ako/TestApp, whose two
// mappings are {command, "empty", false} and {additional, "empty", true}.
func TestCallExternalAction_CanBeEmptyFollowsTheContract(t *testing.T) {
	want := map[string]bool{
		"command":    false, // Nullable="false"
		"additional": true,  // Nullable="true"
		"note":       true,  // no Nullable attribute at all
	}

	mappings := buildCanBeEmptyCall(t)
	if len(mappings) != len(want) {
		t.Fatalf("got %d mappings, want %d", len(mappings), len(want))
	}
	for _, pm := range mappings {
		w, ok := want[pm.ParameterName]
		if !ok {
			t.Errorf("unexpected mapping for %q", pm.ParameterName)
			continue
		}
		if pm.CanBeEmpty != w {
			t.Errorf("%s: CanBeEmpty = %v, want %v — Mendix compares this against "+
				"the contract and reports CE7252 when they disagree",
				pm.ParameterName, pm.CanBeEmpty, w)
		}
		if pm.Argument != "empty" {
			t.Errorf("%s: Argument = %q, want %q", pm.ParameterName, pm.Argument, "empty")
		}
	}

	// Control: the values are not uniformly true. A fix that set CanBeEmpty on
	// every mapping would satisfy the nullable cases and silently break the
	// required one, which is the same CE7252 in the other direction.
	var sawFalse bool
	for _, pm := range mappings {
		if !pm.CanBeEmpty {
			sawFalse = true
		}
	}
	if !sawFalse {
		t.Error("every mapping came back CanBeEmpty=true; the value is not being " +
			"read from the contract")
	}
}

// ParameterType is resolved in the same loop, so a regression there would show
// up as a wrong CanBeEmpty and vice versa. #1020 is what put it in.
func TestCallExternalAction_ParameterTypeStillResolved(t *testing.T) {
	for _, pm := range buildCanBeEmptyCall(t) {
		if pm.ParameterDataType != "String" {
			t.Errorf("%s: ParameterDataType = %q, want String",
				pm.ParameterName, pm.ParameterDataType)
		}
	}
}
