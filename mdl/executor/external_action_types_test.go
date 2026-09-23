// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// probeMetadata declares one action per PARAMETER shape and one per RETURN
// shape, so each can be measured on its own. The verdicts asserted below were
// measured on mxbuild 11.12.0 against exactly this contract: every action was
// written by mxcli and `mx check` run on the result (mendixlabs/mxcli#1089).
const probeMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="Probe" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Person">
        <Key><PropertyRef Name="UserName"/></Key>
        <Property Name="UserName" Type="Edm.String" Nullable="false"/>
      </EntityType>
      <EntityType Name="Ghost">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.String" Nullable="false"/>
      </EntityType>
      <ComplexType Name="Location">
        <Property Name="City" Type="Edm.String"/>
      </ComplexType>
      <EnumType Name="Colour">
        <Member Name="Red" Value="0"/>
      </EnumType>
      <TypeDefinition Name="Alias" UnderlyingType="Edm.String"/>

      <Action Name="PTimeOfDay"><Parameter Name="v" Type="Edm.TimeOfDay" Nullable="true"/></Action>
      <Action Name="RTimeOfDay"><ReturnType Type="Edm.TimeOfDay"/></Action>
      <Action Name="PDuration"><Parameter Name="v" Type="Edm.Duration" Nullable="true"/></Action>
      <Action Name="PBinary"><Parameter Name="v" Type="Edm.Binary" Nullable="true"/></Action>
      <Action Name="RBinary"><ReturnType Type="Edm.Binary"/></Action>
      <Action Name="PComplex"><Parameter Name="v" Type="Probe.Location" Nullable="true"/></Action>
      <Action Name="PStrList"><Parameter Name="v" Type="Collection(Edm.String)" Nullable="true"/></Action>
      <Action Name="PAlias"><Parameter Name="v" Type="Probe.Alias" Nullable="true"/></Action>
      <Action Name="PEnum"><Parameter Name="v" Type="Probe.Colour" Nullable="true"/></Action>
      <Action Name="PEntity"><Parameter Name="v" Type="Probe.Person" Nullable="true"/></Action>
      <Action Name="PEntityList"><Parameter Name="v" Type="Collection(Probe.Person)" Nullable="true"/></Action>
      <Action Name="PUnimported"><Parameter Name="v" Type="Probe.Ghost" Nullable="true"/></Action>
      <Action Name="PDate"><Parameter Name="v" Type="Edm.Date" Nullable="true"/></Action>
      <EntityContainer Name="Container">
        <EntitySet Name="People" EntityType="Probe.Person"/>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

func parseProbe(t *testing.T) *types.EdmxDocument {
	t.Helper()
	doc, err := types.ParseEdmx(probeMetadata)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return doc
}

// importedPerson resolves the external entity for `Person` only — `Ghost` is the
// type nobody imported, which is the difference the messages turn on.
func importedPerson(remoteName string) string {
	if strings.EqualFold(remoteName, "Person") {
		return "Ext.People"
	}
	return ""
}

// TestEdmTimeOfDayIsADateTime is the mapping gap behind mendixlabs/mxcli#1089.
//
// Mendix SUPPORTS Edm.TimeOfDay on an external action — measured, it is the one
// type in the whole EDM primitive set that draws neither CE7253 nor CE7255 and
// still failed. mxcli did not map it, so the call was written with no
// ParameterType (CE7252) or no VariableDataType (CE7269), and no MDL reached
// either field. Typing it as DateTime clears both: 24 errors -> 22 on the probe
// app, with nothing else changed.
func TestEdmTimeOfDayIsADateTime(t *testing.T) {
	if got := edmReturnTypeToKind("Edm.TimeOfDay"); got != "DateTime" {
		t.Errorf("edmReturnTypeToKind(Edm.TimeOfDay) = %q, want DateTime", got)
	}
}

// TestEdmBinaryIsNotCallable pins the other half of the same table. mxcli mapped
// Edm.Binary to DataTypes$BinaryType, which cannot help: Mendix rejects the
// ACTION, not the type mxcli chose —
//
//	CE7255 "Action 'PBinary' ... is not supported. One of the parameters is not supported"
//
// so the only honest answer is to refuse the statement, and a kind here would
// route it past the refusal.
func TestEdmBinaryIsNotCallable(t *testing.T) {
	if got := edmReturnTypeToKind("Edm.Binary"); got != "" {
		t.Errorf("edmReturnTypeToKind(Edm.Binary) = %q, want \"\" — Mendix refuses the action (CE7255)", got)
	}
}

// TestClassifyExternalActionType is the measured truth table, one row per shape.
// Every verdict was produced by mxbuild 11.12.0 on a real project.
func TestClassifyExternalActionType(t *testing.T) {
	doc := parseProbe(t)

	tests := []struct {
		edmType   string
		wantClass externalActionTypeClass
		wantKind  string
		wantEnt   string
		wantList  bool
		note      string
	}{
		// Representable primitives: 0 errors, measured.
		{edmType: "", wantClass: extTypeRepresentable, wantKind: "Void", note: "an action with no return type"},
		{edmType: "Edm.String", wantClass: extTypeRepresentable, wantKind: "String"},
		{edmType: "Edm.Guid", wantClass: extTypeRepresentable, wantKind: "String"},
		{edmType: "Edm.Int64", wantClass: extTypeRepresentable, wantKind: "Long"},
		{edmType: "Edm.Date", wantClass: extTypeRepresentable, wantKind: "DateTime"},
		{edmType: "Edm.DateTimeOffset", wantClass: extTypeRepresentable, wantKind: "DateTime"},
		{edmType: "Edm.TimeOfDay", wantClass: extTypeRepresentable, wantKind: "DateTime", note: "#1089"},

		// Mendix refuses the action outright: CE7255 (+ CE7253 on some).
		{edmType: "Edm.Duration", wantClass: extTypeUnsupported},
		{edmType: "Edm.Binary", wantClass: extTypeUnsupported},
		{edmType: "Edm.Stream", wantClass: extTypeUnsupported},
		{edmType: "Edm.GeographyPoint", wantClass: extTypeUnsupported},
		{edmType: "Probe.Location", wantClass: extTypeUnsupported, note: "a ComplexType"},
		{edmType: "Probe.Alias", wantClass: extTypeUnsupported, note: "a TypeDefinition"},
		{edmType: "Collection(Edm.String)", wantClass: extTypeUnsupported, note: "a collection of primitives"},
		{edmType: "Probe.Nothing", wantClass: extTypeUnsupported, note: "a type the contract does not declare"},

		// Enums build with no type written at all — measured 0 errors, which is
		// why they must not be lumped in with the unsupported set.
		{edmType: "Probe.Colour", wantClass: extTypeRepresentable},

		// Entity types resolve against the project, not the contract.
		{edmType: "Probe.Person", wantClass: extTypeEntity, wantEnt: "Person"},
		{edmType: "Collection(Probe.Person)", wantClass: extTypeEntity, wantEnt: "Person", wantList: true},
		{edmType: "Probe.Ghost", wantClass: extTypeEntity, wantEnt: "Ghost"},
	}

	for _, tt := range tests {
		got := classifyExternalActionType(doc, tt.edmType)
		if got.class != tt.wantClass || got.kind != tt.wantKind || got.entity != tt.wantEnt || got.isList != tt.wantList {
			t.Errorf("classifyExternalActionType(%q) = %+v, want {class:%v kind:%q entity:%q isList:%v} %s",
				tt.edmType, got, tt.wantClass, tt.wantKind, tt.wantEnt, tt.wantList, tt.note)
		}
	}
}

func probeCall(action string, args ...string) externalCall {
	var as []ast.CallArgument
	for _, a := range args {
		as = append(as, ast.CallArgument{Name: a})
	}
	return externalCall{flow: "Ext.ACT_Probe", stmt: &ast.CallExternalActionStmt{
		ActionName: action,
		Arguments:  as,
	}}
}

// TestCheckExternalActionTypesRefusesUnsupported is the issue's shape: the
// statement executed, the build failed with CE7252, and no MDL cleared it —
// because the limitation is the contract's, not the call's. mxcli said nothing.
func TestCheckExternalActionTypesRefusesUnsupported(t *testing.T) {
	doc := parseProbe(t)

	for _, tc := range []struct {
		action string
		want   string // the type the message has to name
	}{
		{"PDuration", "Edm.Duration"},
		{"PBinary", "Edm.Binary"},
		{"PComplex", "Probe.Location"},
		{"PStrList", "Collection(Edm.String)"},
		{"PAlias", "Probe.Alias"},
	} {
		err := checkExternalActionTypes(probeCall(tc.action, "v"), actionNamed(t, doc, tc.action), doc, "Ext.Probe", importedPerson)
		if err == nil {
			t.Errorf("%s: an unsupported parameter type must be refused", tc.action)
			continue
		}
		for _, want := range []string{tc.want, "CE7255", `"v"`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error %q does not mention %q", tc.action, err.Error(), want)
			}
		}
	}

	// The return side of the same table.
	err := checkExternalActionTypes(probeCall("RBinary"), actionNamed(t, doc, "RBinary"), doc, "Ext.Probe", importedPerson)
	if err == nil {
		t.Fatal("an unsupported return type must be refused")
	}
	if !strings.Contains(err.Error(), "Edm.Binary") || !strings.Contains(err.Error(), "CE7255") {
		t.Errorf("error %q should name the return type and CE7255", err.Error())
	}
}

// TestCheckExternalActionTypesNamesTheImport covers the fixable case: the type
// IS supported, the external entity simply has not been imported. The remedy
// existed all along and mxcli never said it — the return side said it, the
// parameter side did not, and the parameter side is CE7252, the code the issue
// is about.
func TestCheckExternalActionTypesNamesTheImport(t *testing.T) {
	doc := parseProbe(t)

	err := checkExternalActionTypes(probeCall("PUnimported", "v"), actionNamed(t, doc, "PUnimported"), doc, "Ext.Probe", importedPerson)
	if err == nil {
		t.Fatal("an entity-typed parameter with no imported entity must be refused")
	}
	for _, want := range []string{"CE7252", "Ghost", "create or modify external entities from Ext.Probe entities (Ghost)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

// TestCheckExternalActionTypesAcceptsWhatBuilds is the control that matters. A
// refusal is only worth having if it cannot fire on a call Mendix accepts, and
// every shape here was measured at 0 errors on mxbuild 11.12.0 — including the
// two that are representable with NO type written (enum) and the bound action's
// binding parameter, which Mendix supplies rather than the statement.
func TestCheckExternalActionTypesAcceptsWhatBuilds(t *testing.T) {
	doc := parseProbe(t)

	for _, action := range []string{"PEnum", "PEntity", "PEntityList", "PDate", "PTimeOfDay"} {
		if err := checkExternalActionTypes(probeCall(action, "v"), actionNamed(t, doc, action), doc, "Ext.Probe", importedPerson); err != nil {
			t.Errorf("%s builds at 0 errors and must not be refused: %v", action, err)
		}
	}
	if err := checkExternalActionTypes(probeCall("RTimeOfDay"), actionNamed(t, doc, "RTimeOfDay"), doc, "Ext.Probe", importedPerson); err != nil {
		t.Errorf("RTimeOfDay builds at 0 errors and must not be refused: %v", err)
	}

	// A bound action's binding parameter is not the statement's to supply, and
	// it is not the statement's to be refused over either.
	bound := &types.EdmAction{
		Name:    "Touch",
		IsBound: true,
		Parameters: []*types.EdmActionParameter{
			{Name: "bindingParameter", Type: "Probe.Ghost"},
			{Name: "note", Type: "Edm.String"},
		},
	}
	if err := checkExternalActionTypes(probeCall("Touch", "note"), bound, doc, "Ext.Probe", importedPerson); err != nil {
		t.Errorf("the binding parameter must be skipped, as it is by the name check: %v", err)
	}
}

// probeFlowBuilder runs the real builder against probeMetadata, so the write
// path is exercised rather than reproduced.
func probeFlowBuilder(t *testing.T) *flowBuilder {
	t.Helper()
	svcID := model.ID("svc-1")
	modID := model.ID("mod-1")
	mb := &mock.MockBackend{
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{{
				BaseElement: model.BaseElement{ID: svcID},
				ContainerID: modID,
				Name:        "Probe",
				Metadata:    probeMetadata,
			}}, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return nil, nil },
	}
	return &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing, backend: mb,
		hierarchy:    &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "Ext"}},
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
	}
}

func buildProbeCall(t *testing.T, script string) *flowBuilder {
	t.Helper()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parsing the probe script: %v", errs)
	}
	call := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.CallExternalActionStmt)
	fb := probeFlowBuilder(t)
	fb.addCallExternalActionAction(call)
	return fb
}

func probeCallAction(t *testing.T, fb *flowBuilder) *microflows.CallExternalAction {
	t.Helper()
	for _, obj := range fb.objects {
		if act, ok := obj.(*microflows.ActionActivity); ok {
			if call, ok := act.Action.(*microflows.CallExternalAction); ok {
				return call
			}
		}
	}
	t.Fatal("the builder produced no CallExternalAction")
	return nil
}

// TestWriterTypesTimeOfDay is the write half of the #1089 fix: the type has to
// reach the BSON, not merely pass the checker.
func TestWriterTypesTimeOfDay(t *testing.T) {
	fb := buildProbeCall(t, "create microflow Ext.ACT_Probe()\nbegin\n"+
		"  call external action Ext.Probe.PTimeOfDay(v = empty);\nend;")
	if errs := fb.GetErrors(); len(errs) > 0 {
		t.Fatalf("a TimeOfDay parameter builds at 0 errors and must not be refused: %v", errs)
	}
	call := probeCallAction(t, fb)
	if len(call.ParameterMappings) != 1 || call.ParameterMappings[0].ParameterDataType != "DateTime" {
		t.Errorf("ParameterType = %+v, want one mapping typed DateTime — untyped is CE7252",
			call.ParameterMappings)
	}

	fb = buildProbeCall(t, "create microflow Ext.ACT_Probe()\nbegin\n"+
		"  $r = call external action Ext.Probe.RTimeOfDay();\nend;")
	if errs := fb.GetErrors(); len(errs) > 0 {
		t.Fatalf("a TimeOfDay return builds at 0 errors and must not be refused: %v", errs)
	}
	if got := probeCallAction(t, fb).ResultDataType; got != "DateTime" {
		t.Errorf("ResultDataType = %q, want DateTime — untyped is CE7269", got)
	}
}

// TestWriterRefusesUntypableExternalAction pins the tier that the reported
// workflow actually runs. `mxcli exec` does NOT run the project-resolved
// reference pass, so a rule wired only into `check --references` still lets the
// statement write an unbuildable call — the exact sequence in the report:
// exec succeeded, `docker check` failed with CE7252, and nothing in MDL cleared it.
func TestWriterRefusesUntypableExternalAction(t *testing.T) {
	for _, tc := range []struct{ action, arg, want string }{
		{"PDuration", "v = empty", "CE7255"},
		{"PComplex", "v = empty", "CE7255"},
		{"PUnimported", "v = empty", "CE7252"},
	} {
		fb := buildProbeCall(t, "create microflow Ext.ACT_Probe()\nbegin\n"+
			"  call external action Ext.Probe."+tc.action+"("+tc.arg+");\nend;")
		errs := fb.GetErrors()
		if len(errs) == 0 {
			t.Errorf("%s: the writer must refuse a call it cannot type", tc.action)
			continue
		}
		if !strings.Contains(strings.Join(errs, "\n"), tc.want) {
			t.Errorf("%s: errors %v do not mention %s", tc.action, errs, tc.want)
		}
	}
}
