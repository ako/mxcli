// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

func oqlRuleIDs(oql string) []string {
	var out []string
	for _, v := range ValidateOQLSyntax(oql) {
		out = append(out, v.RuleID)
	}
	return out
}

func hasOQLRule(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestMDL072_RefusesAQuotedSelectAlias(t *testing.T) {
	// Measured on 11.13.0: OQL rejects a quoted alias for ANY name, not just a
	// reserved one — `as "Total"` is CE0174 "The '\"' part is incomplete or
	// incorrect. You could use here: IDENTIFIER."
	//
	// The alias is the one position where quoting is unavailable, and the two
	// CE0174 texts say so precisely: a SOURCE position lists "ASTERISK, AT_SIGN,
	// OPEN_QUOTE, or IDENTIFIER", the alias position lists only "IDENTIFIER".
	ids := oqlRuleIDs(`select sum(s.Amount) as "Total" from M.Sales as s`)
	if !hasOQLRule(ids, "MDL072") {
		t.Fatalf("no MDL072 for a quoted alias, got %v", ids)
	}
	// MDL030 must NOT also fire: the column plainly HAS an alias, and saying it
	// has none sends the author to add a second one.
	if hasOQLRule(ids, "MDL030") {
		t.Errorf("MDL030 (\"has no as alias\") fired on a column that has one: %v", ids)
	}
}

func TestMDL072_MessageNamesTheRemedyAndTheExceptionToIt(t *testing.T) {
	var v *struct{ Msg, Sug string }
	for _, x := range ValidateOQLSyntax(`select s.Amount as "Month" from M.Sales as s`) {
		if x.RuleID == "MDL072" {
			v = &struct{ Msg, Sug string }{x.Message, x.Suggestion}
		}
	}
	if v == nil {
		t.Fatal("no MDL072")
	}
	// Unquoting is the fix for an ordinary name...
	if !strings.Contains(v.Sug, "as Month") {
		t.Errorf("suggestion should show the unquoted form: %s", v.Sug)
	}
	// ...but NOT for a reserved one, where the column must be renamed. Getting
	// this wrong sends someone in a circle: unquote -> CE0174 -> quote -> CE0174.
	if !strings.Contains(v.Sug, "renamed") {
		t.Errorf("suggestion must say a reserved name needs a rename, not a requote: %s", v.Sug)
	}
	// And it must not leave the impression that quoting is useless everywhere —
	// in a SOURCE position it is exactly the right fix (MDL032 says so).
	if !strings.Contains(v.Sug, "source position") {
		t.Errorf("suggestion should distinguish the source position, where quoting works: %s", v.Sug)
	}
}

func TestQuotedIdentifierInASourcePositionIsAccepted(t *testing.T) {
	// The control for MDL072, and the thing that makes MDL032's advice sound.
	// Measured at 0 errors on 11.13.0:
	//
	//	select s."Month" as MonthNo, sum(s.Amount) as Total
	//	from MyFirstModule.Sales as s group by s."Month"
	oql := `select s."Month" as MonthNo, sum(s.Amount) as Total ` +
		`from MyFirstModule.Sales as s group by s."Month"`
	for _, bad := range []string{"MDL072", "MDL030", "MDL032"} {
		if hasOQLRule(oqlRuleIDs(oql), bad) {
			t.Errorf("%s fired on a query mxbuild accepts at 0 errors", bad)
		}
	}
}

func TestMDL030StillCatchesAGenuinelyMissingAlias(t *testing.T) {
	// Control: MDL072 short-circuits the MDL030 branch, so this asserts the
	// short-circuit did not swallow the case MDL030 exists for.
	if !hasOQLRule(oqlRuleIDs(`select sum(s.Amount) from M.Sales as s`), "MDL030") {
		t.Error("MDL030 no longer reports a column with no alias at all")
	}
}

func TestOQLAliasHelpers(t *testing.T) {
	for _, tc := range []struct {
		in     string
		quoted bool
		bare   string
	}{
		{`"Month"`, true, "Month"},
		{"`Month`", true, "Month"},
		{"Month", false, "Month"},
		{`"`, false, `"`}, // a lone quote is not a quoted identifier
		{"", false, ""},
	} {
		if got := isQuotedOQLIdent(tc.in); got != tc.quoted {
			t.Errorf("isQuotedOQLIdent(%q) = %v, want %v", tc.in, got, tc.quoted)
		}
		if got := unquoteOQLIdent(tc.in); got != tc.bare {
			t.Errorf("unquoteOQLIdent(%q) = %q, want %q", tc.in, got, tc.bare)
		}
	}
}
