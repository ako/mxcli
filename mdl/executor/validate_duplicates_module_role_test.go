// SPDX-License-Identifier: Apache-2.0

// The last survivor of the under-reporting class dbe5cc2a closed for documents:
// `create module role M.Admin` on a role that already exists passed
// `check --references` and then failed at exec, leaving every statement before
// it applied. A module role is not a *document*, so it was never in
// stmtCreateInfo and fell outside that sweep.
//
// Two exec behaviours the check has to match exactly, or it trades a silent
// under-report for a noisy false positive:
//
//   - CREATE OR MODIFY succeeds on an existing role.
//   - A plain CREATE succeeds on a role mxcli AUTO-PROVISIONED (`User`, carrying
//     autoDocumentRoleDescription). execCreateModuleRole adopts the caller's
//     casing and returns nil, so a check that flagged it would be wrong.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// setupModuleRoleConflictCtx gives module M two roles: "Admin", authored, and
// "User", auto-provisioned by mxcli.
func setupModuleRoleConflictCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := mkModule("M")
	h := mkHierarchy(mod)

	ms := &security.ModuleSecurity{
		BaseElement: model.BaseElement{ID: nextID("ms")},
		ContainerID: mod.ID,
		ModuleRoles: []*security.ModuleRole{
			{BaseElement: model.BaseElement{ID: nextID("mr")}, Name: "Admin", Description: "Authored by a person"},
			{BaseElement: model.BaseElement{ID: nextID("mr")}, Name: "User", Description: autoDocumentRoleDescription},
		},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) {
			return ms, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx
}

// The defect itself: exec refuses this, so check must too.
func TestProjectConflicts_ModuleRoleAlreadyExists(t *testing.T) {
	ctx := setupModuleRoleConflictCtx(t)
	assertHasConflict(t, ctx, `create module role M.Admin;`, "M.Admin")
}

// A role that is not there is not a conflict.
func TestProjectConflicts_NewModuleRoleIsClean(t *testing.T) {
	ctx := setupModuleRoleConflictCtx(t)
	assertNoConflicts(t, ctx, `create module role M.Auditor;`)
}

// The re-runnable spelling. Flagging it would break every security script that
// is written to be replayed, which is the form the docs recommend.
func TestProjectConflicts_CreateOrModifyModuleRoleIsClean(t *testing.T) {
	ctx := setupModuleRoleConflictCtx(t)
	assertNoConflicts(t, ctx, `create or modify module role M.Admin description 'x';`)
}

// The false positive worth guarding: exec SUCCEEDS on an auto-provisioned role,
// adopting the caller's casing. A check that reported it would refuse a script
// that works — and mxcli created that role itself, without being asked.
func TestProjectConflicts_AutoProvisionedModuleRoleIsClean(t *testing.T) {
	ctx := setupModuleRoleConflictCtx(t)
	assertNoConflicts(t, ctx, `create module role M.User;`)
}

// A script that drops the role first is re-creating it, not colliding with it.
// Without DROP MODULE ROLE in stmtDropInfo this reads as a conflict.
func TestProjectConflicts_DropThenCreateModuleRoleIsClean(t *testing.T) {
	ctx := setupModuleRoleConflictCtx(t)
	assertNoConflicts(t, ctx, `
		drop module role M.Admin;
		create module role M.Admin;
	`)
}

// Two CREATEs of the same new role in one script collide with each other even
// though neither exists in the project. This is the OTHER consumer of the same
// registry — CheckScriptDuplicates, which needs no project — and it comes for
// free once the type is classified.
func TestScriptDuplicates_ModuleRoleTwiceInOneScript(t *testing.T) {
	assertHasDupViolation(t, `
		create module role M.Auditor;
		create module role M.Auditor;
	`, "M.Auditor")
}

// …and the re-runnable spelling must stay clean there too.
func TestScriptDuplicates_CreateOrModifyModuleRoleTwiceIsClean(t *testing.T) {
	assertNoDupViolations(t, `
		create or modify module role M.Auditor;
		create or modify module role M.Auditor;
	`)
}
