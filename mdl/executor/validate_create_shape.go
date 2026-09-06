// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Two statements that `mxcli check` accepted and `exec` or the build refused.
// Both are answerable from the statement alone, so they run in the no-project
// pass: a divergence that only `-p` catches is still a divergence.

const (
	unqualifiedCreateRule = "MDL074"
	voidReturnAliasRule   = "MDL075"
)

// ValidateCreateIsQualified reports a CREATE whose target name carries no
// module.
//
// `exec` refuses it — "module name is required: objects must be created within
// a module (use ModuleName.ObjectName syntax)" — and `check` passed it, so a
// script stopped partway through with the statements before it already applied
// and none after (mendixlabs/mxcli#1050). exec is not transactional, so "run it
// again" then hits "already exists" on the ones that did land.
//
// The rule is the whole reason MDL insists on qualified names: an unqualified
// one has no module to be created in, and there is no sensible default — the
// first module, the last one created, and "the only one" are all guesses that
// would put a document somewhere the author did not say.
//
// A MODULE is the exception and is excluded rather than special-cased at the
// call site: a module has nothing to be qualified BY.
func ValidateCreateIsQualified(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		if _, ok := stmt.(*ast.CreateModuleStmt); ok {
			continue
		}
		docType, name, _ := stmtCreateInfo(stmt)
		if docType == "" || name == "" || strings.Contains(name, ".") {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   unqualifiedCreateRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s %q has no module — every document is created inside one, so `exec` "+
					"refuses this with \"module name is required\". Write Module.%s",
				docType, name, name),
			Location: linter.Location{DocumentType: docType, DocumentName: name},
			Suggestion: fmt.Sprintf(
				"Qualify it: `Module.%s`. Reported here rather than at exec time because "+
					"exec is not transactional — the statements before this one are already "+
					"applied when it fails, and re-running then hits \"already exists\" on them.",
				name),
		})
	}
	return out
}

// ValidateVoidReturnAlias reports `RETURNS void AS $x`.
//
// The alias names the variable a flow returns, so pairing it with void is a
// contradiction — and mxcli believed the alias: it wrote `return $x` into a flow
// with no such variable, which the build rejects (mendixlabs/mxcli#1041):
//
//	CREATE MICROFLOW … RETURNS void AS $result BEGIN COMMIT $Customer; END;
//	mxcli check --references -> exit 0
//	mx check                 -> [CE0109] "Undefined variable 'result'." at End event
//
// Refused rather than repaired. Emitting a bare `return` would also build, but
// the two spellings mean different things to the author — one of them wrote an
// alias on purpose and meant to return something — and silently dropping it is
// how a flow comes to return nothing while its source still says otherwise.
// `RETURNS void` alone round-trips cleanly and is the fix when void was meant.
func ValidateVoidReturnAlias(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		var name, kind string
		var rt *ast.MicroflowReturnType
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			name, kind, rt = s.Name.String(), "microflow", s.ReturnType
		case *ast.CreateNanoflowStmt:
			name, kind, rt = s.Name.String(), "nanoflow", s.ReturnType
		default:
			continue
		}
		if rt == nil || rt.Variable == "" || !isVoidReturn(rt) {
			continue
		}
		alias := strings.TrimPrefix(rt.Variable, "$")
		out = append(out, linter.Violation{
			RuleID:   voidReturnAliasRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s %s is declared `RETURNS void AS $%s` — an alias names the variable the "+
					"flow returns, so it cannot be paired with void. mxcli writes `return $%s` "+
					"into a flow that has no such variable, which the build rejects as CE0109 "+
					"\"Undefined variable '%s'\"",
				kind, name, alias, alias, alias),
			Location: linter.Location{DocumentType: kind, DocumentName: name},
			Suggestion: fmt.Sprintf(
				"Drop the alias (`RETURNS void`) if the flow returns nothing, or give it the "+
					"type $%s actually holds (`RETURNS <type> AS $%s`).", alias, alias),
		})
	}
	return out
}

// isVoidReturn reports whether a declared return type means "returns nothing".
//
// A nil clause is a flow with no RETURNS at all, which cannot carry an alias, so
// the caller's Variable check is what does the selecting.
func isVoidReturn(rt *ast.MicroflowReturnType) bool {
	return rt == nil || rt.Type.Kind == ast.TypeVoid
}
