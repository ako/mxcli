// SPDX-License-Identifier: Apache-2.0

// Reference validation for the user role in `HOME PAGE … FOR <role>`.
//
// The role was written through to BSON verbatim, and the damage depends on the
// shape of what was written (measured on a blank Mendix 11.13 app):
//
//   - `for Administrator` — a real user role — builds at 0 errors.
//
//   - `for Supervisor` — bare, nonexistent — is CE1613 "The selected user role
//     'Supervisor' no longer exists", a normal build error.
//
//   - `for MyFirstModule.Administrator` — module-qualified — does not reach the
//     checker at all. Mendix fails to LOAD the project.
//
// The load failure verbatim (indented so gofmt does not reflow the quoting):
//
//	StorageLoadException: Role based home page in  has an invalid value ''
//	for property UserRole. The text 'MyFirstModule.Administrator' is not a
//	valid UserRoleIdentifier.
//
// The third is the one this exists for, and it was the form mxcli's own
// documentation recommended (mendixlabs/mxcli#1001). A user role is
// project-level, so its identifier is a bare name; a module role is
// module-scoped and looks almost identical — a blank app has a user role
// `Administrator` and module roles named `Administrator` in three modules, so
// the wrong one reads as correct.
//
// A load failure is worse than a build error: it happens before any checking
// runs, so there is no error code and no location, and `mx check` exits 1
// without the "The app contains: N errors" line that people read as the verdict.
//
// The roles live in the project, so this runs in the --references pass. It is
// also called from the navigation handler, so `exec` refuses what `check`
// refuses rather than writing a project Mendix cannot open.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// navRoleRef is one `FOR <role>` reference and where it was written.
type navRoleRef struct {
	role    ast.QualifiedName
	profile string
}

// navigationRoleRefs collects the role references in one statement.
func navigationRoleRefs(stmt ast.Statement) []navRoleRef {
	s, ok := stmt.(*ast.AlterNavigationStmt)
	if !ok {
		return nil
	}
	var out []navRoleRef
	for _, hp := range s.HomePages {
		if hp.ForRole == nil {
			continue
		}
		out = append(out, navRoleRef{role: *hp.ForRole, profile: s.ProfileName})
	}
	return out
}

// scriptUserRoles collects the user roles the script itself creates.
//
// Without this a script that creates a role and then uses it — the ordinary way
// to write one — would be refused for naming a role that does not exist *yet*.
// The whole-script scriptContext does not track user roles, so they are
// gathered here.
func scriptUserRoles(prog *ast.Program) []string {
	var out []string
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateUserRoleStmt); ok && s.Name != "" {
			out = append(out, s.Name)
		}
	}
	return out
}

// validateNavigationRoles resolves every `HOME PAGE … FOR` role in the program.
func validateNavigationRoles(ctx *ExecContext, prog *ast.Program) []error {
	if !ctx.Connected() {
		return nil
	}

	// Collect first: a program with no role-based home page must not pay for
	// reading project security.
	type located struct {
		ref   navRoleRef
		index int
	}
	var refs []located
	for i, stmt := range prog.Statements {
		for _, ref := range navigationRoleRefs(stmt) {
			refs = append(refs, located{ref: ref, index: i + 1})
		}
	}
	if len(refs) == 0 {
		return nil
	}

	known, err := projectUserRoles(ctx)
	if err != nil {
		// Reading security failed. Reporting every role as unknown on the back of
		// that would be worse than not checking.
		return nil
	}
	known = append(known, scriptUserRoles(prog)...)

	var errs []error
	for _, r := range refs {
		if err := checkNavigationRole(r.ref, known); err != nil {
			errs = append(errs, fmt.Errorf("statement %d: %w", r.index, err))
		}
	}
	return errs
}

// validateNavigationRoleForExec is the same check at execution time. By the time
// a navigation statement runs, a role the script creates is already in the
// project, so the stored roles alone are the right set.
func validateNavigationRoleForExec(ctx *ExecContext, stmt *ast.AlterNavigationStmt) error {
	refs := navigationRoleRefs(stmt)
	if len(refs) == 0 {
		return nil
	}
	known, err := projectUserRoles(ctx)
	if err != nil {
		return nil
	}
	for _, ref := range refs {
		if err := checkNavigationRole(ref, known); err != nil {
			return err
		}
	}
	return nil
}

func projectUserRoles(ctx *ExecContext) ([]string, error) {
	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ps.UserRoles))
	for _, ur := range ps.UserRoles {
		names = append(names, ur.Name)
	}
	return names, nil
}

// checkNavigationRole resolves one reference against the project's user roles.
//
// The two failures need different messages because they need different fixes: a
// module-qualified name is the right role written the wrong way, and an unknown
// bare name is the wrong role.
func checkNavigationRole(ref navRoleRef, known []string) error {
	where := "home page"
	if ref.profile != "" {
		where = fmt.Sprintf("navigation profile %q home page", ref.profile)
	}

	if ref.role.Module != "" {
		msg := fmt.Sprintf(
			"%s: %s is a module-qualified name, but FOR takes a USER role, which is "+
				"project-level and has no module part",
			where, ref.role.String())
		// The bare half is usually the role they meant — a blank app has a user
		// role Administrator and module roles of the same name in three modules.
		if match := matchRole(ref.role.Name, known); match != "" {
			msg += fmt.Sprintf(". Write: for %s", match)
		} else {
			msg += fmt.Sprintf(". Project user roles: %s", listRoles(known))
		}
		msg += ".\n  Mendix cannot LOAD a project with a qualified name here " +
			"(StorageLoadException: not a valid UserRoleIdentifier) — it fails before " +
			"checking runs, so there is no error code and no line number"
		return mdlerrors.NewValidation(msg)
	}

	match := matchRole(ref.role.Name, known)
	if match == ref.role.Name {
		return nil
	}
	if match != "" {
		// Only the casing differs. Mendix matches the role name exactly: measured,
		// `for administrator` is CE1613 on an app whose role is `Administrator`.
		return mdlerrors.NewValidation(fmt.Sprintf(
			"%s: user role %q differs in case from %q — Mendix matches the name exactly "+
				"(MxBuild reports this as CE1613). Write: for %s",
			where, ref.role.Name, match, match))
	}
	return mdlerrors.NewNotFoundMsg("user role", ref.role.Name, fmt.Sprintf(
		"%s: user role %q does not exist (MxBuild reports this as CE1613).\n"+
			"  Project user roles: %s",
		where, ref.role.Name, listRoles(known)))
}

// matchRole finds the declared spelling of a role, ignoring case. Returning the
// project's spelling rather than a bool is what lets the caller say which
// casing is wanted.
func matchRole(name string, known []string) string {
	for _, k := range known {
		if strings.EqualFold(k, name) {
			return k
		}
	}
	return ""
}

func listRoles(known []string) string {
	if len(known) == 0 {
		return "(none defined)"
	}
	sorted := append([]string(nil), known...)
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
}
