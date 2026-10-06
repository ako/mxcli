// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// sendEmailFlow wraps a send email statement in a microflow.
func sendEmailFlow(settings string) string {
	return "create microflow M.F ($Msg: M.Msg, $Doc: M.Doc) begin send email (" + settings + "); end;"
}

// The minimum mxbuild accepts: From, Host, Port and one recipient.
const sendEmailMinimum = "From: 'a@x.com', To: 'b@x.com', Host: 'h', Port: 25"

// mendixlabs/mxcli#1315, ADR-0013: every setting of the Send Email dialog is a
// key of the one property list, and each builds its AST field.
func TestSendEmailSettingsListBuildsEverySetting(t *testing.T) {
	prog := mustBuild(t, sendEmailFlow("\n"+
		"  From: $Msg/Sender,\n"+
		"  To: $Msg/Recipient,\n"+
		"  Cc: 'cc@x.com',\n"+
		"  Bcc: 'bcc@x.com',\n"+
		"  Subject: 'Order {1}' with ({1} = $Msg/Number),\n"+
		"  Body: template 'Hello {1}' with ({1} = $Msg/Name),\n"+
		"  HtmlBody: $Msg/Html,\n"+
		"  Headers: ('X-Id': 'abc', 'X-Priority': '1'),\n"+
		"  Attachment: $Doc,\n"+
		"  Host: @M.SmtpHost,\n"+
		"  Port: @M.SmtpPort,\n"+
		"  SecurityType: SSL,\n"+
		"  CheckServerIdentity: true,\n"+
		"  ConnectionTimeout: 30000,\n"+
		"  Authentication: basic (Username: @M.User, Password: @M.Password),\n"))
	se := sendEmailOf(t, prog)

	for name, e := range map[string]ast.Expression{"From": se.From, "To": se.To, "Cc": se.Cc, "Bcc": se.Bcc, "Host": se.Host, "Port": se.Port} {
		if e == nil {
			t.Errorf("%s not set", name)
		}
	}
	if lit, ok := se.Subject.Text.(*ast.LiteralExpr); !ok || lit.Value != "Order {1}" || len(se.Subject.Params) != 1 {
		t.Errorf("Subject = %+v, want the template text 'Order {1}' and one parameter", se.Subject)
	}
	if se.BodyText == nil || len(se.BodyText.Params) != 1 {
		t.Errorf("Body = %+v", se.BodyText)
	} else if lit, ok := se.BodyText.Text.(*ast.LiteralExpr); !ok || lit.Value != "Hello {1}" {
		t.Errorf("Body text = %#v, want the template text 'Hello {1}'", se.BodyText.Text)
	}
	if se.BodyHTML == nil {
		t.Errorf("HtmlBody not set")
	} else if _, isLit := se.BodyHTML.Text.(*ast.LiteralExpr); isLit {
		t.Errorf("HtmlBody: an expression body must stay an expression, got %#v", se.BodyHTML.Text)
	}
	if len(se.Headers) != 2 || se.Headers[0] != (ast.EmailHeader{Name: "X-Id", Value: "abc"}) {
		t.Errorf("Headers = %+v", se.Headers)
	}
	if se.Attachment != "Doc" {
		t.Errorf("Attachment = %q, want Doc", se.Attachment)
	}
	if se.Security != "SSL" || !se.CheckServerIdentity || se.Timeout != 30000 {
		t.Errorf("SecurityType %q, CheckServerIdentity %v, ConnectionTimeout %d", se.Security, se.CheckServerIdentity, se.Timeout)
	}
	if se.Auth == nil || se.Auth.Username == nil || se.Auth.Password == nil {
		t.Errorf("Authentication = %+v", se.Auth)
	}
}

// Keys and values are case-insensitive words, as in every property list; the
// canonical spelling is what describe prints.
func TestSendEmailSettingsListIgnoresCase(t *testing.T) {
	se := sendEmailOf(t, mustBuild(t, "CREATE MICROFLOW M.F () BEGIN SEND EMAIL (FROM: 'a@x.com', cc: 'b@x.com', HOST: 'h', port: 25, securitytype: None); END;"))
	if se.From == nil || se.Cc == nil || se.Host == nil || se.Port == nil || se.Security != "None" {
		t.Errorf("got %+v", se)
	}
}

// R11: the list is new syntax, so an unknown key, a repeated key or a
// misshapen value is an error, and so is a statement missing what mxbuild
// requires (CE0166, measured on 11.15.0-rc.4).
func TestSendEmailSettingsListIsStrict(t *testing.T) {
	cases := map[string]struct{ settings, want string }{
		"unknown key":           {sendEmailMinimum + ", Subjet: 'x'", "did you mean 'Subject'"},
		"repeated key":          {sendEmailMinimum + ", Port: 26", "sets Port twice"},
		"address with params":   {sendEmailMinimum + ", Cc: 'x' with ({1} = 'y')", "Cc takes an expression"},
		"subject as template":   {sendEmailMinimum + ", Subject: template 'x'", "Subject takes a text"},
		"body a map":            {sendEmailMinimum + ", Body: ('a': 'b')", "Body takes template"},
		"headers not a map":     {sendEmailMinimum + ", Headers: 'X-Id'", "Headers takes a map"},
		"header name a word":    {sendEmailMinimum + ", Headers: (XId: 'x')", "a header name is a string"},
		"header value an expr":  {sendEmailMinimum + ", Headers: ('X-Id': $Msg/Id)", "takes a string, not an expression"},
		"repeated header":       {sendEmailMinimum + ", Headers: ('X-Id': 'a', 'x-id': 'b')", "sets header"},
		"attachment an expr":    {sendEmailMinimum + ", Attachment: $Msg/Doc", "Attachment takes a variable"},
		"unknown security type": {sendEmailMinimum + ", SecurityType: starttls", "SecurityType takes none, ssl or tls"},
		"identity not boolean":  {sendEmailMinimum + ", CheckServerIdentity: 'yes'", "takes true or false"},
		"timeout not a number":  {sendEmailMinimum + ", ConnectionTimeout: @M.Timeout", "number of milliseconds"},
		"auth not basic":        {sendEmailMinimum + ", Authentication: $Msg/User", "Authentication takes basic"},
		"auth missing password": {sendEmailMinimum + ", Authentication: basic (Username: 'u')", "needs Password"},
		"no From":               {"To: 'b@x.com', Host: 'h', Port: 25", "needs From"},
		"no Host":               {"From: 'a@x.com', To: 'b@x.com', Port: 25", "needs Host"},
		"no Port":               {"From: 'a@x.com', To: 'b@x.com', Host: 'h'", "needs Port"},
		"no recipient":          {"From: 'a@x.com', Host: 'h', Port: 25", "needs a recipient"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, errs := Build(sendEmailFlow(tc.settings))
			var msgs []string
			for _, e := range errs {
				msgs = append(msgs, e.Error())
			}
			if !strings.Contains(strings.Join(msgs, "\n"), tc.want) {
				t.Errorf("send email (%s): errors %q, want one containing %q", tc.settings, msgs, tc.want)
			}
		})
	}

	// CONTROL: the minimum is clean, and a Cc or Bcc alone is a recipient —
	// as is an activity without a Subject (mxbuild: no error).
	for _, ok := range []string{sendEmailMinimum, "From: 'a', Bcc: 'b', Host: 'h', Port: 25"} {
		if _, errs := Build(sendEmailFlow(ok)); len(errs) > 0 {
			t.Errorf("send email (%s): unexpected errors %v", ok, errs)
		}
	}
}

// ADR-0013 for a new activity: there is no clause form at all, so the old
// clause spelling is a parse error rather than a deprecated alias.
func TestSendEmailHasNoClauseForm(t *testing.T) {
	if _, errs := Build("create microflow M.F () begin send email from 'a' to 'b' subject 'Hi' host 'h' port 25; end;"); len(errs) == 0 {
		t.Error("the clause form parsed; send email takes only the settings list")
	}
}

func sendEmailOf(t *testing.T, prog *ast.Program) *ast.SendEmailStmt {
	t.Helper()
	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok || len(mf.Body) == 0 {
		t.Fatalf("not a microflow with a body: %#v", prog.Statements[0])
	}
	se, ok := mf.Body[0].(*ast.SendEmailStmt)
	if !ok {
		t.Fatalf("first statement is %T, want *ast.SendEmailStmt", mf.Body[0])
	}
	return se
}
