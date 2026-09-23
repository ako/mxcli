// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"

	"github.com/antlr4-go/antlr/v4"

	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// A workflow's header clauses and a user task's clauses used to be a fixed
// SEQUENCE of optional groups in the grammar, which made each clause optional
// but its POSITION mandatory — `on created microflow` written anywhere but
// between the targeting clauses and `entity` failed with `mismatched input 'ON'
// expecting ';'`, naming neither the clause nor the rule (ako/mxcli#586).
//
// The grammar now takes them as a set. That alone would have RELAXED the
// language in a second way nobody asked for: `page M.A page M.B` would parse
// and the second would silently win. A clause written twice is a better error
// than a clause written out of order, not a licence to accept it, so the
// at-most-once half of the old rule is enforced here — where the message can
// name the clause, which is what the token error could not do.
//
// Three clauses are deliberately exempt because they are list-valued and
// accumulate: `outcomes`, `boundary event` and the header's workflow event
// handlers. A task may carry several boundary events, and under the old grammar
// they were already spelled as a repeated `boundary event` inside one clause.
func (b *Builder) checkWorkflowClausesAtMostOnce(node antlr.Tree) {
	if node == nil {
		return
	}
	owner := workflowClauseOwnerDescription(node)
	seen := make(map[string]int)
	for i := 0; i < node.GetChildCount(); i++ {
		child := node.GetChild(i)
		kind, tok := workflowClauseKind(child)
		if kind != "" && tok != nil {
			if first, dup := seen[kind]; dup {
				b.addError(fmt.Errorf(
					"line %d:%d: duplicate %s clause on %s (already given on line %d) — "+
						"each clause may appear at most once, in any order",
					tok.GetLine(), tok.GetColumn(), kind, owner, first))
			} else {
				seen[kind] = tok.GetLine()
			}
		}
		b.checkWorkflowClausesAtMostOnce(child)
	}
}

// workflowClauseKind names the clause a node spells, in the words the author
// wrote, or "" when the node is not a single-valued workflow clause.
func workflowClauseKind(node antlr.Tree) (string, antlr.Token) {
	switch c := node.(type) {
	case *parser.WorkflowHeaderClauseContext:
		return workflowHeaderClauseKind(c), c.GetStart()
	case *parser.WorkflowUserTaskClauseContext:
		return workflowUserTaskClauseKind(c), c.GetStart()
	case *parser.WorkflowMultiUserTaskClauseContext:
		// The common clauses are wrapped one level deeper here, so unwrap rather
		// than grouping them under the wrapper — otherwise every multi user task
		// clause would sit alone in its own group and no duplicate could be seen.
		switch {
		case c.WorkflowUserTaskClause() != nil:
			if inner, ok := c.WorkflowUserTaskClause().(*parser.WorkflowUserTaskClauseContext); ok {
				return workflowUserTaskClauseKind(inner), c.GetStart()
			}
		case c.WorkflowParticipantsClause() != nil:
			return "PARTICIPANTS", c.GetStart()
		case c.WorkflowCompletionClause() != nil:
			return "DECIDE BY", c.GetStart()
		case c.AWAIT() != nil:
			return "AWAIT ALL USERS", c.GetStart()
		}
	}
	return "", nil
}

func workflowHeaderClauseKind(c *parser.WorkflowHeaderClauseContext) string {
	switch {
	case c.FOLDER() != nil:
		return "FOLDER"
	case c.PARAMETER() != nil:
		return "PARAMETER"
	case c.DISPLAY() != nil:
		return "DISPLAY"
	case c.DESCRIPTION() != nil:
		return "DESCRIPTION"
	case c.EXPORT() != nil:
		return "EXPORT LEVEL"
	case c.OVERVIEW() != nil:
		return "OVERVIEW PAGE"
	case c.DUE() != nil:
		return "DUE DATE"
	}
	// Event handlers accumulate.
	return ""
}

func workflowUserTaskClauseKind(c *parser.WorkflowUserTaskClauseContext) string {
	switch {
	case c.PAGE() != nil:
		return "PAGE"
	case c.TARGETING() != nil:
		// Both spellings fill the SAME slot — a user task stores one UserSource
		// — so `targeting microflow … targeting xpath …` is a duplicate, not two
		// clauses. Under the sequence grammar both were accepted and the second
		// silently won, which is the order-dependence of #586 in its most
		// damaging form: a task targeted by the clause the author wrote last.
		return "TARGETING"
	case c.ON() != nil && c.CREATED() != nil:
		return "ON CREATED MICROFLOW"
	case c.ENTITY() != nil:
		return "ENTITY"
	case c.DUE() != nil:
		return "DUE DATE"
	case c.DESCRIPTION() != nil:
		return "DESCRIPTION"
	case c.OUTCOMES() != nil:
		return "OUTCOMES"
	}
	// Boundary events accumulate.
	return ""
}

// workflowClauseOwnerDescription names the thing the clauses belong to, so the
// error reads "on user task ut1" rather than pointing at a rule name.
func workflowClauseOwnerDescription(node antlr.Tree) string {
	switch c := node.(type) {
	case *parser.CreateWorkflowStatementContext:
		if qn := c.QualifiedName(); qn != nil {
			return "workflow " + qn.GetText()
		}
		return "the workflow"
	case *parser.WorkflowUserTaskStmtContext:
		name := ""
		if id := c.IDENTIFIER(); id != nil {
			name = id.GetText()
		} else if qid := c.QUOTED_IDENTIFIER(); qid != nil {
			name = unquoteIdentifier(qid.GetText())
		}
		kind := "user task"
		if c.MULTI() != nil {
			kind = "multi user task"
		}
		if name == "" {
			return kind
		}
		return kind + " " + name
	}
	return "this statement"
}
