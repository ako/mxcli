// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ADR-0013: a microflow activity's dialog settings are one ( Key: value, … )
// list after its main operand. `call rest service` is the first activity
// migrated: its settings list takes the keys below, named as the consumed REST
// service document names the same concepts. The old clauses are MDL-DEPR720,
// and build the same statement.
//
// The list is new syntax, so an unknown key, a repeated key or a value of the
// wrong shape is an error under every language version (R11).

var restCallSettingKeys = []string{"Headers", "Authentication", "Body", "Timeout"}

var restCallCredentialKeys = []string{"Username", "Password"}

// restCallSettingsInto reads the settings list into stmt. It is lenient: a key
// or value it does not recognise is skipped, and ExitRestCallStatement reports
// it, because the statement builders have no error channel.
func restCallSettingsInto(stmt *ast.RestCallStmt, settings *parser.RestCallSettingsContext) {
	for _, s := range settings.AllRestCallSetting() {
		sc, ok := s.(*parser.RestCallSettingContext)
		if !ok || sc == nil || sc.IdentifierOrKeyword() == nil {
			continue
		}
		vc, ok := sc.RestCallSettingValue().(*parser.RestCallSettingValueContext)
		if !ok || vc == nil {
			continue
		}
		switch canonicalKey(identifierOrKeywordText(sc.IdentifierOrKeyword()), restCallSettingKeys) {
		case "Headers":
			if vc.LPAREN() == nil || vc.BASIC() != nil {
				continue
			}
			for _, e := range vc.AllRestCallMapEntry() {
				ec, ok := e.(*parser.RestCallMapEntryContext)
				if !ok || ec == nil || ec.STRING_LITERAL() == nil || ec.Expression() == nil {
					continue
				}
				stmt.Headers = append(stmt.Headers, ast.RestHeader{
					Name:  unquoteStringLit(ec.STRING_LITERAL()),
					Value: buildSourceExpression(ec.Expression()),
				})
			}
		case "Authentication":
			if vc.BASIC() == nil {
				continue
			}
			auth := &ast.RestAuth{}
			for _, e := range vc.AllRestCallMapEntry() {
				ec, ok := e.(*parser.RestCallMapEntryContext)
				if !ok || ec == nil || ec.IdentifierOrKeyword() == nil || ec.Expression() == nil {
					continue
				}
				switch canonicalKey(identifierOrKeywordText(ec.IdentifierOrKeyword()), restCallCredentialKeys) {
				case "Username":
					auth.Username = buildSourceExpression(ec.Expression())
				case "Password":
					auth.Password = buildSourceExpression(ec.Expression())
				}
			}
			if auth.Username != nil && auth.Password != nil {
				stmt.Auth = auth
			}
		case "Body":
			stmt.Body = restCallBodyValue(vc)
		case "Timeout":
			if vc.Expression() != nil && vc.BINARY_TYPE() == nil && vc.TemplateParams() == nil {
				stmt.Timeout = buildSourceExpression(vc.Expression())
			}
		}
	}
}

// restCallBodyValue builds the request body a `Body:` value states — the same
// RestBody the old `body …` clause builds for the same text.
func restCallBodyValue(vc *parser.RestCallSettingValueContext) *ast.RestBody {
	switch {
	case vc.TEMPLATE() != nil && vc.STRING_LITERAL() != nil:
		// Body: template '…' [with (…)] — the custom request template.
		body := &ast.RestBody{Type: ast.RestBodyCustom, Template: &ast.LiteralExpr{
			Kind: ast.LiteralString, Value: unquoteStringLit(vc.STRING_LITERAL()),
		}}
		if tp := vc.TemplateParams(); tp != nil {
			body.TemplateParams = buildTemplateParams(tp)
		}
		return body
	case vc.MAPPING() != nil && vc.QualifiedName() != nil && vc.VARIABLE() != nil:
		// Body: mapping M.ExportMapping from $Obj
		return &ast.RestBody{
			Type:           ast.RestBodyMapping,
			MappingName:    buildQualifiedName(vc.QualifiedName()),
			SourceVariable: strings.TrimPrefix(vc.VARIABLE().GetText(), "$"),
		}
	case vc.BINARY_TYPE() != nil && vc.Expression() != nil:
		// Body: binary $Doc/Contents
		return &ast.RestBody{Type: ast.RestBodyBinary, Template: buildSourceExpression(vc.Expression())}
	case vc.Expression() != nil && vc.LPAREN() == nil:
		// Body: <expression> [with (…)] — what `body <expression>` built.
		body := &ast.RestBody{Type: ast.RestBodyCustom, Template: buildSourceExpression(vc.Expression())}
		if tp := vc.TemplateParams(); tp != nil {
			body.TemplateParams = buildTemplateParams(tp)
		}
		return body
	}
	return nil
}

// ExitRestCallStatement checks the settings list (R11) or, for the clause
// form, records MDL-DEPR720 with the rewrite `fmt --upgrade` applies.
func (b *Builder) ExitRestCallStatement(ctx *parser.RestCallStatementContext) {
	if settings, ok := ctx.RestCallSettings().(*parser.RestCallSettingsContext); ok && settings != nil {
		b.checkRestCallSettings(settings)
		return
	}
	first := restCallFirstClauseToken(ctx)
	if first == nil {
		return
	}
	b.recordDeprecation(deprecation.RestCallClauses, first, "")
	b.fixLastDeprecation(deprecation.RestCallClauses, restCallClausesFix(ctx), "")
}

// checkRestCallSettings reports an unknown or repeated key, and a value whose
// shape is not the key's.
func (b *Builder) checkRestCallSettings(settings *parser.RestCallSettingsContext) {
	const what = "call rest service"
	seen := map[string]bool{}
	for _, s := range settings.AllRestCallSetting() {
		sc, ok := s.(*parser.RestCallSettingContext)
		if !ok || sc == nil || sc.IdentifierOrKeyword() == nil {
			continue
		}
		line := sc.GetStart().GetLine()
		key := identifierOrKeywordText(sc.IdentifierOrKeyword())
		canonical := canonicalKey(key, restCallSettingKeys)
		if canonical == "" {
			b.addError(unknownKeyError(line, what, key, restCallSettingKeys))
			continue
		}
		if seen[canonical] {
			b.addError(fmt.Errorf("line %d: %s sets %s twice", line, what, canonical))
			continue
		}
		seen[canonical] = true
		vc, ok := sc.RestCallSettingValue().(*parser.RestCallSettingValueContext)
		if !ok || vc == nil {
			continue
		}
		value := nodeText(vc)
		switch canonical {
		case "Headers":
			if vc.LPAREN() == nil || vc.BASIC() != nil {
				b.addError(fmt.Errorf("line %d: %s: Headers takes a map of header names to values, "+
					"Headers: ( 'Accept': 'application/json' ), not %s", line, what, value))
				continue
			}
			names := map[string]bool{}
			for _, e := range vc.AllRestCallMapEntry() {
				ec, ok := e.(*parser.RestCallMapEntryContext)
				if !ok || ec == nil {
					continue
				}
				if ec.STRING_LITERAL() == nil {
					name := nodeText(ec.IdentifierOrKeyword())
					b.addError(fmt.Errorf("line %d: %s: a header name is a string, '%s', not %s",
						ec.GetStart().GetLine(), what, name, name))
					continue
				}
				name := strings.ToLower(unquoteStringLit(ec.STRING_LITERAL()))
				if names[name] {
					b.addError(fmt.Errorf("line %d: %s sets header %s twice", ec.GetStart().GetLine(), what,
						ec.STRING_LITERAL().GetText()))
				}
				names[name] = true
			}
		case "Authentication":
			if vc.BASIC() == nil {
				b.addError(fmt.Errorf("line %d: %s: Authentication takes basic ( Username: …, Password: … ), not %s",
					line, what, value))
				continue
			}
			b.checkRestCallCredentials(line, vc)
		case "Body":
			if vc.LPAREN() != nil || vc.BASIC() != nil {
				b.addError(fmt.Errorf("line %d: %s: Body takes template '…' [with (…)], mapping M.Mapping from $Var, "+
					"binary <expression> or an expression, not %s", line, what, value))
			}
		case "Timeout":
			if vc.Expression() == nil || vc.BINARY_TYPE() != nil || vc.TemplateParams() != nil {
				b.addError(fmt.Errorf("line %d: %s: Timeout takes the number of seconds, an expression, not %s",
					line, what, value))
			}
		}
	}
}

// checkRestCallCredentials checks `basic ( Username: …, Password: … )`: both
// keys, each once, and nothing else.
func (b *Builder) checkRestCallCredentials(line int, vc *parser.RestCallSettingValueContext) {
	const what = "call rest service: Authentication"
	seen := map[string]bool{}
	for _, e := range vc.AllRestCallMapEntry() {
		ec, ok := e.(*parser.RestCallMapEntryContext)
		if !ok || ec == nil {
			continue
		}
		key := ""
		if ec.IdentifierOrKeyword() != nil {
			key = identifierOrKeywordText(ec.IdentifierOrKeyword())
		} else if ec.STRING_LITERAL() != nil {
			key = ec.STRING_LITERAL().GetText()
		}
		canonical := canonicalKey(key, restCallCredentialKeys)
		if canonical == "" {
			b.addError(unknownKeyError(ec.GetStart().GetLine(), what, key, restCallCredentialKeys))
			continue
		}
		if seen[canonical] {
			b.addError(fmt.Errorf("line %d: %s sets %s twice", ec.GetStart().GetLine(), what, canonical))
		}
		seen[canonical] = true
	}
	for _, k := range restCallCredentialKeys {
		if !seen[k] {
			b.addError(fmt.Errorf("line %d: %s needs %s: basic ( Username: …, Password: … )", line, what, k))
		}
	}
}

// restCallFirstClauseToken is the first token of the old clause form, or nil
// when the statement has no clause.
func restCallFirstClauseToken(ctx *parser.RestCallStatementContext) antlr.Token {
	if hs := ctx.AllRestCallHeaderClause(); len(hs) > 0 {
		return hs[0].GetStart()
	}
	if a, ok := ctx.RestCallAuthClause().(*parser.RestCallAuthClauseContext); ok && a != nil {
		return a.GetStart()
	}
	if bc, ok := ctx.RestCallBodyClause().(*parser.RestCallBodyClauseContext); ok && bc != nil {
		return bc.GetStart()
	}
	if tc, ok := ctx.RestCallTimeoutClause().(*parser.RestCallTimeoutClauseContext); ok && tc != nil {
		return tc.GetStart()
	}
	return nil
}

// restCallClausesFix rewrites the clauses as the settings list:
//
//	header 'A' = 'x' header B = $v auth basic $u password $p body '…' timeout 30
//	(Headers: ('A': 'x', 'B': $v), Authentication: basic (Username: $u, Password: $p), Body: template '…', Timeout: 30)
//
// It edits only the keywords between the values, so every value — and any
// rewrite nested inside one — is kept exactly as written. Nil for a statement
// the parser recovered from.
func restCallClausesFix(ctx *parser.RestCallStatementContext) *ast.Fix {
	type group struct {
		edits []ast.TextEdit
		end   int    // rune offset just after the group's last token
		close string // text that closes the group's value
	}
	var groups []group

	if hs := ctx.AllRestCallHeaderClause(); len(hs) > 0 {
		g := group{close: ")"}
		prevStop := -1
		for i, h := range hs {
			hc, ok := h.(*parser.RestCallHeaderClauseContext)
			if !ok || hc == nil || hc.HEADER() == nil || hc.EQUALS() == nil || hc.Expression() == nil ||
				hc.Expression().GetStart() == nil || hc.Expression().GetStop() == nil {
				return nil
			}
			kw := hc.HEADER().GetSymbol()
			var name antlr.Token
			switch {
			case hc.IDENTIFIER() != nil:
				name = hc.IDENTIFIER().GetSymbol()
			case hc.STRING_LITERAL() != nil:
				name = hc.STRING_LITERAL().GetSymbol()
			default:
				return nil
			}
			// `header ` (and the layout before a later one) up to the name.
			lead := ast.TextEdit{Start: kw.GetStart(), Stop: name.GetStart(), Text: "Headers: ("}
			if i > 0 {
				lead = replaceGap(prevStop, name.GetStart(), ", ")
			}
			g.edits = append(g.edits, lead)
			if hc.IDENTIFIER() != nil {
				g.edits = append(g.edits, replaceSpan(name, name, "'"+name.GetText()+"'"))
			}
			nameStop := name.GetStop()
			g.edits = append(g.edits, replaceGap(nameStop, hc.Expression().GetStart().GetStart(), ": "))
			prevStop = hc.Expression().GetStop().GetStop()
		}
		g.end = prevStop + 1
		groups = append(groups, g)
	}

	if a, ok := ctx.RestCallAuthClause().(*parser.RestCallAuthClauseContext); ok && a != nil {
		exprs := a.AllExpression()
		if a.AUTH() == nil || a.BASIC() == nil || len(exprs) != 2 || exprs[0].GetStart() == nil || exprs[0].GetStop() == nil ||
			exprs[1].GetStart() == nil || exprs[1].GetStop() == nil {
			return nil
		}
		basic := a.BASIC().GetSymbol()
		groups = append(groups, group{
			edits: []ast.TextEdit{
				{Start: a.AUTH().GetSymbol().GetStart(), Stop: exprs[0].GetStart().GetStart(),
					Text: "Authentication: " + keywordLike(basic.GetText(), "basic") + " (Username: "},
				replaceGap(exprs[0].GetStop().GetStop(), exprs[1].GetStart().GetStart(), ", Password: "),
			},
			end:   exprs[1].GetStop().GetStop() + 1,
			close: ")",
		})
	}

	if bc, ok := ctx.RestCallBodyClause().(*parser.RestCallBodyClauseContext); ok && bc != nil {
		if bc.BODY() == nil || bc.GetStop() == nil {
			return nil
		}
		kw := bc.BODY().GetSymbol()
		text := "Body:"
		if bc.STRING_LITERAL() != nil && bc.BINARY_TYPE() == nil && bc.MAPPING() == nil {
			// `body '…'` is the custom request template. A quoted body that
			// continues as an expression (`body 'a' + $b`) is the expression
			// alternative, which has no STRING_LITERAL of its own.
			text = "Body: " + keywordLike(kw.GetText(), "template")
		}
		groups = append(groups, group{edits: []ast.TextEdit{replaceSpan(kw, kw, text)}, end: bc.GetStop().GetStop() + 1})
	}

	if tc, ok := ctx.RestCallTimeoutClause().(*parser.RestCallTimeoutClauseContext); ok && tc != nil {
		if tc.TIMEOUT() == nil || tc.GetStop() == nil {
			return nil
		}
		kw := tc.TIMEOUT().GetSymbol()
		groups = append(groups, group{edits: []ast.TextEdit{replaceSpan(kw, kw, "Timeout:")}, end: tc.GetStop().GetStop() + 1})
	}

	if len(groups) == 0 {
		return nil
	}
	first := restCallFirstClauseToken(ctx)
	returns := ctx.RestCallReturnsClause()
	if first == nil || returns == nil || returns.GetStart() == nil {
		return nil
	}
	in := first.GetInputStream()
	before := first.GetStart() // the clauses' first character
	for before > 0 && isBlank(in.GetText(before-1, before-1)) {
		before--
	}
	lead := in.GetText(before, first.GetStart()-1)
	last := groups[len(groups)-1].end
	tail := ""
	if returns.GetStart().GetStart() > last {
		tail = in.GetText(last, returns.GetStart().GetStart()-1)
	}
	// Clauses on lines of their own become the list as describe prints it:
	// `(` after the URL, one setting per line with a trailing comma, and `)`
	// at the statement's indent before `returns`. Anything else — clauses on
	// the statement's line, or a comment between the last clause and
	// `returns` — keeps its layout, the list wrapped around it.
	multiLine := strings.Contains(lead, "\n") && strings.TrimSpace(tail) == ""

	var edits []ast.TextEdit
	for i, g := range groups {
		if i == 0 {
			if multiLine {
				edits = append(edits, insertAt(before, " ("))
			} else {
				g.edits[0].Text = "(" + g.edits[0].Text
			}
		}
		edits = append(edits, g.edits...)
		switch {
		case i < len(groups)-1:
			edits = append(edits, insertAt(g.end, g.close+","))
		case multiLine:
			edits = append(edits, replaceGap(g.end-1, returns.GetStart().GetStart(),
				g.close+",\n"+statementIndent(ctx.GetStart())+") "))
		default:
			edits = append(edits, insertAt(g.end, g.close+")"))
		}
	}
	return &ast.Fix{Edits: edits}
}

// statementIndent is the indentation of the line tok is on.
func statementIndent(tok antlr.Token) string {
	in := tok.GetInputStream()
	i := tok.GetStart()
	for i > 0 && in.GetText(i-1, i-1) != "\n" {
		i--
	}
	j := i
	for j < tok.GetStart() {
		if c := in.GetText(j, j); c != " " && c != "\t" {
			break
		}
		j++
	}
	if j == i {
		return ""
	}
	return in.GetText(i, j-1)
}
