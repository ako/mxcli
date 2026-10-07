// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestFindCommitsInLoops_NoLoop(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{},
			Action:       &microflows.CommitObjectsAction{},
		},
	}

	var violations []linter.Violation
	r := NewNoCommitInLoopRule()
	findCommitsInLoops(objects, testMicroflow(), r, &violations, false)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations outside loop, got %d", len(violations))
	}
}

func TestFindCommitsInLoops_CommitInsideLoop(t *testing.T) {
	loopBody := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.ActionActivity{
				BaseActivity: microflows.BaseActivity{},
				Action:       &microflows.CommitObjectsAction{},
			},
		},
	}
	objects := []microflows.MicroflowObject{
		&microflows.LoopedActivity{
			ObjectCollection: loopBody,
		},
	}

	var violations []linter.Violation
	r := NewNoCommitInLoopRule()
	findCommitsInLoops(objects, testMicroflow(), r, &violations, false)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].RuleID != "CONV011" {
		t.Errorf("expected CONV011, got %s", violations[0].RuleID)
	}
}

func TestFindCommitsInLoops_NilAction(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{},
			Action:       nil,
		},
	}

	var violations []linter.Violation
	r := NewNoCommitInLoopRule()
	findCommitsInLoops(objects, testMicroflow(), r, &violations, true)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations for nil action, got %d", len(violations))
	}
}

func TestNoCommitInLoopRule_NilReader(t *testing.T) {
	r := NewNoCommitInLoopRule()
	ctx := linter.NewLintContextFromDB(nil)
	// Reader() is nil, should return nil
	violations := r.Check(ctx)
	if violations != nil {
		t.Errorf("expected nil with nil reader, got %v", violations)
	}
}

func TestNoCommitInLoopRule_Metadata(t *testing.T) {
	r := NewNoCommitInLoopRule()
	if r.ID() != "CONV011" {
		t.Errorf("ID = %q, want CONV011", r.ID())
	}
	if r.Category() != "performance" {
		t.Errorf("Category = %q, want performance", r.Category())
	}
}

func loopWithAction(action microflows.MicroflowAction) []microflows.MicroflowObject {
	return []microflows.MicroflowObject{
		&microflows.LoopedActivity{
			ObjectCollection: &microflows.MicroflowObjectCollection{
				Objects: []microflows.MicroflowObject{
					&microflows.ActionActivity{Action: action},
				},
			},
		},
	}
}

// mendixlabs/mxcli#1217: `change $T (...) commit;` inside a loop is the same
// round trip per iteration as a separate `commit $T;`, and Studio Pro's
// recommender flags both (MXP004). Only the separate commit was reported.
func TestFindCommitsInLoops_CommitOnChangeOrCreateInsideLoop(t *testing.T) {
	cases := []struct {
		name   string
		action microflows.MicroflowAction
		want   int
	}{
		{"change commit", &microflows.ChangeObjectAction{Commit: microflows.CommitTypeYes}, 1},
		{"change commit without events", &microflows.ChangeObjectAction{Commit: microflows.CommitTypeYesWithoutEvents}, 1},
		{"create commit", &microflows.CreateObjectAction{Commit: microflows.CommitTypeYes}, 1},
		{"create commit without events", &microflows.CreateObjectAction{Commit: microflows.CommitTypeYesWithoutEvents}, 1},
		// CONTROLS: no commit on the activity is no round trip.
		{"change no commit", &microflows.ChangeObjectAction{Commit: microflows.CommitTypeNo}, 0},
		{"create no commit", &microflows.CreateObjectAction{Commit: microflows.CommitTypeNo}, 0},
		{"change commit unset", &microflows.ChangeObjectAction{}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var violations []linter.Violation
			findCommitsInLoops(loopWithAction(tc.action), testMicroflow(), NewNoCommitInLoopRule(), &violations, false)
			if len(violations) != tc.want {
				t.Fatalf("got %d violations, want %d: %v", len(violations), tc.want, violations)
			}
		})
	}
}

// CONTROL: the same committing change outside any loop is one round trip.
func TestFindCommitsInLoops_CommitOnChangeOutsideLoop(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.ActionActivity{Action: &microflows.ChangeObjectAction{Commit: microflows.CommitTypeYes}},
	}
	var violations []linter.Violation
	findCommitsInLoops(objects, testMicroflow(), NewNoCommitInLoopRule(), &violations, false)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations outside loop, got %d", len(violations))
	}
}
