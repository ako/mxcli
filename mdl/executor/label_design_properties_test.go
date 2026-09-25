// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// labelThemeRegistry is renamedThemeJSON (the Atlas Core 4.1.3 "Widget" base
// group, with its renamed Spacing steps) plus Atlas's "Label" group: one
// ColorPicker, "Style". That is the whole set Studio Pro offers a Forms$Label.
func labelThemeRegistry(t *testing.T) *ThemeRegistry {
	t.Helper()
	reg := renamedThemeRegistry(t)
	reg.WidgetProperties["Label"] = []ThemeProperty{{
		Name: "Style", Type: "ColorPicker",
		Options: []ThemeOption{{Name: "Brand Primary"}, {Name: "Brand Secondary"}},
	}}
	return reg
}

// The MDL keyword `label` writes Forms$Label, whose design properties come from
// the theme's "Label" group plus the "Widget" base. With no keyword mapping the
// validator looked up a group literally named "label", found none, and skipped
// the widget — so Feedback's ShareFeedback_Logo label1, which stores the renamed
// 'Spacing bottom': 'Outer none', went unreported while the containers beside
// it got MDL-WIDGET11 (and mxbuild reports CE6087 on all of them).
func TestValidateDesignProperties_LabelIsChecked(t *testing.T) {
	reg := labelThemeRegistry(t)
	vs := allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  container c1 {
    label lblRenamed (Content: 'Attachment', DesignProperties: ['Spacing bottom': 'Outer none'])
    label lblUndefined (Content: 'X', DesignProperties: ['Nonexistent': 'x'])
  }
}`, reg)

	renamed := violationFor(vs, "Spacing bottom")
	if renamed == nil {
		t.Fatalf("label with an old-name design property: no violation (%d total) — the label was not checked", len(vs))
	}
	if renamed.RuleID != "MDL-WIDGET11" || !strings.Contains(renamed.Message, "renamed") {
		t.Errorf("want an MDL-WIDGET11 rename, got %s: %s", renamed.RuleID, renamed.Message)
	}
	if !strings.Contains(renamed.Suggestion, "'Spacing': ['margin-bottom': 'None']") {
		t.Errorf("suggestion should give the current spelling, got: %s", renamed.Suggestion)
	}

	undefined := violationFor(vs, "Nonexistent")
	if undefined == nil {
		t.Fatalf("label with an undefined design property: no violation (%d total)", len(vs))
	}
	if undefined.RuleID != "MDL-WIDGET11" || !strings.Contains(undefined.Message, "not defined") {
		t.Errorf("want MDL-WIDGET11 not defined, got %s: %s", undefined.RuleID, undefined.Message)
	}
}

// CONTROL: what Studio Pro offers a Label — its own "Style" (swatch or free
// colour) and the "Widget" base (Spacing, Align self, Hide on) — must not warn.
func TestValidateDesignProperties_LabelValidPropsPass(t *testing.T) {
	reg := labelThemeRegistry(t)
	vs := allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  label l1 (Content: 'A', DesignProperties: ['Style': 'Brand Primary', 'Align self': 'Left', 'Spacing': ['margin-bottom': 'M'], 'Hide on': ['Phone': on]])
}`, reg)
	if len(vs) != 0 {
		for _, v := range vs {
			t.Errorf("unexpected %s: %s", v.RuleID, v.Message)
		}
	}
}

// The write path resolves the same key to type each value. Without it a Label's
// "Style" was unknown to the builder, so a free colour fell through to the
// "option" default and mxbuild refused the page:
//
//	[CE6085] "Unknown option #ff0000 for design property Style."
//
// Measured on a copy of PedApp (Mendix 11.13.0): exec succeeded, check was silent.
func TestApplyWidgetAppearance_LabelStyleColourIsCustom(t *testing.T) {
	reg := labelThemeRegistry(t)
	prog, errs := visitor.Build(`create page M.P (layout: Atlas_Core.Atlas_Default) {
  label l1 (Content: 'A', DesignProperties: ['Style': '#ff0000'])
  label l2 (Content: 'B', DesignProperties: ['Style': 'Brand Primary'])
}`)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	want := map[string]string{"l1": "custom", "l2": "option"}
	for _, w := range page.Widgets {
		lbl := &pages.Label{}
		if err := applyWidgetAppearance(lbl, w, reg); err != nil {
			t.Fatalf("%s: %v", w.Name, err)
		}
		if len(lbl.DesignProperties) != 1 {
			t.Fatalf("%s: %d design properties written, want 1", w.Name, len(lbl.DesignProperties))
		}
		if got := lbl.DesignProperties[0].ValueType; got != want[w.Name] {
			t.Errorf("%s: Style written as %q, want %q", w.Name, got, want[w.Name])
		}
	}
}

func TestResolveDesignPropsKey_Label(t *testing.T) {
	if got := resolveDesignPropsKey("label"); got != "Label" {
		t.Errorf(`resolveDesignPropsKey("label") = %q, want "Label" (the key Forms$Label reads)`, got)
	}
	if got, want := resolveDesignPropsKey("LABEL"), bsonTypeToDesignPropsKey["Forms$Label"]; got != want {
		t.Errorf("keyword and stored type disagree: %q vs %q", got, want)
	}
}
