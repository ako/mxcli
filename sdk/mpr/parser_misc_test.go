// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"
)

// Issue #585: parseJsonElement asserted `raw[field].(int32)` for every numeric
// facet on a JSON-structure element. Mendix Studio Pro stores these fields as
// BSON int64, so the assertion failed silently and the parsed value defaulted
// to 0 — the same class of bug fixed for StringAttributeType.Length in #583.
//
// Each numeric facet must round-trip across every BSON numeric width that the
// mongo-driver may produce (int32, int64, int, float64) and preserve the
// default when the field is missing.
func TestParseJsonElement_NumericFields_BsonNumericWidths(t *testing.T) {
	type fieldCase struct {
		name    string
		bsonKey string
		read    func(*JsonElement) int
		missing int // expected zero value when field is absent
	}
	fields := []fieldCase{
		{"MinOccurs", "MinOccurs", func(e *JsonElement) int { return e.MinOccurs }, 0},
		{"MaxOccurs", "MaxOccurs", func(e *JsonElement) int { return e.MaxOccurs }, 0},
		{"MaxLength", "MaxLength", func(e *JsonElement) int { return e.MaxLength }, -1},
		{"FractionDigits", "FractionDigits", func(e *JsonElement) int { return e.FractionDigits }, -1},
		{"TotalDigits", "TotalDigits", func(e *JsonElement) int { return e.TotalDigits }, -1},
	}

	widths := []struct {
		name  string
		value any
	}{
		{"int32 (mxcli writer)", int32(42)},
		{"int64 (Studio Pro writer)", int64(42)},
		{"int", int(42)},
		{"float64 (extended JSON)", float64(42)},
	}

	for _, f := range fields {
		for _, w := range widths {
			t.Run(f.name+"/"+w.name, func(t *testing.T) {
				raw := map[string]any{f.bsonKey: w.value}
				elem := parseJsonElement(raw)
				if got := f.read(elem); got != 42 {
					t.Errorf("%s = %d, want 42 (input %T(%v))", f.name, got, w.value, w.value)
				}
			})
		}
		t.Run(f.name+"/missing", func(t *testing.T) {
			elem := parseJsonElement(map[string]any{})
			if got := f.read(elem); got != f.missing {
				t.Errorf("%s = %d, want default %d when field absent", f.name, got, f.missing)
			}
		})
	}
}
