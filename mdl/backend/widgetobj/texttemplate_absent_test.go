// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// SetTextTemplate must work on a property whose stored TextTemplate is nil.
//
// A CONDITIONAL template is stored nil while its condition hides it (#574), so
// the slot a script is about to write into is routinely absent — the widget's
// own defaults put it there. setTextTemplateValue only ever UPDATED an existing
// template and returned the nil element untouched otherwise, so the authored
// text was dropped without a word.
//
// Measured on Slider 2.1.4 / mxbuild 11.13.0: `tooltipType: 'customText',
// tooltip: 'Score is {1}'` stored `tooltipType` correctly and an EMPTY template
// (filled afterwards by ApplyVisibilityRules, which sees the property become
// visible), and the project then failed to build — "'Tooltip' cannot be empty
// when 'Custom' type is chosen". Studio Pro reports the same error and reads the
// template as well-formed and empty, so this was never a serialization problem
// (issue #254).
//
// `SetTextTemplateWithParams` was unaffected because it REPLACES the whole
// template rather than editing it, which is why `tooltip: 'Value {Score}'`
// worked while `tooltip: 'Plain text'` did not — a difference no user could
// have guessed.
func TestSetTextTemplate_OnANilTemplateSlot(t *testing.T) {
	const (
		typeID    = "11111111-1111-1111-1111-111111111111" // enum "tooltipType"
		tooltipID = "22222222-2222-2222-2222-222222222222" // TextTemplate "tooltip", stored nil
	)
	mkProp := func(id, primitiveVal string, tt any) bson.D {
		return bson.D{
			{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
			{Key: "TypePointer", Value: types.UUIDToBlob(id)},
			{Key: "Value", Value: bson.D{
				{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
				{Key: "PrimitiveValue", Value: primitiveVal},
				{Key: "TextTemplate", Value: tt},
			}},
		}
	}
	ob := &Builder{
		widgetID: "com.mendix.widget.custom.slider.Slider",
		object: bson.D{{Key: "Properties", Value: bson.A{
			int32(2),
			mkProp(typeID, "customText", nil),
			mkProp(tooltipID, "", nil), // hidden by the widget's defaults -> nil
		}}},
		propertyTypeIDs: map[string]pages.PropertyTypeIDEntry{
			"tooltipType": {PropertyTypeID: typeID, ValueType: "Enumeration"},
			"tooltip":     {PropertyTypeID: tooltipID, ValueType: "TextTemplate"},
		},
	}

	ob.SetTextTemplate("tooltip", "Score is high")

	tmpl, ok := templateOf(t, ob.object, tooltipID).(bson.D)
	if !ok {
		t.Fatalf("TextTemplate is still %v — the authored text was dropped, and the "+
			"widget then fails to build because its type says Custom", templateOf(t, ob.object, tooltipID))
	}
	if got := templateText(t, tmpl); got != "Score is high" {
		t.Errorf("template text = %q, want %q", got, "Score is high")
	}
	if typ := findField(t, tmpl, "$Type"); typ != "Forms$ClientTemplate" {
		t.Errorf("$Type = %v, want Forms$ClientTemplate", typ)
	}
}

// An existing template is still edited in place, keeping its identity and its
// Parameters — the path every populated caption already takes.
func TestSetTextTemplate_KeepsAnExistingTemplatesEnvelope(t *testing.T) {
	const tooltipID = "22222222-2222-2222-2222-222222222222"
	existing := BuildEmptyClientTemplate()
	ob := &Builder{
		widgetID: "com.example.W",
		object: bson.D{{Key: "Properties", Value: bson.A{
			int32(2),
			bson.D{
				{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
				{Key: "TypePointer", Value: types.UUIDToBlob(tooltipID)},
				{Key: "Value", Value: bson.D{
					{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
					{Key: "TextTemplate", Value: existing},
				}},
			},
		}}},
		propertyTypeIDs: map[string]pages.PropertyTypeIDEntry{
			"tooltip": {PropertyTypeID: tooltipID, ValueType: "TextTemplate"},
		},
	}

	ob.SetTextTemplate("tooltip", "hello")

	tmpl := templateOf(t, ob.object, tooltipID).(bson.D)
	if got := templateText(t, tmpl); got != "hello" {
		t.Errorf("template text = %q, want %q", got, "hello")
	}
	if findField(t, tmpl, "$ID") == nil {
		t.Error("the existing template lost its $ID — an in-place edit must keep the element's identity")
	}
}

func templateText(t *testing.T, tmpl bson.D) string {
	t.Helper()
	template, ok := findField(t, tmpl, "Template").(bson.D)
	if !ok {
		t.Fatal("template has no Template")
	}
	items, ok := findField(t, template, "Items").(bson.A)
	if !ok {
		t.Fatal("Template has no Items")
	}
	for _, it := range items {
		if tr, ok := it.(bson.D); ok {
			if txt, ok := findField(t, tr, "Text").(string); ok {
				return txt
			}
		}
	}
	return ""
}
