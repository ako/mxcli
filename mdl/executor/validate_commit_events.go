// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// MDL067 is a one-release migration note for #895.
//
// Before the fix, a bare `commit $X;` stored WithEvents=false. Studio Pro's
// default is true — measured on a Commit activity dropped into ako/TestApp with
// its properties untouched (CommitActivity.DefaultCommit stores WithEvents=true,
// RefreshInClient=false; the sibling CommitNoEventsWithRefresh stores the
// inverse, so both spellings are written and neither is an omission Mendix fills
// in). mxcli now matches Studio Pro, which means the same bare statement writes
// the opposite value it used to.
//
// Why this needs saying at all: nothing else would say it. The old behaviour
// passed `mxcli check`, `mxcli lint`, Studio Pro's consistency check and
// `mxbuild` — a commit that skips its handlers is a perfectly valid model, so
// the only place the change surfaces is the running app. That is how the
// reporter found it: 1,800 rows written, no handler run, and a person using the
// app was the detector.
//
// Deliberately INFO and deliberately once per microflow. Every bare commit in
// every script ever written is "affected", so a per-statement error or warning
// would bury the scripts that are genuinely at risk under the overwhelming
// majority that simply get the correct behaviour they always assumed. Naming the
// document is the useful granularity: it is the unit the author re-runs.
//
// Only the author knows which they meant, so the note does not guess. It should
// be dropped once the release that carries the change is old news.
func (v *microflowValidator) checkBareCommitEvents(body []ast.MicroflowStatement) {
	bare := 0
	var walk func([]ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			switch st := s.(type) {
			case *ast.MfCommitStmt:
				// `with events` is explicit even though it is now the default:
				// the author said what they wanted, so there is nothing to warn
				// about. Only the silent form is ambiguous — and the visitor
				// cannot tell the two apart from WithoutEvents alone, so the
				// spelling is recovered from the source below.
				if !st.WithoutEvents && !st.ExplicitWithEvents {
					bare++
				}
			case *ast.LoopStmt:
				walk(st.Body)
			case *ast.WhileStmt:
				walk(st.Body)
			case *ast.IfStmt:
				walk(st.ThenBody)
				walk(st.ElseBody)
			case *ast.EnumSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			case *ast.InheritanceSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			}
		}
	}
	walk(body)

	if bare == 0 {
		return
	}
	subject := fmt.Sprintf("%d commit activities use", bare)
	if bare == 1 {
		subject = "1 commit activity uses"
	}
	v.addViolation("MDL067", linter.SeverityInfo,
		fmt.Sprintf("%s the default, which is now WITH EVENTS to match Studio Pro (#895); "+
			"before this release a bare `commit $X;` wrote events OFF", subject),
		"Write `commit $X without events;` for any commit here that must NOT run its event "+
			"handlers; otherwise no change is needed, and spelling the intent out silences this note")
}
