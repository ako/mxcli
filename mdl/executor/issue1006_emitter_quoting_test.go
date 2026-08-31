// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// mendixlabs/mxcli#1006 — DESCRIBE WORKFLOW emitted single-quoted payloads
// without doubling the quotes inside them, so its own output was a syntax error.
//
// The assertion is that the emitted MDL PARSES, not that it contains a
// particular substring. A substring test would have to encode the escape it is
// checking for, which is the thing under test; parsing is the property that
// actually matters and it cannot be satisfied by the wrong escape.

// parsesAsWorkflowBody emits the activities through the real describe path
// (formatWorkflowActivities, which is what appends the statement terminator),
// wraps them in the smallest workflow that can contain them, and reports the
// parse errors if any.
func parsesAsWorkflowBody(t *testing.T, acts ...workflows.WorkflowActivity) []string {
	t.Helper()
	lines := formatWorkflowActivities(&workflows.Flow{Activities: acts}, "  ")
	src := "create workflow M.WF\n  parameter $WorkflowContext: M.E\nbegin\n" +
		strings.Join(lines, "\n") + "\nend workflow;"
	_, errs := visitor.Build(src)
	if len(errs) == 0 {
		return nil
	}
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Error()
	}
	t.Logf("emitted MDL that failed to parse:\n%s", src)
	return out
}

// The reported case: an XPath payload is itself full of single-quoted
// constraints, so this one is not an edge case — every XPath-targeted user task
// hits it.
func TestDescribeWorkflow_XPathUserSourceReparses(t *testing.T) {
	task := &workflows.UserTask{}
	task.Name = "Review"
	task.Caption = "Review"
	task.Page = "M.ReviewPage"
	task.UserSource = &workflows.XPathBasedUserSource{
		XPath: `[System.UserRoles = '[%UserRole_Banker%]']`,
	}
	if errs := parsesAsWorkflowBody(t, task); errs != nil {
		t.Errorf("targeting users xpath does not re-parse: %v", errs)
	}
}

// The same emitter, the group variant — unreported, and broken identically.
func TestDescribeWorkflow_XPathGroupSourceReparses(t *testing.T) {
	task := &workflows.UserTask{}
	task.Name = "Review"
	task.Caption = "Review"
	task.Page = "M.ReviewPage"
	task.UserSource = &workflows.XPathGroupSource{
		XPath: `[System.UserRoles = '[%UserRole_Banker%]']`,
	}
	if errs := parsesAsWorkflowBody(t, task); errs != nil {
		t.Errorf("targeting groups xpath does not re-parse: %v", errs)
	}
}

// A caption or an outcome value carrying an ordinary apostrophe. Neither needs
// an XPath to reach — "Manager's review" is a caption a person would type.
func TestDescribeWorkflow_ApostropheInCaptionAndOutcomeReparses(t *testing.T) {
	task := &workflows.UserTask{}
	task.Name = "Approve"
	task.Caption = "Manager's review"
	task.Page = "M.ReviewPage"
	task.TaskDescription = "Check the customer's limit"
	task.DueDate = "[%CurrentDateTime%]"
	task.Outcomes = []*workflows.UserTaskOutcome{
		{Value: "Won't fix"},
		{Value: "Approved"},
	}
	if errs := parsesAsWorkflowBody(t, task); errs != nil {
		t.Errorf("apostrophes in caption/description/outcome do not re-parse: %v", errs)
	}
}

// mdlQuoted is the whole fix; assert the escape directly so a failure upstream
// is distinguishable from a failure in a caller.
func TestMDLQuoted(t *testing.T) {
	for in, want := range map[string]string{
		"plain":             "'plain'",
		"it's":              "'it''s'",
		`[a = '[%X%]']`:     `'[a = ''[%X%]'']'`,
		"":                  "''",
		`already ''doubled`: `'already ''''doubled'`,
		`back\slash`:        `'back\slash'`, // backslash is not an escape in MDL
	} {
		if got := mdlQuoted(in); got != want {
			t.Errorf("mdlQuoted(%q) = %q, want %q", in, got, want)
		}
	}
}

// The guard against the next omission. Six of twenty-three emit sites in the
// describers escaped nothing, and each was individually plausible — the pattern
// `fmt.Sprintf("... '%s'", v)` puts the quotes and the escaping in different
// places, so forgetting one is invisible at the call site.
//
// A source scan rather than more emit tests: a test can only cover the emit
// positions it happens to construct, and the failure mode here is a NEW site
// added later. `mdlQuoted` carries its own quotes, so a literal `'%s'` in these
// files is by construction a site that is not using it.
func TestDescribers_HaveNoHandRolledStringLiterals(t *testing.T) {
	pattern := regexp.MustCompile(`'%s'`)
	for _, f := range []string{"cmd_workflows.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if pattern.MatchString(line) {
				t.Errorf("%s:%d emits a hand-rolled MDL string literal — use mdlQuoted:\n\t%s",
					f, i+1, strings.TrimSpace(line))
			}
		}
	}
}
