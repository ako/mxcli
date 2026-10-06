// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// emailHeaderName is what Studio Pro allows in a custom header's Name: letters,
// digits and hyphens (Send Email reference guide).
var emailHeaderName = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// checkSendEmail validates the settings of SEND EMAIL that the visitor's
// shape checks leave open. (The visitor already refuses a SecurityType other
// than none/ssl/tls, and every unknown, repeated or misshapen key.)
//
// MDL-EMAIL02 (warning): CheckServerIdentity only with SecurityType: ssl.
// Studio Pro enables the option only for SSL. mxbuild 11.15.0-rc.4 does NOT
// reject it under TLS (0 errors, measured), so this is a warning: the stored
// flag is one the editor would never have let you set.
//
// MDL-EMAIL03 (warning): a custom header's name is letters, digits and hyphens,
// and its value is non-empty without line breaks — the reference guide's rules,
// enforced by Studio Pro's editor. mxbuild 11.15.0-rc.4 accepts 'Bad Name!'
// (measured), so a script is the only way to store one, and it fails only when
// the mail is sent.
func (v *microflowValidator) checkSendEmail(stmt *ast.SendEmailStmt) {
	sec := microflows.EmailSecurityTLS
	if mode, ok := emailSecurityType(stmt.Security); ok {
		sec = mode
	}
	if stmt.CheckServerIdentity && sec != microflows.EmailSecuritySSL {
		v.addViolation("MDL-EMAIL02", linter.SeverityWarning,
			fmt.Sprintf("send email: CheckServerIdentity has no effect without SecurityType: ssl (it is %s)", sec),
			"Studio Pro offers Check Server Identity only for SSL. Set SecurityType: ssl, or drop CheckServerIdentity.")
	}
	for _, h := range stmt.Headers {
		if !emailHeaderName.MatchString(h.Name) {
			v.addViolation("MDL-EMAIL03", linter.SeverityWarning,
				fmt.Sprintf("send email: header name %q may contain only letters, digits and hyphens", h.Name),
				"Rename the header, e.g. Headers: ('X-Correlation-Id': 'value').")
		}
		if h.Value == "" || strings.ContainsAny(h.Value, "\r\n") {
			v.addViolation("MDL-EMAIL03", linter.SeverityWarning,
				fmt.Sprintf("send email: header %q needs a value without line breaks", h.Name),
				"Give the header a single-line, non-empty value.")
		}
	}
}
