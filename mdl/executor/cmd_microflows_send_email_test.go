// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

const sendEmailHeader = "create microflow M.SUB_Send ($Msg: M.Msg, $Doc: M.Doc) returns Boolean\nbegin\n"

// The full statement, every clause at a non-default value. describe prints the
// clauses in this order, so the body is also the expected description.
const sendEmailFull = `send email
    from $Msg/Sender
    to $Msg/Recipient
    cc 'cc@example.com'
    bcc 'bcc@example.com'
    subject 'Order {1} for {2}' with ({1} = $Msg/Number, {2} = $Msg/Name)
    body text 'Hello {1}' with ({1} = $Msg/Name)
    body html '<p>Hello</p>'
    header 'X-Correlation-Id' = 'abc-123'
    header 'X-Priority' = '1'
    attachment $Doc
    host @M.SmtpHost port @M.SmtpPort
    security ssl check server identity
    timeout 30000
    auth basic @M.SmtpUser password @M.SmtpPassword;`

func sendEmailActionOf(t *testing.T, oc *microflows.MicroflowObjectCollection) *microflows.SendEmailAction {
	t.Helper()
	for _, obj := range oc.Objects {
		if aa, ok := obj.(*microflows.ActionActivity); ok {
			if a, ok := aa.Action.(*microflows.SendEmailAction); ok {
				return a
			}
		}
	}
	t.Fatal("no SendEmailAction in the built flow")
	return nil
}

// mendixlabs/mxcli#1315: `send email` builds a SendEmailAction holding exactly
// what the statement says, and describe gives the statement back — a fixed
// point, so describe → exec keeps the activity.
func TestSendEmail_BuildsAndDescribesRoundTrip(t *testing.T) {
	described, oc := describeFold(t, sendEmailHeader+"  "+sendEmailFull+"\n  return true;\nend;")
	a := sendEmailActionOf(t, oc)

	want := &microflows.SendEmailAction{
		BaseElement:         a.BaseElement,
		ErrorHandlingType:   microflows.ErrorHandlingTypeRollback,
		From:                "$Msg/Sender",
		Host:                "@M.SmtpHost",
		Port:                "@M.SmtpPort",
		SecurityType:        microflows.EmailSecuritySSL,
		CheckServerIdentity: true,
		ConnectionTimeout:   30000,
		UseAuthentication:   true,
		Username:            "@M.SmtpUser",
		Password:            "@M.SmtpPassword",
		To:                  "$Msg/Recipient",
		Cc:                  "'cc@example.com'",
		Bcc:                 "'bcc@example.com'",
		Subject:             microflows.EmailTemplate{Text: "Order {1} for {2}", Parameters: []string{"$Msg/Number", "$Msg/Name"}},
		BodyPlainText:       microflows.EmailTemplate{Text: "Hello {1}", Parameters: []string{"$Msg/Name"}},
		BodyHTML:            microflows.EmailTemplate{Text: "<p>Hello</p>"},
		CustomHeaders: []microflows.EmailCustomHeader{
			{Name: "X-Correlation-Id", Value: "abc-123"},
			{Name: "X-Priority", Value: "1"},
		},
		Attachment: "Doc",
	}
	if got, w := fmtAction(a), fmtAction(want); got != w {
		t.Errorf("built action differs\n got  %s\n want %s", got, w)
	}

	if !strings.Contains(described, sendEmailFull) {
		t.Errorf("describe did not give the statement back\n got:\n%s\n want to contain:\n%s", described, sendEmailFull)
	}
	again, _ := describeFold(t, sendEmailHeader+described+"\nend;")
	if again != described {
		t.Errorf("describe is not a fixed point\nfirst:\n%s\n\nsecond:\n%s", described, again)
	}
}

// Defaults are omitted from describe (R12) and the minimal statement builds
// the platform defaults: TLS, 20000 ms, no server-identity check, no auth.
func TestSendEmail_MinimalStatementUsesPlatformDefaults(t *testing.T) {
	minimal := "send email\n    from 'a@example.com'\n    to 'b@example.com'\n    subject 'Hi'\n    host 'smtp.example.com' port 25;"
	described, oc := describeFold(t, sendEmailHeader+"  "+minimal+"\n  return true;\nend;")
	a := sendEmailActionOf(t, oc)
	if a.SecurityType != microflows.EmailSecurityTLS || a.ConnectionTimeout != 20000 || a.CheckServerIdentity || a.UseAuthentication {
		t.Errorf("defaults: security %q timeout %d identity %v auth %v; want TLS 20000 false false",
			a.SecurityType, a.ConnectionTimeout, a.CheckServerIdentity, a.UseAuthentication)
	}
	if !strings.Contains(described, minimal) {
		t.Errorf("describe printed a default\n got:\n%s\n want to contain:\n%s", described, minimal)
	}
}

// A subject that is an expression rather than a template is stored the way
// Studio Pro would hold it: '{1}' with the expression as the parameter — the
// rule `log` applies to its message. Measured on mxbuild 11.15.0-rc.4: clean.
func TestSendEmail_ExpressionSubjectBecomesOneParameterTemplate(t *testing.T) {
	src := "  send email from 'a@x.com' to 'b@x.com' subject $Msg/Subject host 'h' port 25;\n  return true;"
	_, oc := describeFold(t, sendEmailHeader+src+"\nend;")
	a := sendEmailActionOf(t, oc)
	if a.Subject.Text != "{1}" || len(a.Subject.Parameters) != 1 || a.Subject.Parameters[0] != "$Msg/Subject" {
		t.Errorf("Subject = %+v, want {Text:{1} Parameters:[$Msg/Subject]}", a.Subject)
	}
}

// The custom error handler is wired as on any other activity: without it the
// activity stores CustomWithoutRollback with no error flow (CE0011).
func TestSendEmail_CustomErrorHandler(t *testing.T) {
	src := `  send email from 'a@x.com' to 'b@x.com' subject 'Hi' host 'h' port 25
    on error without rollback begin
      log error node 'Mail' 'failed';
      return false;
    end error;
  return true;`
	described, oc := describeFold(t, sendEmailHeader+src+"\nend;")
	if a := sendEmailActionOf(t, oc); a.ErrorHandlingType != microflows.ErrorHandlingTypeCustomWithoutRollback {
		t.Errorf("ErrorHandlingType = %q, want CustomWithoutRollback", a.ErrorHandlingType)
	}
	errorFlows := 0
	for _, f := range oc.Flows {
		if f.IsErrorHandler {
			errorFlows++
		}
	}
	if errorFlows != 1 {
		t.Errorf("got %d error-handler flows, want 1", errorFlows)
	}
	if !strings.Contains(described, "on error without rollback begin") || !strings.Contains(described, "log error node 'Mail' 'failed';") {
		t.Errorf("describe lost the error handler:\n%s", described)
	}
}

// sendEmailViolations validates a statement with clauses spliced in: those
// starting with `header` go before `host`, the rest after `port` (the grammar
// fixes the order).
func sendEmailViolations(t *testing.T, clauses string) map[string]string {
	t.Helper()
	before, after := "", clauses
	if strings.HasPrefix(clauses, "header") {
		before, after = clauses+" ", ""
	}
	prog, errs := visitor.Build("create microflow M.SUB_Send () begin\n  send email from 'a@x.com' to 'b@x.com' subject 'Hi' " +
		before + "host 'h' port 25 " + after + ";\nend;")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	out := map[string]string{}
	for _, v := range ValidateMicroflow(prog.Statements[0].(*ast.CreateMicroflowStmt)) {
		if strings.HasPrefix(v.RuleID, "MDL-EMAIL") {
			out[v.RuleID] = v.Severity.String()
		}
	}
	return out
}

func TestSendEmail_CheckRules(t *testing.T) {
	for _, tc := range []struct {
		clauses string
		want    map[string]string
	}{
		{"", map[string]string{}},
		{"security ssl check server identity", map[string]string{}},
		{"security none", map[string]string{}},
		{"header 'X-Correlation-Id' = 'abc'", map[string]string{}},
		// MDL-EMAIL01: the grammar takes any word; the builder can map three.
		{"security starttls", map[string]string{"MDL-EMAIL01": "error"}},
		// MDL-EMAIL02: a warning, because mxbuild 11.15.0-rc.4 accepts it.
		{"security tls check server identity", map[string]string{"MDL-EMAIL02": "warning"}},
		{"security none check server identity", map[string]string{"MDL-EMAIL02": "warning"}},
		// MDL-EMAIL03: Studio Pro's header rules, also accepted by mxbuild.
		{"header 'Bad Name!' = 'v'", map[string]string{"MDL-EMAIL03": "warning"}},
		{"header 'X-Empty' = ''", map[string]string{"MDL-EMAIL03": "warning"}},
	} {
		t.Run(tc.clauses, func(t *testing.T) {
			got := sendEmailViolations(t, tc.clauses)
			if fmtMap(got) != fmtMap(tc.want) {
				t.Errorf("violations = %v, want %v", got, tc.want)
			}
		})
	}
}

// The activity is server-side SMTP: a nanoflow cannot contain it.
func TestSendEmail_RefusedInNanoflow(t *testing.T) {
	errs := validateNanoflowBody([]ast.MicroflowStatement{&ast.SendEmailStmt{}})
	if len(errs) == 0 || !strings.Contains(errs[0], "SEND EMAIL") {
		t.Fatalf("validateNanoflowBody = %v, want a SEND EMAIL refusal", errs)
	}
}

// 11.13.0 is the floor: the 11.12 EmailMessage still carries the expression
// Subject/MessageBody* that 11.13 deleted, and 10.x has no such activity.
func TestSendEmail_VersionGate(t *testing.T) {
	at := func(major, minor int) *ExecContext {
		ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{
			IsConnectedFunc: func() bool { return true },
			ProjectVersionFunc: func() *types.ProjectVersion {
				return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor}
			},
		}))
		return ctx
	}
	stmt := parseMicroflowStmt(t, "create microflow M.SUB_Send () begin\n  send email from 'a@x.com' to 'b@x.com' subject 'Hi' host 'h' port 25;\nend;")

	for _, v := range [][2]int{{10, 24}, {11, 12}} {
		_, err := buildMicroflowFromStmt(at(v[0], v[1]), stmt, buildFlowOpts{})
		if err == nil || !strings.Contains(err.Error(), "send email") || !strings.Contains(err.Error(), "11.13") {
			t.Errorf("%d.%d: err = %v, want the send email version refusal naming 11.13", v[0], v[1], err)
		}
	}
	if _, err := buildMicroflowFromStmt(at(11, 13), stmt, buildFlowOpts{}); err != nil && strings.Contains(err.Error(), "send email") {
		t.Errorf("11.13 satisfies the minimum; got: %v", err)
	}
}

func parseMicroflowStmt(t *testing.T, src string) *ast.CreateMicroflowStmt {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	return prog.Statements[0].(*ast.CreateMicroflowStmt)
}

func fmtAction(a *microflows.SendEmailAction) string {
	c := *a
	c.BaseElement = model.BaseElement{}
	return fmt.Sprintf("%+v", c)
}

func fmtMap(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k, v := range m {
		keys = append(keys, k+"="+v)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}
