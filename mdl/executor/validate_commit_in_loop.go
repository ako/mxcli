// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// checkCommitInLoop (MDL-PERF01) reports a commit inside a loop, which is one
// database round trip per iteration.
//
// `lint` has known this as CONV011 since long before this rule, and that is the
// problem it exists to solve rather than duplicate: CONV011 reads the STORED
// model, so it can only speak after `exec` has written the microflow, and it has
// no document scope — answering for one microflow meant a project-wide lint
// (~13 s measured on a real project) plus a baseline diff to see what was new.
// At that price the gate gets batched to once per session, and on the project
// that reported it (upstream mendixlabs/mxcli#1186) three CONV011 violations
// shipped under a clean mxbuild log, six accumulating across three microflows
// before anyone looked.
//
// This one reads the MDL the author just wrote, before it is applied, so the
// answer arrives in the same call that would have written the defect. The two
// are complementary, not alternatives: this cannot see a microflow it is not
// being asked to write, and CONV011 cannot see one before it exists.
//
// **The boundary is deliberately CONV011's, not a better one.** A `while true`
// is built as an ExclusiveMerge back-edge rather than a LoopedActivity, so
// CONV011 — which walks LoopedActivity — does not flag a commit inside one, and
// neither does this. A commit there is arguably still N+1 at runtime, but two
// rules for one concept that disagree on what counts is how a pair like this
// starts drifting; if that case is worth reporting it is worth reporting in
// both, and CONV011 is the one that sees the built flow.
//
// A warning, not an error: committing per iteration is sometimes what the author
// means (a long-running job that must not lose work on failure). What it must not
// be is invisible.
func (v *microflowValidator) checkCommitInLoop(body []ast.MicroflowStatement) {
	var walk func(stmts []ast.MicroflowStatement, inLoop bool)
	walk = func(stmts []ast.MicroflowStatement, inLoop bool) {
		for _, s := range stmts {
			switch st := s.(type) {
			case *ast.MfCommitStmt:
				if inLoop {
					v.addViolation("MDL-PERF01", linter.SeverityWarning,
						fmt.Sprintf("commit of $%s is inside a loop, so it runs one database round trip "+
							"per iteration (`lint` reports this as CONV011)", st.Variable),
						fmt.Sprintf("Add $%s to a list inside the loop and commit the list once after it", st.Variable))
				}
			case *ast.LoopStmt:
				walk(st.Body, true)
			case *ast.WhileStmt:
				// See the note above: a `while true` is a back-edge, not a loop
				// object, and CONV011 does not count it either.
				walk(st.Body, inLoop || !isUnconditionalTrueWhile(st))
			case *ast.IfStmt:
				walk(st.ThenBody, inLoop)
				walk(st.ElseBody, inLoop)
			case *ast.EnumSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body, inLoop)
				}
				walk(st.ElseBody, inLoop)
			case *ast.InheritanceSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body, inLoop)
				}
				walk(st.ElseBody, inLoop)
			}
		}
	}
	walk(body, false)
}
