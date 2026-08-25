// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSerializeListOperation_FindByAttribute(t *testing.T) {
	doc := serializeListOperation(&microflows.FindByAttributeOperation{
		BaseElement:  model.BaseElement{ID: "operation-id"},
		ListVariable: "Items",
		Attribute:    "Demo.Item.Code",
		Expression:   "$IteratorItem/ExternalCode",
	})
	fields := listOperationDocMap(doc)

	if got := fields["$Type"]; got != "Microflows$Find" {
		t.Fatalf("$Type = %v, want Microflows$Find", got)
	}
	if got := fields["Attribute"]; got != "Demo.Item.Code" {
		t.Fatalf("Attribute = %v, want Demo.Item.Code", got)
	}
	if got := fields["Expression"]; got != "$IteratorItem/ExternalCode" {
		t.Fatalf("Expression = %v, want $IteratorItem/ExternalCode", got)
	}
	if got := fields["ListName"]; got != "Items" {
		t.Fatalf("ListName = %v, want Items", got)
	}
}

func TestSerializeListOperation_FilterByAssociation(t *testing.T) {
	doc := serializeListOperation(&microflows.FilterByAttributeOperation{
		BaseElement:  model.BaseElement{ID: "operation-id"},
		ListVariable: "Items",
		Association:  "Demo.Item_Category",
		Expression:   "$Category",
	})
	fields := listOperationDocMap(doc)

	if got := fields["$Type"]; got != "Microflows$Filter" {
		t.Fatalf("$Type = %v, want Microflows$Filter", got)
	}
	if got := fields["Association"]; got != "Demo.Item_Category" {
		t.Fatalf("Association = %v, want Demo.Item_Category", got)
	}
	if got := fields["Expression"]; got != "$Category" {
		t.Fatalf("Expression = %v, want $Category", got)
	}
}

// upstream #966, the legacy engine's half.
//
// serializeListOperation had no ListRangeOperation case at all, so it fell
// through to `return nil` — and serializeListOperationAction appends that nil
// under "NewOperation" without checking, which lands in the file as an empty
// sub-document. The result is not a range that lost its bounds; it is a project
// Mendix cannot OPEN. Measured on mxbuild 11.13.0:
//
//	ERROR: System.AggregateException: … (Expected '$ID' as the first property
//	of a storage object, but got 'NewOperation'.)
//	  at StreamingBsonUnitReader.ConstructObject(…)
//
// The control was the same script with `filter` in place of `range`: that one
// wrote a well-formed NewOperation and loaded fine, so the Range case is what
// produced the empty document.
//
// The parser has read the nested CustomRange since it was written (see
// TestParseListOperation_Range), so the writer is the only side that was
// missing — which is why `--engine legacy` was never a workaround for #966.
func TestSerializeListOperation_Range(t *testing.T) {
	doc := serializeListOperation(&microflows.ListRangeOperation{
		BaseElement:      model.BaseElement{ID: "operation-id"},
		ListVariable:     "Items",
		OffsetExpression: "$Skip",
		LimitExpression:  "$Take",
	})
	if doc == nil {
		t.Fatal("serializeListOperation returned nil — the action is written with an empty NewOperation and Mendix cannot load the project")
	}
	fields := listOperationDocMap(doc)

	if got := fields["$Type"]; got != "Microflows$ListRange" {
		t.Fatalf("$Type = %v, want Microflows$ListRange", got)
	}
	if got := fields["ListName"]; got != "Items" {
		t.Errorf("ListName = %v, want Items", got)
	}
	// The bounds live one level down, in a Microflows$CustomRange child — the
	// shape the parser beside this file already expects. Flat keys build a
	// model mxbuild rejects with CE6520.
	cr, ok := fields["CustomRange"].(bson.D)
	if !ok {
		t.Fatalf("CustomRange = %#v, want a bson.D child document", fields["CustomRange"])
	}
	crFields := listOperationDocMap(cr)
	if got := crFields["$Type"]; got != "Microflows$CustomRange" {
		t.Errorf("CustomRange $Type = %v, want Microflows$CustomRange", got)
	}
	if got := crFields["OffsetExpression"]; got != "$Skip" {
		t.Errorf("CustomRange.OffsetExpression = %v, want $Skip", got)
	}
	if got := crFields["LimitExpression"]; got != "$Take" {
		t.Errorf("CustomRange.LimitExpression = %v, want $Take", got)
	}
}

// The write→read pairing within the legacy engine. The parser was already
// right, so this asserts the writer now speaks the same shape the parser reads
// — the property that was missing when `range` was the one list operation
// legacy could parse but not write.
func TestSerializeListOperation_RangeRoundTrips(t *testing.T) {
	doc := serializeListOperation(&microflows.ListRangeOperation{
		BaseElement:      model.BaseElement{ID: "operation-id"},
		ListVariable:     "Items",
		OffsetExpression: "$Skip",
		LimitExpression:  "$Take",
	})

	// Re-present the document the way the parser receives it.
	raw := map[string]any{}
	for _, e := range doc {
		if child, ok := e.Value.(bson.D); ok {
			m := map[string]any{}
			for _, ce := range child {
				m[ce.Key] = ce.Value
			}
			raw[e.Key] = m
			continue
		}
		raw[e.Key] = e.Value
	}

	op, ok := parseListOperation(raw).(*microflows.ListRangeOperation)
	if !ok {
		t.Fatalf("parseListOperation → %T, want *microflows.ListRangeOperation", parseListOperation(raw))
	}
	if op.OffsetExpression != "$Skip" || op.LimitExpression != "$Take" {
		t.Errorf("round trip: offset=%q limit=%q, want $Skip/$Take", op.OffsetExpression, op.LimitExpression)
	}
}

// An operation the writer has no case for must not reach the file as an empty
// NewOperation: that is the unloadable-project shape above, and it is worse
// than the honest alternative (no action → mxbuild's CE0008 "No action
// defined", which names the activity). unknownListOperation stands in for a
// model type a future metamodel adds before this writer learns it.
type unknownListOperation struct{ microflows.HeadOperation }

func TestSerializeListOperationAction_OmitsAnUnserializableOperation(t *testing.T) {
	doc := serializeListOperationAction(&microflows.ListOperationAction{
		BaseElement:    model.BaseElement{ID: "action-id"},
		Operation:      &unknownListOperation{},
		OutputVariable: "Out",
	})
	for _, e := range doc {
		if e.Key == "NewOperation" {
			t.Fatalf("NewOperation = %#v; an operation the writer cannot serialize must be omitted, "+
				"not written as an empty document (Mendix: \"Expected '$ID' as the first property of a storage object\")", e.Value)
		}
	}
}

func listOperationDocMap(doc bson.D) map[string]any {
	fields := make(map[string]any, len(doc))
	for _, elem := range doc {
		fields[elem.Key] = elem.Value
	}
	return fields
}
