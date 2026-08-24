// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// TestCommitBuilderDefaultsToEvents is the #895 regression guard at the layer
// the bug lived in: the AST → semantic model step.
//
// A bare `commit $X;` parses to an MfCommitStmt with WithoutEvents clear, and
// the builder must turn that into WithEvents=true — Mendix's default, and what
// Studio Pro stores for an untouched Commit activity (measured on
// CommitActivity.DefaultCommit in ako/TestApp, 11.13). Before the fix this wrote
// false, which meant every commit mxcli authored skipped its event handlers with
// no tool reporting it.
func TestCommitBuilderDefaultsToEvents(t *testing.T) {
	tests := []struct {
		name           string
		stmt           *ast.MfCommitStmt
		wantWithEvents bool
	}{
		{
			name:           "bare commit means Mendix's default, events on",
			stmt:           &ast.MfCommitStmt{Variable: "Order"},
			wantWithEvents: true,
		},
		{
			name:           "redundant WITH EVENTS means the same thing",
			stmt:           &ast.MfCommitStmt{Variable: "Order", ExplicitWithEvents: true},
			wantWithEvents: true,
		},
		{
			name:           "WITHOUT EVENTS is the only form that turns them off",
			stmt:           &ast.MfCommitStmt{Variable: "Order", WithoutEvents: true},
			wantWithEvents: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb := &flowBuilder{}
			fb.addCommitAction(tt.stmt)

			action := lastAction[*microflows.CommitObjectsAction](t, fb)
			if action.WithEvents != tt.wantWithEvents {
				t.Errorf("WithEvents = %v, want %v", action.WithEvents, tt.wantWithEvents)
			}
		})
	}
}

// TestDeleteAndCreateBuildersWriteRefreshInClient covers the #407 sibling: the
// two activities that had no REFRESH modifier at all, so a Studio Pro activity
// carrying the flag round-tripped back to No.
func TestDeleteAndCreateBuildersWriteRefreshInClient(t *testing.T) {
	t.Run("delete", func(t *testing.T) {
		fb := &flowBuilder{}
		fb.addDeleteAction(&ast.DeleteObjectStmt{Variable: "Order", RefreshInClient: true})
		if a := lastAction[*microflows.DeleteObjectAction](t, fb); !a.RefreshInClient {
			t.Fatal("expected builder to carry RefreshInClient onto the delete")
		}
	})
	t.Run("create", func(t *testing.T) {
		fb := &flowBuilder{}
		fb.addCreateObjectAction(&ast.CreateObjectStmt{
			Variable:        "New",
			EntityType:      ast.QualifiedName{Module: "Mod", Name: "Order"},
			RefreshInClient: true,
		})
		if a := lastAction[*microflows.CreateObjectAction](t, fb); !a.RefreshInClient {
			t.Fatal("expected builder to carry RefreshInClient onto the create")
		}
	})

	// Control: without the modifier both stay at Mendix's default, No. Without
	// this half the test would pass against a builder that hardcoded true.
	t.Run("absent means No", func(t *testing.T) {
		fb := &flowBuilder{}
		fb.addDeleteAction(&ast.DeleteObjectStmt{Variable: "Order"})
		if a := lastAction[*microflows.DeleteObjectAction](t, fb); a.RefreshInClient {
			t.Error("delete: RefreshInClient should default to false")
		}
		fb = &flowBuilder{}
		fb.addCreateObjectAction(&ast.CreateObjectStmt{
			Variable:   "New",
			EntityType: ast.QualifiedName{Module: "Mod", Name: "Order"},
		})
		if a := lastAction[*microflows.CreateObjectAction](t, fb); a.RefreshInClient {
			t.Error("create: RefreshInClient should default to false")
		}
	})
}

// lastAction returns the action of the activity the builder appended last,
// failing the test if it is not of type T.
func lastAction[T microflows.MicroflowAction](t *testing.T, fb *flowBuilder) T {
	t.Helper()

	var zero T
	if len(fb.objects) == 0 {
		t.Fatal("Expected builder to create an action activity")
		return zero
	}
	activity, ok := fb.objects[len(fb.objects)-1].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("Last object = %T, want ActionActivity", fb.objects[len(fb.objects)-1])
		return zero
	}
	action, ok := activity.Action.(T)
	if !ok {
		t.Fatalf("Action = %T, want %T", activity.Action, zero)
		return zero
	}
	return action
}
