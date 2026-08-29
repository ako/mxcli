// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

func TestWarningsDoNotBlockExec(t *testing.T) {
	// A warning that refuses the write is not a warning. This regressed for
	// MDL022 long before MDL071 existed: the exec-time guard took vs[0]
	// regardless of severity, so `check` reported "0 errors, 1 warnings" and
	// `exec` failed the same script — the exact check/exec divergence
	// ValidateProgram's contract forbids.
	warnOnly := []linter.Violation{
		{RuleID: "MDL022", Severity: linter.SeverityWarning, Message: "renamed on write"},
		{RuleID: "MDL071", Severity: linter.SeverityWarning, Message: "OQL reserved word"},
	}
	if v := firstBlockingViolation(warnOnly); v != nil {
		t.Errorf("warnings must not block the write, got %s", v.RuleID)
	}

	// An error still blocks, and the FIRST error is the one reported — a
	// preceding warning must not shadow it.
	mixed := []linter.Violation{
		{RuleID: "MDL071", Severity: linter.SeverityWarning, Message: "OQL reserved word"},
		{RuleID: "MDL021", Severity: linter.SeverityError, Message: "reserved word (CE7247)"},
		{RuleID: "MDL023", Severity: linter.SeverityError, Message: "autonumber has no seed"},
	}
	v := firstBlockingViolation(mixed)
	if v == nil {
		t.Fatal("an error-severity violation must still block the write")
	}
	if v.RuleID != "MDL021" {
		t.Errorf("reported %s, want the first ERROR (MDL021) — a warning must not shadow it", v.RuleID)
	}
}
