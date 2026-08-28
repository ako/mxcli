// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

func assocStmt(name, parent, child string) *ast.CreateAssociationStmt {
	qn := func(s string) ast.QualifiedName {
		if i := strings.Index(s, "."); i >= 0 {
			return ast.QualifiedName{Module: s[:i], Name: s[i+1:]}
		}
		return ast.QualifiedName{Name: s}
	}
	return &ast.CreateAssociationStmt{Name: qn(name), Parent: qn(parent), Child: qn(child)}
}

func TestValidateAssociationModules_RefusesRemoteParent(t *testing.T) {
	// The reported case: an association FROM System.User. `check` passed, `exec`
	// reported success, and the .mpr then would not LOAD —
	// KeyNotFoundException at StreamingBsonUnitReader.ResolvePostponedProperties.
	v := ValidateAssociationModules(assocStmt(
		"FteCapTrack.User_Department", "System.User", "FteCapTrack.Department"))
	if len(v) != 1 {
		t.Fatalf("got %d violations, want 1", len(v))
	}
	if v[0].RuleID != "MDL070" {
		t.Errorf("RuleID = %q, want MDL070", v[0].RuleID)
	}
	if v[0].Severity != linter.SeverityError {
		t.Errorf("severity = %v, want error — this writes a project that cannot be opened", v[0].Severity)
	}
	// The message has to name both modules, or the author cannot tell which end
	// of their statement is the problem.
	for _, want := range []string{"System", "FteCapTrack", "System.User"} {
		if !strings.Contains(v[0].Message, want) {
			t.Errorf("message does not mention %q: %s", want, v[0].Message)
		}
	}
}

func TestValidateAssociationModules_NotSystemSpecific(t *testing.T) {
	// Measured: `from Administration.Account to MyFirstModule.Department` fails
	// identically. The rule is about the module boundary, not about System — a
	// guard that only knew System would pass this straight through to the same
	// unopenable model.
	v := ValidateAssociationModules(assocStmt(
		"MyFirstModule.Account_Department", "Administration.Account", "MyFirstModule.Department"))
	if len(v) != 1 {
		t.Fatalf("got %d violations, want 1 for a non-System remote parent", len(v))
	}
}

func TestValidateAssociationModules_AcceptsSupportedShapes(t *testing.T) {
	// Control. Each of these is either measured at 0 errors or is the ordinary
	// same-module case; a guard that refuses one of them is worse than no guard.
	for _, tc := range []struct {
		what                string
		name, parent, child string
	}{
		// The supported cross-module direction: local FROM, remote TO. Stored as
		// a CrossAssociation whose ChildRef is BY_NAME, so nothing dangles.
		{"local parent, remote child", "MyFirstModule.Department_User", "MyFirstModule.Department", "System.User"},
		// Plain same-module association.
		{"same module", "M.A_B", "M.A", "M.B"},
		// An unqualified endpoint means "this association's module".
		{"unqualified parent", "M.A_B", "A", "M.B"},
		// Module names are resolved case-insensitively, so a case difference is
		// the same module and must not be refused.
		{"parent module differing only in case", "M.A_B", "m.A", "M.B"},
	} {
		if v := ValidateAssociationModules(assocStmt(tc.name, tc.parent, tc.child)); len(v) != 0 {
			t.Errorf("%s: got %d violations, want 0: %+v", tc.what, len(v), v)
		}
	}
}

func TestExecCreateAssociation_RefusesRemoteParentWithoutTouchingTheBackend(t *testing.T) {
	// `check` catching this is not enough: `exec --no-check` skips validation
	// entirely, and the result there is a project that cannot be opened. The
	// backend is left unconfigured on purpose — every MockBackend method errors
	// with "not configured", so if the guard ever stops firing this test fails on
	// the backend call rather than passing quietly.
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return true }}
	ctx, _ := newMockCtx(t, withBackend(mb))

	err := execCreateAssociation(ctx, assocStmt(
		"MyFirstModule.User_Department", "System.User", "MyFirstModule.Department"))
	if err == nil {
		t.Fatal("exec accepted an association whose FROM entity is in another module")
	}
	for _, want := range []string{"System.User", "MyFirstModule"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q: %v", want, err)
		}
	}
	// The remedy has to travel with the refusal — this is the one message the
	// author sees when they ran with --no-check.
	if !strings.Contains(err.Error(), "join entity") {
		t.Errorf("refusal does not offer the remedies: %v", err)
	}
}
