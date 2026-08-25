// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"
)

// `COMMENT 'text'` on a CREATE statement was parsed and thrown away: the visitor
// stored it on a field the executor never read, so the statement reported
// success and set no documentation at all. Measured on 11.13.0 — entity,
// enumeration, association, module, microflow, nanoflow and rule all dropped it,
// and the text appeared nowhere in the written project.
//
// The `/** … */` doc comment does work, and carries more (@param, @returns), so
// the dead option is gone rather than wired up: one spelling beats one that
// works and one that lies. For MODULE it could never have been wired —
// Projects$Module has no Documentation property.
//
// COMMENT survives everywhere it does something: constants, JSON structures,
// image collections, database connections, workflow activities, and
// ALTER … SET COMMENT. Associations keep it too — there NEITHER spelling works,
// which is a separate defect, and removing the option would have left no inline
// way to say it at all.

// parseErr returns the combined parse errors for a script, or "" when it parses.
func parseErr(t *testing.T, script string) string {
	t.Helper()
	_, errs := Build(script)
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "\n")
}

func TestCreateCommentOptionIsRejected(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"entity", "create entity M.E ( Name : string(50) ) comment 'x';"},
		{"enumeration", "create enumeration M.E ( A 'Alpha' ) comment 'x';"},
		{"module", "create module M comment 'x';"},
		{"microflow", "create microflow M.MF () comment 'x' begin log 'y'; end;"},
		{"nanoflow", "create nanoflow M.NF () comment 'x' begin log 'y'; end;"},
		{"rule", "create rule M.R () returns boolean comment 'x' begin return true; end;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseErr(t, tc.script); got == "" {
				t.Fatalf("`comment` was accepted on create %s; it sets no documentation, so it must not parse", tc.name)
			}
		})
	}
}

// The control. Without it, "comment is rejected" could be satisfied by breaking
// the option everywhere, including the six places it does real work.
func TestCreateCommentOptionSurvivesWhereItWorks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"constant", "create constant M.C type string default 'v' comment 'x';"},
		{"json structure", `create json structure M.J comment 'x' snippet '{"a": 1}';`},
		{"image collection", "create image collection M.I comment 'x';"},
		{"association", "create association M.A_B from M.A to M.B comment 'x';"},
		{"alter entity set comment", "alter entity M.E set comment 'x';"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseErr(t, tc.script); got != "" {
				t.Fatalf("`comment` no longer parses on %s, where it does work:\n%s", tc.name, got)
			}
		})
	}
}

// A removed option must say what to write instead. "no viable alternative" sends
// the author hunting for a different spelling of a thing that no longer exists.
func TestRemovedCreateCommentErrorNamesTheReplacement(t *testing.T) {
	got := parseErr(t, "create microflow M.MF () comment 'x' begin log 'y'; end;")
	if got == "" {
		t.Fatal("expected a parse error")
	}
	for _, want := range []string{"/**", "set no documentation"} {
		if !strings.Contains(got, want) {
			t.Errorf("error does not mention %q:\n%s", want, got)
		}
	}
}

// A CREATE is usually written across several lines, so the offending line is the
// option on its own — which is the shape that actually occurs in the wild (14 of
// them in one example file). A hint that only matched the single-line form would
// have missed every real case.
func TestRemovedCreateCommentHintFiresOnItsOwnLine(t *testing.T) {
	got := parseErr(t, "create microflow M.MF ()\nreturns string\ncomment 'x'\nbegin\n  log 'y';\nend;")
	if got == "" {
		t.Fatal("expected a parse error")
	}
	if !strings.Contains(got, "set no documentation") {
		t.Errorf("no hint on a multi-line statement, where the option sits on its own line:\n%s", got)
	}
}

// The hint keys off the source line, so it must not fire on the statements that
// still take a COMMENT — a constant with an unrelated syntax error should get
// the ordinary message, not advice to delete a valid option.
func TestRemovedCreateCommentHintDoesNotFireOnConstants(t *testing.T) {
	got := parseErr(t, "create constant M.C type string default comment 'x';")
	if got == "" {
		t.Skip("this malformed constant happens to parse; nothing to assert")
	}
	if strings.Contains(got, "set no documentation") {
		t.Errorf("the removed-COMMENT hint fired on a constant, where COMMENT is valid:\n%s", got)
	}
}
