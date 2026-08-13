// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// upstream #881, legacy engine. Both engines share the semantic model, so a fix
// in one is invisible to anyone running the other (MXCLI_ENGINE=legacy). These
// mirror the modelsdk tests against the legacy parser/serializer.

// A CustomRange must survive the round trip: the reader dispatches on $Type, and
// the writer selects the variant. Before this, "Custom" was unrepresentable and
// a bounded import silently became unbounded on the next exec.
func TestLegacyImportXmlActionRoundTripsCustomRange(t *testing.T) {
	doc := serializeImportXmlAction(&microflows.ImportXmlAction{
		BaseElement: model.BaseElement{ID: model.ID("a-1")},
		ResultHandling: &microflows.ResultHandlingMapping{
			BaseElement:      model.BaseElement{ID: model.ID("rh-1")},
			MappingID:        model.ID("M.IMM"),
			ResultEntityID:   model.ID("M.Root"),
			ResultVariable:   "Out",
			LimitExpression:  "10",
			OffsetExpression: "5",
		},
		XmlDocumentVariable: "Resp",
	})

	rhFields := bsonDMap(asD(t, bsonDMap(doc)["ResultHandling"]))
	call := bsonDMap(asD(t, rhFields["ImportMappingCall"]))
	rangeDoc := bsonDMap(asD(t, call["Range"]))

	if got := rangeDoc["$Type"]; got != "Microflows$CustomRange" {
		t.Fatalf("Range $Type = %v, want Microflows$CustomRange", got)
	}
	if got := rangeDoc["LimitExpression"]; got != "10" {
		t.Errorf("LimitExpression = %v, want 10", got)
	}
	if got := rangeDoc["OffsetExpression"]; got != "5" {
		t.Errorf("OffsetExpression = %v, want 5", got)
	}
	if _, ok := rangeDoc["SingleObject"]; ok {
		t.Error("a CustomRange must not carry SingleObject — a bounded range is always bounded")
	}
}

// The read side of the same. The result entity also lives on the ResultHandling,
// not on the ImportMappingCall, which is where the legacy parser looked — so it
// came back empty for everything mxcli or Studio Pro writes.
func TestLegacyParseImportXmlActionReadsCustomRange(t *testing.T) {
	got := parseImportXmlAction(map[string]any{
		"$ID":                     "a-1",
		"XmlDocumentVariableName": "Resp",
		"ResultHandling": map[string]any{
			"$ID":                "rh-1",
			"ResultVariableName": "Out",
			"ImportMappingCall": map[string]any{
				"ReturnValueMapping":    "M.IMM",
				"ForceSingleOccurrence": false,
				"Range": map[string]any{
					"$Type":            "Microflows$CustomRange",
					"LimitExpression":  "10",
					"OffsetExpression": "5",
				},
			},
			"VariableType": map[string]any{
				"$Type":  "DataTypes$ListType",
				"Entity": "M.Root",
			},
		},
	})

	if got.ResultHandling == nil {
		t.Fatal("ResultHandling missing")
	}
	h := got.ResultHandling
	if h.LimitExpression != "10" || h.OffsetExpression != "5" {
		t.Errorf("limit/offset = %q/%q, want 10/5", h.LimitExpression, h.OffsetExpression)
	}
	if h.SingleObject {
		t.Error("SingleObject = true, want false (ListType variable)")
	}
	if string(h.ResultEntityID) != "M.Root" {
		t.Errorf("ResultEntityID = %q, want M.Root — VariableType is stored on the "+
			"ResultHandling, not on the ImportMappingCall", h.ResultEntityID)
	}
}

// The shape Mendix ships in the blank app: range All against an OBJECT variable.
// The range and the variable's cardinality are separate axes, and folding one
// into the other describes this as `first` — rewriting the activity on re-exec.
func TestLegacyParseImportXmlActionSeparatesRangeFromCardinality(t *testing.T) {
	got := parseImportXmlAction(map[string]any{
		"$ID":                     "a-1",
		"XmlDocumentVariableName": "Resp",
		"ResultHandling": map[string]any{
			"$ID":                "rh-1",
			"ResultVariableName": "Out",
			"ImportMappingCall": map[string]any{
				"ReturnValueMapping":    "M.IMM",
				"ForceSingleOccurrence": false,
				"Range":                 map[string]any{"$Type": "Microflows$ConstantRange", "SingleObject": false},
			},
			"VariableType": map[string]any{"$Type": "DataTypes$ObjectType", "Entity": "M.Root"},
		},
	})

	h := got.ResultHandling
	if h == nil {
		t.Fatal("ResultHandling missing")
	}
	if h.RangeSingleObject == nil || *h.RangeSingleObject {
		t.Errorf("RangeSingleObject = %v, want explicit false — the range is All", h.RangeSingleObject)
	}
	if !h.SingleObject {
		t.Error("SingleObject = false, want true — the stored ObjectType is the authority " +
			"on the variable's cardinality, and mxbuild rejects the mismatch with CE0243")
	}
}

// The writer's variant choice must read the RANGE's flag, not the variable's:
// serializing Mendix's own All-against-an-object shape as First changes it.
func TestLegacySerializeImportXmlActionWritesRangeFlagNotCardinality(t *testing.T) {
	no := false
	doc := serializeImportXmlAction(&microflows.ImportXmlAction{
		BaseElement: model.BaseElement{ID: model.ID("a-1")},
		ResultHandling: &microflows.ResultHandlingMapping{
			BaseElement:       model.BaseElement{ID: model.ID("rh-1")},
			MappingID:         model.ID("M.IMM"),
			ResultEntityID:    model.ID("M.Root"),
			ResultVariable:    "Out",
			SingleObject:      true, // an object-rooted mapping
			RangeSingleObject: &no,  // …with the range left at All
		},
		XmlDocumentVariable: "Resp",
	})

	rhFields := bsonDMap(asD(t, bsonDMap(doc)["ResultHandling"]))
	call := bsonDMap(asD(t, rhFields["ImportMappingCall"]))
	rangeDoc := bsonDMap(asD(t, call["Range"]))
	if got := rangeDoc["SingleObject"]; got != false {
		t.Errorf("Range.SingleObject = %v, want false — the range is All", got)
	}
	varType := bsonDMap(asD(t, rhFields["VariableType"]))
	if got := varType["$Type"]; got != "DataTypes$ObjectType" {
		t.Errorf("VariableType = %v, want DataTypes$ObjectType — the variable follows the "+
			"mapping, not the range", got)
	}
}

// asD narrows a nested BSON value so a missing sub-document fails at the field
// it is missing from rather than as a bare type-assertion panic.
func asD(t *testing.T, v any) primitive.D {
	t.Helper()
	d, ok := v.(primitive.D)
	if !ok {
		t.Fatalf("expected a BSON document, got %T", v)
	}
	return d
}
