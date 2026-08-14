// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// TestParseRange_StudioProShapes pins how Studio Pro actually stores a database
// retrieve's Range, measured rather than assumed.
//
// Source: ako/TestApp, MyFirstModule.RetrieveExamples on Mendix 11.13.0 — one
// retrieve per UI option, dumped with `mxcli bson dump --type microflow`:
//
//	All                     Microflows$ConstantRange {SingleObject:false}
//	First                   Microflows$ConstantRange {SingleObject:true}
//	Custom (limit 4, off 2) Microflows$CustomRange   {LimitExpression:"4", OffsetExpression:"2"}
//
// This matters beyond the parser. Range is a POLYMORPHIC child, the shape that
// has produced repeated data loss when a reader pulls one scalar out of it
// without dispatching on $Type (DomainModels$RuleInfo, and the import-mapping
// Range in #881). Pinning the real variants is what makes the dispatch here
// checkable instead of folkloric.
func TestParseRange_StudioProShapes(t *testing.T) {
	tests := []struct {
		name       string
		raw        map[string]any
		wantType   microflows.RangeType
		wantLimit  string
		wantOffset string
	}{
		{
			name:     "All",
			raw:      map[string]any{"$Type": "Microflows$ConstantRange", "SingleObject": false},
			wantType: microflows.RangeTypeAll,
		},
		{
			name:     "First",
			raw:      map[string]any{"$Type": "Microflows$ConstantRange", "SingleObject": true},
			wantType: microflows.RangeTypeFirst,
		},
		{
			name: "Custom",
			raw: map[string]any{
				"$Type":            "Microflows$CustomRange",
				"LimitExpression":  "4",
				"OffsetExpression": "2",
			},
			wantType:   microflows.RangeTypeCustom,
			wantLimit:  "4",
			wantOffset: "2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRange(tt.raw)
			if got == nil {
				t.Fatal("parseRange returned nil")
			}
			if got.RangeType != tt.wantType {
				t.Errorf("RangeType = %v, want %v", got.RangeType, tt.wantType)
			}
			if got.Limit != tt.wantLimit {
				t.Errorf("Limit = %q, want %q", got.Limit, tt.wantLimit)
			}
			if got.Offset != tt.wantOffset {
				t.Errorf("Offset = %q, want %q", got.Offset, tt.wantOffset)
			}
		})
	}
}

// TestParseRange_ConstantRangeWithLimitIsUnobserved documents the tolerance
// branch rather than endorsing it.
//
// No Studio Pro document has been seen storing Limit/Offset on a ConstantRange;
// the branch exists only for formats we have not sampled. modelsdk cannot read
// this shape at all (gen binds only SingleObject on ConstantRange), so if it
// ever turns up in a real project the engines diverge and the fix belongs in
// gen, not here. This test exists so that discovery lands on a named case.
func TestParseRange_ConstantRangeWithLimitIsUnobserved(t *testing.T) {
	got := parseRange(map[string]any{
		"$Type":           "Microflows$ConstantRange",
		"SingleObject":    false,
		"LimitExpression": "10",
	})
	if got.RangeType != microflows.RangeTypeCustom || got.Limit != "10" {
		t.Errorf("legacy tolerance changed: got %v/%q — if this is now intended, "+
			"check whether modelsdk's rangeFromGen was taught to read it too",
			got.RangeType, got.Limit)
	}
}
