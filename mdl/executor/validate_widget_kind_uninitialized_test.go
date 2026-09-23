// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// projectWithInstalledHTMLElement builds the state every `mxcli new` project is
// in: the widget's .mpk sits in widgets/, and .mxcli/widgets/ has never been
// written because nobody ran `mxcli widget init`.
//
// A temp dir rather than testdata/expr-checker/minimal.mpr, which is in exactly
// this state too — the registry generates .def.json files into the project as a
// side effect, and a test must not decide whether the next test sees them.
func projectWithInstalledHTMLElement(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	const mpk = "com.mendix.widget.web.HTMLElement.mpk"
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "expr-checker", "widgets", mpk))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "widgets", mpk), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "App.mpr")
}

// htmlElementTree is what `describe page` emits for an HTML Element the project
// already contains — the widget by its MDL name, with the two containers its
// definition declares.
func htmlElementTree() []*ast.WidgetV3 {
	return []*ast.WidgetV3{{
		Type:          "htmlelement",
		Name:          "frame",
		TypeIsGeneric: true,
		Properties:    map[string]any{"tagName": "div"},
		Children: []*ast.WidgetV3{
			{Type: "attribute", Name: "attribute1", TypeIsGeneric: true,
				Properties: map[string]any{"attributeName": "data-x"}},
			{Type: "tagcontentcontainer", Name: "tagcontentcontainer1", TypeIsGeneric: true,
				Properties: map[string]any{}},
		},
	}}
}

// mendixlabs/mxcli#1135, route 2.
//
// `check` is meant to be the strict gate and `exec` the thing that runs. Here it
// was inverted: with the .mpk installed but no .def.json extracted, `check -p
// --references` reported three MDL-WIDGET25 errors — so exec refused to run —
// while `exec --no-check` wrote the page and generated the definitions on its
// way past. Measured on a blank 11.12.2 project:
//
//	check --references  -> `htmlelement` is not a widget in this project
//	                       `attribute` is not a widget in this project
//	                       `tagcontentcontainer` is not a widget in this project
//	exec --no-check     -> info: updated widget definitions ... / Created page
//
// The cause is that the two read different registries. The page builder calls
// RefreshStaleWidgetDefinitions before loading user definitions
// (cmd_pages_builder.go); LoadWidgetRegistry, which check and the LSP use, did
// not — so the validator knew the nine embedded widgets and nothing else.
//
// The self-healing is what made this read as flaky: the FIRST exec writes the
// definitions, and every check after it passes.
func TestValidateWidgetKind_InstalledWidgetNeedsNoWidgetInit(t *testing.T) {
	got := widgetKindViolations(t, projectWithInstalledHTMLElement(t), htmlElementTree())
	if containsRule(got, "MDL-WIDGET25") {
		t.Errorf("an installed widget was reported as absent from the project: %v", got)
	}
	if containsRule(got, "MDL-WIDGET26") {
		t.Errorf("a container the widget declares was reported as undeclared: %v", got)
	}
}

// The control for the above, and the reason the fix cannot be "stop reporting".
// A typo is still a typo in the same project: the .mpk that makes `htmlelement`
// real says nothing about `htmlelemnt`.
func TestValidateWidgetKind_TypoStillReportedInThatProject(t *testing.T) {
	got := widgetKindViolations(t, projectWithInstalledHTMLElement(t), []*ast.WidgetV3{
		{Type: "htmlelemnt", Name: "frame", TypeIsGeneric: true, Properties: map[string]any{}},
	})
	if !containsRule(got, "MDL-WIDGET25") {
		t.Errorf("a misspelt widget was not reported: %v", got)
	}
}
