// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// renamedThemeJSON mirrors the shape of Atlas Core 4.1.3's
// themesource/atlas_core/web/design-properties.json: old names live in
// `oldNames` at three levels — on a property (DivContainer "Align content
// (deprecated)"), on an option (DynamicText "Weight"), and on one side of a
// Spacing margin/padding step, spelled "<old key>::<old value>".
const renamedThemeJSON = `{
  "Widget": [
    {
      "name": "Spacing", "type": "Spacing",
      "margin": [
        {"name": "None",
         "top": {"class": "spacing-outer-top-none", "oldNames": ["Spacing top::None", "Spacing top::Outer none"]},
         "bottom": {"class": "spacing-outer-bottom-none", "oldNames": ["Spacing bottom::None", "Spacing bottom::Outer none"]}},
        {"name": "S",
         "top": {"class": "spacing-outer-top", "oldNames": ["Spacing top::Small", "Spacing top::Outer small"]},
         "bottom": {"class": "spacing-outer-bottom", "oldNames": ["Spacing bottom::Small", "Spacing bottom::Outer small"]}},
        {"name": "M",
         "top": {"class": "spacing-outer-top-medium", "oldNames": ["Spacing top::Medium", "Spacing top::Outer medium"]},
         "bottom": {"class": "spacing-outer-bottom-medium", "oldNames": ["Spacing bottom::Medium", "Spacing bottom::Outer medium"]}}
      ],
      "padding": [
        {"name": "L",
         "bottom": {"class": "spacing-inner-bottom-large", "oldNames": ["Spacing bottom::Inner large"]}}
      ]
    },
    {"name": "Align self", "type": "Dropdown", "oldNames": ["Align Self"],
     "options": [{"name": "Left", "class": "pull-left"}, {"name": "Right", "class": "pull-right"}]},
    {"name": "Hide on", "type": "ToggleButtonGroup", "multiSelect": true,
     "options": [{"name": "Phone", "class": "hide-phone", "oldNames": ["Hide on phone", "Hide On Phone"]}]}
  ],
  "DivContainer": [
    {"name": "Align content (deprecated)", "type": "Dropdown", "oldNames": ["Align content"],
     "options": [
       {"name": "Right align as a row", "oldNames": ["Right align as row"], "class": "row-right"},
       {"name": "Left align as a column", "oldNames": ["Left align as column"], "class": "col-left"}
     ]},
    {"name": "Background color", "type": "Dropdown",
     "options": [{"name": "Brand Primary", "oldNames": ["Primary"], "class": "background-primary"}]}
  ],
  "DynamicText": [
    {"name": "Weight", "type": "ToggleButtonGroup", "oldNames": ["Font Weight"],
     "options": [{"name": "Light", "class": "text-light"}]}
  ]
}`

func renamedThemeRegistry(t *testing.T) *ThemeRegistry {
	t.Helper()
	props, err := parseDesignPropertiesJSON([]byte(renamedThemeJSON))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return &ThemeRegistry{WidgetProperties: props}
}

func allDesignPropViolations(t *testing.T, src string, reg *ThemeRegistry) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		out = append(out, ValidateDesignPropertiesForStatement(stmt, reg)...)
	}
	return out
}

func violationFor(vs []linter.Violation, key string) *linter.Violation {
	for i := range vs {
		if strings.Contains(vs[i].Message, `"`+key+`"`) {
			return &vs[i]
		}
	}
	return nil
}

// TestValidateDesignProperties_RenamedKeyNamesItsReplacement is the
// ShareFeedback_Logo case (Feedback v4.0.2 on Atlas Core 4.1.3): stored keys
// Atlas has RENAMED. mxbuild refuses them on a live page with CE6087 "Design
// properties have been renamed in your theme", so they must still be flagged —
// but as a rename, naming the current property and its current spelling, not as
// "not defined for this widget type" with a list of unrelated keys.
func TestValidateDesignProperties_RenamedKeyNamesItsReplacement(t *testing.T) {
	reg := renamedThemeRegistry(t)
	vs := allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  container c1 (designproperties: ['Align content': 'Right align as a row', 'Spacing bottom': 'Outer medium']) {
    dynamictext t1 (content: 'x', designproperties: ['Spacing top': 'Outer small', 'Font Weight': 'Light'])
  }
  container c2 (designproperties: ['Spacing bottom': 'Inner large', 'Hide on phone': on, 'Align content': 'Left align as column']) {}
}`, reg)

	cases := []struct {
		key, renamedTo, suggestion string
	}{
		{"Align content", "Align content (deprecated)", `'Align content (deprecated)': 'Right align as a row'`},
		{"Font Weight", "Weight", `'Weight': 'Light'`},
	}
	for _, c := range cases {
		v := violationFor(vs, c.key)
		if v == nil {
			t.Errorf("%q: expected a violation, got none (%d total)", c.key, len(vs))
			continue
		}
		if v.RuleID != "MDL-WIDGET11" {
			t.Errorf("%q: rule %s, want MDL-WIDGET11", c.key, v.RuleID)
		}
		if strings.Contains(v.Message, "not defined") {
			t.Errorf("%q: reported as undefined, want a rename: %s", c.key, v.Message)
		}
		if !strings.Contains(v.Message, `renamed to "`+c.renamedTo+`"`) || !strings.Contains(v.Message, "CE6087") {
			t.Errorf("%q: message should name %q and CE6087: %s", c.key, c.renamedTo, v.Message)
		}
		if !strings.Contains(v.Suggestion, c.suggestion) {
			t.Errorf("%q: suggestion should contain %s, got: %s", c.key, c.suggestion, v.Suggestion)
		}
	}

	// Every legacy key on the page is flagged exactly once, each as a rename,
	// with the value mapped to the current spelling.
	wantSuggestions := []string{
		`'Spacing': ['margin-bottom': 'M']`,                      // Spacing bottom: Outer medium
		`'Spacing': ['margin-top': 'S']`,                         // Spacing top: Outer small
		`'Spacing': ['padding-bottom': 'L']`,                     // Spacing bottom: Inner large
		`'Hide on': ['Phone': on]`,                               // multi-select option's old toggle
		`'Align content (deprecated)': 'Left align as a column'`, // old key AND old value
	}
	for _, want := range wantSuggestions {
		found := false
		for _, v := range vs {
			if strings.Contains(v.Suggestion, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("no violation suggests %s", want)
		}
	}
	if len(vs) != 7 {
		t.Errorf("expected 7 violations (one per legacy key), got %d", len(vs))
	}
	for _, v := range vs {
		if strings.Contains(v.Message, "not defined") {
			t.Errorf("legacy key reported as undefined: %s", v.Message)
		}
	}
}

// TestValidateDesignProperties_UnknownKeyStillUndefined is the control: a key
// that is neither current nor an old name keeps the "not defined" warning, so
// recognising old names does not weaken the typo check. mxbuild reports these
// as CE6083 "not supported by your theme" (measured on 11.13.0).
func TestValidateDesignProperties_UnknownKeyStillUndefined(t *testing.T) {
	reg := renamedThemeRegistry(t)
	vs := allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  container c1 (designproperties: ['Spacing bottomx': 'Outer medium', 'Primary': on]) {}
}`, reg)
	for _, key := range []string{"Spacing bottomx", "Primary"} {
		v := violationFor(vs, key)
		if v == nil || v.RuleID != "MDL-WIDGET11" || !strings.Contains(v.Message, "not defined") {
			t.Errorf("%q: expected MDL-WIDGET11 \"not defined\", got %+v", key, v)
		}
	}

	// A legacy Spacing key with a value that no old name covers is still a
	// rename, but its value has no current spelling to offer.
	vs = allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  container c1 (designproperties: ['Spacing bottom': 'Bogus']) {}
}`, reg)
	v := violationFor(vs, "Spacing bottom")
	if v == nil || !strings.Contains(v.Message, `renamed to "Spacing"`) {
		t.Fatalf("expected a rename for Spacing bottom, got %+v", v)
	}
	if !strings.Contains(v.Suggestion, "Outer medium") {
		t.Errorf("unmapped value should list the old values it accepted, got: %s", v.Suggestion)
	}

	// Current names are clean.
	vs = allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  container c1 (designproperties: ['Align content (deprecated)': 'Right align as a row', 'Spacing': ['margin-bottom': 'M']]) {
    dynamictext t1 (content: 'x', designproperties: ['Weight': 'Light'])
  }
}`, reg)
	if len(vs) != 0 {
		t.Errorf("current names should be clean, got %v", vs)
	}
}
