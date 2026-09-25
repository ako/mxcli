// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"

	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// The consumed OData client's expression-typed properties. Each holds ONE Mendix
// expression, which MDL writes as-is: `HttpUsername: 'admin'` is the string
// 'admin' (the value Studio Pro stores, quotes included), `@Mod.Const` reads a
// constant, and `'Bearer ' + @Mod.Token` concatenates. Header values are the
// same kind. PROPOSAL_first_class_expressions.md §6.4.
var odataClientExpressionProps = map[string]bool{
	"httpusername":      true,
	"httppassword":      true,
	"clientcertificate": true,
}

func isODataClientExpressionProp(name string) bool {
	return odataClientExpressionProps[strings.ToLower(name)]
}

// ruleSourceText is the author's text for a rule, whitespace included — the
// characters between its first and last token in the input. GetText() would
// concatenate the tokens without the whitespace between them, turning
// `'a' + @M.C` into `'a'+@M.C` and `if $x then` into `if$xthen`.
func ruleSourceText(ctx antlr.ParserRuleContext) string {
	if ctx == nil {
		return ""
	}
	start, stop := ctx.GetStart(), ctx.GetStop()
	if start == nil || stop == nil || stop.GetStop() < start.GetStart() {
		return ctx.GetText()
	}
	return start.GetInputStream().GetText(start.GetStart(), stop.GetStop())
}

// odataExpressionValue returns the expression an OData client expression
// property holds, exactly as written, whichever grammar alternative matched it,
// and whether it is a single string literal.
func odataExpressionValue(valueCtx parser.IOdataPropertyValueContext, exprCtx parser.IExpressionContext) (string, bool) {
	if valueCtx != nil {
		vc := valueCtx.(*parser.OdataPropertyValueContext)
		return ruleSourceText(vc), vc.STRING_LITERAL() != nil
	}
	if exprCtx != nil {
		return ruleSourceText(exprCtx.(antlr.ParserRuleContext)), false
	}
	return "", false
}

// ExitOdataPropertyAssignment refuses an expression where only a plain value is
// read. The grammar admits `name: <expression>` for every OData property list
// because it cannot tell the names apart; without this, `Path: 'a' + 'b'` would
// reach a visitor that reads only the plain alternatives and store nothing.
func (b *Builder) ExitOdataPropertyAssignment(ctx *parser.OdataPropertyAssignmentContext) {
	if ctx.Expression() == nil {
		return
	}
	name := identifierOrKeywordText(ctx.IdentifierOrKeyword())
	if _, onClient := ctx.GetParent().(*parser.CreateODataClientStatementContext); onClient && isODataClientExpressionProp(name) {
		return
	}
	b.addError(odataExpressionNotAllowed(name, ctx.Expression()))
}

// ExitOdataAlterAssignment is the ALTER twin of ExitOdataPropertyAssignment.
func (b *Builder) ExitOdataAlterAssignment(ctx *parser.OdataAlterAssignmentContext) {
	if ctx.Expression() == nil {
		return
	}
	name := identifierOrKeywordText(ctx.IdentifierOrKeyword())
	if alter, ok := ctx.GetParent().(*parser.AlterStatementContext); ok && alter.CLIENT() != nil && isODataClientExpressionProp(name) {
		return
	}
	b.addError(odataExpressionNotAllowed(name, ctx.Expression()))
}

func odataExpressionNotAllowed(name string, expr parser.IExpressionContext) error {
	return fmt.Errorf(
		"property %s takes a plain value, not an expression: %s — "+
			"only an OData client's HttpUsername, HttpPassword, ClientCertificate and header values take an expression",
		name, ruleSourceText(expr.(antlr.ParserRuleContext)))
}
