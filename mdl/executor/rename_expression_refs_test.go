// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#1671: RENAME CONSTANT reported success and left every use of the
// constant inside an expression naming the old one — mx check: CE1613 "The
// selected constant '…' no longer exists". A constant is always written
// @Module.Name, so the reference sweep's whole-string match never saw one, not
// even a constant that was the whole expression. An enumeration value inside a
// longer expression kept its old name too (CE0117), and a cross-module MOVE of
// a constant had the same gap through the other sweep primitive.
func TestRenameConstantAndEnumeration_RewritesTheirUsesInsideExpressions(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const literal = `'literal mentions Shop1671.C_Root, @Shop1671.C_Root and Shop1671.Status.Open'`
	setup := `create module Shop1671;
create module Other1671;
create constant Shop1671.C_Root (Type: String, DefaultValue: 'https://example.com');
create constant Shop1671.C_Limit (Type: Integer, DefaultValue: '10');
create enumeration Shop1671.Status (Open 'Open', Closed 'Closed');
create microflow Shop1671.UseConstants () returns String
begin
  declare $Url String = @Shop1671.C_Root + '/orders';
  declare $Max Integer = @Shop1671.C_Limit * 2;
  log info node 'Shop' 'root is ' + @Shop1671.C_Root;
  declare $S enum Shop1671.Status = Shop1671.Status.Open;
  if $S = Shop1671.Status.Open and $Max > @Shop1671.C_Limit then
    return @Shop1671.C_Root;
  end if;
  return ` + literal + `;
end;`
	if err := afRun(t, exec, setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	describe := func() string {
		t.Helper()
		out.Reset()
		if err := afRun(t, exec, "describe microflow Shop1671.UseConstants;"); err != nil {
			t.Fatalf("describe: %v\n%s", err, out.String())
		}
		return out.String()
	}
	run := func(stmt string) string {
		t.Helper()
		out.Reset()
		if err := afRun(t, exec, stmt); err != nil {
			t.Fatalf("%s: %v\n%s", stmt, err, out.String())
		}
		return out.String()
	}
	wantIn := func(desc string, frags ...string) {
		t.Helper()
		for _, f := range frags {
			if !strings.Contains(desc, f) {
				t.Errorf("describe lacks %q:\n%s", f, desc)
			}
		}
	}
	// stale counts the old spelling outside the string literal, which must
	// keep mentioning it verbatim.
	stale := func(desc, old string) int {
		t.Helper()
		if !strings.Contains(desc, literal) {
			t.Errorf("the string literal was changed; want it verbatim:\n%s", desc)
		}
		return strings.Count(strings.ReplaceAll(desc, literal, ""), old)
	}

	report := run("rename constant Shop1671.C_Root to C_Base;")
	if !strings.Contains(report, "Updated 3 reference(s)") {
		t.Errorf("rename constant should report the three expression uses it rewrote:\n%s", report)
	}
	desc := describe()
	wantIn(desc, "@Shop1671.C_Base + '/orders'", "'root is ' + @Shop1671.C_Base", "return @Shop1671.C_Base;")
	if n := stale(desc, "@Shop1671.C_Root"); n != 0 {
		t.Errorf("%d use(s) of @Shop1671.C_Root left after the rename:\n%s", n, desc)
	}

	run("rename constant Shop1671.C_Limit to C_Max;")
	desc = describe()
	// An XPath holding a constant (`[Total > @M.C]`) is covered by the
	// rewriter's own tests: MDL cannot spell a constant inside an XPath yet.
	wantIn(desc, "@Shop1671.C_Max * 2", "$Max > @Shop1671.C_Max")
	if n := stale(desc, "@Shop1671.C_Limit"); n != 0 {
		t.Errorf("%d use(s) of @Shop1671.C_Limit left after the rename:\n%s", n, desc)
	}

	run("rename enumeration Shop1671.Status to State;")
	desc = describe()
	wantIn(desc, "$S = Shop1671.State.Open and")
	if n := stale(desc, "Shop1671.Status"); n != 0 {
		t.Errorf("%d use(s) of Shop1671.Status left after the rename:\n%s", n, desc)
	}

	// The cross-module move goes through UpdateQualifiedNameInAllUnits, the
	// sweep's other primitive.
	run("move constant Shop1671.C_Base to Other1671;")
	desc = describe()
	wantIn(desc, "@Other1671.C_Base + '/orders'", "return @Other1671.C_Base;")
	if n := stale(desc, "@Shop1671.C_Base"); n != 0 {
		t.Errorf("%d use(s) of @Shop1671.C_Base left after the move:\n%s", n, desc)
	}
}
