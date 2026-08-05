// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// The whole point of converting a stored widget to map form is to run
// widgets.AugmentTemplate's reconciliation passes over it and convert back. That is
// only safe if a conversion with NO reconciliation in between is byte-identical:
// otherwise every synced widget picks up spurious differences, and on a structure
// where key order is a documented CE0463 cause those differences are not cosmetic.
//
// map[string]any is unordered, so the round trip re-derives key order by sorting.
// This test is what justifies that assumption.
func TestWidgetRoundTripIsByteStable(t *testing.T) {
	id := func(b byte) primitive.Binary {
		return primitive.Binary{Subtype: 0x00, Data: bytes.Repeat([]byte{b}, 16)}
	}

	// A widget shaped like the real thing: ordered alphabetically, IDs as binary,
	// a paired PropertyType/WidgetProperty bound by TypePointer, array markers, and
	// a nested ObjectType.
	widget := bson.D{
		{Key: "$ID", Value: id(0x01)},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "dgTest"},
		{Key: "Object", Value: bson.D{
			{Key: "$ID", Value: id(0x02)},
			{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
			{Key: "Properties", Value: bson.A{
				float64(2),
				bson.D{
					{Key: "$ID", Value: id(0x03)},
					{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
					{Key: "TypePointer", Value: id(0x05)},
					{Key: "Value", Value: bson.D{
						{Key: "$ID", Value: id(0x04)},
						{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
						{Key: "PrimitiveValue", Value: "true"},
					}},
				},
			}},
		}},
		{Key: "Type", Value: bson.D{
			{Key: "$ID", Value: id(0x06)},
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "ObjectType", Value: bson.D{
				{Key: "$ID", Value: id(0x07)},
				{Key: "$Type", Value: "CustomWidgets$WidgetObjectType"},
				{Key: "PropertyTypes", Value: bson.A{
					float64(2),
					bson.D{
						{Key: "$ID", Value: id(0x05)},
						{Key: "$Type", Value: "CustomWidgets$WidgetPropertyType"},
						{Key: "Caption", Value: "Advanced"},
						{Key: "Category", Value: "Behavior::Selection"},
						{Key: "Description", Value: ""},
						{Key: "IsDefault", Value: false},
						{Key: "PropertyKey", Value: "advanced"},
						{Key: "ValueType", Value: bson.D{
							{Key: "$ID", Value: id(0x08)},
							{Key: "$Type", Value: "CustomWidgets$WidgetValueType"},
							{Key: "AllowUpload", Value: false},
							{Key: "EnumerationValues", Value: bson.A{float64(2)}},
							{Key: "Required", Value: true},
							{Key: "Translations", Value: bson.A{float64(2)}},
							{Key: "Type", Value: "Boolean"},
						}},
					},
				}},
			}},
			{Key: "WidgetId", Value: "com.mendix.widget.web.datagrid.Datagrid"},
		}},
	}

	original, err := bson.Marshal(widget)
	if err != nil {
		t.Fatalf("marshal original: %v", err)
	}

	asMap := widgetToMap(widget)
	if _, ok := asMap.(map[string]any); !ok {
		t.Fatalf("widgetToMap returned %T, want map[string]any", asMap)
	}

	back, ok := mapToWidgetDoc(asMap).(bson.D)
	if !ok {
		t.Fatal("mapToWidgetDoc did not return a bson.D")
	}
	encoded, err := bson.Marshal(back)
	if err != nil {
		t.Fatalf("marshal round-tripped: %v", err)
	}

	if !bytes.Equal(original, encoded) {
		t.Errorf("round trip changed the document\n original %d bytes\n  encoded %d bytes", len(original), len(encoded))
		var a, b bson.D
		_ = bson.Unmarshal(original, &a)
		_ = bson.Unmarshal(encoded, &b)
		t.Errorf("original: %v", a)
		t.Errorf("encoded : %v", b)
	}
}

// A TypePointer must survive as binary and still equal the PropertyType's $ID —
// breaking that pairing yields a project Mendix cannot load at all.
func TestWidgetRoundTripPreservesTypePointerBinding(t *testing.T) {
	ptID := primitive.Binary{Subtype: 0x00, Data: bytes.Repeat([]byte{0xAB}, 16)}

	doc := bson.D{
		{Key: "PropertyTypes", Value: bson.A{bson.D{
			{Key: "$ID", Value: ptID},
			{Key: "PropertyKey", Value: "advanced"},
		}}},
		{Key: "Properties", Value: bson.A{bson.D{
			{Key: "TypePointer", Value: ptID},
		}}},
	}

	back, ok := mapToWidgetDoc(widgetToMap(doc)).(bson.D)
	if !ok {
		t.Fatal("round trip did not return a bson.D")
	}

	pts, _ := arrField(back, "PropertyTypes")
	props, _ := arrField(back, "Properties")
	if len(pts) != 1 || len(props) != 1 {
		t.Fatalf("arrays lost: %d PropertyTypes, %d Properties", len(pts), len(props))
	}
	gotID, ok := idOf(pts[0].(bson.D))
	if !ok {
		t.Fatal("$ID did not survive as an ID")
	}
	gotPtr, ok := idField(props[0].(bson.D), "TypePointer")
	if !ok {
		t.Fatal("TypePointer did not survive as an ID")
	}
	if gotID != gotPtr {
		t.Errorf("pairing broken: $ID %s != TypePointer %s", gotID, gotPtr)
	}
}

// AugmentTemplate adds a property to an object-list property (DataGrid2 columns) by
// giving every list entry a copy of the same constructed node, so a placeholder that
// appears once per entry must become a DIFFERENT id per entry. Remapping by value gave
// them all one id, which Mendix accepts on load and on `mx check` and then rejects at
// SAVE time with "Duplicate Guid in unit page template" — after `mx update-widgets` has
// already collapsed mprcontents/, leaving the project flattened and unloadable.
// Reported against PR #89; 18 units and 432 excess occurrences on the reference fixture.
func TestEnsureUniqueWidgetIDsSeparatesListEntries(t *testing.T) {
	// Three list entries that each carry a copy of the same node, all pointing at the
	// one shared PropertyType — which is correct and must NOT be rewritten.
	const sharedPT = "aaaaaaaaaaaa4aaaaaaaaaaaaaaaaaaa"
	entry := func() map[string]any {
		return map[string]any{
			"$ID":         "bbbbbbbbbbbb4bbbbbbbbbbbbbbbbbbb",
			"$Type":       "CustomWidgets$WidgetProperty",
			"TypePointer": sharedPT,
			"Value": map[string]any{
				"$ID":   "cccccccccccc4ccccccccccccccccccc",
				"$Type": "CustomWidgets$WidgetValue",
			},
		}
	}
	obj := map[string]any{
		"$ID":   "dddddddddddd4ddddddddddddddddddd",
		"$Type": "CustomWidgets$WidgetObject",
		"Objects": []any{
			float64(2),
			map[string]any{"$ID": "e1", "Properties": []any{float64(2), entry()}},
			map[string]any{"$ID": "e2", "Properties": []any{float64(2), entry()}},
			map[string]any{"$ID": "e3", "Properties": []any{float64(2), entry()}},
		},
	}

	out := ensureUniqueWidgetIDs(obj, map[string]bool{})

	ids := map[string]int{}
	pointers := map[string]int{}
	var walk func(any)
	walk = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			if id, ok := n["$ID"].(string); ok {
				ids[id]++
			}
			if p, ok := n["TypePointer"].(string); ok {
				pointers[p]++
			}
			for _, val := range n {
				walk(val)
			}
		case []any:
			for _, item := range n {
				walk(item)
			}
		}
	}
	walk(out)

	for id, n := range ids {
		if n > 1 {
			t.Errorf("$ID %q appears %d times — Mendix refuses to save a duplicate GUID", id, n)
		}
	}
	// 1 object + 3 entries + 3 properties + 3 values = 10 distinct ids.
	if len(ids) != 10 {
		t.Errorf("got %d distinct $IDs, want 10", len(ids))
	}
	// The shared schema pointer is legitimately repeated and must survive untouched.
	if pointers[sharedPT] != 3 {
		t.Errorf("TypePointer to the shared PropertyType survived %d of 3 times (%v) — "+
			"rewriting it would unbind each column from its schema", pointers[sharedPT], pointers)
	}
}

// The test above exercises ensureUniqueWidgetIDs directly, so it passes even if the
// call is removed from applyToWidget — it proves the function works, not that it is
// wired in. widgetIDsAreUnique is the backstop that catches the wiring: it runs on the
// encoded unit immediately before the write, and apply refuses rather than persisting
// a document Mendix would accept now and reject at save time. Verified by deleting the
// ensureUniqueWidgetIDs call and re-running the fixture, which then aborts with
// "refusing to write" instead of corrupting the project.
func TestWidgetIDsAreUniqueDetectsDuplicates(t *testing.T) {
	dup := primitive.Binary{Subtype: 0x00, Data: bytes.Repeat([]byte{0x7C}, 16)}
	other := primitive.Binary{Subtype: 0x00, Data: bytes.Repeat([]byte{0x2E}, 16)}

	clean := bson.D{
		{Key: "$ID", Value: dup},
		{Key: "Widgets", Value: bson.A{bson.D{{Key: "$ID", Value: other}}}},
	}
	if _, ok := widgetIDsAreUnique(clean); !ok {
		t.Error("reported a duplicate in a document that has none")
	}

	dirty := bson.D{
		{Key: "$ID", Value: dup},
		{Key: "Widgets", Value: bson.A{
			bson.D{{Key: "$ID", Value: other}},
			bson.D{{Key: "$ID", Value: dup}}, // same id as the root
		}},
	}
	got, ok := widgetIDsAreUnique(dirty)
	if ok {
		t.Fatal("missed a duplicate GUID — this is the check that stops the project being corrupted")
	}
	if got == "" {
		t.Error("did not name the offending id")
	}
}
