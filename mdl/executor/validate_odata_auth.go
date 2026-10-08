// SPDX-License-Identifier: Apache-2.0

// Check-time validation of a published OData service's authentication.
//
// Custom authentication is the only method that names a target: a microflow
// taking the request's HttpHeader list and returning a User. Mendix rejects the
// service without one — CE0333 "Please select a microflow to use for
// authentication" — and the grammar makes the name optional, so `authentication
// microflow` alone parses and executes into a model that cannot build.
//
// mxcli-formula1 §40.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateODataAuth flags a Microflow authentication method with no microflow.
func ValidateODataAuth(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		var name ast.QualifiedName
		var types []string
		var mf string
		switch s := stmt.(type) {
		case *ast.CreateODataServiceStmt:
			name, types, mf = s.Name, s.AuthenticationTypes, s.AuthMicroflow
		case *ast.AlterODataServiceStmt:
			name, types, mf = s.Name, s.AuthenticationTypes, s.AuthMicroflow
		default:
			continue
		}
		if !namesMicroflowAuth(types) || mf != "" {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-ODATA04",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf("odata service %s: the `microflow` authentication method names no microflow",
				name.String()),
			Suggestion: "Custom authentication is the only method that carries a target — write " +
				"`Authentication: (microflow Module.Authenticate)`. Mendix refuses to build the " +
				"service otherwise (CE0333 \"Please select a microflow to use for authentication\"). " +
				"The microflow takes the request's System.HttpHeader list and returns a " +
				"System.User; returning empty denies the request.",
		})
	}
	return out
}

// namesMicroflowAuth reports whether the methods include custom authentication.
func namesMicroflowAuth(types []string) bool {
	for _, t := range types {
		if strings.EqualFold(t, "Microflow") {
			return true
		}
	}
	return false
}
