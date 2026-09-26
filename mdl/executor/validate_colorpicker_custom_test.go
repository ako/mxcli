// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// colourThemeJSON is the shape of Atlas Core 4.1.3's ColorPickers, trimmed. The
// distinction that matters is `property`: "Border color" names the CSS property
// a custom colour is written to, "Background color" and the Label's "Style" do
// not. A Dropdown and a ToggleButtonGroup are the controls — neither takes a
// free value.
const colourThemeJSON = `{
  "Widget": [],
  "DivContainer": [
    {"name": "Border color", "type": "ColorPicker", "property": "border-color",
     "options": [{"name": "Default", "class": "border-default"}, {"name": "Primary", "class": "border-primary"}]},
    {"name": "Background color", "type": "ColorPicker",
     "options": [{"name": "Brand Primary", "class": "background-primary"}]},
    {"name": "Grow / shrink (self)", "type": "Dropdown",
     "options": [{"name": "1 (auto)", "class": "flex-1"}, {"name": "2", "class": "flex-2"}]},
    {"name": "Overflow", "type": "ToggleButtonGroup",
     "options": [{"name": "Visible", "class": "ov-visible"}, {"name": "Hidden", "class": "ov-hidden"}]}
  ],
  "Label": [
    {"name": "Style", "type": "ColorPicker",
     "options": [{"name": "Brand Primary", "oldNames": ["Primary"], "class": "label-primary"}]}
  ]
}`

func colourThemeRegistry(t *testing.T) *ThemeRegistry {
	t.Helper()
	props, err := parseDesignPropertiesJSON([]byte(colourThemeJSON))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return &ThemeRegistry{WidgetProperties: props}
}

func widget12For(vs []linter.Violation, widget string) *linter.Violation {
	for i := range vs {
		if vs[i].RuleID == "MDL-WIDGET12" && strings.Contains(vs[i].Message, `widget "`+widget+`"`) {
			return &vs[i]
		}
	}
	return nil
}

// A ColorPicker takes a swatch OR a custom colour. The builder already knows it
// — resolveDesignPropertyValueType writes an off-list value on a ColorPicker as
// Forms$CustomDesignPropertyValue — and mxbuild accepts it (0 errors, Mendix
// 11.13.0). Where the theme names a CSS `property`, the compiled page applies
// the value verbatim (`style:{borderColor:"#ff0000"}`), so a CSS colour there is
// a correct authoring choice and MDL-WIDGET12's "not an allowed value" was a
// false positive.
func TestValidateDesignProperties_ColorPickerAcceptsCSSColour(t *testing.T) {
	reg := colourThemeRegistry(t)
	vs := allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  container c1 (DesignProperties: ['Border color': '#ff0000'])
  container c2 (DesignProperties: ['Border color': '#F00'])
  container c3 (DesignProperties: ['Border color': '#ff000080'])
  container c4 (DesignProperties: ['Border color': 'rgb(255, 0, 0)'])
  container c5 (DesignProperties: ['Border color': 'hsl(0 100% 50% / 0.5)'])
  container c6 (DesignProperties: ['Border color': 'RebeccaPurple'])
  container c7 (DesignProperties: ['Border color': 'transparent'])
  container c8 (DesignProperties: ['Border color': 'var(--brand-primary)'])
  container c9 (DesignProperties: ['Border color': 'Primary'])
}`, reg)
	for _, v := range vs {
		t.Errorf("unexpected %s: %s", v.RuleID, v.Message)
	}
}

// A value that is no CSS colour still builds — mxbuild accepted 'banana' and
// '#zzzzzz' — and the compiled page writes `borderColor:"banana"`, which the
// browser discards. Still a warning, but it must say why, not claim the
// ColorPicker only takes its swatches.
func TestValidateDesignProperties_ColorPickerFlagsNonColour(t *testing.T) {
	reg := colourThemeRegistry(t)
	vs := allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  container cBanana (DesignProperties: ['Border color': 'banana'])
  container cBadHex (DesignProperties: ['Border color': '#zzzzzz'])
}`, reg)
	for _, name := range []string{"cBanana", "cBadHex"} {
		v := widget12For(vs, name)
		if v == nil {
			t.Fatalf("%s: no MDL-WIDGET12 for a value that is not a colour", name)
		}
		if !strings.Contains(v.Message, "not a CSS colour") || !strings.Contains(v.Message, "border-color") {
			t.Errorf("%s: message should say it is not a CSS colour for border-color, got: %s", name, v.Message)
		}
	}
}

// Where the theme gives the ColorPicker NO `property`, a custom colour has
// nowhere to go: mxbuild accepts it and the compiled page carries neither a
// class nor a style for it (measured on Atlas's Label "Style", GroupBox "Style"
// and DivContainer "Background" / "Shade"). The warning stays, and says that.
func TestValidateDesignProperties_ColorPickerWithoutPropertyIsNotRendered(t *testing.T) {
	reg := colourThemeRegistry(t)
	vs := allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  label lbl (Content: 'x', DesignProperties: ['Style': '#ff0000'])
  container cBg (DesignProperties: ['Background color': '#00ff00'])
}`, reg)
	for _, name := range []string{"lbl", "cBg"} {
		v := widget12For(vs, name)
		if v == nil {
			t.Fatalf("%s: a custom colour the theme cannot render drew no warning", name)
		}
		if !strings.Contains(v.Message, "not rendered") {
			t.Errorf("%s: message should say the custom colour is not rendered, got: %s", name, v.Message)
		}
	}
}

// A swatch misspelt is not a custom colour the author chose: the builder would
// write "brand primary" as one, and it would render as nothing. Say which
// swatch was meant.
func TestValidateDesignProperties_ColorPickerNearSwatchNamesIt(t *testing.T) {
	reg := colourThemeRegistry(t)
	vs := allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  label lCase (Content: 'x', DesignProperties: ['Style': 'brand primary'])
  label lOld (Content: 'x', DesignProperties: ['Style': 'Primary'])
}`, reg)
	for _, name := range []string{"lCase", "lOld"} {
		v := widget12For(vs, name)
		if v == nil {
			t.Fatalf("%s: no MDL-WIDGET12", name)
		}
		if !strings.Contains(v.Message+v.Suggestion, `"Brand Primary"`) {
			t.Errorf("%s: should name the swatch \"Brand Primary\", got: %s / %s", name, v.Message, v.Suggestion)
		}
	}
}

// CONTROL: a swatch passes, and the non-ColorPicker option properties keep
// flagging an off-list value exactly as before — the builder writes it as an
// Option there and mxbuild refuses it (CE6085 for a Dropdown's unknown option,
// CE6084 for a Custom on a ToggleButtonGroup).
func TestValidateDesignProperties_OptionPropertiesStillFlagFreeValues(t *testing.T) {
	reg := colourThemeRegistry(t)
	vs := allDesignPropViolations(t, `create page M.P (layout: Atlas_Core.Atlas_Default) {
  container cSwatch (DesignProperties: ['Background color': 'Brand Primary', 'Border color': 'Default'])
  container cDrop (DesignProperties: ['Grow / shrink (self)': '#ff0000'])
  container cTbg (DesignProperties: ['Overflow': '#ff0000'])
}`, reg)
	if v := widget12For(vs, "cSwatch"); v != nil {
		t.Errorf("a swatch was flagged: %s", v.Message)
	}
	for _, name := range []string{"cDrop", "cTbg"} {
		v := widget12For(vs, name)
		if v == nil {
			t.Fatalf("%s: an off-list value on a non-ColorPicker is no longer flagged", name)
		}
		if !strings.Contains(v.Message, "not an allowed value") {
			t.Errorf("%s: want the not-an-allowed-value message, got: %s", name, v.Message)
		}
	}
}

func TestIsCSSColour(t *testing.T) {
	for v, want := range map[string]bool{
		"#fff": true, "#FFFF": true, "#a1b2c3": true, "#a1b2c3d4": true,
		"rgb(1,2,3)": true, "RGBA(1, 2, 3, .5)": true, "hsl(0 100% 50%)": true,
		"oklch(70% 0.1 200)": true, "color-mix(in srgb, red, blue)": true,
		"var(--x)": true, "red": true, "Transparent": true, "currentColor": true,
		"banana": false, "#12": false, "#12345": false, "#zzz": false, "": false,
		" ": false, "rgb(1,2,3": false, "url(x)": false, "red blue": false,
	} {
		if got := isCSSColour(v); got != want {
			t.Errorf("isCSSColour(%q) = %v, want %v", v, got, want)
		}
	}
}
