// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/antlr4-go/antlr/v4"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// R5 (ADR-0010): a page or snippet variable's default is an expression, and
// an expression is written bare. It took a string whose CONTENT was the
// expression — `$show: boolean = 'true'` — which stays the deprecated alias
// MDL-DEPR086 and keeps that meaning under every language version: mdl 1 is
// frozen, so making the string a Mendix string waits for mdl 2.
//
// The one default that cannot be written bare yet is a string: the bare
// `'abc'` is this very alias, so `$s: string = '''abc'''` and the empty `''`
// stay as they are and are not reported.

// ExitVariableDeclaration records MDL-DEPR086 on a default written in a
// string, with the rewrite that takes it out.
func (b *Builder) ExitVariableDeclaration(ctx *parser.VariableDeclarationContext) {
	lit := ctx.STRING_LITERAL()
	if lit == nil {
		return
	}
	content := unquoteStringLit(lit)
	if content == "" || isLoneStringLiteral(content) {
		return
	}
	b.recordDeprecation(deprecation.QuotedVariableDefault, lit.GetSymbol(), "page variable default")
	fix, why := variableDefaultFix(lit, content)
	b.fixLastDeprecation(deprecation.QuotedVariableDefault, fix, why)
}

// variableDefaultFix replaces the string by its content, when that content
// reads back bare as the same default.
func variableDefaultFix(lit antlr.TerminalNode, content string) (*ast.Fix, string) {
	if holdsInterpretedEscape(lit) {
		return nil, "the default's string holds a backslash escape; write the expression bare by hand"
	}
	if !BareExpression(content) || !VariableDefaultReadsBack(content) {
		return nil, "the string " + lit.GetText() + " does not read back as the same bare expression; write it bare by hand"
	}
	t := lit.GetSymbol()
	text := content
	// A string needs no space to part it from its neighbours; a bare
	// expression does: `='true'`.
	if is := t.GetInputStream(); is != nil {
		if t.GetStart() > 0 && gluesToWord(is.GetText(t.GetStart()-1, t.GetStart()-1)) {
			text = " " + text
		}
		if t.GetStop()+1 < is.Size() && gluesToWord(is.GetText(t.GetStop()+1, t.GetStop()+1)) {
			text += " "
		}
	}
	return &ast.Fix{Edits: []ast.TextEdit{{Start: t.GetStart(), Stop: t.GetStop() + 1, Text: text}}}, ""
}

// VariableDefaultReadsBack reports whether `$v: Boolean = <expr>` stores
// exactly expr as the default. describe asks it before writing a default bare.
func VariableDefaultReadsBack(expr string) bool {
	if expr == "" || isLoneStringLiteral(expr) {
		return false
	}
	ctx, ok := parseRule("$v: Boolean = "+expr, func(p *parser.MDLParser) antlr.ParserRuleContext {
		return p.VariableDeclaration()
	})
	if !ok {
		return false
	}
	vd := ctx.(*parser.VariableDeclarationContext)
	return vd.STRING_LITERAL() == nil && buildSingleVariableDeclaration(vd).DefaultValue == expr
}

// isLoneStringLiteral reports whether s is exactly one string literal.
func isLoneStringLiteral(s string) bool {
	ctx, ok := parseRule(s, func(p *parser.MDLParser) antlr.ParserRuleContext { return p.Expression() })
	return ok && loneLiteralToken(ctx) != nil
}
