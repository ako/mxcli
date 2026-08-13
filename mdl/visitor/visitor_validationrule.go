// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateValidationRuleStatement builds a CreateValidationRuleStmt from
// CREATE VALIDATION RULE FOR Module.Entity.Attribute <constraint> FEEDBACK '...'.
func (b *Builder) ExitCreateValidationRuleStatement(ctx *parser.CreateValidationRuleStatementContext) {
	stmt := &ast.CreateValidationRuleStmt{
		Attribute: buildQualifiedName(ctx.QualifiedName()),
	}
	if s := ctx.STRING_LITERAL(); s != nil {
		stmt.Feedback = unquoteString(s.GetText())
	}

	constraint, ok := ctx.ValidationRuleConstraint().(*parser.ValidationRuleConstraintContext)
	if !ok || constraint == nil {
		return
	}

	switch {
	case constraint.REGEX() != nil:
		stmt.Kind = ast.ValidationRuleRegEx
		stmt.RegularExpression = buildQualifiedName(constraint.QualifiedName())

	case constraint.RANGE() != nil:
		stmt.Kind = ast.ValidationRuleRange
		rng, ok := constraint.ValidationRuleRange().(*parser.ValidationRuleRangeContext)
		if !ok || rng == nil {
			return
		}
		// The grammar admits `from X to Y`, `from X` and `to Y`. Which bounds
		// are present is the whole signal — it decides Mendix's TypeOfRange —
		// so read the FROM/TO tokens rather than counting literals, which
		// cannot tell a lone `to Y` from a lone `from X`.
		lits := rng.AllLiteral()
		if rng.FROM() != nil && len(lits) > 0 {
			v := lits[0].GetText()
			stmt.Min = &v
		}
		if rng.TO() != nil && len(lits) > 0 {
			v := lits[len(lits)-1].GetText()
			stmt.Max = &v
		}
	}

	b.statements = append(b.statements, stmt)
}
