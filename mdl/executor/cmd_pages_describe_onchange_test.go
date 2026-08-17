// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// TestParseRawWidget_OnChangeOnEveryInputWidget covers the read half of ledger
// #14. DESCRIBE PAGE is how the dropped OnChange was found, and it could only
// find it on `textbox` — every other input widget parsed its Label and Attribute
// and ignored OnChangeAction, so a page that *did* carry the action still
// described as if it did not. That makes DESCRIBE unable to confirm its own
// round trip for these five widgets.
func TestParseRawWidget_OnChangeOnEveryInputWidget(t *testing.T) {
	// `dropdown` is absent: DESCRIBE has no Forms$DropDown case at all, which is a
	// separate gap from this one and is left alone here.
	types := map[string]string{
		"textbox":      "Forms$TextBox",
		"textarea":     "Forms$TextArea",
		"datepicker":   "Forms$DatePicker",
		"checkbox":     "Forms$CheckBox",
		"radiobuttons": "Forms$RadioButtonGroup",
	}

	for mdlName, bsonType := range types {
		t.Run(mdlName, func(t *testing.T) {
			ctx, _ := newMockCtx(t)

			raw := map[string]any{
				"$Type": bsonType,
				"Name":  "w1",
				"OnChangeAction": map[string]any{
					"$Type": "Forms$MicroflowAction",
					"MicroflowSettings": map[string]any{
						"$Type":     "Forms$MicroflowSettings",
						"Microflow": "MyFirstModule.ACT_Apply",
					},
				},
			}

			got := parseRawWidget(ctx, raw)
			if len(got) != 1 {
				t.Fatalf("expected 1 widget, got %d", len(got))
			}
			if got[0].OnChange == "" {
				t.Fatalf("%s: OnChangeAction was not read back — DESCRIBE cannot see it", bsonType)
			}
			if !strings.Contains(got[0].OnChange, "ACT_Apply") {
				t.Errorf("%s: OnChange = %q, want it to name the microflow", bsonType, got[0].OnChange)
			}
		})
	}
}

// TestParseRawWidget_ComboBoxOnChangeEvent is the pluggable half, and it is
// about data loss rather than visibility.
//
// The ComboBox stores its action under the widget property `onChangeEvent`, so
// the built-in OnChangeAction path above does not see it. The write path was
// correct — the action reached BSON as a Forms$MicroflowAction — but DESCRIBE
// emitted `combobox cbo (Label: 'X')` with no OnChange, so a
// describe → edit → exec cycle *deleted* it. Measured on a real 11.13 project:
// 1 Forms$MicroflowAction before the round trip, 0 after, while the datepicker
// beside it survived.
func TestParseRawWidget_ComboBoxOnChangeEvent(t *testing.T) {
	ctx, _ := newMockCtx(t)

	// Shape mirrors a stored ComboBox: the property key lives in
	// Type.PropertyTypes and the value points back at it by TypePointer, so the
	// action is only reachable through that indirection — which is why the
	// built-in OnChangeAction reader never saw it.
	raw := map[string]any{
		"$Type": "CustomWidgets$CustomWidget",
		"Name":  "cboKind",
		"Type": map[string]any{
			"$Type":    "CustomWidgets$WidgetType",
			"WidgetId": "com.mendix.widget.web.combobox.Combobox",
			"PropertyTypes": []any{int32(3), map[string]any{
				"$ID":         "pt-onchange",
				"$Type":       "CustomWidgets$WidgetPropertyType",
				"PropertyKey": "onChangeEvent",
			}},
		},
		"Object": map[string]any{
			"$Type": "CustomWidgets$WidgetObject",
			"Properties": []any{int32(3), map[string]any{
				"$Type":       "CustomWidgets$WidgetProperty",
				"TypePointer": "pt-onchange",
				"Value": map[string]any{
					"$Type": "CustomWidgets$WidgetValue",
					"Action": map[string]any{
						"$Type": "Forms$MicroflowAction",
						"MicroflowSettings": map[string]any{
							"$Type":     "Forms$MicroflowSettings",
							"Microflow": "MyFirstModule.ACT_Apply",
						},
					},
				},
			}},
		},
	}

	got := parseRawWidget(ctx, raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(got))
	}
	if got[0].OnChange == "" {
		t.Fatal("ComboBox onChangeEvent was not read back — a describe→exec round trip deletes it")
	}
	if !strings.Contains(got[0].OnChange, "ACT_Apply") {
		t.Errorf("OnChange = %q, want it to name the microflow", got[0].OnChange)
	}
}
