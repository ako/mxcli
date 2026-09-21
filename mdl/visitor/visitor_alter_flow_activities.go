// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// exitAlterFlowActivitiesStatement builds
//
//	ALTER MICROFLOW Mod.Flow  DISABLE ACTIVITIES WHERE …
//	ALTER MICROFLOWS [IN Mod] ENABLE  ACTIVITIES WHERE …
func (b *Builder) exitAlterFlowActivitiesStatement(ctx *parser.AlterFlowActivitiesStatementContext) {
	stmt := &ast.AlterFlowActivitiesStmt{}

	switch {
	case ctx.FlowFlavourSingular() != nil:
		stmt.Flavour = flavourFromText(ctx.FlowFlavourSingular().GetText())
		if qn := ctx.QualifiedName(); qn != nil {
			stmt.Name = buildQualifiedName(qn)
		}
	case ctx.FlowFlavourPlural() != nil:
		stmt.Bulk = true
		stmt.Flavour = flavourFromText(ctx.FlowFlavourPlural().GetText())
		if id := ctx.IdentifierOrKeyword(); id != nil {
			stmt.Module = identifierOrKeywordText(id)
		}
	}

	if t := ctx.ActivityToggle(); t != nil {
		stmt.Disable = strings.EqualFold(t.GetText(), "disable")
	}
	if f := ctx.ActivityFilter(); f != nil {
		stmt.Filter = buildActivityFilter(f.(*parser.ActivityFilterContext))
	}

	b.statements = append(b.statements, stmt)
}

// flavourFromText maps the keyword to the flavour, singular or plural.
func flavourFromText(text string) ast.FlowFlavour {
	switch strings.ToLower(strings.TrimSuffix(strings.ToLower(text), "s")) {
	case "nanoflow":
		return ast.FlowNanoflow
	case "rule":
		return ast.FlowRule
	default:
		return ast.FlowMicroflow
	}
}

// buildActivityFilter reads the WHERE into the shared filter type. Nothing is
// validated here — CheckActivityFilter owns that, so `check` and `exec` apply
// one set of rules — but nothing is dropped either: an unknown column reaches
// the AST and is reported by name.
func buildActivityFilter(ctx *parser.ActivityFilterContext) types.ActivityFilter {
	var f types.ActivityFilter
	for _, c := range ctx.AllActivityCondition() {
		f.Conditions = append(f.Conditions, buildActivityCondition(c.(*parser.ActivityConditionContext)))
	}
	return f
}

func buildActivityCondition(ctx *parser.ActivityConditionContext) types.ActivityCondition {
	cond := types.ActivityCondition{}
	if col := ctx.ActivityColumn(); col != nil {
		cond.Column = strings.ToLower(col.GetText())
	}
	switch {
	case ctx.LIKE() != nil:
		cond.Like = true
		if lit := ctx.STRING_LITERAL(); lit != nil {
			cond.Values = []string{unquoteString(lit.GetText())}
		}
	case ctx.IN() != nil:
		cond.Negate = ctx.NOT() != nil
		for _, v := range ctx.AllActivityValue() {
			cond.Values = append(cond.Values, activityValueText(v.(*parser.ActivityValueContext)))
		}
	default:
		cond.Negate = ctx.NOT_EQUALS() != nil
		if vs := ctx.AllActivityValue(); len(vs) > 0 {
			cond.Values = []string{activityValueText(vs[0].(*parser.ActivityValueContext))}
		}
	}
	return cond
}

// activityValueText reads one value, quoted or bare. A quoted value is the
// spelling for anything with a space in it — `'call javascript action'` — so
// the quotes are stripped here and the word reaches the filter as written.
func activityValueText(ctx *parser.ActivityValueContext) string {
	if lit := ctx.STRING_LITERAL(); lit != nil {
		return unquoteString(lit.GetText())
	}
	if id := ctx.IdentifierOrKeyword(); id != nil {
		return identifierOrKeywordText(id)
	}
	return strings.TrimSpace(ctx.GetText())
}
