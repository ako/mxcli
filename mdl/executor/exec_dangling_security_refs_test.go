// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// exec stored the names a user-role or demo-user statement gives it without
// resolving them:
//
//	create user role R ( ModuleRoles: (Shop.NopeRole) );       -> Created user role: R
//	alter user role User add module roles (Shop.NopeRole);     -> Added module roles …
//	create demo user 'd' ( …, UserRoles: (NopeUserRole) );     -> Created demo user: d
//	create demo user 'd' ( …, Entity: Shop.Nope, … );          -> Created demo user: d
//
// and MxBuild reported the dangling name as CE1613 a build later. A script run
// with exec alone — no `check -p --references` first — corrupted the model.
// exec now refuses with the words check uses (validate_grant_refs.go), before
// the statement writes anything.

// securityRefsCtx serves a project with module Shop (roles User, Manager),
// module Administration (entity Account) and user roles User and
// Administrator. Every write the statements could make is recorded.
type securityRefsFixture struct {
	ctx    *ExecContext
	ps     *security.ProjectSecurity
	shop   *security.ModuleSecurity
	writes []string
}

func newSecurityRefsFixture(t *testing.T) *securityRefsFixture {
	t.Helper()
	shopMod := &model.Module{BaseElement: model.BaseElement{ID: nextID("mod")}, Name: "Shop"}
	adminMod := &model.Module{BaseElement: model.BaseElement{ID: nextID("mod")}, Name: "Administration"}
	f := &securityRefsFixture{
		ps: &security.ProjectSecurity{
			BaseElement: model.BaseElement{ID: nextID("ps")},
			UserRoles: []*security.UserRole{
				{Name: "User", ModuleRoles: []string{"Shop.User"}},
				{Name: "Administrator", ModuleRoles: []string{"Shop.Manager"}},
			},
		},
		shop: &security.ModuleSecurity{
			ContainerID: shopMod.ID,
			ModuleRoles: []*security.ModuleRole{{Name: "User"}, {Name: "Manager"}},
		},
	}
	adminSec := &security.ModuleSecurity{ContainerID: adminMod.ID}
	adminDM := &domainmodel.DomainModel{
		ContainerID: adminMod.ID,
		Entities:    []*domainmodel.Entity{{Name: "Account"}},
	}
	record := func(what string) error { f.writes = append(f.writes, what); return nil }
	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{shopMod, adminMod}, nil },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) { return f.ps, nil },
		GetModuleSecurityFunc: func(id model.ID) (*security.ModuleSecurity, error) {
			if id == shopMod.ID {
				return f.shop, nil
			}
			return adminSec, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{adminDM}, nil
		},
		AddUserRoleFunc: func(_ model.ID, name string, _ []string, _ bool) error {
			return record("AddUserRole " + name)
		},
		AlterUserRoleModuleRolesFunc: func(_ model.ID, name string, _ bool, _ []string) error {
			return record("AlterUserRoleModuleRoles " + name)
		},
		SetUserRolePropertiesFunc: func(_ model.ID, name string, _ backend.UserRoleProperties) error {
			return record("SetUserRoleProperties " + name)
		},
		AddDemoUserFunc: func(_ model.ID, name, _, _ string, _ []string) error {
			return record("AddDemoUser " + name)
		},
		RemoveDemoUserFunc: func(_ model.ID, name string) error {
			return record("RemoveDemoUser " + name)
		},
	}
	f.ctx, _ = newMockCtx(t, withBackend(mb))
	return f
}

func (f *securityRefsFixture) exec(t *testing.T, src string) error {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("want one statement, got %d", len(prog.Statements))
	}
	switch s := prog.Statements[0].(type) {
	case *ast.CreateUserRoleStmt:
		return execCreateUserRole(f.ctx, s)
	case *ast.AlterUserRoleStmt:
		return execAlterUserRole(f.ctx, s)
	case *ast.CreateDemoUserStmt:
		return execCreateDemoUser(f.ctx, s)
	}
	t.Fatalf("unexpected statement %T", prog.Statements[0])
	return nil
}

const demoPassword = "Probe!23456789"

func TestExecRefusesDanglingSecurityReferences(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string
	}{
		{"create user role with an unknown module role",
			"create user role Probe ( ModuleRoles: (Shop.User, Shop.NopeRole) );",
			[]string{"create user role Probe", "module role Shop.NopeRole does not exist", "module Shop has: Manager, User"}},
		{"create user role with a role of an unknown module",
			"create user role Probe ( ModuleRoles: (NopeMod.User) );",
			[]string{"module role NopeMod.User does not exist"}},
		{"create or modify user role adding an unknown module role",
			"create or modify user role User ( ModuleRoles: (Shop.NopeRole) );",
			[]string{"create user role User", "module role Shop.NopeRole does not exist"}},
		{"create user role with an unknown manageable role",
			"create user role Probe ( ModuleRoles: (Shop.User), ManageableRoles: (NopeUserRole) );",
			[]string{"user role NopeUserRole does not exist"}},
		{"alter user role adding an unknown module role",
			"alter user role User add module roles (Shop.NopeRole);",
			[]string{"alter user role User", "module role Shop.NopeRole does not exist", "module Shop has: Manager, User"}},
		{"demo user with an unknown user role",
			"create demo user 'probe' ( Password: '" + demoPassword + "', Entity: Administration.Account, UserRoles: (User, NopeUserRole) );",
			[]string{"create demo user 'probe'", "user role NopeUserRole does not exist"}},
		{"demo user with an unknown entity",
			"create demo user 'probe' ( Password: '" + demoPassword + "', Entity: Administration.Nope, UserRoles: (User) );",
			[]string{"create demo user 'probe'", "entity Administration.Nope does not exist"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newSecurityRefsFixture(t)
			err := f.exec(t, "mdl 1;\n"+c.src)
			if err == nil {
				t.Fatalf("exec accepted a dangling reference; writes: %v", f.writes)
			}
			for _, w := range c.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error must contain %q, got: %v", w, err)
				}
			}
			if len(f.writes) != 0 {
				t.Errorf("refused statement still wrote: %v", f.writes)
			}
		})
	}
}

// The controls: stored roles, System's module roles, stored entities, and a
// drop of a module role the user role does not hold (a no-op exec reports as
// such) keep working — each writes exactly what it did before.
func TestExecDanglingSecurityReferences_Controls(t *testing.T) {
	cases := []struct{ name, src string }{
		{"user role with stored and System module roles",
			"create user role Probe ( ModuleRoles: (Shop.User, System.User, System.Administrator) );"},
		{"user role with a stored manageable role",
			"create user role Probe ( ModuleRoles: (Shop.User), ManageableRoles: (User) );"},
		{"alter user role adding a stored module role",
			"alter user role User add module roles (Shop.Manager, System.User);"},
		{"alter user role dropping a module role it does not hold, even an unknown one",
			"alter user role User drop module roles (Shop.Gone);"},
		{"demo user with stored user role and entity",
			"create demo user 'probe' ( Password: '" + demoPassword + "', Entity: Administration.Account, UserRoles: (User) );"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newSecurityRefsFixture(t)
			if err := f.exec(t, "mdl 1;\n"+c.src); err != nil {
				t.Fatalf("a resolvable statement was refused: %v", err)
			}
			if len(f.writes) == 0 {
				t.Errorf("control wrote nothing")
			}
		})
	}
}

// Resolution is against the model as it stands when the statement runs, so a
// module role an earlier statement created resolves — nothing is cached from a
// previous statement.
func TestExecDanglingSecurityReferences_RoleCreatedEarlierResolves(t *testing.T) {
	f := newSecurityRefsFixture(t)
	const src = "mdl 1;\ncreate user role Auditor ( ModuleRoles: (Shop.Auditor) );"
	if err := f.exec(t, src); err == nil {
		t.Fatalf("Shop.Auditor does not exist yet; exec accepted it")
	}
	// What `create module role Shop.Auditor;` above it leaves in the model.
	f.shop.ModuleRoles = append(f.shop.ModuleRoles, &security.ModuleRole{Name: "Auditor"})
	if err := f.exec(t, src); err != nil {
		t.Fatalf("a module role created earlier must resolve: %v", err)
	}
	// And a user role created earlier resolves for a demo user.
	f.ps.UserRoles = append(f.ps.UserRoles, &security.UserRole{Name: "Auditor"})
	if err := f.exec(t, "mdl 1;\ncreate demo user 'auditor' ( Password: '"+demoPassword+
		"', Entity: Administration.Account, UserRoles: (Auditor) );"); err != nil {
		t.Fatalf("a user role created earlier must resolve: %v", err)
	}
}
