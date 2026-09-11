// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1067 (a): a module role written without its module qualifier
// passed BOTH `mxcli check` and `check --references`, and was only rejected once
// the script was already running against a project. Since mxcli does not run a
// script in one transaction, every statement before the GRANT had landed.
//
// The qualifier is missing in the script text, so no project is needed to see
// it — the same argument MDL-GRANT01 makes for the cross-module check next door.
//
// The `create user role` variant is worse than a late error: it did not fail at
// all. `create user role R (Wide)` stored the module role reference as ".Wide"
// and reported success at every mxcli gate; mxbuild then refused the project
// with CE1613 "The selected module role '.Wide' no longer exists."
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// bareRole is a role name with no module — what the grammar produces for
// `Wide`, since qualifiedName's module part is optional.
func bareRole(name string) ast.QualifiedName { return ast.QualifiedName{Name: name} }

func qualRole(mod, name string) ast.QualifiedName {
	return ast.QualifiedName{Module: mod, Name: name}
}

// Every statement that takes a moduleRoleList must be covered. Missing one
// leaves the same defect alive behind a different keyword, which is how the
// entity form survived MDL-GRANT01 in the first place: that rule was written
// for the five document grants and entity was simply never added to the switch.
func TestValidateGrantRoles_UnqualifiedRoleIsReported(t *testing.T) {
	cases := []struct {
		name string
		stmt ast.Statement
	}{
		{"grant on entity", &ast.GrantEntityAccessStmt{
			Entity: qualRole("Sales", "Order"), Roles: []ast.QualifiedName{bareRole("Wide")},
		}},
		{"grant execute on microflow", &ast.GrantMicroflowAccessStmt{
			Microflow: qualRole("Sales", "MF"), Roles: []ast.QualifiedName{bareRole("Wide")},
		}},
		{"grant execute on nanoflow", &ast.GrantNanoflowAccessStmt{
			Nanoflow: qualRole("Sales", "NF"), Roles: []ast.QualifiedName{bareRole("Wide")},
		}},
		{"grant view on page", &ast.GrantPageAccessStmt{
			Page: qualRole("Sales", "Pg"), Roles: []ast.QualifiedName{bareRole("Wide")},
		}},
		{"grant execute on workflow", &ast.GrantWorkflowAccessStmt{
			Workflow: qualRole("Sales", "Wf"), Roles: []ast.QualifiedName{bareRole("Wide")},
		}},
		{"grant access on odata service", &ast.GrantODataServiceAccessStmt{
			Service: qualRole("Sales", "Svc"), Roles: []ast.QualifiedName{bareRole("Wide")},
		}},
		{"grant access on published rest service", &ast.GrantPublishedRestServiceAccessStmt{
			Service: qualRole("Sales", "Rest"), Roles: []ast.QualifiedName{bareRole("Wide")},
		}},
		{"create user role", &ast.CreateUserRoleStmt{
			Name: "Admin", ModuleRoles: []ast.QualifiedName{bareRole("Wide")},
		}},
		{"alter user role", &ast.AlterUserRoleStmt{
			Name: "Admin", Add: true, ModuleRoles: []ast.QualifiedName{bareRole("Wide")},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateGrantRoles(&ast.Program{Statements: []ast.Statement{tc.stmt}})
			if len(got) != 1 {
				t.Fatalf("got %d violations, want 1: %+v", len(got), got)
			}
			if got[0].RuleID != "MDL-GRANT02" {
				t.Errorf("RuleID = %q, want MDL-GRANT02", got[0].RuleID)
			}
			if got[0].Severity != linter.SeverityError {
				t.Errorf("Severity = %v, want error — exec refuses this", got[0].Severity)
			}
			if !strings.Contains(got[0].Message, "Wide") {
				t.Errorf("message must name the offending role, got: %s", got[0].Message)
			}
			if got[0].Suggestion == "" {
				t.Error("a violation the user must act on needs a suggestion")
			}
		})
	}
}

// The qualified form is the correct one and must stay silent, or the rule just
// trains people to ignore it.
func TestValidateGrantRoles_QualifiedRoleIsClean(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.GrantEntityAccessStmt{
			Entity: qualRole("Sales", "Order"), Roles: []ast.QualifiedName{qualRole("Sales", "Wide")},
		},
		&ast.CreateUserRoleStmt{
			Name: "Admin", ModuleRoles: []ast.QualifiedName{qualRole("Sales", "Wide"), qualRole("HR", "Reader")},
		},
	}}
	if got := ValidateGrantRoles(prog); len(got) != 0 {
		t.Errorf("qualified roles must not be reported, got %d: %+v", len(got), got)
	}
}

// An unqualified role on a DOCUMENT grant used to trip MDL-GRANT01, whose
// message says the role belongs to another module and tells the reader to pick
// one from the document's own module. That diagnosis is wrong — there is no
// other module, only a missing qualifier — and the advice does not fix it. The
// qualification check has to win, or the report sends people the wrong way.
func TestValidateGrantRoles_UnqualifiedBeatsCrossModuleDiagnosis(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.GrantMicroflowAccessStmt{
			Microflow: qualRole("Sales", "MF"), Roles: []ast.QualifiedName{bareRole("Wide")},
		},
	}}
	got := ValidateGrantRoles(prog)
	if len(got) != 1 {
		t.Fatalf("got %d violations, want exactly 1 (not both rules): %+v", len(got), got)
	}
	if got[0].RuleID != "MDL-GRANT02" {
		t.Fatalf("RuleID = %q, want MDL-GRANT02 — the cross-module message misdiagnoses this", got[0].RuleID)
	}
	if strings.Contains(got[0].Message, "CE0148") {
		t.Error("this is not the cross-module defect; the message must not cite CE0148")
	}
}

// A cross-module grant is still MDL-GRANT01. The new rule must not swallow it.
func TestValidateGrantRoles_CrossModuleStillReported(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.GrantMicroflowAccessStmt{
			Microflow: qualRole("Sales", "MF"), Roles: []ast.QualifiedName{qualRole("HR", "Reader")},
		},
	}}
	got := ValidateGrantRoles(prog)
	if len(got) != 1 || got[0].RuleID != "MDL-GRANT01" {
		t.Fatalf("cross-module grant must stay MDL-GRANT01, got: %+v", got)
	}
}
