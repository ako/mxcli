// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestMfDataType_Primitives(t *testing.T) {
	cases := []struct {
		dt   microflows.DataType
		want string
	}{
		{&microflows.BooleanType{}, "Boolean"},
		{&microflows.IntegerType{}, "Integer"},
		{&microflows.DecimalType{}, "Decimal"},
		{&microflows.StringType{}, "String"},
		{&microflows.DateTimeType{}, "DateTime"},
		{&microflows.DateType{}, "DateTime"}, // Date maps to DateTime
	}
	for _, c := range cases {
		got, ent, enum, err := mfDataType(c.dt)
		if err != nil || got != c.want || ent != "" || enum != "" {
			t.Errorf("mfDataType(%s) = %q/%q/%q/%v; want %q", c.dt.GetTypeName(), got, ent, enum, err, c.want)
		}
	}
}

func TestMfDataType_Void(t *testing.T) {
	got, _, _, err := mfDataType(nil)
	if err != nil || got != "Void" {
		t.Errorf("nil data type should be Void, got %q/%v", got, err)
	}
}

func TestMfDataType_ObjectAndEnumeration(t *testing.T) {
	got, ent, _, err := mfDataType(&microflows.ObjectType{EntityQualifiedName: "Sales.Order"})
	if err != nil || got != "Object" || ent != "Sales.Order" {
		t.Errorf("object type wrong: %q/%q/%v", got, ent, err)
	}
	got, ent, _, err = mfDataType(&microflows.ListType{EntityQualifiedName: "Sales.Order"})
	if err != nil || got != "List" || ent != "Sales.Order" {
		t.Errorf("list type wrong: %q/%q/%v", got, ent, err)
	}
	got, _, enum, err := mfDataType(&microflows.EnumerationType{EnumerationQualifiedName: "M.Status"})
	if err != nil || got != "Enumeration" || enum != "M.Status" {
		t.Errorf("enumeration type wrong: %q/%q/%v", got, enum, err)
	}
}

func TestMfDataType_UnsupportedErrors(t *testing.T) {
	if _, _, _, err := mfDataType(&microflows.LongType{}); err == nil {
		t.Error("Long is not in the PED param/return enum and should error")
	}
}

func TestMapMicroflowAction_CreateVariable(t *testing.T) {
	m, err := mapMicroflowAction(&microflows.CreateVariableAction{
		VariableName: "Result",
		DataType:     &microflows.IntegerType{},
		InitialValue: "$N * 5",
	})
	if err != nil {
		t.Fatalf("mapMicroflowAction: %v", err)
	}
	if m["$Type"] != "Microflows$CreateVariableAction" || m["variableName"] != "Result" ||
		m["variableType"] != "Integer" || m["initialValue"] != "$N * 5" {
		t.Fatalf("unexpected mapping: %+v", m)
	}
}

func TestMapMicroflowAction_Unsupported(t *testing.T) {
	if _, err := mapMicroflowAction(&microflows.CommitObjectsAction{}); err == nil {
		t.Error("an unmapped action type should error")
	}
}

func TestMfVariableType(t *testing.T) {
	if got, err := mfVariableType(&microflows.StringType{}); err != nil || got != "String" {
		t.Errorf("String: %q/%v", got, err)
	}
	if got, err := mfVariableType(&microflows.DateType{}); err != nil || got != "DateTime" {
		t.Errorf("Date should map to DateTime: %q/%v", got, err)
	}
	if _, err := mfVariableType(&microflows.ObjectType{}); err == nil {
		t.Error("object variable type should be rejected (primitives only)")
	}
}
