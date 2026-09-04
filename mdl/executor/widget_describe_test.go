// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// A widget was the only MDL extension point with no in-language DESCRIBE, which
// is why `widget init` had to generate documentation — and why that
// documentation could drift from what the parser accepts. The statement is only
// worth having if it answers without a project, which is the state an agent is
// in when it asks "what can I write here?".
func TestDescribeWidget_AnswersFromEmbeddedKnowledgeWithNoProject(t *testing.T) {
	desc, err := DescribeWidget("combobox", "")
	if err != nil {
		t.Fatalf("DescribeWidget with no project: %v", err)
	}
	if desc.WidgetID != "com.mendix.widget.web.combobox.Combobox" {
		t.Errorf("WidgetID = %q", desc.WidgetID)
	}
	if desc.Source != "embedded template" {
		t.Errorf("Source = %q, want the embedded fallback", desc.Source)
	}
	if len(desc.Properties) == 0 {
		t.Error("no properties — an empty description answers nothing")
	}
}

// The full widget id must work as well as the MDL keyword: it is what a widget
// package, a page's BSON and the generated docs all carry, and for a widget with
// no keyword it is the only name there is.
func TestDescribeWidget_AcceptsTheWidgetIdAsWellAsTheKeyword(t *testing.T) {
	byKeyword, err := DescribeWidget("combobox", "")
	if err != nil {
		t.Fatal(err)
	}
	byID, err := DescribeWidget("com.mendix.widget.web.combobox.Combobox", "")
	if err != nil {
		t.Fatal(err)
	}
	if byKeyword.WidgetID != byID.WidgetID {
		t.Errorf("keyword gave %q, id gave %q", byKeyword.WidgetID, byID.WidgetID)
	}
	if len(byKeyword.Properties) != len(byID.Properties) {
		t.Errorf("property counts differ: %d vs %d", len(byKeyword.Properties), len(byID.Properties))
	}
}

// An unknown widget must say so rather than returning an empty description that
// reads as "this widget has no properties".
func TestDescribeWidget_UnknownWidgetIsAnError(t *testing.T) {
	if _, err := DescribeWidget("notawidget", ""); err == nil {
		t.Fatal("want an error for an unknown widget, got none")
	}
}

// The project's installed .mpk is preferred over the embedded template, because
// it is version-accurate and is the only place a Marketplace widget appears.
// This is the control for the no-project test above: without it, "embedded
// template" there is equally consistent with the .mpk path never running.
func TestDescribeWidget_PrefersTheProjectPackageOverTheEmbeddedTemplate(t *testing.T) {
	desc, err := DescribeWidget("combobox", "../../testdata/expr-checker/minimal.mpr")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	if desc.Source != "project .mpk" {
		t.Errorf("Source = %q, want the project package to win", desc.Source)
	}
}

// The rendered form is what a reader actually sees, and it is shared with
// `mxcli widget describe` — so a change that broke it would break both.
func TestPrintWidgetDescription_RendersTheHeaderAndProperties(t *testing.T) {
	desc, err := DescribeWidget("combobox", "")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	PrintWidgetDescription(&sb, *desc)
	out := sb.String()
	for _, want := range []string{"Widget:", "ID:", "Kind:", "Properties ("} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q:\n%s", want, out)
		}
	}
}
