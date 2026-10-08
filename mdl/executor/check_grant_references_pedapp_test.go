// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// A security statement's references were never resolved at check time:
//
//	grant read * on entity System.Nope to M.R;                -> Check passed
//	grant read * on entity M.E to M.NopeRole;                  -> Check passed
//	grant execute on microflow M.Nope to M.R;                  -> Check passed
//	grant read (NopeAttr) on entity M.E to M.R;                -> Check passed
//	create user role R ( ModuleRoles: (M.NopeRole) );          -> Check passed
//
// Measured on Evora Factory Management (10.24.15). exec refuses every grant
// shape after the statements before it are written; the user-role and
// demo-user shapes it does not resolve at all, and writes the dangling name
// for MxBuild to report as CE1613.
//
// Run on the real path: the parsed script through ValidateProgram and the
// executor's reference check, against the Studio Pro-authored PedApp fixture.
func TestCheckGrantReferences_UnknownNameIsRefused(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string // every one must appear in the report
	}{
		{"entity grant on System.Nope (the reported shape)",
			"grant read * on entity System.Nope to Administration.User;",
			[]string{"entity System.Nope does not exist", "grant on entity System.Nope"}},
		{"entity grant on a user-module entity that does not exist",
			"grant read * on entity Administration.Nope to Administration.User;",
			[]string{"entity Administration.Nope does not exist"}},
		{"entity grant to a module role that does not exist",
			"grant read * on entity Administration.Account to Administration.NopeRole;",
			[]string{"module role Administration.NopeRole does not exist", "module Administration has: Administrator, User"}},
		{"entity grant to a role of a module that does not exist",
			"grant read * on entity Administration.Account to NopeMod.User;",
			[]string{"module role NopeMod.User does not exist"}},
		{"entity grant, deprecated form",
			"grant Administration.NopeRole on Administration.Account (read *);",
			[]string{"module role Administration.NopeRole does not exist"}},
		{"entity grant naming a member the entity does not have",
			"grant read (FullName, NopeAttr) on entity Administration.Account to Administration.User;",
			[]string{"entity Administration.Account has no member(s) NopeAttr"}},
		{"entity grant on a System entity that exists — exec refuses every System entity",
			"grant read * on entity System.User to Administration.User;",
			[]string{"cannot grant access on System.User"}},
		{"entity revoke, unknown entity",
			"revoke all on entity Administration.Nope from Administration.User;",
			[]string{"entity Administration.Nope does not exist", "revoke on entity"}},
		{"entity revoke, unknown role",
			"revoke all on entity Administration.Account from Administration.NopeRole;",
			[]string{"module role Administration.NopeRole does not exist"}},
		{"microflow grant, unknown microflow",
			"grant execute on microflow MyFirstModule.Nope to MyFirstModule.User;",
			[]string{"microflow MyFirstModule.Nope does not exist", "grant execute on microflow MyFirstModule.Nope"}},
		{"microflow grant, unknown role",
			"grant execute on microflow MyFirstModule.MyFirstLogic to MyFirstModule.NopeRole;",
			[]string{"module role MyFirstModule.NopeRole does not exist"}},
		{"microflow revoke, unknown microflow",
			"revoke execute on microflow MyFirstModule.Nope from MyFirstModule.User;",
			[]string{"microflow MyFirstModule.Nope does not exist"}},
		{"nanoflow grant, unknown nanoflow",
			"grant execute on nanoflow FeedbackModule.Nope to FeedbackModule.User;",
			[]string{"nanoflow FeedbackModule.Nope does not exist"}},
		{"nanoflow grant, unknown role",
			"grant execute on nanoflow FeedbackModule.ACT_Feedback_ClearForm to FeedbackModule.NopeRole;",
			[]string{"module role FeedbackModule.NopeRole does not exist"}},
		{"page grant, unknown page",
			"grant view on page MyFirstModule.Nope to MyFirstModule.User;",
			[]string{"page MyFirstModule.Nope does not exist"}},
		{"page grant, unknown role",
			"grant view on page MyFirstModule.Home_Web to MyFirstModule.NopeRole;",
			[]string{"module role MyFirstModule.NopeRole does not exist"}},
		{"page revoke, unknown page",
			"revoke view on page MyFirstModule.Nope from MyFirstModule.User;",
			[]string{"page MyFirstModule.Nope does not exist"}},
		{"OData service grant, unknown service",
			"grant access on published odata service MyFirstModule.Nope to MyFirstModule.User;",
			[]string{"published OData service MyFirstModule.Nope does not exist"}},
		{"published REST service grant, unknown service",
			"grant access on published rest service MyFirstModule.Nope to MyFirstModule.User;",
			[]string{"published REST service MyFirstModule.Nope does not exist"}},
		{"user role holding an unknown module role (exec writes it, CE1613)",
			"create user role Probe ( ModuleRoles: (Administration.NopeRole, System.User) );",
			[]string{"module role Administration.NopeRole does not exist", "create user role Probe"}},
		{"alter user role adding an unknown module role (exec writes it)",
			"alter user role User add module roles (Administration.NopeRole);",
			[]string{"module role Administration.NopeRole does not exist"}},
		{"alter of a user role that does not exist",
			"alter user role NopeUserRole add module roles (Administration.User);",
			[]string{"user role NopeUserRole does not exist"}},
		{"demo user with an unknown user role (exec writes it)",
			"create demo user 'probe' ( Password: 'Probe!23456789', UserRoles: (NopeUserRole) );",
			[]string{"user role NopeUserRole does not exist"}},
		{"demo user with an unknown entity (exec writes it)",
			"create demo user 'probe' ( Password: 'Probe!23456789', Entity: Administration.Nope, UserRoles: (User) );",
			[]string{"entity Administration.Nope does not exist"}},
		{"guest access to an unknown user role",
			"alter app security ( EnableGuestAccess: true, GuestUserRole: NopeUserRole );",
			[]string{"user role NopeUserRole does not exist"}},
		{"grant to a role the script creates BELOW it",
			"grant read * on entity Administration.Account to Administration.Later;\ncreate module role Administration.Later;",
			[]string{"module role Administration.Later does not exist yet — it is created later in this script"}},
		{"grant to <Module>.User where the module has a role of its own, so none is auto-created",
			"create module AutoRoleNo;\ncreate module role AutoRoleNo.Editor;\ncreate microflow AutoRoleNo.F () begin end;\n" +
				"grant execute on microflow AutoRoleNo.F to AutoRoleNo.User;",
			[]string{"module role AutoRoleNo.User does not exist", "module AutoRoleNo has: Editor"}},
		{"grant to a role the script dropped above it",
			"drop module role Administration.User;\ngrant read * on entity Administration.Account to Administration.User;",
			[]string{"module role Administration.User does not exist"}},
	}

	exec, _, dir := openPedAppCopy(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(agreeCheck(t, exec, dir, "mdl 1;\n"+c.src), "\n")
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("check --references must report %q, reported:\n%s", w, got)
				}
			}
		})
	}
}

// The controls: real roles, real documents, real members, System module roles
// and what the script itself creates above the grant must keep passing — a
// reference check louder than the model is a blocker, not a safety net. So must
// the revokes exec treats as no-ops, which keep a cleanup script re-runnable.
func TestCheckGrantReferences_ResolvableNamesPass(t *testing.T) {
	cases := []struct{ name, src string }{
		{"entity grant on a stored entity to a stored role",
			"grant read * on entity Administration.Account to Administration.User;"},
		{"entity grant naming stored members",
			"grant read (FullName, IsLocalUser), write (FullName) on entity Administration.Account to Administration.Administrator;"},
		{"entity revoke",
			"revoke all on entity Administration.Account from Administration.User;"},
		{"microflow grant",
			"grant execute on microflow MyFirstModule.MyFirstLogic to MyFirstModule.User;"},
		{"nanoflow grant",
			"grant execute on nanoflow FeedbackModule.ACT_Feedback_ClearForm to FeedbackModule.User;"},
		{"page grant",
			"grant view on page MyFirstModule.Home_Web to MyFirstModule.User;"},
		{"user role holding stored and System module roles",
			"create user role Probe ( ModuleRoles: (Administration.User, System.User) );"},
		{"alter user role with System.Administrator",
			"alter user role Administrator add module roles (System.Administrator);"},
		{"demo user with stored user role and entity",
			"create demo user 'probe' ( Password: 'Probe!23456789', Entity: Administration.Account, UserRoles: (User) );"},
		{"guest access to a stored user role, other casing",
			"alter app security ( EnableGuestAccess: true, GuestUserRole: user );"},
		{"document revoke from a role that no longer exists (exec: nothing to revoke)",
			"revoke execute on microflow MyFirstModule.MyFirstLogic from MyFirstModule.Gone;"},
		{"alter user role dropping a role it does not hold (exec: unchanged)",
			"alter user role User drop module roles (Administration.Gone);"},
		{"grant to a module role the script creates above it",
			"create module role Administration.Auditor;\ngrant read * on entity Administration.Account to Administration.Auditor;"},
		{"grant on an entity the script creates above it",
			"create persistent entity MyFirstModule.Thing (Name: String(10));\ngrant read *, write * on entity MyFirstModule.Thing to MyFirstModule.User;"},
		{"grant naming a member of an entity the script creates",
			"create persistent entity MyFirstModule.Thing (Name: String(10));\ngrant read (Name) on entity MyFirstModule.Thing to MyFirstModule.User;"},
		{"grant naming an association the script adds to a stored entity",
			"create persistent entity MyFirstModule.Thing (Name: String(10));\n" +
				"create association Administration.Account_Thing from Administration.Account to MyFirstModule.Thing;\n" +
				"grant read (FullName, Account_Thing) on entity Administration.Account to Administration.User;"},
		{"grant on a microflow the script creates above it",
			"create microflow MyFirstModule.Later () begin end;\ngrant execute on microflow MyFirstModule.Later to MyFirstModule.User;"},
		{"grant to the <Module>.User role creating a flow in a role-less module auto-creates (doctype-tests/02b)",
			"create module AutoRoleYes;\ncreate nanoflow AutoRoleYes.N () begin end;\n" +
				"grant execute on nanoflow AutoRoleYes.N to AutoRoleYes.User;"},
		{"user role and demo user the script creates above",
			"create module role MyFirstModule.Auditor;\ncreate user role Auditor ( ModuleRoles: (MyFirstModule.Auditor, System.User) );\n" +
				"create demo user 'auditor' ( Password: 'Probe!23456789', UserRoles: (Auditor) );"},
	}

	exec, _, dir := openPedAppCopy(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := agreeCheck(t, exec, dir, "mdl 1;\n"+c.src); len(got) != 0 {
				t.Fatalf("a resolvable reference was refused:\n%s", strings.Join(got, "\n"))
			}
		})
	}
}

// Check predicts exec: for the shapes exec refuses, both refuse with the same
// subject; for the controls both accept. Each case runs on a fresh copy.
func TestCheckGrantReferences_AgreesWithExec(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"unknown entity", "grant read * on entity Administration.Nope to Administration.User;", "Administration.Nope"},
		{"unknown role", "grant execute on microflow MyFirstModule.MyFirstLogic to MyFirstModule.NopeRole;", "MyFirstModule.NopeRole"},
		{"unknown member", "grant read (NopeAttr) on entity Administration.Account to Administration.User;", "NopeAttr"},
		{"System entity", "grant read * on entity System.User to Administration.User;", "cannot grant access on System.User"},
		{"control: stored entity and role", "grant read * on entity Administration.Account to Administration.User;", ""},
		{"control: auto-created document role", "create module AutoRoleYes;\ncreate microflow AutoRoleYes.F () begin end;\ngrant execute on microflow AutoRoleYes.F to AutoRoleYes.User;", ""},
		{"control: script-created role", "create module role Administration.Auditor;\ngrant read * on entity Administration.Account to Administration.Auditor;", ""},

		// The user-role and demo-user shapes, which exec used to write
		// unresolved for MxBuild to report as CE1613.
		{"user role, unknown module role",
			"create user role Probe ( ModuleRoles: (Administration.NopeRole, System.User) );",
			"module role Administration.NopeRole does not exist (module Administration has: Administrator, User)"},
		{"user role, unknown manageable role",
			"create user role Probe ( ModuleRoles: (Administration.User), ManageableRoles: (NopeUserRole) );",
			"user role NopeUserRole does not exist"},
		{"create or modify user role, unknown module role",
			"create or modify user role User ( ModuleRoles: (Administration.NopeRole) );",
			"module role Administration.NopeRole does not exist"},
		{"alter user role add, unknown module role",
			"alter user role User add module roles (Administration.NopeRole);",
			"module role Administration.NopeRole does not exist (module Administration has: Administrator, User)"},
		{"demo user, unknown user role",
			"create demo user 'probe' ( Password: 'Probe!23456789', UserRoles: (NopeUserRole) );",
			"user role NopeUserRole does not exist"},
		{"demo user, unknown entity",
			"create demo user 'probe' ( Password: 'Probe!23456789', Entity: Administration.Nope, UserRoles: (User) );",
			"entity Administration.Nope does not exist"},
		{"control: user role with System module roles",
			"create user role Probe ( ModuleRoles: (Administration.User, System.User, System.Administrator) );", ""},
		{"control: alter user role drop of a role it does not hold",
			"alter user role User drop module roles (Administration.Gone);", ""},
		{"control: user role and demo user over roles the script creates above",
			"create module role MyFirstModule.Auditor;\ncreate user role Auditor ( ModuleRoles: (MyFirstModule.Auditor, System.User) );\n" +
				"alter user role Auditor add module roles (Administration.User);\n" +
				"create demo user 'auditor' ( Password: 'Probe!23456789', Entity: Administration.Account, UserRoles: (Auditor) );", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exec, _, dir := openPedAppCopy(t)
			assertAgree(t, exec, dir, "mdl 1;\n"+c.src, c.want)
		})
	}
}
