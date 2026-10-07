// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"sort"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/mendixexpr"
)

// backslashIsLiteral is ADR-0010 R11's string rule: a doubled apostrophe is the only escape in
// a string literal. Under mdl 0 a backslash also escapes — `\n` is a newline,
// `\'` an apostrophe, `\\` one backslash — which contradicts Mendix's own
// expressions and makes `'C:\temp'` a tab. It changes what text means, so it is
// tied to the header (#732).
var backslashIsLiteral = langver.Change{
	Code:  "MDL-V1-ESCAPE",
	Since: langver.V1,
	Old:   "a backslash in a string literal is an escape (`\\n` is a newline, `\\t` a tab, `\\'` an apostrophe, `\\\\` one backslash)",
	New:   "an ordinary character, as in a Mendix expression; `''` is the only escape",
}

// newScriptStream is the character stream a script is lexed from. Whether a
// backslash escapes decides where a string literal ends, so the rule is fixed
// here, from the header, before the first token (see StrictEscapeStream).
// implicit is the language of input without a header (BuildSession); a
// script's is V0.
func newScriptStream(input string, implicit langver.Version) antlr.CharStream {
	var is antlr.CharStream = antlr.NewInputStream(input)
	v, written := langver.ScanWrittenHeader(input)
	if !written {
		v = implicit
	}
	if backslashIsLiteral.Applies(v) {
		is = &parser.StrictEscapeStream{CharStream: is}
	}
	return is
}

// unquoteStringLit is the value of a STRING_LITERAL (a terminal node, or a
// rule whose text is one), read under the escape rule it was lexed with.
//
// A nil node reads as "": Build walks a failed parse on purpose, so a rule that
// requires a STRING_LITERAL can still arrive here without one (an unquoted
// value under error recovery, mendixlabs/mxcli#1331). The listener has already
// recorded the syntax error, and that is what the author must see — not a panic.
func unquoteStringLit(n interface{ GetText() string }) string {
	if n == nil {
		return ""
	}
	text := n.GetText()
	if !lexedWithStrictEscapes(n) {
		return unquoteString(text)
	}
	if len(text) >= 2 && text[0] == '\'' && text[len(text)-1] == '\'' {
		text = text[1 : len(text)-1]
	}
	return strings.ReplaceAll(text, "''", "'")
}

func lexedWithStrictEscapes(n any) bool {
	var tok antlr.Token
	switch x := n.(type) {
	case antlr.TerminalNode:
		tok = x.GetSymbol()
	case antlr.ParserRuleContext:
		tok = x.GetStart()
	}
	return tok != nil && parser.HasStrictEscapes(tok.GetInputStream())
}

// noteBackslashEscapes records MDL-V1-ESCAPE for every string literal of an
// mdl 0 script whose value would differ under mdl 1: one holding an escape
// unquoteString interprets. A backslash before any other character is kept
// as written under both, so it is not reported.
func (b *Builder) noteBackslashEscapes(tokens []antlr.Token) {
	if backslashIsLiteral.Applies(b.langVersion) {
		return
	}
	added := false
	for _, t := range tokens {
		if t.GetTokenType() != parser.MDLLexerSTRING_LITERAL || !hasInterpretedEscape(t.GetText()) {
			continue
		}
		b.langNotes = append(b.langNotes, ast.LanguageNote{
			Line:    t.GetLine(),
			Code:    backslashIsLiteral.Code,
			Message: "the string " + t.GetText() + ": " + backslashIsLiteral.Warning(b.langVersion),
		})
		b.langNotes[len(b.langNotes)-1].Fix, b.langNotes[len(b.langNotes)-1].NoFix = b.escapeFix(t)
		added = true
	}
	if added {
		sort.SliceStable(b.langNotes, func(i, j int) bool { return b.langNotes[i].Line < b.langNotes[j].Line })
	}
}

// hasInterpretedEscape reports whether an mdl 0 string literal's text holds a
// backslash escape unquoteString turns into something else.
func hasInterpretedEscape(lit string) bool {
	for i := 0; i+1 < len(lit); i++ {
		if lit[i] != '\\' {
			continue
		}
		switch lit[i+1] {
		case 'n', 'r', 't', '\\', '\'':
			return true
		}
	}
	return false
}

// VisitTerminal collects the string literals of the tree for escapeFix.
func (b *Builder) VisitTerminal(node antlr.TerminalNode) {
	if t := node.GetSymbol(); t != nil && t.GetTokenType() == parser.MDLLexerSTRING_LITERAL {
		if b.stringLits == nil {
			b.stringLits = map[int]antlr.TerminalNode{}
		}
		b.stringLits[t.GetTokenIndex()] = node
	}
}

// escapeFix is the rewrite that keeps an mdl 0 string literal's meaning under
// mdl 1, where a backslash is an ordinary character.
//
// A string literal means its unescaped value under mdl 0 wherever it is — on
// its own (a name, a caption), in an expression the builder re-renders, and in
// one it stores as written (storedExpressionSource) — so the rewrite writes
// that value with a doubled apostrophe as the only escape. In an expression
// stored as written that is also the text mdl 0 stores for the literal, and
// under mdl 1 the text is stored as written: the same expression. Leaving such
// a literal alone, as this did while mdl 0 passed its escapes through to the
// model, left `\'` in it, and the mdl 1 script did not parse (ako/mxcli#820).
//
// An escaped line break in a re-rendered expression cannot be requoted in
// place: writing the break into the source makes the builder store the
// expression as written instead of re-rendering it, which is not what it
// stored before when the source is not already spelled the way the renderer
// spells it. So the whole expression is replaced by what mdl 0 stored — its
// rendering (storedExpressionFix), which stored as written under mdl 1 is the
// same expression (ako/mxcli#804). The exception is a text template written as
// one literal (mdl-examples/bug-tests/264-log-node-expression-roundtrip.mdl):
// under mdl 1 that literal is the template text whether or not it spans lines
// (templateLineBreak, #746), which is what the one-line literal was under
// mdl 0, so it is requoted in place.
func (b *Builder) escapeFix(t antlr.Token) (*ast.Fix, string) {
	requoted := requoteForV1(t.GetText())
	rewrite := &ast.Fix{Edits: []ast.TextEdit{replaceSpan(t, t, requoted)}}
	node := b.stringLits[t.GetTokenIndex()]
	if node == nil {
		return rewrite, ""
	}
	var top antlr.ParserRuleContext
	for p := node.GetParent(); p != nil; p = p.GetParent() {
		if e, ok := p.(*parser.ExpressionContext); ok {
			top = e
		}
	}
	if top == nil {
		return rewrite, ""
	}
	template, _ := templateMessageOf(top)
	source := strings.TrimSpace(extractExpressionText(top))
	// Stored as written: requoted in place. A line break the requote writes
	// into it keeps it stored as written under mdl 1.
	if shouldPreserveExpressionSource(source, false) {
		return rewrite, ""
	}
	if !template && b.holdsEscapedLineBreak(top) {
		return b.storedExpressionFix(top), ""
	}
	return rewrite, ""
}

// holdsEscapedLineBreak reports whether a string literal of expr spells a line
// break with an mdl 0 escape. Such an expression is rewritten as a whole, so
// none of its literals may also be requoted in place.
func (b *Builder) holdsEscapedLineBreak(expr antlr.ParserRuleContext) bool {
	for i := expr.GetStart().GetTokenIndex(); i <= expr.GetStop().GetTokenIndex(); i++ {
		if lit := b.stringLits[i]; lit != nil && strings.ContainsAny(unquoteString(lit.GetText()), "\r\n") &&
			hasInterpretedEscape(lit.GetText()) {
			return true
		}
	}
	return false
}

// storedExpressionFix replaces a re-rendered expression holding an escaped
// line break by the Mendix expression mdl 0 stores for it: the renderer's
// output, whose string literals hold the line break itself. Under mdl 1 that
// source spans lines, so it is stored as written, and what is written is what
// mdl 0 stored. A second escaped literal in the same expression is covered by
// the first one's edit and adds none.
func (b *Builder) storedExpressionFix(expr antlr.ParserRuleContext) *ast.Fix {
	start := expr.GetStart().GetTokenIndex()
	if b.storedExprs[start] {
		return &ast.Fix{}
	}
	if b.storedExprs == nil {
		b.storedExprs = map[int]bool{}
	}
	b.storedExprs[start] = true
	stored := mendixexpr.String(buildExpression(expr.(parser.IExpressionContext)))
	return &ast.Fix{Edits: []ast.TextEdit{replaceSpan(expr.GetStart(), expr.GetStop(), stored)}}
}

// stringLiteralEnd is the index just past the string literal whose opening
// apostrophe is at s[i], read under the string rule strict names: a doubled
// apostrophe is an apostrophe under both, and under mdl 0 (strict false) a
// backslash also takes the next character with it, so `\'` does not end the
// literal. An unterminated literal ends at len(s).
func stringLiteralEnd(s string, i int, strict bool) int {
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			if !strict {
				j++
			}
		case '\'':
			if j+1 < len(s) && s[j+1] == '\'' {
				j++
				continue
			}
			return j + 1
		}
	}
	return len(s)
}

// storedExpressionSource is the Mendix expression an expression stored as
// written stores: its source text, each string literal in it spelled the way
// Mendix spells the literal's value — an apostrophe doubled, a backslash and a
// control character as themselves, which is what Studio Pro stores.
//
// Under mdl 1 that is the source itself (ADR-0010 R11). Under mdl 0 a literal
// holding a backslash escape means its unescaped value here as everywhere
// else, so it is rewritten: `'C:\\temp'` is stored `'C:\temp'`, `'it\'s'`
// with a doubled apostrophe and `'a\nb'` with the line break in it. It used to
// be stored as written, so the mdl 0 escape reached the model: a backslash too
// many, an expression Mendix reads as another one, or one that does not parse
// (ako/mxcli#820). An expression the builder renders from its tree stores the
// same value through mendixexpr.QuoteLiteral.
func storedExpressionSource(source string, strict bool) string {
	if strict || !strings.Contains(source, `\`) {
		return source
	}
	var b strings.Builder
	b.Grow(len(source))
	for i := 0; i < len(source); {
		if source[i] != '\'' {
			b.WriteByte(source[i])
			i++
			continue
		}
		end := stringLiteralEnd(source, i, false)
		b.WriteString(mendixexpr.QuoteLiteral(unquoteString(source[i:end])))
		i = end
	}
	return b.String()
}

// requoteForV1 writes an mdl 0 string literal so that it has the same value
// under mdl 1, where a backslash is an ordinary character and a doubled apostrophe the only
// escape.
func requoteForV1(lit string) string {
	return "'" + strings.ReplaceAll(unquoteString(lit), "'", "''") + "'"
}

// templateMessageOf reports whether expr is, in full, the message of a log,
// show message or validation feedback and one string literal: the text of a
// template, which a line break does not turn into an expression under mdl 1.
// hasParams reports whether the statement binds template parameters.
func templateMessageOf(expr antlr.ParserRuleContext) (template, hasParams bool) {
	e, ok := expr.(*parser.ExpressionContext)
	if !ok || loneStringLiteral(e) == nil {
		return false, false
	}
	switch p := e.GetParent().(type) {
	case *parser.LogStatementContext:
		return logMessageExpression(p) == e, p.LogTemplateParams() != nil
	case *parser.ShowMessageStatementContext:
		return p.Expression() == e, p.TemplateParams() != nil || p.OBJECTS() != nil
	case *parser.ValidationFeedbackStatementContext:
		return p.Expression() == e, p.TemplateParams() != nil || p.OBJECTS() != nil
	}
	return false, false
}
