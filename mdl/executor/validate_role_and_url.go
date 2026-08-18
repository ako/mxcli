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
// entities, so the app is unusable for anyone holding only that role. The
// remedy is always the same — add System.User — which is why this is worth
// saying at authoring time rather than after a build.
func ValidateUserRoleSystemModuleRole(stmt *ast.CreateUserRoleStmt) []linter.Violation {
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
	return []linter.Violation{{
		RuleID:   "MDL-SEC20",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"user role %q has no System module role, so nobody holding it can sign in or read "+
				"System entities (MxBuild reports this as CE0156). Add System.User: "+
				"CREATE USER ROLE %s (%s, System.User)",
			stmt.Name, stmt.Name, joinQualified(stmt.ModuleRoles)),
	}}
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
