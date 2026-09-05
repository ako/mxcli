// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"
)

// fullWidgetValue builds a widget-value sub-document the way the encoder
// actually writes one: EVERY field present, almost all of them empty.
//
// That shape is the whole point. extractObjectListItem tests the fields in
// order and used to `continue` as soon as a KEY EXISTED, so the first branch
// (DataSource) consumed every property and the ones below it never ran. The
// item came back with no props and the caller dropped it, taking the entire
// object list out of the DESCRIBE output.
func fullWidgetValue(typePointer string, set map[string]any) map[string]any {
	v := map[string]any{
		"$ID":               "v-" + typePointer,
		"$Type":             "CustomWidgets$WidgetValue",
		"Action":            map[string]any{"$Type": "Forms$NoAction"},
		"AttributeRef":      map[string]any{},
		"DataSource":        map[string]any{},
		"EntityRef":         map[string]any{},
		"Expression":        "",
		"PrimitiveValue":    "",
		"TextTemplate":      map[string]any{},
		"TranslatableValue": map[string]any{},
		"Widgets":           []any{int32(3)},
		"Objects":           []any{int32(3)},
	}
	for k, val := range set {
		v[k] = val
	}
	return map[string]any{"TypePointer": typePointer, "Value": v}
}

// An item whose sub-properties carry real values must produce them, even though
// every other field of each value is present-but-empty.
func TestExtractObjectListItem_EmptyFieldsDoNotSwallowTheProperty(t *testing.T) {
	nested := map[string]string{
		"p1": "attributeName",
		"p2": "attributeValueType",
	}
	item := map[string]any{
		"Properties": []any{
			int32(3),
			fullWidgetValue("p1", map[string]any{"PrimitiveValue": "data-x"}),
			fullWidgetValue("p2", map[string]any{"PrimitiveValue": "expression"}),
		},
	}

	got := extractObjectListItem(&ExecContext{}, item, nested)
	if len(got.Props) != 2 {
		t.Fatalf("got %d props, want 2 — an empty DataSource/Action/AttributeRef must not "+
			"consume the property before PrimitiveValue is reached:\n%+v", len(got.Props), got.Props)
	}
	byKey := map[string]string{}
	for _, p := range got.Props {
		byKey[p.Key] = p.Value
	}
	if byKey["attributeName"] != "data-x" {
		t.Errorf("AttributeName = %q, want %q (props: %+v)", byKey["attributeName"], "data-x", got.Props)
	}
	if byKey["attributeValueType"] != "expression" {
		t.Errorf("AttributeValueType = %q, want %q", byKey["attributeValueType"], "expression")
	}
}

// The control for the branch ordering: a property that genuinely IS a
// datasource must still be taken by the datasource branch and must NOT fall
// through to PrimitiveValue. Without this, "stop consuming on empty" could be
// implemented by removing the branches altogether and the test above would
// still pass.
func TestExtractObjectListItem_RealDataSourceStillWins(t *testing.T) {
	nested := map[string]string{"p1": "staticDataSource"}
	item := map[string]any{
		"Properties": []any{
			int32(3),
			fullWidgetValue("p1", map[string]any{
				"PrimitiveValue": "SHOULD NOT BE READ",
				"DataSource": map[string]any{
					"$Type":  "Forms$ListenTargetSource",
					"Widget": "someWidget",
				},
			}),
		},
	}

	got := extractObjectListItem(&ExecContext{}, item, nested)
	for _, p := range got.Props {
		if p.Value == "SHOULD NOT BE READ" {
			t.Errorf("a real datasource property fell through to PrimitiveValue: %+v", got.Props)
		}
	}
}

// A child slot is any property whose Value holds widgets — read off the
// document rather than looked up by name, so a widget nobody has thought about
// round-trips too. DESCRIBE previously reconstructed only a Gallery's `content`
// and `filtersPlaceholder`, by asking for those keys BY NAME.
func TestExtractChildSlots(t *testing.T) {
	w := map[string]any{
		"Type": map[string]any{
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					int32(3),
					map[string]any{"$ID": "t1", "PropertyKey": "tagContentContainer"},
					map[string]any{"$ID": "t2", "PropertyKey": "emptySlot"},
				},
			},
		},
		"Object": map[string]any{
			"Properties": []any{
				int32(3),
				map[string]any{
					"TypePointer": "t1",
					"Value": map[string]any{"Widgets": []any{int32(3),
						map[string]any{"$Type": "Forms$DynamicText", "Name": "t"}}},
				},
				// Empty: the marker only. Emitting these would put a `slot { }`
				// block on nearly every pluggable widget.
				map[string]any{
					"TypePointer": "t2",
					"Value":       map[string]any{"Widgets": []any{int32(3)}},
				},
			},
		},
	}

	got := extractChildSlots(&ExecContext{}, w, "")
	if len(got) != 1 {
		t.Fatalf("got %d slots, want 1 (the populated one only): %+v", len(got), got)
	}
	if got[0].Keyword != "tagcontentcontainer" {
		t.Errorf("Keyword = %q, want %q", got[0].Keyword, "tagcontentcontainer")
	}
	if len(got[0].Widgets) == 0 {
		t.Error("slot reconstructed with no widgets — the recursion into parseRawWidget did not run")
	}
}

func TestExtractChildSlots_NoContainers(t *testing.T) {
	if got := extractChildSlots(&ExecContext{}, map[string]any{}, ""); len(got) != 0 {
		t.Errorf("got %+v, want none", got)
	}
}
