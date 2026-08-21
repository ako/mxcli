// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// guestCall records what reached the backend, so a test can tell "refused"
// from "written with the wrong arguments".
type guestCall struct {
	called  bool
	enabled bool
	role    string
}

func guestMock(stored *security.ProjectSecurity, rec *guestCall) *mock.MockBackend {
	return &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) { return stored, nil },
		SetProjectGuestAccessFunc: func(_ model.ID, enabled bool, role string) error {
			rec.called, rec.enabled, rec.role = true, enabled, role
			return nil
		},
	}
}

func guestStmt(on bool, role string) *ast.AlterProjectSecurityStmt {
	return &ast.AlterProjectSecurityStmt{GuestAccessEnabled: &on, GuestUserRole: role}
}

func projectWithRoles(guestRole string) *security.ProjectSecurity {
	return &security.ProjectSecurity{
		GuestUserRole: guestRole,
		UserRoles:     []*security.UserRole{{Name: "Administrator"}, {Name: "Anonymous"}},
	}
}

func TestAlterProjectSecurityGuestAccess(t *testing.T) {
	t.Run("on with a known role writes it", func(t *testing.T) {
		var rec guestCall
		ctx, buf := newMockCtx(t, withBackend(guestMock(projectWithRoles(""), &rec)))

		assertNoError(t, execAlterProjectSecurity(ctx, guestStmt(true, "Anonymous")))

		if !rec.called || !rec.enabled || rec.role != "Anonymous" {
			t.Errorf("backend got called=%v enabled=%v role=%q; want true/true/Anonymous",
				rec.called, rec.enabled, rec.role)
		}
		assertContainsStr(t, buf.String(), "Anonymous")
	})

	t.Run("role is stored under its declared casing", func(t *testing.T) {
		var rec guestCall
		ctx, _ := newMockCtx(t, withBackend(guestMock(projectWithRoles(""), &rec)))

		assertNoError(t, execAlterProjectSecurity(ctx, guestStmt(true, "anonymous")))

		if rec.role != "Anonymous" {
			t.Errorf("stored role %q, want Anonymous — a reference stored with the caller's "+
				"casing does not match the role it names", rec.role)
		}
	})

	// mxbuild raises CE0133 on guest access with no role, so ON must be refused
	// rather than producing a project that will not build.
	t.Run("on with no role and none stored is refused", func(t *testing.T) {
		var rec guestCall
		ctx, _ := newMockCtx(t, withBackend(guestMock(projectWithRoles(""), &rec)))

		err := execAlterProjectSecurity(ctx, guestStmt(true, ""))
		if err == nil {
			t.Fatal("expected a refusal; enabling guest access with no role builds as CE0133")
		}
		if !strings.Contains(err.Error(), "CE0133") {
			t.Errorf("error does not name the build error it prevents: %v", err)
		}
		if rec.called {
			t.Error("backend was written to despite the refusal")
		}
	})

	// The counterpart: re-enabling a project that already has one should not
	// force the operator to retype it. This is why ROLE is optional.
	t.Run("on with no role but one stored is allowed", func(t *testing.T) {
		var rec guestCall
		ctx, buf := newMockCtx(t, withBackend(guestMock(projectWithRoles("Anonymous"), &rec)))

		assertNoError(t, execAlterProjectSecurity(ctx, guestStmt(true, "")))

		if !rec.called || !rec.enabled {
			t.Fatalf("backend got called=%v enabled=%v; want true/true", rec.called, rec.enabled)
		}
		if rec.role != "" {
			t.Errorf("role %q was rewritten; an empty role means keep the stored one", rec.role)
		}
		assertContainsStr(t, buf.String(), "Anonymous")
	})

	// mxbuild does NOT check this reference — a nonexistent role builds with the
	// same error count as a valid one — so a typo is silent unless caught here.
	t.Run("unknown role is refused", func(t *testing.T) {
		var rec guestCall
		ctx, _ := newMockCtx(t, withBackend(guestMock(projectWithRoles(""), &rec)))

		err := execAlterProjectSecurity(ctx, guestStmt(true, "Anonymus"))
		if err == nil {
			t.Fatal("expected a refusal for a role the project does not have")
		}
		if !strings.Contains(err.Error(), "Administrator") {
			t.Errorf("error does not list the roles that do exist: %v", err)
		}
		if rec.called {
			t.Error("backend was written to despite the refusal")
		}
	})

	// OFF with a role set is valid Mendix (measured: same error count as guest
	// off with no role), so the stored role is kept rather than cleared.
	t.Run("off keeps the stored role", func(t *testing.T) {
		var rec guestCall
		ctx, buf := newMockCtx(t, withBackend(guestMock(projectWithRoles("Anonymous"), &rec)))

		assertNoError(t, execAlterProjectSecurity(ctx, guestStmt(false, "")))

		if !rec.called || rec.enabled {
			t.Fatalf("backend got called=%v enabled=%v; want true/false", rec.called, rec.enabled)
		}
		if rec.role != "" {
			t.Errorf("role %q was written on OFF; the stored one must be left alone", rec.role)
		}
		assertContainsStr(t, buf.String(), "disabled")
	})

	// The three ALTER PROJECT SECURITY forms share one AST node.
	t.Run("a level statement does not touch guest access", func(t *testing.T) {
		var rec guestCall
		ctx, _ := newMockCtx(t, withBackend(guestMock(projectWithRoles("Anonymous"), &rec)))

		assertNoError(t, execAlterProjectSecurity(ctx, &ast.AlterProjectSecurityStmt{SecurityLevel: "Production"}))

		if rec.called {
			t.Error("ALTER PROJECT SECURITY LEVEL wrote guest access")
		}
	})
}
