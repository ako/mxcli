// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateMicroflowDocumentProperties (MDL-MF01…MF03) reports the platform
// rules on a microflow's URL, export level and concurrency clauses.
//
// It calls the SAME function the writer calls — MicroflowDocumentPropertyProblems,
// over the rules in mdl/types — because a script that `mxcli check` accepts and
// `exec` then refuses is the drift the layout-placeholder rule was moved into
// mdl/types to prevent (mendixlabs/mxcli#1063). Check reports every problem with
// a rule ID; exec refuses on the first.
//
// The rules themselves, and why each is a rule rather than a preference:
//
//	MDL-MF01  a {Name} placeholder must name a parameter of this microflow
//	MDL-MF02  a path parameter may not also be a search parameter → CE5612
//	MDL-MF03  DISALLOW CONCURRENT EXECUTION needs a handler      → CE4899
//
// MF02 is the one that is easy to get wrong and impossible to see without a
// build: it was found by seeding a real project and running mx check, after the
// first version of this feature's own test fixture used one parameter for both
// and described a document Mendix refuses to build.
func validateMicroflowDocumentProperties(stmt ast.Statement) []linter.Violation {
	mf, ok := stmt.(*ast.CreateMicroflowStmt)
	if !ok {
		return nil
	}
	problems := MicroflowDocumentPropertyProblems(mf)
	if len(problems) == 0 {
		return nil
	}
	out := make([]linter.Violation, 0, len(problems))
	for _, p := range problems {
		out = append(out, linter.Violation{
			RuleID:   ruleIDForProblem(p),
			Severity: linter.SeverityError,
			Message:  fmt.Sprintf("microflow %s: %s", mf.Name.String(), p),
		})
	}
	return out
}

// ruleIDForProblem maps a problem sentence to its rule ID. The mapping is on the
// sentence rather than a typed error because the rules live in mdl/types, which
// must not depend on the linter's vocabulary — the same separation that lets the
// writer use them without importing the checker.
func ruleIDForProblem(problem string) string {
	switch {
	case strings.Contains(problem, "CE5612"):
		return "MDL-MF02"
	case strings.Contains(problem, "CE4899"):
		return "MDL-MF03"
	case strings.Contains(problem, "MicroflowsExportLevel"):
		return "MDL-MF04"
	default:
		return "MDL-MF01"
	}
}
