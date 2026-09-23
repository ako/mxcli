// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#526: lint rule SEC005 reports "strict mode disabled" and MDL had no
// statement that would clear it — a rule with no remedy, recorded on the
// reporting project as the one finding that stayed Open with "needs Studio Pro".
//
// StrictMode was read everywhere and written nowhere: security_read.go reads it,
// `show security` prints it, the Starlark rule lints it, and
// ProjectSecurity.SetStrictMode existed in gen and was never called.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

type strictModeCapture struct {
	called  bool
	unitID  model.ID
	enabled bool
}

func newStrictModeCtx(t *testing.T) (*ExecContext, *strictModeCapture) {
	t.Helper()
	cap := &strictModeCapture{}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{
				BaseElement: model.BaseElement{ID: model.ID("ps-1")},
				UserRoles:   []*security.UserRole{{Name: "Administrator"}},
			}, nil
		},
		SetProjectStrictModeFunc: func(unitID model.ID, enabled bool) error {
			cap.called, cap.unitID, cap.enabled = true, unitID, enabled
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx, cap
}

func TestAlterProjectSecurity_StrictMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{{"on", true}, {"off", false}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cap := newStrictModeCtx(t)
			enabled := tc.want
			err := execAlterProjectSecurity(ctx, &ast.AlterProjectSecurityStmt{
				StrictModeEnabled: &enabled,
			})
			if err != nil {
				t.Fatalf("alter failed: %v", err)
			}
			if !cap.called {
				t.Fatal("backend was never asked to set strict mode")
			}
			if cap.enabled != tc.want {
				t.Errorf("strict mode set to %v, want %v", cap.enabled, tc.want)
			}
			if cap.unitID != model.ID("ps-1") {
				t.Errorf("unit id = %q, want the project security element", cap.unitID)
			}
		})
	}
}

// A statement about something else must not touch strict mode. The field is a
// pointer precisely so "said nothing" is distinguishable from "asked for off" —
// a bool would silently disable strict mode on every DEMO USERS toggle.
func TestAlterProjectSecurity_StrictModeUntouchedByOtherClauses(t *testing.T) {
	ctx, cap := newStrictModeCtx(t)
	demo := true
	if err := execAlterProjectSecurity(ctx, &ast.AlterProjectSecurityStmt{
		DemoUsersEnabled: &demo,
	}); err != nil {
		t.Fatalf("alter failed: %v", err)
	}
	if cap.called {
		t.Error("a DEMO USERS statement wrote strict mode")
	}
}

// The grammar half: both spellings reach the AST with the right value, and the
// new STRICT/MODE tokens stay usable as ordinary identifiers.
func TestParse_AlterProjectSecurityStrictMode(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{"ALTER PROJECT SECURITY STRICT MODE ON;", true},
		{"ALTER PROJECT SECURITY STRICT MODE OFF;", false},
	} {
		prog, errs := visitor.Build(tc.src)
		if len(errs) > 0 {
			t.Fatalf("%s: parse failed: %v", tc.src, errs)
		}
		if len(prog.Statements) != 1 {
			t.Fatalf("%s: got %d statements, want 1", tc.src, len(prog.Statements))
		}
		stmt, ok := prog.Statements[0].(*ast.AlterProjectSecurityStmt)
		if !ok {
			t.Fatalf("%s: got %T, want *ast.AlterProjectSecurityStmt", tc.src, prog.Statements[0])
		}
		if stmt.StrictModeEnabled == nil {
			t.Fatalf("%s: StrictModeEnabled not set", tc.src)
		}
		if *stmt.StrictModeEnabled != tc.want {
			t.Errorf("%s: StrictModeEnabled = %v, want %v", tc.src, *stmt.StrictModeEnabled, tc.want)
		}
	}
}

// STRICT and MODE are new lexer tokens, so they must stay usable as names —
// `mode` in particular is an entirely plausible attribute. A new keyword left
// out of the parser's `keyword` rule silently breaks every model already using
// the word, which is the trap this guards.
func TestParse_StrictAndModeRemainUsableAsIdentifiers(t *testing.T) {
	const src = `CREATE ENTITY Sales.Shipment (
  Mode: String(20),
  Strict: Boolean
);`
	if _, errs := visitor.Build(src); len(errs) > 0 {
		t.Errorf("an attribute named Mode or Strict no longer parses: %v", errs)
	}
}
