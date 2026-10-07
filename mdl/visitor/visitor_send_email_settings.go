// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ADR-0013: the Send Email activity's dialog settings are one ( Key: value, … )
// list. The activity has no main operand and no result, so the list follows the
// verb: `send email ( … ) [on error …]`. It is new syntax, so there is no clause
// form and no deprecation; an unknown key, a repeated key or a value of the
// wrong shape is an error under every language version (R11).
//
// Keys are the metamodel's names for the settings (Microflows$EmailMessage,
// EmailConnectionConfig), shortened only where the stored name carries a
// storage suffix: SubjectTemplate → Subject, MessageBodyPlainTextTemplate →
// Body, MessageBodyHtmlTemplate → HtmlBody, CustomHeaders → Headers (the
// consumed REST service's key for the same concept), EmailId → From (Studio
// Pro's "Email ID (From)"). ConnectionTimeout keeps the metamodel name: it is
// milliseconds, where REST's Timeout is seconds.
var sendEmailSettingKeys = []string{
	"From", "To", "Cc", "Bcc", "Subject", "Body", "HtmlBody", "Headers", "Attachment",
	"Host", "Port", "SecurityType", "CheckServerIdentity", "ConnectionTimeout", "Authentication",
}

// sendEmailRequiredKeys are the settings mxbuild refuses to leave empty, and
// sendEmailRecipientKeys the ones of which it needs at least one. Measured on
// mxbuild 11.15.0-rc.4, one omitted setting per activity: CE0166 "The 'Email ID
// (From)' / 'Server Host' / 'Server Port' property is required." and "The 'To,
// Cc, or Bcc' property is required."; an activity without a Subject is clean.
var sendEmailRequiredKeys = []string{"From", "Host", "Port"}

var sendEmailRecipientKeys = []string{"To", "Cc", "Bcc"}

var sendEmailSecurityTypes = []string{"None", "SSL", "TLS"}

var bareVariable = regexp.MustCompile(`^\$[A-Za-z_][A-Za-z0-9_]*$`)

// buildSendEmailStatement reads `send email ( … )` into a SendEmailStmt. It is
// lenient — a key or value it does not recognise is skipped — because the
// statement builders have no error channel; ExitSendEmailStatement reports it.
func buildSendEmailStatement(ctx parser.ISendEmailStatementContext) *ast.SendEmailStmt {
	if ctx == nil {
		return nil
	}
	c := ctx.(*parser.SendEmailStatementContext)
	stmt := &ast.SendEmailStmt{}
	if settings, ok := c.SendEmailSettings().(*parser.SendEmailSettingsContext); ok && settings != nil {
		sendEmailSettingsInto(stmt, settings)
	}
	if errClause := c.OnErrorClause(); errClause != nil {
		stmt.ErrorHandling = buildOnErrorClause(errClause)
	}
	return stmt
}

func sendEmailSettingsInto(stmt *ast.SendEmailStmt, settings *parser.SendEmailSettingsContext) {
	for _, s := range settings.AllSendEmailSetting() {
		sc, ok := s.(*parser.SendEmailSettingContext)
		if !ok || sc == nil || sc.IdentifierOrKeyword() == nil {
			continue
		}
		vc, ok := sc.SendEmailSettingValue().(*parser.SendEmailSettingValueContext)
		if !ok || vc == nil {
			continue
		}
		key := canonicalKey(identifierOrKeywordText(sc.IdentifierOrKeyword()), sendEmailSettingKeys)
		// A plain expression value: everything but the map, basic and template
		// shapes, and without template parameters.
		plain := func() ast.Expression {
			if vc.Expression() == nil || vc.TemplateParams() != nil {
				return nil
			}
			return buildSourceExpression(vc.Expression())
		}
		switch key {
		case "From":
			stmt.From = plain()
		case "To":
			stmt.To = plain()
		case "Cc":
			stmt.Cc = plain()
		case "Bcc":
			stmt.Bcc = plain()
		case "Host":
			stmt.Host = plain()
		case "Port":
			stmt.Port = plain()
		case "Subject":
			if t := emailTemplateValue(vc, false); t != nil {
				stmt.Subject = *t
			}
		case "Body":
			stmt.BodyText = emailTemplateValue(vc, true)
		case "HtmlBody":
			stmt.BodyHTML = emailTemplateValue(vc, true)
		case "Headers":
			if vc.LPAREN() == nil || vc.BASIC() != nil {
				continue
			}
			for _, e := range vc.AllRestCallMapEntry() {
				ec, ok := e.(*parser.RestCallMapEntryContext)
				if !ok || ec == nil || ec.STRING_LITERAL() == nil || ec.Expression() == nil {
					continue
				}
				if lit := loneStringLiteral(ec.Expression()); lit != nil {
					stmt.Headers = append(stmt.Headers, ast.EmailHeader{
						Name:  unquoteStringLit(ec.STRING_LITERAL()),
						Value: unquoteStringLit(lit),
					})
				}
			}
		case "Attachment":
			if vc.Expression() != nil && vc.TemplateParams() == nil {
				if v := strings.TrimSpace(nodeText(vc.Expression())); bareVariable.MatchString(v) {
					stmt.Attachment = strings.TrimPrefix(v, "$")
				}
			}
		case "SecurityType":
			if vc.Expression() != nil && vc.TemplateParams() == nil {
				if mode := canonicalKey(strings.TrimSpace(nodeText(vc.Expression())), sendEmailSecurityTypes); mode != "" {
					stmt.Security = mode
				}
			}
		case "CheckServerIdentity":
			if vc.Expression() != nil && vc.TemplateParams() == nil {
				stmt.CheckServerIdentity = strings.EqualFold(strings.TrimSpace(nodeText(vc.Expression())), "true")
			}
		case "ConnectionTimeout":
			if vc.Expression() != nil && vc.TemplateParams() == nil {
				if n, err := strconv.Atoi(strings.TrimSpace(nodeText(vc.Expression()))); err == nil {
					stmt.Timeout = n
				}
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
		}
	}
}

// emailTemplateValue reads a subject or body. The subject's template is a string
// literal (`Subject: 'Order {1}' with (…)`), as a `log` message's is; a body's
// is `template '…'`, as a REST body's is (ADR-0013's value shapes). In both, any
// other expression is kept as an expression, which the builder stores as the
// template '{1}' with the expression as its parameter. nil for a value of
// another shape.
func emailTemplateValue(vc *parser.SendEmailSettingValueContext, body bool) *ast.EmailTemplateClause {
	out := &ast.EmailTemplateClause{}
	switch {
	case body && vc.TEMPLATE() != nil && vc.STRING_LITERAL() != nil:
		out.Text = &ast.LiteralExpr{Kind: ast.LiteralString, Value: unquoteStringLit(vc.STRING_LITERAL())}
	case vc.Expression() != nil && vc.TEMPLATE() == nil:
		if lit := loneStringLiteral(vc.Expression()); lit != nil && !body {
			out.Text = &ast.LiteralExpr{Kind: ast.LiteralString, Value: unquoteStringLit(lit)}
		} else {
			out.Text = buildSourceExpression(vc.Expression())
		}
	default:
		return nil
	}
	if tp := vc.TemplateParams(); tp != nil {
		out.Params = buildTemplateParams(tp)
	}
	return out
}

// ExitSendEmailStatement checks the settings list (R11).
func (b *Builder) ExitSendEmailStatement(ctx *parser.SendEmailStatementContext) {
	if settings, ok := ctx.SendEmailSettings().(*parser.SendEmailSettingsContext); ok && settings != nil {
		b.checkSendEmailSettings(settings)
	}
}

// checkSendEmailSettings reports an unknown or repeated key, a value whose shape
// is not the key's, a missing required key, and a missing recipient.
func (b *Builder) checkSendEmailSettings(settings *parser.SendEmailSettingsContext) {
	const what = "send email"
	line := settings.GetStart().GetLine()
	seen := map[string]bool{}
	for _, s := range settings.AllSendEmailSetting() {
		sc, ok := s.(*parser.SendEmailSettingContext)
		if !ok || sc == nil || sc.IdentifierOrKeyword() == nil {
			continue
		}
		kline := sc.GetStart().GetLine()
		key := identifierOrKeywordText(sc.IdentifierOrKeyword())
		canonical := canonicalKey(key, sendEmailSettingKeys)
		if canonical == "" {
			b.addError(unknownKeyError(kline, what, key, sendEmailSettingKeys))
			continue
		}
		if seen[canonical] {
			b.addError(fmt.Errorf("line %d: %s sets %s twice", kline, what, canonical))
			continue
		}
		seen[canonical] = true
		vc, ok := sc.SendEmailSettingValue().(*parser.SendEmailSettingValueContext)
		if !ok || vc == nil {
			continue
		}
		if msg := sendEmailValueProblem(canonical, vc); msg != "" {
			b.addError(fmt.Errorf("line %d: %s: %s, not %s", kline, what, msg, nodeText(vc)))
			continue
		}
		switch canonical {
		case "Headers":
			b.checkSendEmailHeaders(vc)
		case "Authentication":
			b.checkRestCallCredentialsFor("send email: Authentication", kline, vc.AllRestCallMapEntry())
		}
	}
	for _, k := range sendEmailRequiredKeys {
		if !seen[k] {
			b.addError(fmt.Errorf("line %d: %s needs %s (mxbuild: CE0166)", line, what, k))
		}
	}
	hasRecipient := false
	for _, k := range sendEmailRecipientKeys {
		hasRecipient = hasRecipient || seen[k]
	}
	if !hasRecipient {
		b.addError(fmt.Errorf("line %d: %s needs a recipient: %s (mxbuild: CE0166)", line, what,
			strings.Join(sendEmailRecipientKeys, ", ")))
	}
}

// sendEmailValueProblem says what shape key takes when vc has another one, or
// returns "".
func sendEmailValueProblem(key string, vc *parser.SendEmailSettingValueContext) string {
	isMap := vc.LPAREN() != nil && vc.BASIC() == nil
	isBasic := vc.BASIC() != nil
	isTemplate := vc.TEMPLATE() != nil
	isExpr := vc.Expression() != nil && !isTemplate
	hasParams := vc.TemplateParams() != nil
	text := ""
	if vc.Expression() != nil {
		text = strings.TrimSpace(nodeText(vc.Expression()))
	}
	switch key {
	case "From", "To", "Cc", "Bcc", "Host", "Port":
		if !isExpr || hasParams {
			return key + " takes an expression"
		}
	case "Subject":
		if !isExpr {
			return "Subject takes a text '…' [with ({1} = …)] or an expression"
		}
	case "Body", "HtmlBody":
		if !isTemplate && !isExpr {
			return key + " takes template '…' [with ({1} = …)] or an expression"
		}
	case "Headers":
		if !isMap {
			return "Headers takes a map of header names to values, Headers: ( 'X-Correlation-Id': 'value' )"
		}
	case "Attachment":
		if !isExpr || hasParams || !bareVariable.MatchString(text) {
			return "Attachment takes a variable, Attachment: $Document"
		}
	case "SecurityType":
		if !isExpr || hasParams || canonicalKey(text, sendEmailSecurityTypes) == "" {
			return "SecurityType takes none, ssl or tls"
		}
	case "CheckServerIdentity":
		if !isExpr || hasParams || (!strings.EqualFold(text, "true") && !strings.EqualFold(text, "false")) {
			return "CheckServerIdentity takes true or false"
		}
	case "ConnectionTimeout":
		if n, err := strconv.Atoi(text); !isExpr || hasParams || err != nil || n <= 0 {
			return "ConnectionTimeout takes a number of milliseconds"
		}
	case "Authentication":
		if !isBasic {
			return "Authentication takes basic ( Username: …, Password: … )"
		}
	}
	return ""
}

// checkSendEmailHeaders: a header's name is a string, its value a string
// literal — Microflows$EmailCustomHeader stores both as plain text, not as an
// expression — and no name is set twice.
func (b *Builder) checkSendEmailHeaders(vc *parser.SendEmailSettingValueContext) {
	names := map[string]bool{}
	for _, e := range vc.AllRestCallMapEntry() {
		ec, ok := e.(*parser.RestCallMapEntryContext)
		if !ok || ec == nil {
			continue
		}
		eline := ec.GetStart().GetLine()
		if ec.STRING_LITERAL() == nil {
			name := nodeText(ec.IdentifierOrKeyword())
			b.addError(fmt.Errorf("line %d: send email: a header name is a string, '%s', not %s", eline, name, name))
			continue
		}
		if ec.Expression() == nil || loneStringLiteral(ec.Expression()) == nil {
			b.addError(fmt.Errorf("line %d: send email: header %s takes a string, not an expression — "+
				"Studio Pro stores a custom header's value as plain text", eline, ec.STRING_LITERAL().GetText()))
		}
		name := strings.ToLower(unquoteStringLit(ec.STRING_LITERAL()))
		if names[name] {
			b.addError(fmt.Errorf("line %d: send email sets header %s twice", eline, ec.STRING_LITERAL().GetText()))
		}
		names[name] = true
	}
}
