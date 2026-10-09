// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func deprecationViolations(t *testing.T, src string, policy deprecation.Policy) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var out []linter.Violation
	for _, v := range ApplyDeprecationPolicy(ValidateProgram(prog, ""), policy) {
		if deprecation.IsDeprecationCode(v.RuleID) {
			out = append(out, v)
		}
	}
	return out
}

// `check` and `exec` share ValidateProgram, so a deprecated spelling warns in
// both. The canonical script is the control: it must stay silent, or the
// warning would fire on everything.
func TestDeprecatedSpellingsWarnThroughValidateProgram(t *testing.T) {
	old := "create or replace enumeration M.Color (Red 'Red');\nshow entities in M;\n"
	canon := "create or modify enumeration M.Color (Red 'Red');\nlist entities in M;\n"

	got := deprecationViolations(t, old, deprecation.Warn)
	if len(got) != 2 || got[0].RuleID != deprecation.CreateOrReplace || got[1].RuleID != deprecation.Show {
		t.Fatalf("got %+v, want MDL-DEPR001 then MDL-DEPR002", got)
	}
	for _, v := range got {
		if v.Severity != linter.SeverityWarning {
			t.Errorf("%s severity = %v, want warning — an error would refuse working scripts", v.RuleID, v.Severity)
		}
	}
	if !strings.Contains(got[0].Message, "line 1") || !strings.Contains(got[0].Message, "create or modify") {
		t.Errorf("message must name the line and the canonical form: %q", got[0].Message)
	}
	if !strings.Contains(got[1].Message, "line 2") || !strings.Contains(got[1].Message, "list") {
		t.Errorf("message must name the line and the canonical form: %q", got[1].Message)
	}
	if summary := linter.Summarize(ValidateProgram(mustProgram(t, old), "")); summary.Errors > 0 {
		t.Errorf("deprecated spellings produced %d error(s) by default; exec would refuse the script", summary.Errors)
	}

	if got := deprecationViolations(t, canon, deprecation.Warn); len(got) != 0 {
		t.Errorf("canonical script reported %+v", got)
	}
}

func TestDeprecationPolicyErrorFailsTheRun(t *testing.T) {
	src := "show modules;"
	got := deprecationViolations(t, src, deprecation.Error)
	if len(got) != 1 || got[0].Severity != linter.SeverityError {
		t.Fatalf("under --deprecations=error got %+v, want one error", got)
	}
	// Only registry codes are promoted: another warning stays a warning.
	other := []linter.Violation{{RuleID: "MDL065", Severity: linter.SeverityWarning}}
	if v := ApplyDeprecationPolicy(other, deprecation.Error); v[0].Severity != linter.SeverityWarning {
		t.Errorf("MDL065 promoted to %v; only MDL-DEPRnnn codes may be", v[0].Severity)
	}
}

func mustProgram(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return prog
}

// Every deprecation warning ends by pointing at the code's registry entry, so
// the reader is one command from the old form, the new form, the rewrite and
// the version that refuses it (ako/mxcli#714 decision 3). Driven by every
// registry entry's own example, so an entry added later is covered.
func TestDeprecationWarningsEndWithHelpPointer(t *testing.T) {
	for _, e := range deprecation.All() {
		if e.RemovedIn <= 0 {
			continue
		}
		got := deprecationViolations(t, e.Example, deprecation.Warn)
		found := false
		for _, v := range got {
			if v.RuleID != e.Code {
				continue
			}
			found = true
			if want := "(mxcli help " + e.Code + ")"; !strings.HasSuffix(v.Message, want) {
				t.Errorf("%s: warning does not end with %s: %q", e.Code, want, v.Message)
			}
		}
		if !found {
			t.Errorf("%s: its example recorded no warning under its own code: %+v", e.Code, got)
		}
	}
}

// MDL-DEPR081's message says "same meaning", so the form it tells you to write
// must be the one the brackets stored. Dropping the brackets alone rebinds a
// bare attribute: `Visible: [Active]` roots Active in $currentObject, while
// `Visible: Active` does not — a hand migration from the old text changed every
// such condition (sudoku FINDINGS #63). fmt --upgrade was already right.
func TestBracketedWidgetConditionMessageNamesCurrentObject(t *testing.T) {
	src := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dataview dv (DataSource: $E) { " +
		"textbox t (Attribute: Name, Visible: [Active]) } };"
	got := deprecationViolations(t, src, deprecation.Warn)
	if len(got) != 1 || got[0].RuleID != deprecation.BracketedWidgetCondition {
		t.Fatalf("got %+v, want one %s", got, deprecation.BracketedWidgetCondition)
	}
	msg := got[0].Message
	write := msg[strings.Index(msg, "write `"):]
	write = write[:strings.Index(write, "— same meaning")]
	if !strings.Contains(write, "$currentObject/") {
		t.Errorf("the form the message says to write drops the $currentObject binding: %q", msg)
	}
}
