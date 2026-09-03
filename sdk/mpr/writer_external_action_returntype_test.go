// SPDX-License-Identifier: Apache-2.0

package mpr

import "testing"

// TestSerializeExternalActionReturnType covers the DataTypes$ element written
// into CallExternalAction.VariableDataType.
//
// Object and List were unreachable before: the resolver mapped only EDM
// primitives and returned "" for anything else, so an action returning an entity
// (or a collection of them) got NO VariableDataType at all, and Mendix reported
// CE7269 "The return type for remote action '<x>' has changed"
// (mendixlabs/mxcli#1020). Both carry an Entity — a DataTypes$ObjectType without
// one is as unaligned as no type at all.
func TestSerializeExternalActionReturnType(t *testing.T) {
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
		{name: "empty is void", kind: "", wantType: "DataTypes$VoidType"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := serializeExternalActionReturnType(tt.kind, tt.entity)

			var gotType, gotEntity string
			var hasEntity, hasID bool
			for _, e := range doc {
				switch e.Key {
				case "$Type":
					gotType, _ = e.Value.(string)
				case "Entity":
					gotEntity, _ = e.Value.(string)
					hasEntity = true
				case "$ID":
					hasID = true
				}
			}

			if gotType != tt.wantType {
				t.Errorf("$Type = %q, want %q", gotType, tt.wantType)
			}
			if !hasID {
				t.Error("every DataTypes$ element needs its own $ID")
			}
			if tt.wantEntity == "" {
				if hasEntity {
					t.Errorf("a primitive return must not carry an Entity (got %q)", gotEntity)
				}
				return
			}
			if gotEntity != tt.wantEntity {
				t.Errorf("Entity = %q, want %q", gotEntity, tt.wantEntity)
			}
		})
	}
}
