// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// TestReadJavaActionByName_RoundTrip guards the codec-native java-action read:
// create an action, then ReadJavaActionByName must return its name, parameters
// (with types) and return type — the microflow builder relies on this to resolve
// a java-action call's parameter types (else "entity type params will be empty").
func TestReadJavaActionByName_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	ja := &javaactions.JavaAction{
		ContainerID: mod.ID,
		Name:        "ZzJa",
		Parameters: []*javaactions.JavaActionParameter{
			{Name: "Count", IsRequired: true, ParameterType: &javaactions.IntegerType{}},
			{Name: "Cust", IsRequired: true, ParameterType: &javaactions.EntityType{Entity: "MyFirstModule.Thing"}},
		},
		ReturnType: &javaactions.BooleanType{},
	}
	ja.ID = model.ID("")
	if err := b.CreateJavaAction(ja); err != nil {
		t.Fatalf("CreateJavaAction: %v", err)
	}

	got, err := b.ReadJavaActionByName("MyFirstModule.ZzJa")
	if err != nil {
		t.Fatalf("ReadJavaActionByName: %v", err)
	}
	if got.Name != "ZzJa" || len(got.Parameters) != 2 {
		t.Fatalf("got name=%q params=%d, want ZzJa/2", got.Name, len(got.Parameters))
	}
	if _, ok := got.Parameters[0].ParameterType.(*javaactions.IntegerType); !ok {
		t.Errorf("param 0 type = %T, want *IntegerType", got.Parameters[0].ParameterType)
	}
	ent, ok := got.Parameters[1].ParameterType.(*javaactions.EntityType)
	if !ok || ent.Entity != "MyFirstModule.Thing" {
		t.Errorf("param 1 type = %#v, want EntityType{MyFirstModule.Thing}", got.Parameters[1].ParameterType)
	}
	if _, ok := got.ReturnType.(*javaactions.BooleanType); !ok {
		t.Errorf("return type = %T, want *BooleanType", got.ReturnType)
	}
}

// A microflow-typed parameter — the shape MCPServer.AddTool's ExecutingMicroflow
// and every other "register a callback" java action uses — read back as a String,
// because codeActionBasicFromGen had no case for it and fell through to the
// default. The microflow builder then authored the callback as a
// BasicCodeActionParameterValue holding 'Module.MyFlow' as a literal instead of a
// MicroflowParameterValue, and `mx check` reported CE0115. mxcli-chat FINDINGS §36.
func TestReadJavaActionByName_MicroflowParameterType(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	ja := &javaactions.JavaAction{
		ContainerID: mod.ID,
		Name:        "ZzRegisterTool",
		Parameters: []*javaactions.JavaActionParameter{
			{Name: "Name", IsRequired: true, ParameterType: &javaactions.StringType{}},
			{Name: "ExecutingMicroflow", IsRequired: true, ParameterType: &javaactions.MicroflowType{}},
		},
	}
	if err := b.CreateJavaAction(ja); err != nil {
		t.Fatalf("CreateJavaAction: %v", err)
	}

	got, err := b.ReadJavaActionByName("MyFirstModule.ZzRegisterTool")
	if err != nil {
		t.Fatalf("ReadJavaActionByName: %v", err)
	}
	if len(got.Parameters) != 2 {
		t.Fatalf("params = %d, want 2", len(got.Parameters))
	}
	if _, ok := got.Parameters[0].ParameterType.(*javaactions.StringType); !ok {
		t.Errorf("param 0 type = %T, want *StringType", got.Parameters[0].ParameterType)
	}
	if _, ok := got.Parameters[1].ParameterType.(*javaactions.MicroflowType); !ok {
		t.Fatalf("param 1 type = %T, want *MicroflowType — a microflow callback read back as this "+
			"is what makes the caller author a literal string and fail CE0115",
			got.Parameters[1].ParameterType)
	}
}

// DESCRIBE JAVA ACTION printed `ContextObject: entity <>` for a parameter
// declared `entity <pEntity> not null` (mendixlabs/mxcli#1034). The stored
// parameter type holds only a BY_ID pointer to the TypeParameter; the reader
// carried the ID across but never resolved it to the name, which is all the
// describer prints. The JavaScript-action reader already did this resolution.
func TestReadJavaActionByName_ResolvesTypeParameterNames(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	tp := &javaactions.TypeParameterDef{Name: "pEntity"}
	tp.ID = model.ID("0b6f0d0e-1034-4a00-8000-000000000001")
	ja := &javaactions.JavaAction{
		ContainerID:    mod.ID,
		Name:           "ZzJaTypeParam",
		TypeParameters: []*javaactions.TypeParameterDef{tp},
		Parameters: []*javaactions.JavaActionParameter{
			{Name: "ContextObject", IsRequired: true,
				ParameterType: &javaactions.EntityTypeParameterType{TypeParameterID: tp.ID, TypeParameterName: "pEntity"}},
			{Name: "Obj", IsRequired: true,
				ParameterType: &javaactions.TypeParameter{TypeParameterID: tp.ID, TypeParameter: "pEntity"}},
		},
		ReturnType: &javaactions.TypeParameter{TypeParameterID: tp.ID, TypeParameter: "pEntity"},
	}
	if err := b.CreateJavaAction(ja); err != nil {
		t.Fatalf("CreateJavaAction: %v", err)
	}

	got, err := b.ReadJavaActionByName("MyFirstModule.ZzJaTypeParam")
	if err != nil {
		t.Fatalf("ReadJavaActionByName: %v", err)
	}
	if len(got.Parameters) != 2 {
		t.Fatalf("params = %d, want 2", len(got.Parameters))
	}
	etp, ok := got.Parameters[0].ParameterType.(*javaactions.EntityTypeParameterType)
	if !ok {
		t.Fatalf("param 0 type = %T, want *EntityTypeParameterType", got.Parameters[0].ParameterType)
	}
	if etp.TypeParameterName != "pEntity" {
		t.Errorf("entity type parameter name = %q, want %q (describe prints `entity <%s>`)",
			etp.TypeParameterName, "pEntity", etp.TypeParameterName)
	}
	if p, ok := got.Parameters[1].ParameterType.(*javaactions.TypeParameter); !ok || p.TypeParameter != "pEntity" {
		t.Errorf("param 1 type = %#v, want TypeParameter{pEntity}", got.Parameters[1].ParameterType)
	}
	if r, ok := got.ReturnType.(*javaactions.TypeParameter); !ok || r.TypeParameter != "pEntity" {
		t.Errorf("return type = %#v, want TypeParameter{pEntity}", got.ReturnType)
	}
}
