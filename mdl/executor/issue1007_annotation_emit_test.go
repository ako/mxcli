// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// mendixlabs/mxcli#1007 — DESCRIBE WORKFLOW emitted `annotation '<text>';`, the
// one construct MDL-WF04 exists to refuse. The describer and the validator
// disagreed, each stating its position in a comment, and the describer's was the
// stale one.
//
// The assertion is the full contract, not just "no annotation statement": the
// emitted MDL must PARSE and must pass ValidateWorkflow. Checking only that the
// keyword is gone would also pass for an emit that dropped the text entirely.

// describeAndValidate emits the activities through the real describe path, wraps
// them in the smallest containing workflow, and returns the parse errors and the
// validator's rule IDs.
func describeAndValidate(t *testing.T, acts ...workflows.WorkflowActivity) (src string, parseErrs []string, ruleIDs []string) {
	t.Helper()
	lines := formatWorkflowActivities(&workflows.Flow{Activities: acts}, "  ")
	src = "create workflow M.WF\n  parameter $WorkflowContext: M.E\nbegin\n" +
		strings.Join(lines, "\n") + "\nend workflow;"
	prog, errs := visitor.Build(src)
	for _, e := range errs {
		parseErrs = append(parseErrs, e.Error())
	}
	if len(parseErrs) > 0 {
		return src, parseErrs, nil
	}
	for _, stmt := range prog.Statements {
		if wf, ok := stmt.(*ast.CreateWorkflowStmt); ok {
			for _, v := range ValidateWorkflow(wf) {
				ruleIDs = append(ruleIDs, v.RuleID)
			}
		}
	}
	return src, nil, ruleIDs
}

// An annotation ATTACHED to an activity — the reporter's case. 13 of these came
// out of one 23-activity workflow.
func TestDescribeWorkflow_AttachedAnnotationPassesOwnCheck(t *testing.T) {
	jump := &workflows.JumpToActivity{TargetActivity: "Review"}
	jump.Name = "j1"
	jump.Annotation = "Source: Receive + Set busState = New Opening"

	src, parseErrs, rules := describeAndValidate(t, jump)
	if parseErrs != nil {
		t.Fatalf("emitted MDL does not parse: %v\n%s", parseErrs, src)
	}
	for _, r := range rules {
		if r == "MDL-WF04" {
			t.Errorf("describe output still trips MDL-WF04:\n%s", src)
		}
	}
	// The text must survive as something a reader can see — dropping it silently
	// would satisfy the two checks above.
	if !strings.Contains(src, "Source: Receive + Set busState = New Opening") {
		t.Errorf("the annotation text was dropped:\n%s", src)
	}
	if strings.Contains(src, "annotation '") {
		t.Errorf("still emitting an `annotation` statement:\n%s", src)
	}
}

// A standalone annotation (a canvas sticky note) read back from the model, which
// goes through a different branch of formatWorkflowActivities and had the same
// bug. Note the terminator: the branch must mark itself a comment, or `;` gets
// appended to the last line.
func TestDescribeWorkflow_StandaloneAnnotationPassesOwnCheck(t *testing.T) {
	ann := &workflows.WorkflowAnnotationActivity{Description: "sticky note on the canvas"}

	src, parseErrs, rules := describeAndValidate(t, ann)
	if parseErrs != nil {
		t.Fatalf("emitted MDL does not parse: %v\n%s", parseErrs, src)
	}
	for _, r := range rules {
		if r == "MDL-WF04" {
			t.Errorf("describe output still trips MDL-WF04:\n%s", src)
		}
	}
	if !strings.Contains(src, "sticky note on the canvas") {
		t.Errorf("the annotation text was dropped:\n%s", src)
	}
	if strings.Contains(src, "; --") || strings.Contains(src, "note;") {
		t.Errorf("a terminator was appended to a comment line:\n%s", src)
	}
}

// An annotation may contain newlines, and `--` runs to end of line — so every
// line needs its own prefix or the tail becomes stray tokens, which is the same
// failure mode the statement form had.
func TestDescribeWorkflow_MultiLineAnnotationCommentsEveryLine(t *testing.T) {
	jump := &workflows.JumpToActivity{TargetActivity: "Review"}
	jump.Name = "j1"
	jump.Annotation = "first line\nsecond line\nthird line"

	src, parseErrs, _ := describeAndValidate(t, jump)
	if parseErrs != nil {
		t.Fatalf("multi-line annotation does not parse: %v\n%s", parseErrs, src)
	}
	for _, want := range []string{"first line", "second line", "third line"} {
		if !strings.Contains(src, want) {
			t.Errorf("%q missing from:\n%s", want, src)
		}
	}
	for _, line := range strings.Split(src, "\n") {
		for _, part := range []string{"second line", "third line"} {
			if strings.Contains(line, part) && !strings.Contains(line, "--") {
				t.Errorf("continuation line is not commented: %q", line)
			}
		}
	}
}

// The control for the whole change: an activity with no annotation must emit
// exactly what it did before, with no stray comment line.
//
// The fixture carries the jump's target as a real activity. It did not, and the
// test asserted "no violations at all" — which held until MDL-WF05 landed and
// correctly reported the dangling target. Both PRs were green alone and red
// together, because neither CI run saw the other's change; a fixture that is a
// VALID workflow is what makes the assertion mean what it says.
func TestDescribeWorkflow_NoAnnotationEmitsNoComment(t *testing.T) {
	target := &workflows.UserTask{}
	target.Name = "Review"
	target.Caption = "Review"
	target.Page = "M.ReviewPage"

	jump := &workflows.JumpToActivity{TargetActivity: "Review"}
	jump.Name = "j1"

	src, parseErrs, rules := describeAndValidate(t, target, jump)
	if parseErrs != nil {
		t.Fatalf("parse: %v\n%s", parseErrs, src)
	}
	if len(rules) > 0 {
		t.Errorf("unexpected violations %v for a valid workflow with no annotation:\n%s", rules, src)
	}
	if strings.Contains(src, "annotation") {
		t.Errorf("emitted an annotation for an activity that has none:\n%s", src)
	}
}
