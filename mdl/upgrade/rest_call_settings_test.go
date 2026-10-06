// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"
)

// ADR-0013 / MDL-DEPR720: `fmt --upgrade` rewrites call rest service's clauses
// as its settings list, editing only the keywords between the values. The
// visitor test (mdl/visitor/rest_call_settings_test.go) proves each pair builds
// the same statement; this one proves the rewrite produces that pair, also
// next to the rewrites of `rest call` (MDL-DEPR094) and `returns none`
// (MDL-DEPR024) in the same statement.
func TestUpgrade_RestCallClausesBecomeSettingsList(t *testing.T) {
	flow := func(stmt string) string {
		return "create microflow M.F ($T: String, $U: String) begin\n  " + stmt + ";\nend;\n"
	}
	cases := []struct{ old, want string }{
		{flow("$R = call rest service get 'https://x' header 'Accept' = 'text/html' header XKey = $U timeout 300 returns String"),
			flow("$R = call rest service get 'https://x' (Headers: ('Accept': 'text/html', 'XKey': $U), Timeout: 300) returns String")},
		{flow("$R = call rest service post 'https://x' auth basic $U password $T body '{1}' with ({1} = $T) returns response"),
			flow("$R = call rest service post 'https://x' (Authentication: basic (Username: $U, Password: $T), Body: template '{1}' with ({1} = $T)) returns response")},
		{flow("call rest service put 'https://x' body $T returns nothing"),
			flow("call rest service put 'https://x' (Body: $T) returns nothing")},
		{flow("$R = call rest service post 'https://x' body binary $T returns M.Download"),
			flow("$R = call rest service post 'https://x' (Body: binary $T) returns M.Download")},
		{flow("$R = call rest service post 'https://x' body mapping M.EXM from $T returns mapping M.IMM as M.E on error continue"),
			flow("$R = call rest service post 'https://x' (Body: mapping M.EXM from $T) returns mapping M.IMM as M.E on error continue")},
		{flow("$R = REST CALL GET 'https://x' AUTH BASIC 'u' PASSWORD 'it''s' BODY 'a' TIMEOUT 1 RETURNS NONE"),
			flow("$R = CALL REST SERVICE GET 'https://x' (Authentication: BASIC (Username: 'u', Password: 'it''s'), Body: TEMPLATE 'a', Timeout: 1) RETURNS NOTHING")},
		// Clauses on lines of their own become the list as describe prints
		// it: one setting per line, trailing comma, `) returns` at the
		// statement's indent.
		{flow("$R = call rest service get 'https://x'\n    header 'A' = 'b'\n    header 'C' = 'd'\n    timeout 30\n    returns String"),
			flow("$R = call rest service get 'https://x' (\n    Headers: ('A': 'b', 'C': 'd'),\n    Timeout: 30,\n  ) returns String")},
		{flow("$R = call rest service post 'https://x' with ({1} = $T)\n    body '{1}' with (\n      {1} = $U\n    )\n    returns String\n    on error continue"),
			flow("$R = call rest service post 'https://x' with ({1} = $T) (\n    Body: template '{1}' with (\n      {1} = $U\n    ),\n  ) returns String\n    on error continue")},
		// A comment before `returns` is kept, so the layout is too.
		{flow("$R = call rest service get 'https://x'\n    timeout 30 -- seconds\n    returns String"),
			flow("$R = call rest service get 'https://x'\n    (Timeout: 30) -- seconds\n    returns String")},
	}
	for _, c := range cases {
		res := mustUpgrade(t, c.old, Options{})
		if res.Source != c.want {
			t.Errorf("upgrade of\n%s got:\n%s want:\n%s", c.old, res.Source, c.want)
		}
		if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
			t.Errorf("upgrade is not idempotent on\n%s", res.Source)
		}
	}
}
