// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for statements that name module roles.
//
// Two rules live here, both answerable from the script text alone:
//
//   - MDL-GRANT01 — a page/microflow/nanoflow/service granted to a role from a
//     different module. Mendix stores document access as references to the
//     document's OWN module roles only, so this builds with CE0148 ("reselect
//     roles"). See issue #836.
//   - MDL-GRANT02 — a module role named without its module qualifier. See issue
//     mendixlabs/mxcli#1067.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateGrantRoles reports module-role defects that need no project:
// MDL-GRANT02 for an unqualified role, MDL-GRANT01 for a cross-module document
// grant.
//
// This lives in the no-project pass rather than the --references pass on
// purpose: the checks compare names already present in the script, so requiring
// -p would withhold an answer mxcli can always give. It also means a plain
// `mxcli check` catches them, not only `check --references`.
//
// The two rules are ordered, not combined. An unqualified role has no module to
// compare, so the cross-module check would read the empty module as "some other
// module" and report a mismatch — a true-sounding message with the wrong
// diagnosis and advice that does not fix it. Qualification is therefore settled
// first, and a statement reported for MDL-GRANT02 is not tested for MDL-GRANT01.
func ValidateGrantRoles(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		if v := validateRoleQualification(stmt); len(v) > 0 {
			out = append(out, v...)
			continue
		}
		if err := validateCrossModuleGrant(stmt); err != nil {
			out = append(out, linter.Violation{
				RuleID:   "MDL-GRANT01",
				Severity: linter.SeverityError,
				Message:  err.Error(),
				Suggestion: "Grant a module role from the document's own module, then map the user role to it " +
					"(`alter user role <UserRole> add <DocModule>.<Role>`).",
			})
		}
	}
	return out
}

// moduleRoleList returns the module roles a statement names, and a label for
// the statement to put in a message.
//
// Every statement built from the grammar's `moduleRoleList` belongs here. The
// entity grant is the one that made this a bug: MDL-GRANT01 was written for the
// five document grants, so `grant Wide on Sales.Order (...)` — the commonest
// GRANT of all — had no check-time coverage at all, and neither did the
// workflow grant or either user-role statement.
func moduleRoleList(stmt ast.Statement) (roles []ast.QualifiedName, what string) {
	switch s := stmt.(type) {
	case *ast.GrantEntityAccessStmt:
		return s.Roles, "grant on " + s.Entity.String()
	case *ast.GrantMicroflowAccessStmt:
		return s.Roles, "grant execute on microflow " + s.Microflow.String()
	case *ast.GrantNanoflowAccessStmt:
		return s.Roles, "grant execute on nanoflow " + s.Nanoflow.String()
	case *ast.GrantPageAccessStmt:
		return s.Roles, "grant view on page " + s.Page.String()
	case *ast.GrantWorkflowAccessStmt:
		return s.Roles, "grant execute on workflow " + s.Workflow.String()
	case *ast.GrantODataServiceAccessStmt:
		return s.Roles, "grant access on OData service " + s.Service.String()
	case *ast.GrantPublishedRestServiceAccessStmt:
		return s.Roles, "grant access on published REST service " + s.Service.String()
	case *ast.CreateUserRoleStmt:
		return s.ModuleRoles, "create user role " + s.Name
	case *ast.AlterUserRoleStmt:
		return s.ModuleRoles, "alter user role " + s.Name
	}
	return nil, ""
}

// validateRoleQualification reports (MDL-GRANT02) a module role written without
// its module.
//
// The grammar spells a module role as `qualifiedName`, whose module part is
// optional, so `Wide` parses as cleanly as `Sales.Wide` and reaches the executor
// with an empty Module. What happens next depends on the statement, and neither
// outcome is acceptable from a script that just passed `check`:
//
//   - a GRANT fails at exec, by which point every earlier statement has already
//     been written (mxcli does not run a script in one transaction);
//   - `create user role R (Wide)` does not fail at all. It stores the reference
//     as ".Wide" and reports success, and the defect surfaces only when mxbuild
//     refuses the project with CE1613 "The selected module role '.Wide' no
//     longer exists."
func validateRoleQualification(stmt ast.Statement) []linter.Violation {
	roles, what := moduleRoleList(stmt)
	if what == "" {
		return nil
	}
	var out []linter.Violation
	for _, role := range roles {
		if role.Module != "" {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-GRANT02",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s names the module role %q without a module — a module role is always "+
					"Module.Role, and mxcli cannot tell which module %q belongs to",
				what, role.Name, role.Name),
			Suggestion: fmt.Sprintf(
				"Qualify it, e.g. `<Module>.%s`. Run `show module roles` to list the roles a project has.",
				role.Name),
		})
	}
	return out
}
