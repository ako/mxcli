// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// widgetKindViolations runs the widget-tree validator over one page body.
func widgetKindViolations(t *testing.T, projectPath string, widgets []*ast.WidgetV3) []string {
	t.Helper()
	registry := LoadWidgetRegistry(projectPath)
	if registry == nil {
		t.Fatal("no widget registry")
	}
	var out []string
	for _, v := range validateWidgetTree(widgets, registry, "page X") {
		out = append(out, v.RuleID+": "+v.Message)
	}
	return out
}

func pluggable(id, name string, children ...*ast.WidgetV3) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type:       "pluggablewidget",
		Name:       name,
		Properties: map[string]any{"WidgetType": id},
		Children:   children,
	}
}

// An unknown widget id reaches `exec` and fails there, while `check` passes.
// The parser cannot catch it — the id is a string literal — so the validator is
// the only thing that could, and today it says nothing.
func TestValidateWidgetKind_UnknownWidgetIDIsReported(t *testing.T) {
	got := widgetKindViolations(t, "", []*ast.WidgetV3{
		pluggable("com.acme.widget.NotAWidget", "w1"),
	})
	if !containsRule(got, "MDL-WIDGET25") {
		t.Fatalf("unknown widget id not reported; got %v", got)
	}
	if !containsText(got, "com.acme.widget.NotAWidget") {
		t.Errorf("the message does not name the widget: %v", got)
	}
}

// The control for the above: a widget mxcli knows must stay silent, or the rule
// is simply "always complain".
func TestValidateWidgetKind_KnownWidgetIDIsSilent(t *testing.T) {
	got := widgetKindViolations(t, "", []*ast.WidgetV3{
		pluggable("com.mendix.widget.web.combobox.Combobox", "w1"),
	})
	if containsRule(got, "MDL-WIDGET25") {
		t.Errorf("a known widget was reported as unknown: %v", got)
	}
}

// A container keyword the parent does not declare — `group` on a widget whose
// definition has no such object list. It parses (GROUP is in the grammar) and
// the validator used to SKIP it, because isUniversalObjectListKeyword treats
// the keyword as always-an-item wherever it appears.
//
// Tested against a synthetic definition rather than through the tree walk: no
// EMBEDDED widget declares an object list, and the fixture project has no
// extracted defs, so neither route can express the control below.
func TestValidateWidgetKind_ContainerNotDeclaredByTheParentIsReported(t *testing.T) {
	registry := LoadWidgetRegistry("")
	parent := &WidgetDefinition{
		MDLName: "PARENTWIDGET",
		ObjectLists: []ObjectListMapping{
			{MDLContainer: "SERIES", PropertyKey: "series"},
		},
	}
	got := validateWidgetKind(&ast.WidgetV3{Type: "group", Name: "g1"},
		registry, parent, objectListMappingSet(parent), "page X")

	if len(got) == 0 || got[0].RuleID != "MDL-WIDGET26" {
		t.Fatalf("an undeclared container was not reported; got %v", got)
	}
	if !strings.Contains(got[0].Message, "group") {
		t.Errorf("the message does not name the container: %s", got[0].Message)
	}
	// It must say what the parent DOES declare, or the reader is left guessing.
	if !strings.Contains(got[0].Message, "series") {
		t.Errorf("the message does not name the parent's real containers: %s", got[0].Message)
	}
}

// The control: the same keyword on a parent that DOES declare it stays silent.
// Without this, the rule above passes against a build that rejects everything.
func TestValidateWidgetKind_ContainerDeclaredByTheParentIsSilent(t *testing.T) {
	registry := LoadWidgetRegistry("")
	parent := &WidgetDefinition{
		MDLName: "PARENTWIDGET",
		ObjectLists: []ObjectListMapping{
			{MDLContainer: "GROUP", PropertyKey: "groups"},
		},
	}
	if got := validateWidgetKind(&ast.WidgetV3{Type: "group", Name: "g1"},
		registry, parent, objectListMappingSet(parent), "page X"); len(got) != 0 {
		t.Errorf("the parent declares `group`, so it must not be reported: %v", got)
	}
}

// A declared CHILD SLOT is equally legitimate. Object lists are not the whole
// vocabulary of a widget body.
func TestValidateWidgetKind_DeclaredChildSlotIsSilent(t *testing.T) {
	registry := LoadWidgetRegistry("")
	parent := &WidgetDefinition{
		MDLName:    "PARENTWIDGET",
		ChildSlots: []ChildSlotMapping{{MDLContainer: "GROUP", PropertyKey: "groups"}},
	}
	if got := validateWidgetKind(&ast.WidgetV3{Type: "group", Name: "g1"},
		registry, parent, nil, "page X"); len(got) != 0 {
		t.Errorf("a declared child slot must not be reported: %v", got)
	}
}

// An unresolvable parent must silence the rule. In a project that never ran
// `widget init` the registry knows only the embedded widgets, so every real
// parent looks container-less — reporting there would bury correct MDL.
func TestValidateWidgetKind_UnresolvableParentSilencesTheContainerRule(t *testing.T) {
	registry := LoadWidgetRegistry("")
	if got := validateWidgetKind(&ast.WidgetV3{Type: "group", Name: "g1"},
		registry, nil, nil, "page X"); len(got) != 0 {
		t.Errorf("with no resolvable parent the rule must stay silent: %v", got)
	}
}

// An ordinary widget nested inside a pluggable widget's slot is legitimate and
// must not be mistaken for an undeclared container.
func TestValidateWidgetKind_OrdinaryNestedWidgetIsSilent(t *testing.T) {
	const fixture = "../../testdata/expr-checker/minimal.mpr"
	got := widgetKindViolations(t, fixture, []*ast.WidgetV3{
		pluggable("com.mendix.widget.web.htmlelement.HTMLElement", "h",
			&ast.WidgetV3{Type: "container", Name: "c1"}),
	})
	if containsRule(got, "MDL-WIDGET26") {
		t.Errorf("a plain container widget was reported as an undeclared container: %v", got)
	}
}

func containsRule(msgs []string, rule string) bool {
	for _, m := range msgs {
		if strings.Contains(m, rule) {
			return true
		}
	}
	return false
}

func containsText(msgs []string, text string) bool {
	for _, m := range msgs {
		if strings.Contains(m, text) {
			return true
		}
	}
	return false
}
