// SPDX-License-Identifier: Apache-2.0

// Two statically decidable rules that MxBuild reports and `mxcli check` did not.
//
// Both were found by a real project (mxcli-dbreplication, finding F10): four MDL
// scripts passed `mxcli check` with 0 errors and executed cleanly, and the first
// real validation then failed with three errors, two of them these. Neither is
// exotic, and neither needs the project — the MDL alone says everything.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// systemModuleName is the module whose roles make a user role able to sign in.
const systemModuleName = "System"

// ValidateUserRoleSystemModuleRole reports MDL-SEC20 (MxBuild CE0156) for a user
// role built only from application module roles.
//
// A user role with no System module role cannot sign in or touch System
// entities, so the app is unusable for anyone holding only that role.
//
// **It is an error only when security is on.** Measured on Mendix 11.13: the
// same role is CE0156 at security level Prototype and *no error at all* at
// level Off, where roles are stored but not validated. A blank project ships
// Off, so reporting this as an error unconditionally fails scripts that are
// perfectly valid for the project they target — it broke two of mxcli's own
// examples, which is how the over-reach was caught.
//
// So: an error when the script itself turns security on, because then the author
// has said which world they are in; a warning otherwise, since the script may
// well be applied to a project that has it on and the advice still holds.
func ValidateUserRoleSystemModuleRole(stmt *ast.CreateUserRoleStmt, securityEnabled bool) []linter.Violation {
	if stmt == nil || len(stmt.ModuleRoles) == 0 {
		// A role with no module roles at all is a different (and legitimate)
		// thing — a placeholder to be extended later by ALTER USER ROLE.
		return nil
	}
	for _, r := range stmt.ModuleRoles {
		if strings.EqualFold(r.Module, systemModuleName) {
			return nil
		}
	}
	severity := linter.SeverityWarning
	when := "if this project has security enabled, "
	if securityEnabled {
		severity = linter.SeverityError
		when = "this script enables security, so "
	}
	return []linter.Violation{{
		RuleID:   "MDL-SEC20",
		Severity: severity,
		Message: fmt.Sprintf(
			"user role %q has no System module role — %snobody holding it can sign in or read "+
				"System entities (MxBuild reports this as CE0156 once security is on; at security "+
				"level Off it is not flagged). Add System.User: CREATE USER ROLE %s (%s, System.User)",
			stmt.Name, when, stmt.Name, joinQualified(stmt.ModuleRoles)),
	}}
}

// programEnablesSecurity reports whether the script sets a project security
// level other than Off — the condition that makes CE0156 real.
func programEnablesSecurity(prog *ast.Program) bool {
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.AlterProjectSecurityStmt)
		if !ok {
			continue
		}
		if s.SecurityLevel != "" && !strings.EqualFold(s.SecurityLevel, "Off") {
			return true
		}
	}
	return false
}

func joinQualified(names []ast.QualifiedName) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = n.String()
	}
	return strings.Join(parts, ", ")
}

// ValidatePageURLParameters reports MDL-PAGE20 (MxBuild CE5601) when a page has
// both parameters and a URL, and the URL does not name every parameter.
//
// Mendix builds a deep link from the URL, so each parameter needs a `{Name}`
// segment to be bound from it. Without one the page cannot be opened by URL and
// the build fails. The check runs only when a URL is set: a page with parameters
// and no URL is perfectly normal.
func ValidatePageURLParameters(stmt *ast.CreatePageStmtV3) []linter.Violation {
	if stmt == nil || stmt.URL == "" || len(stmt.Parameters) == 0 {
		return nil
	}
	var missing []string
	for _, p := range stmt.Parameters {
		if p.Name == "" {
			continue
		}
		if !urlBindsParameter(stmt.URL, p.Name) {
			missing = append(missing, p.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	suggested := stmt.URL
	for _, name := range missing {
		suggested = strings.TrimRight(suggested, "/") + "/{" + name + "}"
	}
	return []linter.Violation{{
		RuleID:   "MDL-PAGE20",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"page %s has a Url but no segment for parameter %s — Mendix binds each page "+
				"parameter from the URL, so the page cannot be opened by link and the build "+
				"fails (MxBuild reports this as CE5601). Try Url: '%s'",
			stmt.Name.String(), quoteList(missing), suggested),
	}}
}

// urlBindsParameter reports whether the URL has a segment binding this
// parameter. Mendix allows an attribute path inside the segment — the common
// form is `{Customer/Name}`, which binds the Customer parameter by one of its
// attributes — so matching `{Name}` exactly would flag correct URLs. The match
// is on the segment's leading identifier.
func urlBindsParameter(url, param string) bool {
	rest := url
	for {
		open := strings.Index(rest, "{")
		if open < 0 {
			return false
		}
		rest = rest[open+1:]
		close := strings.Index(rest, "}")
		if close < 0 {
			return false
		}
		seg := rest[:close]
		rest = rest[close+1:]
		if head, _, _ := strings.Cut(seg, "/"); strings.EqualFold(strings.TrimPrefix(head, "$"), param) {
			return true
		}
	}
}

func quoteList(names []string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = fmt.Sprintf("%q", n)
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}
