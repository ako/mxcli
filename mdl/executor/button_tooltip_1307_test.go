// SPDX-License-Identifier: Apache-2.0

// An action button's tooltip (mendixlabs/mxcli#1307).
//
// Reported: "A `tooltip:` on an `actionbutton` in `create page` passes `check`
// and `exec` with no warning, but the tooltip is never stored", and `describe`
// does not show it. The grammar parsed it, the validator listed `Tooltip` as a
// known property (so MDL-WIDGET07 stayed quiet), and the codec wrote whatever
// `pages.ActionButton.Tooltip` held — but buildButtonV3 never filled it, so
// the codec always wrote an empty Texts$Text.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// buildButtonFromMDL runs the real parser and page builder over one button.
func buildButtonFromMDL(t *testing.T, button string) *pages.ActionButton {
	t.Helper()
	src := "create page Mod.P (Title: 'T') {\n  " + button + "\n}"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", button, errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	widget, err := pb.buildWidgetV3(page.Widgets[0])
	if err != nil {
		t.Fatalf("building %q: %v", button, err)
	}
	return widget.(*pages.ActionButton)
}

func TestButtonTooltip_IsStored(t *testing.T) {
	for _, kw := range []string{"actionbutton", "linkbutton"} {
		t.Run(kw, func(t *testing.T) {
			btn := buildButtonFromMDL(t, kw+" btnEdit (Caption: 'Edit', Tooltip: 'Edit this order')")
			if btn.Tooltip == nil || len(btn.Tooltip.Translations) == 0 {
				t.Fatalf("Tooltip not stored: %+v", btn.Tooltip)
			}
			if got := btn.Tooltip.Translations[authoringLanguage(nil)]; got != "Edit this order" {
				t.Errorf("Tooltip translations = %v, want the default language to hold %q",
					btn.Tooltip.Translations, "Edit this order")
			}
		})
	}
}

// Control: a button with no tooltip stores none, so the codec keeps writing
// the empty Texts$Text Studio Pro writes.
func TestButtonTooltip_AbsentStaysNil(t *testing.T) {
	if btn := buildButtonFromMDL(t, "actionbutton b (Caption: 'Go')"); btn.Tooltip != nil {
		t.Errorf("Tooltip = %+v for a button that declared none", btn.Tooltip)
	}
}

// DESCRIBE must print the stored tooltip, and its output must rebuild it.
func TestDescribeButtonTooltip_RoundTrips(t *testing.T) {
	stored := map[string]any{
		"$Type": "Forms$ActionButton",
		"Name":  "btnEdit",
		"Tooltip": map[string]any{
			"$Type": "Texts$Text",
			"Items": []any{int32(3), map[string]any{
				"$Type":        "Texts$Translation",
				"LanguageCode": "en_US",
				"Text":         "Edit this order",
			}},
		},
	}
	out := describeButton(t, stored)
	if !strings.Contains(out, "Tooltip: 'Edit this order'") {
		t.Fatalf("describe did not emit the tooltip:\n%s", out)
	}

	// Control: an empty Texts$Text (what every tooltip-less button stores) is silent.
	empty := map[string]any{
		"$Type":   "Forms$ActionButton",
		"Name":    "btnEdit",
		"Tooltip": map[string]any{"$Type": "Texts$Text", "Items": []any{int32(3)}},
	}
	if out := describeButton(t, empty); strings.Contains(out, "Tooltip") {
		t.Errorf("an empty tooltip produced output:\n%s", out)
	}

	btn := buildButtonFromMDL(t, "actionbutton btnEdit (Caption: 'Edit', Tooltip: 'Edit this order')")
	if btn.Tooltip == nil || btn.Tooltip.Translations[authoringLanguage(nil)] != "Edit this order" {
		t.Errorf("replaying DESCRIBE's tooltip did not rebuild it: %+v", btn.Tooltip)
	}
}
