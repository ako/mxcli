// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func navRole(module, name string) *ast.QualifiedName {
	return &ast.QualifiedName{Module: module, Name: name}
}

// TestCheckNavigationRole covers the three outcomes, which were measured on a
// blank Mendix 11.13 app and are three different severities:
//
//	for Administrator                  → 0 errors
//	for Supervisor                     → CE1613, a normal build error
//	for MyFirstModule.Administrator    → StorageLoadException, project unloadable
func TestCheckNavigationRole(t *testing.T) {
	known := []string{"Administrator", "User"}

	tests := []struct {
		name      string
		role      *ast.QualifiedName
		wantErr   bool
		wantParts []string
	}{
		{
			name: "a real user role resolves",
			role: navRole("", "Administrator"),
		},
		{
			name: "the other real user role resolves",
			role: navRole("", "User"),
		},
		{
			// The reported case. The module part is the whole defect, so the message
			// has to name the bare form to write instead.
			name:    "module-qualified role names the bare form to use",
			role:    navRole("MyFirstModule", "Administrator"),
			wantErr: true,
			wantParts: []string{
				"MyFirstModule.Administrator",
				"USER role",
				"for Administrator",
				// The consequence is worth stating: this one is not a build error.
				"cannot LOAD",
			},
		},
		{
			// A qualified name whose bare half is not a role either: there is no
			// single form to suggest, so list what exists.
			name:      "module-qualified unknown role lists the real ones",
			role:      navRole("MyFirstModule", "Supervisor"),
			wantErr:   true,
			wantParts: []string{"USER role", "Administrator, User"},
		},
		{
			name:      "bare unknown role is reported as CE1613",
			role:      navRole("", "Supervisor"),
			wantErr:   true,
			wantParts: []string{"Supervisor", "CE1613", "Administrator, User"},
		},
		{
			// Measured: Mendix matches the name exactly, so this really is CE1613
			// and not a harmless spelling.
			name:      "case mismatch names the declared casing",
			role:      navRole("", "administrator"),
			wantErr:   true,
			wantParts: []string{"differs in case", "for Administrator", "CE1613"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkNavigationRole(navRoleRef{role: *tt.role, profile: "Responsive"}, known)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tt.role.String(), err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error for %q", tt.role.String())
			}
			// The profile is the only location a navigation error has.
			if !strings.Contains(err.Error(), "Responsive") {
				t.Errorf("error %q does not name the profile", err.Error())
			}
			for _, want := range tt.wantParts {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err.Error(), want)
				}
			}
		})
	}
}

// TestCheckNavigationRoleNoRolesDefined covers a project with security off and
// no user roles: the message must still be readable rather than trailing an
// empty list.
func TestCheckNavigationRoleNoRolesDefined(t *testing.T) {
	err := checkNavigationRole(navRoleRef{role: *navRole("", "Administrator")}, nil)
	if err == nil {
		t.Fatal("expected an error when the project has no user roles")
	}
	if !strings.Contains(err.Error(), "(none defined)") {
		t.Errorf("error %q should say the project defines no user roles", err.Error())
	}
}

// TestNavigationRoleRefs pins collection: a reference the walker misses is a
// reference nothing checks.
func TestNavigationRoleRefs(t *testing.T) {
	tests := []struct {
		name string
		stmt ast.Statement
		want []string
	}{
		{
			name: "default home page carries no role",
			stmt: &ast.AlterNavigationStmt{ProfileName: "Responsive", HomePages: []ast.NavHomePageDef{
				{IsPage: true, Target: ast.QualifiedName{Module: "M", Name: "Home"}},
			}},
		},
		{
			name: "role-based home page",
			stmt: &ast.AlterNavigationStmt{ProfileName: "Responsive", HomePages: []ast.NavHomePageDef{
				{IsPage: true, Target: ast.QualifiedName{Module: "M", Name: "Home"}},
				{IsPage: true, Target: ast.QualifiedName{Module: "M", Name: "Admin"}, ForRole: navRole("", "Administrator")},
			}},
			want: []string{"Administrator"},
		},
		{
			// Several roles in one profile: checking only the first would let the
			// rest through.
			name: "every role-based home page is collected",
			stmt: &ast.AlterNavigationStmt{ProfileName: "Responsive", HomePages: []ast.NavHomePageDef{
				{ForRole: navRole("", "Administrator")},
				{ForRole: navRole("M", "Manager")},
			}},
			want: []string{"Administrator", "M.Manager"},
		},
		{
			// HOME MICROFLOW takes a role the same way HOME PAGE does.
			name: "home microflow carries a role too",
			stmt: &ast.AlterNavigationStmt{ProfileName: "Phone", HomePages: []ast.NavHomePageDef{
				{IsPage: false, Target: ast.QualifiedName{Module: "M", Name: "MF"}, ForRole: navRole("", "User")},
			}},
			want: []string{"User"},
		},
		{
			name: "an unrelated statement contributes nothing",
			stmt: &ast.CreateUserRoleStmt{Name: "Administrator"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := navigationRoleRefs(tt.stmt)
			if len(refs) != len(tt.want) {
				t.Fatalf("got %d refs %v, want %d %v", len(refs), refs, len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if got := refs[i].role.String(); got != w {
					t.Errorf("ref[%d] = %q, want %q", i, got, w)
				}
			}
		})
	}
}

// TestScriptUserRoles covers the over-reach guard: a script that creates a role
// and then uses it is the ordinary way to write one, and must not be refused for
// naming a role that does not exist in the project yet.
func TestScriptUserRoles(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.CreateUserRoleStmt{Name: "Supervisor"},
		&ast.CreateUserRoleStmt{Name: ""}, // must not become an empty known role
		&ast.AlterNavigationStmt{ProfileName: "Responsive", HomePages: []ast.NavHomePageDef{
			{ForRole: navRole("", "Supervisor")},
		}},
	}}

	roles := scriptUserRoles(prog)
	if len(roles) != 1 || roles[0] != "Supervisor" {
		t.Fatalf("scriptUserRoles = %v, want [Supervisor]", roles)
	}
	// Resolving against the script's own roles must succeed even though the
	// project (here, empty) has none.
	if err := checkNavigationRole(navRoleRef{role: *navRole("", "Supervisor")}, roles); err != nil {
		t.Errorf("a role the script creates must resolve: %v", err)
	}
}
