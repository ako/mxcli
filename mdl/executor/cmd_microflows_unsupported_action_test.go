// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// upstream #863: an activity the describer renders as a line comment had its
// entire error handler dropped.
//
// emitActivityStatement returned as soon as the formatted statement started with
// "--", before `errorHandlerFlow` was ever consulted. The early return is itself
// correct — annotations emitted before a line comment make exec fail with
// "no viable alternative at input '@position...'" — but it also skipped the
// error-branch traversal below it, so every activity the reader cannot map lost
// its handler silently. The reporter's nanoflow went in with 10 activities and
// came out with 7.
//
// The handler cannot be emitted as executable MDL against a comment, so it is
// emitted commented-out: describe→exec still cannot rebuild the activity, but
// the branch is visible in the artifact instead of vanishing from it.
func TestDescribe_UnsupportedAction_KeepsErrorHandler(t *testing.T) {
	e := newTestExecutor()

	// An action the reader could not map leaves Action nil — the shape
	// SynchronizeAction produces today.
	unmapped := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: mkObj("sync"),
			ErrorHandlingType:   microflows.ErrorHandlingTypeCustomWithoutRollback,
		},
	}

	activityMap := map[model.ID]microflows.MicroflowObject{
		mkID("start"): &microflows.StartEvent{BaseMicroflowObject: mkObj("start")},
		mkID("sync"):  unmapped,
		mkID("after"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("after")},
			Action:       &microflows.CommitObjectsAction{CommitVariable: "Obj", WithEvents: true},
		},
		// The three error-branch activities that disappeared.
		mkID("err1"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("err1")},
			Action:       &microflows.CreateObjectAction{EntityQualifiedName: "Mod.ErrLog", OutputVariable: "E"},
		},
		mkID("err2"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("err2")},
			Action:       &microflows.CommitObjectsAction{CommitVariable: "E", WithEvents: true},
		},
		mkID("err3"): &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("err3")},
			Action:       &microflows.RollbackObjectAction{RollbackVariable: "Obj"},
		},
		mkID("end"): &microflows.EndEvent{BaseMicroflowObject: mkObj("end")},
	}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		mkID("start"): {mkFlow("start", "sync")},
		mkID("sync"):  {mkFlow("sync", "after"), mkErrorFlow("sync", "err1")},
		mkID("after"): {mkFlow("after", "end")},
		mkID("err1"):  {mkFlow("err1", "err2")},
		mkID("err2"):  {mkFlow("err2", "err3")},
	}

	var lines []string
	visited := make(map[model.ID]bool)
	e.traverseFlow(mkID("start"), activityMap, flowsByOrigin, nil, visited, nil, nil, &lines, 1, nil, 0, nil)

	out := strings.Join(lines, "\n")

	// Every error-branch activity must appear somewhere in the output.
	for _, want := range []string{"Mod.ErrLog", "commit $E;", "rollback $Obj;"} {
		if !strings.Contains(out, want) {
			t.Errorf("error-branch statement %q is missing from DESCRIBE output — silently dropped:\n%s", want, out)
		}
	}

	// The handler must be marked as such, not just spilled into the main flow:
	// re-executing this output must not run the error branch unconditionally.
	if !strings.Contains(out, "on error") {
		t.Errorf("output does not mark the error handler:\n%s", out)
	}

	// Every line carrying an error-branch statement must be commented out — an
	// error block cannot attach to a comment, so emitting it as live MDL would
	// move those activities into the main flow on the next exec.
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(line, "Mod.ErrLog") || strings.Contains(line, "commit $E;") ||
			strings.Contains(line, "rollback $Obj;") || strings.Contains(trimmed, "on error") {
			if !strings.HasPrefix(trimmed, "--") {
				t.Errorf("error-handler line is live MDL, not a comment — exec would run it in the main flow: %q", line)
			}
		}
	}
}

// The comment path must stay free of @annotations: the grammar accepts
// `annotation*` only as a prefix of a real statement, so a line comment preceded
// by @position fails exec with "no viable alternative at input '@position...'".
// This is why emitActivityStatement returns early, and the #863 fix must not
// undo it.
func TestDescribe_UnsupportedAction_NoAnnotationsBeforeComment(t *testing.T) {
	e := newTestExecutor()

	activityMap := map[model.ID]microflows.MicroflowObject{
		mkID("start"): &microflows.StartEvent{BaseMicroflowObject: mkObj("start")},
		mkID("sync"):  &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("sync")}},
		mkID("end"):   &microflows.EndEvent{BaseMicroflowObject: mkObj("end")},
	}
	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		mkID("start"): {mkFlow("start", "sync")},
		mkID("sync"):  {mkFlow("sync", "end")},
	}

	var lines []string
	visited := make(map[model.ID]bool)
	e.traverseFlow(mkID("start"), activityMap, flowsByOrigin, nil, visited, nil, nil, &lines, 1, nil, 0, nil)

	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "@") {
			continue
		}
		// An annotation is only legal if a real (non-comment) statement follows.
		if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "--") {
			t.Errorf("annotation %q precedes a line comment — exec fails with "+
				"\"no viable alternative at input '@position...'\"", line)
		}
	}
}
