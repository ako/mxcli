// SPDX-License-Identifier: Apache-2.0

// DESCRIBE has to find a pluggable widget's click action under the key the
// WIDGET stores it at, not under the one MDL calls it.
//
// Mendix's own widgets suffix their action slots — a BadgeButton's click slot is
// `onClickEvent`, a HeatMap's is `onClickAction` — and the WRITER already knows
// this: actionSourceForKey strips one Event/Action suffix before matching, which
// is how `onClick:` reaches those widgets at all (ledger #14). The READER looked
// up the literal string "onClick", so it found the action only on the widgets
// whose key happens to be spelled exactly that.
//
// The result was a one-way write. Measured on a real 11.13.0 project: authoring
// `onClick: microflow …` on a BadgeButton puts `onClickEvent` and a
// `Forms$MicroflowAction` in the page unit and builds at 0 errors, and
// `describe page` then renders `badgebutton bb` with no properties at all — so a
// describe → exec round-trip drops the wiring. (upstream #956)
package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// buildCustomWidgetClickWidget builds a pluggable-widget map whose click action
// is stored under an arbitrary property key.
func buildCustomWidgetClickWidget(widgetID, propertyKey, actionType, microflow string) map[string]any {
	const typeID = "type-id-click"
	action := map[string]any{"$Type": actionType}
	if microflow != "" {
		action["MicroflowSettings"] = map[string]any{
			"$Type":     "Forms$MicroflowSettings",
			"Microflow": microflow,
		}
	}
	return map[string]any{
		"Type": map[string]any{
			"WidgetId": widgetID,
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					map[string]any{"$ID": typeID, "PropertyKey": propertyKey},
				},
			},
		},
		"Object": map[string]any{
			"Properties": []any{
				map[string]any{"TypePointer": typeID, "Value": map[string]any{"Action": action}},
			},
		},
	}
}

// Every spelling the writer accepts must read back, or `onClick:` is a one-way
// write on that widget. The table is the writer's own rule restated: one
// Event/Action suffix stripped, then matched.
func TestCustomWidgetActionForSource_ReadsEverySpellingTheWriterAccepts(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())

	cases := []struct {
		name        string
		widgetID    string
		propertyKey string
	}{
		{"plain onClick (DataGrid2)", "com.mendix.widget.web.datagrid.Datagrid", "onClick"},
		{"onClickEvent (BadgeButton)", "com.mendix.widget.custom.badgebutton.BadgeButton", "onClickEvent"},
		{"onClickAction (HeatMap)", "com.mendix.widget.custom.heatmap.HeatMap", "onClickAction"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := buildCustomWidgetClickWidget(tc.widgetID, tc.propertyKey, "Forms$MicroflowClientAction", "M.OnClick")
			got := customWidgetActionForSource(ctx, w, "OnClick")
			if got == nil {
				t.Fatalf("no action read back from %q — mxcli writes this key and DESCRIBE "+
					"cannot see it, so the wiring is lost on a describe→exec round-trip", tc.propertyKey)
			}
			if ty := extractString(got["$Type"]); ty != "Forms$MicroflowClientAction" {
				t.Errorf("action $Type = %q, want Forms$MicroflowClientAction", ty)
			}
		})
	}
}

// The default empty action still reads as unset, on every spelling — otherwise
// fixing the lookup fills every description with `onClick: ` noise.
func TestCustomWidgetActionForSource_NoActionReadsAsUnset(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())

	for _, key := range []string{"onClick", "onClickEvent", "onClickAction"} {
		w := buildCustomWidgetClickWidget("com.example.W", key, "Forms$NoAction", "")
		if got := customWidgetActionForSource(ctx, w, "OnClick"); got != nil {
			t.Errorf("%s: NoAction read as set (%v), want unset", key, got)
		}
	}
}

// A slot that is not the click slot must not answer to OnClick. Without this the
// lookup could "fix" the bug by returning whatever action it found first, and a
// widget with several slots would describe the wrong one — which is worse than
// dropping it, because the output looks right.
func TestCustomWidgetActionForSource_DoesNotMatchAnUnrelatedSlot(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())

	for _, key := range []string{"onSelectionChange", "createFileAction", "onUploadSuccessFile", "onChangeEvent"} {
		w := buildCustomWidgetClickWidget("com.example.W", key, "Forms$MicroflowClientAction", "M.Other")
		if got := customWidgetActionForSource(ctx, w, "OnClick"); got != nil {
			t.Errorf("%s answered to OnClick (%v) — DESCRIBE would emit another slot's "+
				"action as the click action", key, got)
		}
	}
}

// The generic pluggable path had no OnChange read at all — only OnClick — so a
// Slider/RangeSlider/StarRating stored its action and described back without it.
// Those three widgets have exactly one action slot, so the round-trip did not
// lose a detail, it lost the wiring.
//
// Asserted against BOTH sources on the same helper: a fix that widens the click
// lookup while leaving the change slot unread passes a click-only test.
func TestCustomWidgetActionForSource_ReadsTheChangeSlotToo(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())

	cases := []struct {
		propertyKey string
		source      string
	}{
		{"onChange", "OnChange"},      // Slider, RangeSlider, StarRating
		{"onChangeEvent", "OnChange"}, // Combobox
		{"onClick", "OnClick"},        // DataGrid2
	}

	for _, tc := range cases {
		t.Run(tc.propertyKey, func(t *testing.T) {
			w := buildCustomWidgetClickWidget("com.example.W", tc.propertyKey, "Forms$MicroflowClientAction", "M.Handler")
			if got := customWidgetActionForSource(ctx, w, tc.source); got == nil {
				t.Fatalf("%s not read back for source %s — mxcli writes this slot and "+
					"DESCRIBE cannot see it", tc.propertyKey, tc.source)
			}
		})
	}

	// The two sources must not answer for each other, or a widget carrying both
	// describes the same action twice.
	w := buildCustomWidgetClickWidget("com.example.W", "onChange", "Forms$MicroflowClientAction", "M.Handler")
	if got := customWidgetActionForSource(ctx, w, "OnClick"); got != nil {
		t.Errorf("the change slot answered to OnClick (%v)", got)
	}
}

// The generic pluggable branch emits the widget at all only when it has
// something to say. OnChange was not among the things that counted, so a Slider
// whose ONLY property is its action described back as a bare
// `pluggablewidget '…' sl` — the action read correctly and then had nowhere to go.
func TestDescribeEmitsGenericPluggableOnChange(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}

	outputWidgetMDLV3(ctx, rawWidget{
		Type:       "CustomWidgets$CustomWidget",
		RenderMode: "slider",
		Name:       "sl",
		WidgetID:   "com.mendix.widget.custom.slider.Slider",
		OnChange:   "microflow M.OnChanged",
	}, 0)

	out := buf.String()
	if !strings.Contains(out, "OnChange: microflow M.OnChanged") {
		t.Errorf("no OnChange in the description — the slot round-trips to nothing:\n%s", out)
	}
}
