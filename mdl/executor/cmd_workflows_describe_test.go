// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// formatSingleActivity is a test helper that wraps a single activity in a Flow
// and runs formatWorkflowActivities to get the DESCRIBE output.
func formatSingleActivity(act workflows.WorkflowActivity, indent string) []string {
	flow := &workflows.Flow{
		Activities: []workflows.WorkflowActivity{act},
	}
	return formatWorkflowActivities(flow, indent)
}

// --- P0: strip Module.Microflow prefix from parameter names ---

func TestFormatCallMicroflowTask_ParameterNameStripping(t *testing.T) {
	tests := []struct {
		name          string
		paramName     string
		wantParamName string
	}{
		{
			name:          "three-segment name strips prefix",
			paramName:     "WorkflowBaseline.CallMF.Entity",
			wantParamName: "Entity",
		},
		{
			name:          "two-segment name strips prefix",
			paramName:     "CallMF.Entity",
			wantParamName: "Entity",
		},
		{
			name:          "single-segment name unchanged",
			paramName:     "Entity",
			wantParamName: "Entity",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := &workflows.CallMicroflowTask{
				Microflow: "Module.SomeMicroflow",
				ParameterMappings: []*workflows.ParameterMapping{
					{Parameter: tc.paramName, Expression: "$WorkflowContext"},
				},
			}
			task.Name = "callMfTask"
			task.Caption = "Call MF"

			lines := formatCallMicroflowTask(task, "  ")
			output := strings.Join(lines, "\n")

			wantFragment := tc.wantParamName + " = "
			if !strings.Contains(output, wantFragment) {
				t.Errorf("expected output to contain %q, got:\n%s", wantFragment, output)
			}

			// Ensure the full qualified prefix is NOT in the parameter position
			if tc.paramName != tc.wantParamName && strings.Contains(output, tc.paramName+" = ") {
				t.Errorf("output should not contain full qualified name %q as parameter, got:\n%s", tc.paramName, output)
			}
		})
	}
}

// --- P2a: DESCRIBE outputs Caption with COMMENT 'caption' format ---

func TestFormatJumpTo_CaptionCommentFormat(t *testing.T) {
	tests := []struct {
		name    string
		caption string
		actName string
		want    string
	}{
		{
			name:    "caption used over name",
			caption: "Go Back to Review",
			actName: "jumpAct1",
			want:    "jump to target1 comment 'Go Back to Review'",
		},
		{
			// Was: fell back to the activity name and rendered
			// `comment 'jumpAct1'`. That comment was never authored — echoing it
			// made a plain `jump to X;` round-trip as `jump to X comment '…'`
			// (issuetracker #16). An absent caption must emit no comment clause.
			name:    "no comment clause when caption empty",
			caption: "",
			actName: "jumpAct1",
			want:    "jump to target1;",
		},
		{
			// buildJumpTo defaults Caption to the TARGET name, which is the exact
			// shape issuetracker #16 reported. It carries no authored information,
			// so it must not be echoed either.
			name:    "no comment clause when caption is the derived target name",
			caption: "target1",
			actName: "jumpAct2",
			want:    "jump to target1;",
		},
		{
			name:    "caption with single quote escaped",
			caption: "it's done",
			actName: "jumpAct1",
			want:    "jump to target1 comment 'it''s done'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			activity := &workflows.JumpToActivity{
				TargetActivity: "target1",
			}
			activity.Name = tc.actName
			activity.Caption = tc.caption

			lines := formatSingleActivity(activity, "")
			output := strings.Join(lines, "\n")

			if !strings.Contains(output, tc.want) {
				t.Errorf("expected output to contain %q, got:\n%s", tc.want, output)
			}
		})
	}
}

func TestFormatWaitForTimer_CaptionCommentFormat(t *testing.T) {
	tests := []struct {
		name    string
		caption string
		actName string
		delay   string
		want    string
	}{
		{
			name:    "caption with delay",
			caption: "Wait 2 Hours",
			actName: "waitAct1",
			delay:   "${PT2H}",
			// The stored name is not derivable from the caption, so describe
			// emits it — dropping it is what broke `jump to` (ako/mxcli#408).
			want: "wait for timer waitAct1 '${PT2H}' comment 'Wait 2 Hours'",
		},
		{
			name:    "name fallback no delay",
			caption: "",
			actName: "waitAct1",
			delay:   "",
			// Caption falls back to the name here, so the name is derivable and
			// the clause is suppressed: unchanged output.
			want: "wait for timer comment 'waitAct1'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			activity := &workflows.WaitForTimerActivity{
				DelayExpression: tc.delay,
			}
			activity.Name = tc.actName
			activity.Caption = tc.caption

			lines := formatSingleActivity(activity, "")
			output := strings.Join(lines, "\n")

			if !strings.Contains(output, tc.want) {
				t.Errorf("expected output to contain %q, got:\n%s", tc.want, output)
			}
		})
	}
}

func TestFormatCallWorkflowActivity_CaptionCommentFormat(t *testing.T) {
	tests := []struct {
		name    string
		caption string
		actName string
		want    string
	}{
		{
			name:    "caption used",
			caption: "Run Sub-Workflow",
			actName: "callWf1",
			// callWf1 is not the called workflow's name, so it is emitted.
			want: "call workflow Module.SubFlow as callWf1 comment 'Run Sub-Workflow'",
		},
		{
			name:    "name fallback",
			caption: "",
			actName: "callWf1",
			want:    "call workflow Module.SubFlow as callWf1 comment 'callWf1'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			activity := &workflows.CallWorkflowActivity{
				Workflow: "Module.SubFlow",
			}
			activity.Name = tc.actName
			activity.Caption = tc.caption

			lines := formatCallWorkflowActivity(activity, "")
			output := strings.Join(lines, "\n")

			if !strings.Contains(output, tc.want) {
				t.Errorf("expected output to contain %q, got:\n%s", tc.want, output)
			}
		})
	}
}

// TestExclusiveSplit_NormalizesWorkflowContextExpression pins issuetracker #17:
// a decision's condition was written verbatim while call-microflow parameter
// mappings were normalized, so the documented `$workflowContext` reached Mendix
// as an undefined variable and the build failed CE0117 "Error(s) in expression".
// mxcli always names the context parameter `WorkflowContext`, and Mendix
// expressions are case-sensitive, so every case variant must normalize to it.
func TestExclusiveSplit_NormalizesWorkflowContextExpression(t *testing.T) {
	for _, written := range []string{
		"$workflowContext/Title = 'x'",
		"$WORKFLOWCONTEXT/Title = 'x'",
		"$WorkflowContext/Title = 'x'",
	} {
		split := &workflows.ExclusiveSplitActivity{Expression: written}
		split.Name = "Decision"
		autoBindActivitiesInFlow(nil, []workflows.WorkflowActivity{split}, contextExprNormalizer{})
		if want := "$WorkflowContext/Title = 'x'"; split.Expression != want {
			t.Errorf("expression %q normalized to %q, want %q", written, split.Expression, want)
		}
	}
}

// TestContextExprNormalizer_AliasesDeclaredParameterName pins the other half of
// issuetracker #17: `create workflow … parameter $Ctx: …` lets the author name
// the context, but mxcli stores the parameter as `WorkflowContext` regardless —
// so `$Ctx` in an expression was an undefined variable (CE0117). The declared
// name must be aliased onto the stored one.
func TestContextExprNormalizer_AliasesDeclaredParameterName(t *testing.T) {
	tests := []struct {
		name     string
		declared string
		expr     string
		want     string
	}{
		{
			name:     "declared alias rewritten",
			declared: "$Ctx",
			expr:     "$Ctx/Total > 1000",
			want:     "$WorkflowContext/Total > 1000",
		},
		{
			name:     "declared alias without sigil",
			declared: "Request",
			expr:     "$Request/Status = 'New'",
			want:     "$WorkflowContext/Status = 'New'",
		},
		{
			name:     "canonical name still normalized when an alias is declared",
			declared: "$Ctx",
			expr:     "$workflowContext/Total > 1000",
			want:     "$WorkflowContext/Total > 1000",
		},
		{
			// A variable that merely starts with the declared name must not be
			// mangled — the alias is a whole-word match.
			name:     "longer variable sharing the prefix is untouched",
			declared: "$Ctx",
			expr:     "$CtxItem/Total > $Ctx/Limit",
			want:     "$CtxItem/Total > $WorkflowContext/Limit",
		},
		{
			name:     "no declared name normalizes casing only",
			declared: "",
			expr:     "$workflowcontext/Total > 1000",
			want:     "$WorkflowContext/Total > 1000",
		},
		{
			name:     "empty expression stays empty",
			declared: "$Ctx",
			expr:     "",
			want:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := newContextExprNormalizer(tc.declared).rewrite(tc.expr)
			if got != tc.want {
				t.Errorf("rewrite(%q) with declared %q = %q, want %q", tc.expr, tc.declared, got, tc.want)
			}
		})
	}
}
