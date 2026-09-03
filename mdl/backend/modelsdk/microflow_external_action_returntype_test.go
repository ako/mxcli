// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// TestExternalActionReturnTypeToGen covers the DataTypes$ element written into
// CallExternalAction.VariableDataType, asserted on the ENCODED document so the
// Entity is checked as a stored key rather than as a Go field.
//
// Object and List were unreachable before: the resolver mapped only EDM
// primitives, so an action returning an entity got no VariableDataType at all
// and Mendix reported CE7269 "The return type for remote action '<x>' has
// changed" (mendixlabs/mxcli#1020).
func TestExternalActionReturnTypeToGen(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		entity     string
		wantType   string
		wantEntity string // "" = the key must be absent
	}{
		{name: "object return", kind: "Object", entity: "Trippin.Airport",
			wantType: "DataTypes$ObjectType", wantEntity: "Trippin.Airport"},
		{name: "list return", kind: "List", entity: "Trippin.Person",
			wantType: "DataTypes$ListType", wantEntity: "Trippin.Person"},
		{name: "boolean", kind: "Boolean", wantType: "DataTypes$BooleanType"},
		{name: "string", kind: "String", wantType: "DataTypes$StringType"},
		{name: "integer", kind: "Integer", wantType: "DataTypes$IntegerType"},
		{name: "long is an integer", kind: "Long", wantType: "DataTypes$IntegerType"},
		{name: "decimal", kind: "Decimal", wantType: "DataTypes$DecimalType"},
		{name: "datetime", kind: "DateTime", wantType: "DataTypes$DateTimeType"},
		{name: "binary", kind: "Binary", wantType: "DataTypes$BinaryType"},
		{name: "void", kind: "Void", wantType: "DataTypes$VoidType"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := externalActionReturnTypeToGen(tt.kind, tt.entity)
			if g == nil {
				t.Fatal("externalActionReturnTypeToGen returned nil")
			}
			if g.TypeName() != tt.wantType {
				t.Errorf("$Type = %q, want %q", g.TypeName(), tt.wantType)
			}

			// Encoding is the real check: a mistyped property fails here rather
			// than at Studio Pro.
			raw, err := (&codec.Encoder{}).Encode(g)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			doc := bson.Raw(raw)

			val, lookupErr := doc.LookupErr("Entity")
			if tt.wantEntity == "" {
				if lookupErr == nil {
					t.Errorf("a primitive return must not carry an Entity (got %v)", val)
				}
				return
			}
			if lookupErr != nil {
				t.Fatalf("Entity key missing — an ObjectType/ListType without one is as unaligned as no type at all")
			}
			if got, ok := val.StringValueOK(); !ok || got != tt.wantEntity {
				t.Errorf("Entity = %v, want %q", val, tt.wantEntity)
			}
		})
	}
}

// TestCallExternalActionParameterTypeIsWritten covers the CE7252 half.
//
// generated/metamodel declares ExternalActionParameterMapping.ParameterType
// WITHOUT omitempty, and mxcli never wrote it. Measured on Mendix 11.13: a call
// with any parameter produced CE7252 "The parameters for remote action '<x>'
// have changed" plus one CE0117 "Error(s) in expression" per argument — an
// argument cannot be type-checked against a parameter that has no type. With
// ParameterType written, the same project builds at 0 errors.
func TestCallExternalActionParameterTypeIsWritten(t *testing.T) {
	act := &microflows.CallExternalAction{
		ConsumedODataService: "Ext.TripPin",
		Name:                 "FindAirport",
		ResultVariableName:   "Airport",
		ResultDataType:       "Object",
		ResultEntity:         "Ext.Airport",
		ParameterMappings: []*microflows.ExternalActionParameterMapping{
			{ParameterName: "code", Argument: "'EHAM'", ParameterDataType: "String"},
			{ParameterName: "near", Argument: "$A", ParameterDataType: "Object", ParameterEntity: "Ext.Airport"},
		},
	}

	g := callExternalActionToGen(act)
	if g == nil {
		t.Fatal("callExternalActionToGen returned nil")
	}
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	doc := bson.Raw(raw)

	// The result variable's type still round-trips alongside the new parameter types.
	vdt, err := doc.LookupErr("VariableDataType")
	if err != nil {
		t.Fatal("VariableDataType missing")
	}
	if got, _ := bson.Raw(vdt.Value).Lookup("$Type").StringValueOK(); got != "DataTypes$ObjectType" {
		t.Errorf("VariableDataType $Type = %q, want DataTypes$ObjectType", got)
	}

	arr, err := doc.LookupErr("ParameterMappings")
	if err != nil {
		t.Fatal("ParameterMappings missing")
	}
	vals, err := bson.Raw(arr.Value).Values()
	if err != nil {
		t.Fatalf("read ParameterMappings: %v", err)
	}
	// The typed-array marker leads the list; the mappings follow it.
	var seen int
	want := map[string][2]string{
		"code": {"DataTypes$StringType", ""},
		"near": {"DataTypes$ObjectType", "Ext.Airport"},
	}
	for _, v := range vals {
		md, ok := v.DocumentOK()
		if !ok {
			continue
		}
		name, _ := md.Lookup("ParameterName").StringValueOK()
		exp, tracked := want[name]
		if !tracked {
			continue
		}
		seen++
		pt, err := md.LookupErr("ParameterType")
		if err != nil {
			t.Errorf("%s: ParameterType missing — this is CE7252", name)
			continue
		}
		ptDoc := bson.Raw(pt.Value)
		if got, _ := ptDoc.Lookup("$Type").StringValueOK(); got != exp[0] {
			t.Errorf("%s: ParameterType $Type = %q, want %q", name, got, exp[0])
		}
		if exp[1] != "" {
			if got, _ := ptDoc.Lookup("Entity").StringValueOK(); got != exp[1] {
				t.Errorf("%s: ParameterType Entity = %q, want %q", name, got, exp[1])
			}
		}
	}
	if seen != len(want) {
		t.Errorf("found %d parameter mappings, want %d", seen, len(want))
	}
}
