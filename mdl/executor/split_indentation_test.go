// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// indentOf returns the number of leading spaces on the first line containing want.
func indentOf(t *testing.T, lines []string, want string) int {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, want) {
			return len(line) - len(strings.TrimLeft(line, " "))
		}
	}
	t.Fatalf("no line containing %q in:\n%s", want, strings.Join(lines, "\n"))
	return -1
}

func renderMicroflowBody(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("statement 0: got %T, want *ast.CreateMicroflowStmt", prog.Statements[0])
	}
	// Renders through the LIVE describer. This test used to drive the diff's
	// own AST-to-MDL renderer, which #997 deleted: two renderers for one
	// language drift, and diff's had fallen far enough behind to report
	// activities it could not print as deletions. The indentation rule below
	// is now the describer's, which is the only place it still exists.
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing, measurer: &layoutMeasurer{}}
	oc := fb.buildFlowGraph(mf.Body, nil)
	e := newTestExecutor()
	built := &microflows.Microflow{ObjectCollection: oc}
	return formatMicroflowActivities(e.newExecContext(t.Context()), built, nil, nil)
}

// Both splits used to render a branch body at the SAME column as its branch
// keyword, while `if` indented correctly. The result was unreadable once
// anything nested: a nested `if`'s `else` landed exactly where a reader expects
// a case branch, in output where `else` on a `case` is an MDL008 error. (#913)
//
// The describer indents a branch body from its branch keyword; flattening it
// again fails this test.
func TestSplitBranchBodiesIndentFromTheirBranchKeyword(t *testing.T) {
	t.Run("enum split", func(t *testing.T) {
		lines := renderMicroflowBody(t, `CREATE MICROFLOW Sample.F ($Status: Enumeration(Sample.Status))
RETURNS String
BEGIN
  case $Status
    when Open then
      return 'open';
    when (empty) then
      return 'unset';
  end case;
END;`)
		assertBranchIndent(t, lines, "case $Status", "when Open then", "return 'open';", "end case;")
	})

	t.Run("type split", func(t *testing.T) {
		lines := renderMicroflowBody(t, `CREATE MICROFLOW Sample.F ($Animal: Sample.Animal)
RETURNS String
BEGIN
  split type $Animal
    when Sample.Dog then
      return 'woof';
    when (empty) then
      return 'none';
  end split;
END;`)
		assertBranchIndent(t, lines, "split type $Animal", "when Sample.Dog then", "return 'woof';", "end split;")
	})

	// The reference the two splits must match. If `if` ever changes, the splits
	// should follow it rather than this test being relaxed.
	t.Run("if is the reference", func(t *testing.T) {
		lines := renderMicroflowBody(t, `CREATE MICROFLOW Sample.F ($Flag: Boolean)
RETURNS String
BEGIN
  if $Flag then
    return 'yes';
  else
    return 'no';
  end if;
END;`)
		if got, want := indentOf(t, lines, "return 'yes';"), indentOf(t, lines, "if $Flag then")+2; got != want {
			t.Errorf("if body indent = %d, want %d (one level in from `if`)", got, want)
		}
	})
}

func assertBranchIndent(t *testing.T, lines []string, opener, branch, body, terminator string) {
	t.Helper()
	openIndent := indentOf(t, lines, opener)
	branchIndent := indentOf(t, lines, branch)
	bodyIndent := indentOf(t, lines, body)
	endIndent := indentOf(t, lines, terminator)

	if branchIndent != openIndent+2 {
		t.Errorf("branch indent = %d, want %d (one level in from %q)", branchIndent, openIndent+2, opener)
	}
	if bodyIndent != branchIndent+2 {
		t.Errorf("branch body indent = %d, want %d (one level in from %q).\n"+
			"A body at the same column as its branch keyword is the #913 defect:\n%s",
			bodyIndent, branchIndent+2, branch, strings.Join(lines, "\n"))
	}
	if endIndent != openIndent {
		t.Errorf("terminator indent = %d, want %d (flush with %q)", endIndent, openIndent, opener)
	}
}

// The type split renders the branch keyword and the empty branch in the
// unified spelling. A regression here would reintroduce `case` as a branch
// introducer, which is the overloading #913 reported.
func TestTypeSplitRendersUnifiedSpelling(t *testing.T) {
	out := strings.Join(renderMicroflowBody(t, `CREATE MICROFLOW Sample.F ($Animal: Sample.Animal)
RETURNS String
BEGIN
  split type $Animal
    when Sample.Dog then
      return 'woof';
    when (empty) then
      return 'none';
  end split;
END;`), "\n")

	for _, want := range []string{"when Sample.Dog then", "when (empty) then"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"case Sample.Dog", "\nelse", "  else"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("output still uses the legacy spelling %q:\n%s", unwanted, out)
		}
	}
}
