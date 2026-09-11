// SPDX-License-Identifier: Apache-2.0

// attributeBsonToMDL read the attribute's type object from raw["Type"]. That is
// the SDK name; Mendix stores it under "NewType" (the storage-name split in
// CLAUDE.md, per property rather than per $Type). No real document carries a
// "Type" key, so the lookup never hit and EVERY attribute rendered as
// "Unknown" — on both engines, since this renderer does not touch the backend.
//
// The cost is not the ugly word. It is that "Unknown" == "Unknown": narrowing
// FullName from String(200) to String(50) produced "1 modified" and an empty
// diff, because both sides rendered identically. Found alongside
// mendixlabs/mxcli#1080.
package executor

import (
	"strings"
	"testing"
)

func attrRaw(name string, typeObj map[string]any) map[string]any {
	return map[string]any{
		"$Type":   "DomainModels$Attribute",
		"Name":    name,
		"NewType": typeObj,
	}
}

func TestAttributeBsonToMDL_ReadsStorageNameNewType(t *testing.T) {
	cases := []struct {
		name    string
		typeObj map[string]any
		want    string
	}{
		{"string with length", map[string]any{"$Type": "DomainModels$StringAttributeType", "Length": int32(200)}, "Name: String(200)"},
		{"unbounded string", map[string]any{"$Type": "DomainModels$StringAttributeType", "Length": int32(0)}, "Name: String"},
		{"boolean", map[string]any{"$Type": "DomainModels$BooleanAttributeType"}, "Name: Boolean"},
		{"integer", map[string]any{"$Type": "DomainModels$IntegerAttributeType"}, "Name: Integer"},
		{"enumeration", map[string]any{"$Type": "DomainModels$EnumerationAttributeType"}, "Name: Enumeration"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := attributeBsonToMDL(nil, attrRaw("Name", tc.typeObj))
			if got != tc.want {
				t.Errorf("attributeBsonToMDL = %q, want %q", got, tc.want)
			}
			if strings.Contains(got, "Unknown") {
				t.Errorf("attribute rendered as Unknown: %q — two attributes of "+
					"different types then diff to nothing", got)
			}
		})
	}
}

// The whole point, stated as a diff: a length change must not render the same
// on both sides.
func TestAttributeBsonToMDL_LengthChangeIsVisible(t *testing.T) {
	before := attributeBsonToMDL(nil, attrRaw("FullName",
		map[string]any{"$Type": "DomainModels$StringAttributeType", "Length": int32(200)}))
	after := attributeBsonToMDL(nil, attrRaw("FullName",
		map[string]any{"$Type": "DomainModels$StringAttributeType", "Length": int32(50)}))
	if before == after {
		t.Fatalf("String(200) and String(50) both render as %q — diff-local "+
			"counts the unit as modified and shows no change", before)
	}
}

// Documents written before the rename still carry "Type"; keep reading it, so
// the fix is additive rather than a swap.
func TestAttributeBsonToMDL_StillReadsLegacyTypeKey(t *testing.T) {
	raw := map[string]any{
		"$Type": "DomainModels$Attribute",
		"Name":  "Legacy",
		"Type":  map[string]any{"$Type": "DomainModels$IntegerAttributeType"},
	}
	if got := attributeBsonToMDL(nil, raw); got != "Legacy: Integer" {
		t.Errorf("attributeBsonToMDL = %q, want %q", got, "Legacy: Integer")
	}
}
