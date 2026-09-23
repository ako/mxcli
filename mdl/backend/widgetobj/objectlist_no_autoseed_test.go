// SPDX-License-Identifier: Apache-2.0

// An object list the author never wrote must stay EMPTY.
//
// mxcli used to seed one placeholder row into any object list whose nested
// properties were all "simple" (nothing Attribute/Expression/TextTemplate/
// Widgets/DataSource). Studio Pro leaves such a list empty, so the seeded row
// made the stored instance disagree with the installed .mpk and headless
// mxbuild rejected the page with CE0463 "the definition of this widget has
// changed" — while `mxcli check` and `mx check` both reported zero errors.
//
// Measured against the shipped widget templates, the seeding reached exactly
// two properties and helped neither:
//
//	barcodescanner  barcodeFormats  required, nested {Enumeration}
//	htmlelement     events          optional, nested {Action,Boolean,Enumeration}
//
// Both cases are covered below. This is the inverse of the #891 rule in
// objectlist_required_texttemplate_test.go: an object-list item the author DID
// write still gets its required TextTemplate filled — that path is unchanged.
package widgetobj

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// emptyObjectListWidget builds a widget object holding one object-list property
// with no rows — the shape a template has before the author writes anything.
func emptyObjectListWidget(entry pages.PropertyTypeIDEntry) bson.D {
	return bson.D{
		{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "TypePointer", Value: types.UUIDToBlob("00000000000000000000000000000000")},
		{Key: "Properties", Value: bson.A{
			int32(2),
			bson.D{
				{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
				{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
				{Key: "TypePointer", Value: types.UUIDToBlob(entry.PropertyTypeID)},
				{Key: "Value", Value: bson.D{
					{Key: "$ID", Value: types.UUIDToBlob(types.GenerateID())},
					{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
					{Key: "Objects", Value: bson.A{int32(2)}},
					{Key: "PrimitiveValue", Value: ""},
					{Key: "TypePointer", Value: types.UUIDToBlob(entry.ValueTypeID)},
					{Key: "Widgets", Value: bson.A{int32(2)}},
				}},
			},
		}},
	}
}

// countWidgetObjects reports how many CustomWidgets$WidgetObject nodes sit
// below the root — i.e. how many object-list rows exist.
func countWidgetObjects(v any) int {
	n := 0
	switch node := v.(type) {
	case bson.D:
		for _, e := range node {
			if e.Key == "$Type" && e.Value == "CustomWidgets$WidgetObject" {
				n++
			}
		}
		for _, e := range node {
			n += countWidgetObjects(e.Value)
		}
	case bson.A:
		for _, e := range node {
			n += countWidgetObjects(e)
		}
	}
	return n
}

// barcodeFormatsEntry mirrors Barcode Scanner 2.5.0's `barcodeFormats`: the XML
// omits `required`, and the schema default for an absent attribute is true
// (mpk.go: `Required: p.Required != "false"`), so it arrives here Required.
func barcodeFormatsEntry() pages.PropertyTypeIDEntry {
	return pages.PropertyTypeIDEntry{
		PropertyTypeID: "00000000000000000000000000000011",
		ValueTypeID:    "00000000000000000000000000000012",
		ObjectTypeID:   "00000000000000000000000000000013",
		Required:       true,
		NestedKeyOrder: []string{"barcodeFormat"},
		NestedPropertyIDs: map[string]pages.PropertyTypeIDEntry{
			"barcodeFormat": {
				PropertyTypeID: "00000000000000000000000000000014",
				ValueTypeID:    "00000000000000000000000000000015",
				ValueType:      "Enumeration",
				DefaultValue:   "AZTEC",
			},
		},
	}
}

// htmlElementEventsEntry mirrors HTML element's `events`: optional, and with no
// nested DataSource it fell past the old not-required guard too.
func htmlElementEventsEntry() pages.PropertyTypeIDEntry {
	return pages.PropertyTypeIDEntry{
		PropertyTypeID: "00000000000000000000000000000021",
		ValueTypeID:    "00000000000000000000000000000022",
		ObjectTypeID:   "00000000000000000000000000000023",
		Required:       false,
		NestedKeyOrder: []string{"eventName", "eventStopPropagation", "eventAction"},
		NestedPropertyIDs: map[string]pages.PropertyTypeIDEntry{
			"eventName": {
				PropertyTypeID: "00000000000000000000000000000024",
				ValueTypeID:    "00000000000000000000000000000025",
				ValueType:      "Enumeration",
				DefaultValue:   "onClick",
			},
			"eventStopPropagation": {
				PropertyTypeID: "00000000000000000000000000000026",
				ValueTypeID:    "00000000000000000000000000000027",
				ValueType:      "Boolean",
				DefaultValue:   "true",
			},
			"eventAction": {
				PropertyTypeID: "00000000000000000000000000000028",
				ValueTypeID:    "00000000000000000000000000000029",
				ValueType:      "Action",
			},
		},
	}
}

// Goes through the Builder method the write pipeline actually calls
// (mdl/backend/mutation.go and mdl/executor/widget_engine.go), not a helper, so
// reinstating the seeding fails this test.
func TestEnsureRequiredObjectLists_LeavesUnwrittenListsEmpty(t *testing.T) {
	cases := []struct {
		name        string
		propertyKey string
		entry       pages.PropertyTypeIDEntry
	}{
		{"barcodescanner barcodeFormats (required, nested Enumeration)", "barcodeFormats", barcodeFormatsEntry()},
		{"htmlelement events (optional, nested Action/Boolean/Enumeration)", "events", htmlElementEventsEntry()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := emptyObjectListWidget(tc.entry)
			// The root itself is one WidgetObject; any extra is a seeded row.
			if before := countWidgetObjects(root); before != 1 {
				t.Fatalf("fixture is wrong: expected only the root WidgetObject, got %d", before)
			}

			ob := New(
				"com.example.widget.Test",
				bson.D{},
				root,
				map[string]pages.PropertyTypeIDEntry{tc.propertyKey: tc.entry},
				"00000000000000000000000000000013",
				nil,
			)
			ob.EnsureRequiredObjectLists()

			if got := countWidgetObjects(ob.object); got != 1 {
				t.Errorf("object list %q was auto-populated: %d WidgetObject nodes, want 1 (the root only). "+
					"A row Studio Pro does not write is CE0463 at mxbuild time.", tc.propertyKey, got)
			}
		})
	}
}
