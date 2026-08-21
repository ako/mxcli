// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"fmt"
	"strings"
	"testing"
)

// TestRoundtripWorkflow_Comprehensive tests all workflow MDL syntax in a single roundtrip.
//
// Activity types covered:
//   - USER TASK (PAGE, TARGETING MICROFLOW, DUE DATE, OUTCOMES with nested, BOUNDARY EVENT x2)
//   - MULTI USER TASK (PAGE, TARGETING MICROFLOW, OUTCOMES)
//   - CALL MICROFLOW (WITH params, OUTCOMES TRUE/FALSE)
//   - DECISION (expression, OUTCOMES TRUE/FALSE with nested JUMP TO and WAIT FOR TIMER)
//   - PARALLEL SPLIT (PATH 1 with USER TASK, PATH 2 with CALL WORKFLOW)
//   - WAIT FOR TIMER (with ISO 8601 delay)
//   - WAIT FOR NOTIFICATION (with BOUNDARY EVENT NON INTERRUPTING TIMER)
//   - JUMP TO (inside DECISION outcome)
//   - CALL WORKFLOW (sub-workflow with parameter expression)
func TestRoundtripWorkflow_Comprehensive(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	mod := testModule

	// --- Prerequisites ---

	// Context entity for both workflows
	if err := env.executeMDL(`create or modify persistent entity ` + mod + `.WfCtxEntity (
		Score: Integer,
		IsApproved: Boolean default false
	);`); err != nil {
		t.Fatalf("create WfCtxEntity: %v", err)
	}

	// Microflow: single-user targeting
	if err := env.executeMDL(`create microflow ` + mod + `.GetSingleReviewer () returns String begin end;`); err != nil {
		t.Fatalf("create GetSingleReviewer: %v", err)
	}

	// Microflow: multi-user targeting
	if err := env.executeMDL(`create microflow ` + mod + `.GetMultiReviewers () returns String begin end;`); err != nil {
		t.Fatalf("create GetMultiReviewers: %v", err)
	}

	// Microflow: called by CALL MICROFLOW (returns Boolean)
	if err := env.executeMDL(`create microflow ` + mod + `.ScoreCalc (Score: Integer) returns Boolean begin end;`); err != nil {
		t.Fatalf("create ScoreCalc: %v", err)
	}

	// Sub-workflow for CALL WORKFLOW
	if err := env.executeMDL(`create workflow ` + mod + `.SubApprovalFlow
  parameter $WorkflowContext: ` + mod + `.WfCtxEntity
begin
  user task SubTask 'Sub-Approval'
    page ` + mod + `.SubPage
    outcomes 'Done' { };
end workflow;`); err != nil {
		t.Fatalf("create SubApprovalFlow: %v", err)
	}

	// --- Main comprehensive workflow ---
	createMDL := `create workflow ` + mod + `.ComprehensiveFlow
  parameter $WorkflowContext: ` + mod + `.WfCtxEntity
begin

  -- NB: no standalone annotation here. Mendix places it in the activity flow,
  -- which accepts only flow elements, and the resulting .mpr cannot be LOADED
  -- (issuetracker #15) — mxcli refuses it, so it cannot appear in a round-trip
  -- test. See TestCreateWorkflow_StandaloneAnnotationRefused.
  user task ReviewTask 'Review Request'
    page ` + mod + `.ReviewPage
    targeting microflow ` + mod + `.GetSingleReviewer
    outcomes
      'Approve' { }
      'Reject' { }
    boundary event interrupting timer '${PT24H}' non interrupting timer '${PT1H}';

  multi user task MultiReviewTask 'Multi-Person Review'
    page ` + mod + `.MultiReviewPage
    targeting microflow ` + mod + `.GetMultiReviewers
    outcomes 'Complete' { };

  call microflow ` + mod + `.ScoreCalc
    with (Score = '$WorkflowContext/Score')
    outcomes
      true -> { }
      false -> { };

  decision '$WorkflowContext/IsApproved'
    outcomes
      true -> {
        wait for timer '${PT2H}';
      }
      false -> {
        jump to ReviewTask;
      };

  parallel split
    path 1 {
      user task FinalApprove 'Final Approval'
        page ` + mod + `.ApprovePage
        outcomes 'Approved' { };
    }
    path 2 {
      call workflow ` + mod + `.SubApprovalFlow;
    };

  wait for notification;

end workflow;`

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("create ComprehensiveFlow: %v", err)
	}

	output, err := env.describeMDL(`describe workflow ` + mod + `.ComprehensiveFlow;`)
	if err != nil {
		t.Fatalf("describe ComprehensiveFlow: %v", err)
	}

	t.Logf("describe output:\n%s", output)

	checks := []struct {
		label   string
		keyword string
	}{
		{"user task", "user task ReviewTask"},
		{"outcome approve", "'Approve'"},
		{"outcome reject", "'Reject'"},
		{"boundary interrupting", "boundary event interrupting timer '${PT24H}'"},
		{"boundary non interrupting", "boundary event non interrupting timer '${PT1H}'"},
		{"multi user task", "multi user task MultiReviewTask"},
		{"call microflow with", "call microflow " + mod + ".ScoreCalc with (Score ="},
		{"outcomes true", "true ->"},
		{"outcomes false", "false ->"},
		{"decision", "decision '$WorkflowContext/IsApproved'"},
		{"wait for timer", "wait for timer '${PT2H}'"},
		{"jump to", "jump to ReviewTask"},
		{"parallel split", "parallel split"},
		{"path 1", "path 1"},
		{"path 2", "path 2"},
		{"call workflow", "call workflow " + mod + ".SubApprovalFlow"},
		{"wait for notification", "wait for notification"},
		{"parameter", "parameter $WorkflowContext: " + mod + ".WfCtxEntity"},
	}

	var failed []string
	for _, c := range checks {
		if !strings.Contains(output, c.keyword) {
			failed = append(failed, c.label+": "+c.keyword)
		}
	}
	if len(failed) > 0 {
		t.Errorf("describe output missing %d expected keywords:\n  %s\n\nFull output:\n%s",
			len(failed), strings.Join(failed, "\n  "), output)
	}
}

// Run on BOTH engines. setupTestEnv defaults to legacy, and legacy could always
// read boundary events — which is exactly why the default (modelsdk) engine
// having no boundary-event reader at all stayed invisible: DESCRIBE emitted
// nothing on the engine users actually run, so describe → exec silently dropped
// the timer and its handler flow, while this test stayed green (issue #948).
// Same shape as the TableMappings gap in roundtrip_dbconnection_test.go.
func TestRoundtripWorkflow_BoundaryEventInterrupting(t *testing.T) {
	for _, eng := range gateEngines {
		t.Run(eng.name, func(t *testing.T) { testRoundtripWorkflowBoundaryInterrupting(t, eng) })
	}
}

func testRoundtripWorkflowBoundaryInterrupting(t *testing.T, eng gateEngine) {
	env := setupTestEnvWithBackend(t, eng.factory)
	defer env.teardown()

	createMDL := `create workflow ` + testModule + `.WfBoundaryInt
  parameter $WorkflowContext: ` + testModule + `.TestEntitySimple
begin
  user task act1 'Review'
    page ` + testModule + `.ReviewPage
    outcomes 'Approve' { }
    boundary event interrupting timer '${PT1H}'
    ;
end workflow;`

	if err := env.executeMDL(`create or modify persistent entity ` + testModule + `.TestEntitySimple (Name: String(100));`); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create workflow: %v", err)
	}

	output, err := env.describeMDL(`describe workflow ` + testModule + `.WfBoundaryInt;`)
	if err != nil {
		t.Fatalf("Failed to describe workflow: %v", err)
	}

	if !strings.Contains(output, "boundary event interrupting timer") {
		t.Errorf("Expected describe output to contain 'boundary event interrupting timer', got:\n%s", output)
	}
}

// Both engines, for the reason on the interrupting variant above.
func TestRoundtripWorkflow_BoundaryEventNonInterrupting(t *testing.T) {
	for _, eng := range gateEngines {
		t.Run(eng.name, func(t *testing.T) { testRoundtripWorkflowBoundaryNonInterrupting(t, eng) })
	}
}

func testRoundtripWorkflowBoundaryNonInterrupting(t *testing.T, eng gateEngine) {
	env := setupTestEnvWithBackend(t, eng.factory)
	defer env.teardown()

	createMDL := `create workflow ` + testModule + `.WfBoundaryNonInt
  parameter $WorkflowContext: ` + testModule + `.TestEntitySimple2
begin
  user task act1 'Review'
    page ` + testModule + `.ReviewPage
    outcomes 'Approve' { }
    boundary event non interrupting timer '${PT2H}'
    ;
end workflow;`

	if err := env.executeMDL(`create or modify persistent entity ` + testModule + `.TestEntitySimple2 (Name: String(100));`); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create workflow: %v", err)
	}

	output, err := env.describeMDL(`describe workflow ` + testModule + `.WfBoundaryNonInt;`)
	if err != nil {
		t.Fatalf("Failed to describe workflow: %v", err)
	}

	if !strings.Contains(output, "boundary event non interrupting timer") {
		t.Errorf("Expected describe output to contain 'boundary event non interrupting timer', got:\n%s", output)
	}
}

func TestRoundtripWorkflow_MultiUserTask(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	createMDL := `create workflow ` + testModule + `.WfMultiUser
  parameter $WorkflowContext: ` + testModule + `.TestEntityMulti
begin
  multi user task act1 'Caption'
    page ` + testModule + `.ReviewPage
    outcomes 'Approve' { }
    ;
end workflow;`

	if err := env.executeMDL(`create or modify persistent entity ` + testModule + `.TestEntityMulti (Name: String(100));`); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create workflow: %v", err)
	}

	output, err := env.describeMDL(`describe workflow ` + testModule + `.WfMultiUser;`)
	if err != nil {
		t.Fatalf("Failed to describe workflow: %v", err)
	}

	if !strings.Contains(output, "multi user task") {
		t.Errorf("Expected describe output to contain 'multi user task', got:\n%s", output)
	}
}

// TestCreateWorkflow_StandaloneAnnotationRefused replaces the two round-trip
// tests that used to assert a standalone `annotation` survives write → read →
// describe → re-execute.
//
// It does survive that loop — mxcli's own reader is tolerant — but the loop
// never loaded the project in Mendix, so it proved nothing about validity. It
// does not: mxcli writes the annotation into the workflow's activity flow, and
// Mendix constructs every child of that list with a Flow parent, which no
// annotation type accepts. The .mpr cannot be LOADED at all — `mx check` dies
// at "Loading the mpr file" with
//
//	System.InvalidOperationException: Type Mendix.Modeler.Workflows.Model.Annotation
//	does not contain a constructor with a parameter of type
//	...Workflows.Model.Flow
//
// (reproduced on mxbuild 11.12.1 with the guard stubbed out). The old tests
// were pinning that defect in place. mxcli now refuses the construct, so the
// behaviour to lock in is the refusal. (issuetracker #15)
func TestCreateWorkflow_StandaloneAnnotationRefused(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create or modify persistent entity ` + testModule + `.TestEntityAnnot (Name: String(100));`); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	cases := []struct {
		name string
		body string
	}{
		{
			name: "annotation alone in the body",
			body: "  annotation 'This is a workflow note';",
		},
		{
			name: "annotation preceding an activity",
			body: "  annotation 'I am a note';\n  wait for timer 'addDays([%CurrentDateTime%], 1)' comment 'Timer';",
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := fmt.Sprintf("%s.WfAnnotRefused%d", testModule, i)
			err := env.executeMDL(`create workflow ` + name + `
  parameter $WorkflowContext: ` + testModule + `.TestEntityAnnot
begin
` + tc.body + `
end workflow;`)
			if err == nil {
				t.Fatal("standalone annotation was accepted — it writes an .mpr Mendix cannot load")
			}
			for _, want := range []string{"MDL-WF04", "cannot load"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal should mention %q, got: %v", want, err)
				}
			}
		})
	}
}

func TestRoundtripWorkflow_CallMicroflowWithParams(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	createMDL := `create workflow ` + testModule + `.WfCallMf
  parameter $WorkflowContext: ` + testModule + `.TestEntityCallMf
begin
  call microflow ` + testModule + `.SomeMicroflow with (Amount = '$WorkflowContext/Amount')
    outcomes true -> { } false -> { };
end workflow;`

	if err := env.executeMDL(`create or modify persistent entity ` + testModule + `.TestEntityCallMf (Amount: Decimal);`); err != nil {
		t.Fatalf("Failed to create entity: %v", err)
	}

	if err := env.executeMDL(`create microflow ` + testModule + `.SomeMicroflow (Amount: Decimal) returns Boolean begin end;`); err != nil {
		t.Fatalf("Failed to create microflow: %v", err)
	}

	if err := env.executeMDL(createMDL); err != nil {
		t.Fatalf("Failed to create workflow: %v", err)
	}

	output, err := env.describeMDL(`describe workflow ` + testModule + `.WfCallMf;`)
	if err != nil {
		t.Fatalf("Failed to describe workflow: %v", err)
	}

	if !strings.Contains(output, "with (") {
		t.Errorf("Expected describe output to contain 'with (', got:\n%s", output)
	}
}
