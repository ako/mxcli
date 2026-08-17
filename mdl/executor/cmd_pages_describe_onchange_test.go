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
