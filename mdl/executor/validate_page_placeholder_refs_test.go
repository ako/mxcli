// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1149, the half underneath the reported one. A page's widgets
// live in TWO fields — `Widgets` is the bare body, and content addressed to a
// named layout placeholder is held apart in `Placeholders` (#532) — and the
// three page validators in validate.go were wired to the first alone. So every
// reference inside a `placeholder X { … }` block was validated by nothing.
//
// That is not an edge shape: `placeholder Main { … }` is what mxcli's own
// skills, its bug-test examples and #1057's own repro all write. Measured on a
// blank Mendix 11.14.0 project, the same button in the two positions:
//
//	create or replace page MyFirstModule.PhMf (…) {
//	  placeholder Main {
//	    actionbutton btn (Action: MICROFLOW MyFirstModule.NoSuchMicroflow)
//	  }
//	};                                  -> ✓ All references valid
//
//	create or replace page MyFirstModule.BareMf (…) {
//	  actionbutton btn (Action: MICROFLOW MyFirstModule.NoSuchMicroflow)
//	};                                  -> microflow not found: …NoSuchMicroflow
//
// validateIconRefs (#1008) and forEachWidget each had to grow the same
// placeholder arm on their own; this is the third copy of one walk, which is
// why allPageWidgets collects the roots once.

// parsePageStmt parses one CREATE PAGE statement through the real parser, so the
// test is about how the AST actually comes out rather than how it is imagined.
func parsePageStmt(t *testing.T, src string) *ast.CreatePageStmtV3 {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("source does not parse: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("parsed %T, want *ast.CreatePageStmtV3", prog.Statements[0])
	}
	return stmt
}

const placeholderButtonPage = `create or replace page Mod.P (Title: 'T', Layout: Atlas_Core.Atlas_Default) {
  placeholder Main {
    actionbutton btn (Caption: 'Go', Action: MICROFLOW Mod.NoSuchMicroflow)
  }
}`

const bareButtonPage = `create or replace page Mod.P (Title: 'T', Layout: Atlas_Core.Atlas_Default) {
  actionbutton btn (Caption: 'Go', Action: MICROFLOW Mod.NoSuchMicroflow)
}`

// The reported escape: a reference inside a placeholder block must be resolved.
func TestPageRefs_PlaceholderContentIsValidated(t *testing.T) {
	ctx, _ := newMockCtx(t)
	stmt := parsePageStmt(t, placeholderButtonPage)

	// The parser really does hold this content apart — if it ever stops doing
	// so, this test would pass for the wrong reason.
	if len(stmt.Widgets) != 0 || len(stmt.Placeholders) == 0 {
		t.Fatalf("expected the button in Placeholders and nothing in Widgets; got %d bare, %d placeholders",
			len(stmt.Widgets), len(stmt.Placeholders))
	}

	errs := validateWidgetReferences(ctx, allPageWidgets(stmt), newScriptContext())
	if len(errs) != 1 || !strings.Contains(errs[0], "Mod.NoSuchMicroflow") {
		t.Fatalf("a microflow reference inside `placeholder Main { … }` was not resolved; got %v", errs)
	}
}

// CONTROL: the same button in the bare body, which has always been reported.
// It is what makes the test above a statement about the placeholder walk rather
// than about whether the check works at all.
func TestPageRefs_BareBodyStillValidated(t *testing.T) {
	ctx, _ := newMockCtx(t)
	stmt := parsePageStmt(t, bareButtonPage)

	errs := validateWidgetReferences(ctx, allPageWidgets(stmt), newScriptContext())
	if len(errs) != 1 || !strings.Contains(errs[0], "Mod.NoSuchMicroflow") {
		t.Fatalf("the bare-body control stopped working; got %v", errs)
	}
}

// CONTROL: a page whose references all resolve stays clean through the merged
// walk — otherwise the fix is indistinguishable from reporting everything.
func TestPageRefs_PlaceholderContentThatResolvesStaysSilent(t *testing.T) {
	ctx, _ := newMockCtx(t)
	sc := newScriptContext()
	sc.microflows["Mod.NoSuchMicroflow"] = true // created earlier in the same script

	stmt := parsePageStmt(t, placeholderButtonPage)
	if errs := validateWidgetReferences(ctx, allPageWidgets(stmt), sc); len(errs) != 0 {
		t.Errorf("a resolvable reference inside a placeholder was reported: %v", errs)
	}
}

// allPageWidgets must merge, not replace: a page with both a bare body and a
// placeholder block has to have BOTH walked. Returning only one set would make
// the test above pass while re-opening the hole from the other side.
func TestAllPageWidgets_MergesBothFields(t *testing.T) {
	stmt := parsePageStmt(t, `create or replace page Mod.P (Title: 'T', Layout: Atlas_Core.Atlas_Default) {
  container bare { dynamictext d1 (Content: 'x') }
  placeholder Sidebar { container inPh { dynamictext d2 (Content: 'y') } }
}`)

	var names []string
	for _, w := range allPageWidgets(stmt) {
		names = append(names, w.Name)
	}
	if len(names) != 2 {
		t.Fatalf("got roots %v, want the bare container and the placeholder's", names)
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "bare") || !strings.Contains(joined, "inPh") {
		t.Errorf("got roots %v, want both `bare` and `inPh`", names)
	}
}
