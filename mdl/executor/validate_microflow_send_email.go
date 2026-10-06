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

// checkSendEmail validates the clauses of SEND EMAIL that the grammar cannot.
//
// MDL-EMAIL01 (error): the security mode is none, ssl or tls. The grammar
// accepts any word there so SSL and TLS need not be keywords; the builder
// cannot map anything else.
//
// MDL-EMAIL02 (warning): `check server identity` only with `security ssl`.
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
	if stmt.Security != "" {
		mode, ok := emailSecurityType(stmt.Security)
		if !ok {
			v.addViolation("MDL-EMAIL01", linter.SeverityError,
				fmt.Sprintf("send email: unknown security mode %q", stmt.Security),
				"Write `security none`, `security ssl` or `security tls` (the default when the clause is omitted).")
		} else {
			sec = mode
		}
	}
	if stmt.CheckServerIdentity && sec != microflows.EmailSecuritySSL {
		v.addViolation("MDL-EMAIL02", linter.SeverityWarning,
			fmt.Sprintf("send email: `check server identity` has no effect without `security ssl` (security is %s)", sec),
			"Studio Pro offers Check Server Identity only for SSL. Use `security ssl check server identity`, or drop `check server identity`.")
	}
	for _, h := range stmt.Headers {
		if !emailHeaderName.MatchString(h.Name) {
			v.addViolation("MDL-EMAIL03", linter.SeverityWarning,
				fmt.Sprintf("send email: header name %q may contain only letters, digits and hyphens", h.Name),
				"Rename the header, e.g. 'X-Correlation-Id'.")
		}
		if h.Value == "" || strings.ContainsAny(h.Value, "\r\n") {
			v.addViolation("MDL-EMAIL03", linter.SeverityWarning,
				fmt.Sprintf("send email: header %q needs a value without line breaks", h.Name),
				"Give the header a single-line, non-empty value.")
		}
	}
}
