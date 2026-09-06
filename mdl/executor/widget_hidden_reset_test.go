// SPDX-License-Identifier: Apache-2.0

// mxcli-formula1 FINDINGS §69/§142: every Image widget mxcli authored on Mendix
// 11.13 failed the build with CE0463 "the definition of this widget has changed".
//
// Established first, because the skill says to and because it decides whose bug
// it is (diagnose-ce0463.md, Step 0):
//
//	baseline blank project, mxcli never ran        -> 0 CE0463
//	  …and it ships 10 Studio Pro-authored widgets of the SAME widget id, which pass
//	the same project + one mxcli-authored Image     -> 1 CE0463, naming that widget
//
// So the tool is the variable: Case B, mxcli emits something the package does not
// accept.
//
// The exhaustive path diff against those Studio Pro widgets came back with every
// path present on both sides and four differing values, two of which were content
// (the widget's Name, and the image it points at). What remained:
//
//	widthUnit   mine='auto'    reference='pixels'
//	heightUnit  mine='auto'    reference='pixels'
//
// and a before/after diff of `mx update-widgets` (which clears the error) changed
// exactly two values: width and height, 48 -> 100.
//
// Two hypotheses were tested and FALSIFIED, which is why they are written down:
//
//   - Key order. mxcli's widget node is not in the reference's alphabetical order.
//     It is a documented CE0463 cause, and it is not this one: the same page's
//     mxcli-authored Datagrid and Badge carry the identical key order and pass.
//   - The width VALUE. Authoring Width: 48 explicitly passes, so 48 is not
//     rejected. (Authoring it explicitly is also what makes the property visible.)
//
// The cause is the interaction. The Image widget hides `width` when `widthUnit`
// is "auto", and a hidden property must hold its DECLARED DEFAULT — mxcli's own
// MDL-WIDGET10 says so, in as many words:
//
//	property `width` is hidden when `widthUnit` is "auto" — a non-default value
//	there fails the build with CE0463 (the default is "100")
//
// The engine skipped the mapping for a hidden property, which leaves the widget
// TEMPLATE's captured value in place. For Image that value is 48, and the
// declared default is 100. So `mxcli check` refused what `mxcli exec` emitted by
// default — and the template alone is enough to see it: image.json's own
// ValueType declares width's DefaultValue as "100" while its Object stores "48".
//
// Skipping was never the invariant. The engine's own comment states it correctly
// — "all 34 declared properties are stored, the hidden ones AT THEIR DEFAULT, so
// hidden means default-valued, not absent" — and then skips, which only coincides
// with the default when the template happens to have captured it.

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// imageDef is the Image widget reduced to the properties this is about: the unit
// that does the hiding, and the dimension it hides.
func imageDef() *WidgetDefinition {
	return &WidgetDefinition{
		WidgetID: "com.mendix.widget.web.image.Image",
		MDLName:  "image",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "widthUnit", Source: "WidthUnit", Operation: "primitive", Value: "auto"},
			{PropertyKey: "width", Source: "Width", Operation: "primitive"},
			{PropertyKey: "heightUnit", Source: "HeightUnit", Operation: "primitive", Value: "auto"},
			{PropertyKey: "height", Source: "Height", Operation: "primitive"},
		},
		PropertyVisibility: []types.WidgetVisibilityRule{
			{PropertyKey: "width", HiddenWhen: &types.WidgetVisibilityCondition{
				PropertyKey: "widthUnit", Operator: "eq", Value: "auto"}},
			{PropertyKey: "height", HiddenWhen: &types.WidgetVisibilityCondition{
				PropertyKey: "heightUnit", Operator: "eq", Value: "auto"}},
		},
	}
}

// The declared defaults, as widgetPropertyDefaults lifts them from the installed
// .mpk. Measured on the 11.13 Image widget.
func imageDefaults() map[string]string {
	return map[string]string{"width": "100", "height": "100", "widthunit": "auto", "heightunit": "auto"}
}

// The reported case: `image img (...)` with no dimensions. Both hidden properties
// must be RESET to their declared default, not left at whatever the template
// captured.
func TestHiddenProperties_ResetToTheirDeclaredDefault(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	hidden := e.hiddenUnnamedProperties(imageDef(), &ast.WidgetV3{Name: "img"}, imageDefaults(), nil)

	for _, key := range []string{"width", "height"} {
		reset, ok := hidden[key]
		if !ok {
			t.Fatalf("%s is hidden when its unit is auto and was not pruned at all", key)
		}
		if reset != "100" {
			t.Errorf("%s reset value = %q, want %q — skipping leaves the template's 48, "+
				"which is the CE0463", key, reset, "100")
		}
	}
}

// CONTROL 1: a property that is NOT hidden must not be touched. A reset applied
// to a visible property would overwrite what the user asked for.
func TestHiddenProperties_VisiblePropertyIsNotReset(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	w := &ast.WidgetV3{Name: "img", Properties: map[string]any{"WidthUnit": "pixels", "Width": "48"}}

	hidden := e.hiddenUnnamedProperties(imageDef(), w, imageDefaults(), nil)
	if _, ok := hidden["width"]; ok {
		t.Error("width is visible when widthUnit is pixels — resetting it would discard Width: 48")
	}
	// …and the other axis is independent: heightUnit is still auto here.
	if _, ok := hidden["height"]; !ok {
		t.Error("height is still hidden (heightUnit defaults to auto) and must still be reset")
	}
}

// CONTROL 2: a property the SCRIPT named is left to MDL-WIDGET10, which reports
// it as an error at check time. Silently resetting it would discard the user's
// value and turn a diagnosable mistake into a mystery.
func TestHiddenProperties_ExplicitlyNamedIsLeftToTheChecker(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	w := &ast.WidgetV3{Name: "img", Properties: map[string]any{"Width": "48"}}

	if _, ok := e.hiddenUnnamedProperties(imageDef(), w, imageDefaults(), nil)["width"]; ok {
		t.Error("a hidden property the script named must not be silently reset — MDL-WIDGET10 reports it")
	}
}

// CONTROL 3: with no declared default, the property is still pruned but carries
// no reset. mxcli does not invent a value it could not look up; skipping is the
// old behaviour and remains the fallback.
func TestHiddenProperties_NoDeclaredDefaultMeansNoReset(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	hidden := e.hiddenUnnamedProperties(imageDef(), &ast.WidgetV3{Name: "img"}, nil, nil)

	reset, ok := hidden["width"]
	if !ok {
		t.Fatal("the property is hidden regardless of whether a default could be found")
	}
	if reset != "" {
		t.Errorf("reset = %q, want empty — no default was available to reset to", reset)
	}
}

// The File Uploader case this mechanism was built for (#956) must keep working:
// there the pruned property is a DATASOURCE, which has no declared default, so it
// is skipped exactly as before.
func TestHiddenProperties_DataSourcePruningIsUnchanged(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	def := uploadModeDef()

	filesMode := e.hiddenUnnamedProperties(def, &ast.WidgetV3{
		Name: "fu", Properties: map[string]any{"DataSource": "assoc"}}, nil, nil)
	if _, ok := filesMode["associatedimages"]; !ok {
		t.Error("associatedImages was not pruned under the default uploadMode — this is #956's CE0463")
	}
	if _, ok := filesMode["associatedfiles"]; ok {
		t.Error("associatedFiles was pruned under uploadMode files — the widget would lose its data source")
	}
}

// The writer and the checker must read the SAME declared defaults, or mxcli can
// author what it then refuses. That is exactly what happened here: MDL-WIDGET10
// knew the default was "100" while the writer emitted the template's 48.
func TestHiddenProperties_WriterAndCheckerShareTheDefaultsSource(t *testing.T) {
	// widgetPropertyDefaults is the checker's source (validate_widget_hidden.go).
	// With no project it yields nothing, which is the "cannot look it up" case —
	// the point here is that the writer calls the same function, so the two can
	// never disagree about what a default is.
	if got := widgetPropertyDefaults("", "com.mendix.widget.web.image.Image"); len(got) != 0 {
		t.Fatalf("expected no defaults without a project, got %d", len(got))
	}
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	hidden := e.hiddenUnnamedProperties(imageDef(), &ast.WidgetV3{Name: "img"},
		widgetPropertyDefaults("", "com.mendix.widget.web.image.Image"), nil)
	if reset := hidden["width"]; reset != "" {
		t.Errorf("reset = %q, want empty when the defaults source has nothing", reset)
	}
}
