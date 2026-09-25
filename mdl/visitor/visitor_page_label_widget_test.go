// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `label` is the widget keyword for Forms$Label — Studio Pro's Label widget,
// still loadable on Mendix 11 and carried by stock marketplace modules
// (Administration.Account_Edit, FeedbackModule.ShareFeedback). DESCRIBE had no
// keyword to emit for it and wrote `statictext (Content: …)` with no name,
// which does not parse (and `statictext` writes Forms$Text, which Mendix 11
// cannot load).
func TestLabelWidgetParses(t *testing.T) {
	src := "create page M.P (Title: 'x', Layout: A.L) { label label4 (Content: 'Attachment', Class: 'text-semibold') };"
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	w := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
	if !strings.EqualFold(w.Type, "label") || w.Name != "label4" {
		t.Errorf("widget = %s %q, want label label4", w.Type, w.Name)
	}
	// A built-in keyword, not a generic type: a generic one must resolve to a
	// pluggable widget definition, and `check -p` refused `label` as
	// MDL-WIDGET25 "not a widget in this project".
	if w.TypeIsGeneric {
		t.Error("label parsed as a generic widget type; it must be an enumerated widgetTypeV3 token")
	}
	if w.GetContent() != "Attachment" {
		t.Errorf("Content = %q", w.GetContent())
	}
}

// `Label:` stays a property on other widgets — the keyword is shared.
func TestLabelPropertyStillParses(t *testing.T) {
	src := "create page M.P (Title: 'x', Layout: A.L) { textbox tb (Label: 'Name', Attribute: Name) };"
	if _, errs := Build(src); len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
}
