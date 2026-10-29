// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genWf "github.com/mendixlabs/mxcli/modelsdk/gen/workflows"
	"github.com/mendixlabs/mxcli/modelsdk/version"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ako/mxcli#1771: below 11.2 a user task's targeting is stored as UserSource,
// the shape canon.LowerToProjectShape writes on every unit write. describe
// reads what is stored, so a task written to such a project must read back as
// the targeting the statement gave it — or describe → exec drops it.

// storedUserTask writes act as a workflow for a project of version pv — the
// encoder, then the lowering and the floor every unit write passes — and reads
// the stored document back.
func storedUserTask(t *testing.T, act *workflows.UserTask, pv *types.ProjectVersion) *workflows.UserTask {
	t.Helper()
	wf := &workflows.Workflow{
		Name:      "WF",
		Parameter: &workflows.WorkflowParameter{EntityRef: "M.Ctx"},
		Flow:      &workflows.Flow{Activities: []workflows.WorkflowActivity{act}},
	}
	raw, err := encodeWorkflow(wf, pv)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	v := version.Version{Major: pv.MajorVersion, Minor: pv.MinorVersion, Patch: pv.PatchVersion}
	if raw, err = canon.LowerToProjectShape(raw, &v); err != nil {
		t.Fatalf("%s: lower: %v", v, err)
	}
	if raw, err = canon.StripUndeclaredProperties(raw, &v); err != nil {
		t.Fatalf("%s: floor: %v", v, err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	f, _ := el.(*genWf.Workflow).Flow().(*genWf.Flow)
	back := workflowFlowFromGen(f)
	if len(back.Activities) != 1 {
		t.Fatalf("%s: read %d activities, want 1", v, len(back.Activities))
	}
	ut, ok := back.Activities[0].(*workflows.UserTask)
	if !ok {
		t.Fatalf("%s: read back %T, want a user task", v, back.Activities[0])
	}
	return ut
}

// Below 10.12 a user task is a Workflows$UserTask, and a multi-user task's
// rule, participants and `await all users` live in its UserTaskCompletion (a
// Workflows$MultiInputCompletion). Each reads back as the statement gave it,
// as on 10.12 and later.
func TestWorkflowRead_UserTaskInTheMendix9Shape(t *testing.T) {
	outcomes := func() []*workflows.UserTaskOutcome {
		return []*workflows.UserTaskOutcome{{Value: "Approve"}, {Value: "Reject"}}
	}
	for _, tc := range []struct {
		name  string
		pv    *types.ProjectVersion
		multi bool
		cc    *workflows.CompletionCriteria
		input *workflows.TargetUserInput
		await bool
	}{
		{"single", &types.ProjectVersion{MajorVersion: 9, MinorVersion: 24, PatchVersion: 30}, false, nil, nil, false},
		{"consensus", &types.ProjectVersion{MajorVersion: 9, MinorVersion: 24, PatchVersion: 30}, true,
			&workflows.CompletionCriteria{Kind: "Consensus", FallbackOutcome: "Reject"}, &workflows.TargetUserInput{Kind: "All"}, false},
		{"veto, two participants", &types.ProjectVersion{MajorVersion: 9, MinorVersion: 24, PatchVersion: 30}, true,
			&workflows.CompletionCriteria{Kind: "Veto", VetoOutcome: "Reject"}, &workflows.TargetUserInput{Kind: "Absolute", Amount: 2}, false},
		{"majority, await all", &types.ProjectVersion{MajorVersion: 10, MinorVersion: 6}, true,
			&workflows.CompletionCriteria{Kind: "Majority", CompletionType: "Absolute", FallbackOutcome: "Approve"},
			&workflows.TargetUserInput{Kind: "Percentage", Percentage: 50}, true},
		{"control: 10.24", &types.ProjectVersion{MajorVersion: 10, MinorVersion: 24, PatchVersion: 24}, true,
			&workflows.CompletionCriteria{Kind: "Veto", VetoOutcome: "Reject"}, &workflows.TargetUserInput{Kind: "Absolute", Amount: 2}, true},
	} {
		ut := &workflows.UserTask{Page: "M.TaskPage", IsMulti: tc.multi, Outcomes: outcomes(),
			UserSource:         &workflows.XPathBasedUserSource{XPath: "[Name = 'admin']"},
			CompletionCriteria: tc.cc, TargetUserInput: tc.input, AwaitAllUsers: tc.await}
		ut.Name, ut.Caption = "Review", "Review"

		got := storedUserTask(t, ut, tc.pv)
		if got.IsMulti != tc.multi {
			t.Errorf("%s: read back multi=%v, want %v", tc.name, got.IsMulti, tc.multi)
		}
		if !reflect.DeepEqual(got.UserSource, ut.UserSource) {
			t.Errorf("%s: targeting read back as %#v", tc.name, got.UserSource)
		}
		if !tc.multi {
			continue
		}
		if got.CompletionCriteria == nil || *got.CompletionCriteria != *tc.cc {
			t.Errorf("%s: rule read back as %+v, want %+v", tc.name, got.CompletionCriteria, tc.cc)
		}
		if got.TargetUserInput == nil || *got.TargetUserInput != *tc.input {
			t.Errorf("%s: participants read back as %+v, want %+v", tc.name, got.TargetUserInput, tc.input)
		}
		if got.AwaitAllUsers != tc.await {
			t.Errorf("%s: await all users read back as %v, want %v", tc.name, got.AwaitAllUsers, tc.await)
		}
	}
}

func TestWorkflowRead_UserTaskTargetingInTheMendix10Shape(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source workflows.UserSource
	}{
		{"users microflow", &workflows.MicroflowBasedUserSource{Microflow: "M.ACT_Users"}},
		{"users xpath", &workflows.XPathBasedUserSource{XPath: "[Name = 'admin']"}},
		{"none", &workflows.NoUserSource{}},
	} {
		for _, pv := range []*types.ProjectVersion{
			{MajorVersion: 10, MinorVersion: 12, PatchVersion: 19},
			{MajorVersion: 10, MinorVersion: 24, PatchVersion: 24},
			{MajorVersion: 11, MinorVersion: 1},
			{MajorVersion: 11, MinorVersion: 13}, // control: the 11.2 shape
		} {
			for _, multi := range []bool{false, true} {
				ut := &workflows.UserTask{Page: "M.TaskPage", IsMulti: multi, UserSource: tc.source}
				ut.Name, ut.Caption = "Review", "Review"
				ut.Outcomes = []*workflows.UserTaskOutcome{{Value: "Approve"}, {Value: "Reject"}}

				got := storedUserTask(t, ut, pv)
				if got.IsMulti != multi {
					t.Errorf("%s %d.%d multi=%v: read back multi=%v", tc.name, pv.MajorVersion, pv.MinorVersion, multi, got.IsMulti)
				}
				if !reflect.DeepEqual(got.UserSource, tc.source) {
					t.Errorf("%s %d.%d multi=%v: targeting read back as %#v, want %#v",
						tc.name, pv.MajorVersion, pv.MinorVersion, multi, got.UserSource, tc.source)
				}
			}
		}
	}
}
