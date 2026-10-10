// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// widget07Hits checks one page against the no-project registry and returns
// the MDL-WIDGET07 violations it draws.
func widget07Hits(t *testing.T, body string) int {
	t.Helper()
	prog, errs := visitor.Build("create page Shop.P (title: 'P', layout: Atlas_Core.Atlas_Default) {\n" + body + "\n}")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	n := 0
	for _, v := range ValidateWidgetPropertiesForStatement(prog.Statements[0], LoadWidgetRegistry("")) {
		if v.RuleID == "MDL-WIDGET07" {
			n++
			t.Logf("%s", v.Message)
		}
	}
	return n
}

// TestExplicitWidgetIDSkipsBuiltinAllowList: with no project, a project's own
// pluggable widget (`pluggablewidget '<id>'`) has no definition, and fell
// through to the BUILT-IN allow-list — so every one of its properties was
// "silently dropped on write" while exec wrote them all. With a project an
// unknown id is MDL-WIDGET25, so the built-in list is never the right judge.
func TestExplicitWidgetIDSkipsBuiltinAllowList(t *testing.T) {
	if n := widget07Hits(t, `pluggablewidget 'com.acme.Unknown' w (foo: 1, bar: 'x')`); n != 0 {
		t.Errorf("explicit widget id with no project: %d MDL-WIDGET07, want 0", n)
	}
}

// TestBuiltinWidgetStillGetsAllowList is the control: the built-in check must
// still fire for a built-in widget under the same conditions.
func TestBuiltinWidgetStillGetsAllowList(t *testing.T) {
	if n := widget07Hits(t, `container c (bogus: 1)`); n != 1 {
		t.Errorf("built-in container with a bogus property: %d MDL-WIDGET07, want 1", n)
	}
}
