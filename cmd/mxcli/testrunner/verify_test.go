// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// TestVerifyCanariesFail is the regression test for FINDINGS #48: no OQL, well
// formed or not, true or not, against a real entity or an invented one, could
// make a @verify fail. Each row below is one the reporter measured as a wrongly
// green PASS. They must now either compare (and fail) or be rejected outright.
func TestVerifyCanariesFail(t *testing.T) {
	// Comparable: these parse, and would run against the app.
	for _, raw := range []string{
		"select count(*) from Sudoku.Game = 999999",
		"select count(*) from Sudoku.Cell = 0",
		"select count(*) from Sudoku.NoSuchEntity = 1",
	} {
		v, err := ParseVerify(raw)
		if err != nil {
			t.Errorf("ParseVerify(%q): %v", raw, err)
			continue
		}
		if v.Query == "" || v.Operator == "" || v.Expected == "" {
			t.Errorf("ParseVerify(%q) produced an incomplete assertion: %+v", raw, v)
		}
	}

	// Not comparable: these must be errors, not passes.
	for _, raw := range []string{
		"select count(*) frm Sudoku.Cell = 1", // malformed: no FROM
		"this is not a query",                 // not OQL at all
		"select count(*) from Sudoku.Cell",    // no assertion at all
	} {
		if v, err := ParseVerify(raw); err == nil {
			t.Errorf("ParseVerify(%q) accepted it as %+v; want an error", raw, v)
		}
	}
}

// TestParseVerifySplitsOnTheLastTopLevelComparison pins the parse rule. The
// expected value is a literal at the very end, so the split is the last
// comparison operator outside quotes and parentheses — which is what keeps a
// WHERE clause's own `=` out of it.
func TestParseVerifySplitsOnTheLastTopLevelComparison(t *testing.T) {
	cases := []struct{ raw, query, op, expected string }{
		{"select count(*) from Mod.E = 81", "select count(*) from Mod.E", "=", "81"},
		{"select count(*) from Mod.E where Value = 5 = 81",
			"select count(*) from Mod.E where Value = 5", "=", "81"},
		{"select count(*) from Mod.E where Name = 'a = b' = 2",
			"select count(*) from Mod.E where Name = 'a = b'", "=", "2"},
		{"select count(*) from Mod.E where Id in (select Id from Mod.F where X = 1) = 3",
			"select count(*) from Mod.E where Id in (select Id from Mod.F where X = 1)", "=", "3"},
		{"select count(*) from Mod.E > 0", "select count(*) from Mod.E", ">", "0"},
		{"select count(*) from Mod.E >= 1", "select count(*) from Mod.E", ">=", "1"},
		{"select count(*) from Mod.E <> 0", "select count(*) from Mod.E", "!=", "0"},
		{"select Name from Mod.E = 'Widget'", "select Name from Mod.E", "=", "'Widget'"},
	}
	for _, c := range cases {
		v, err := ParseVerify(c.raw)
		if err != nil {
			t.Errorf("ParseVerify(%q): %v", c.raw, err)
			continue
		}
		if v.Query != c.query || v.Operator != c.op || v.Expected != c.expected {
			t.Errorf("ParseVerify(%q) = {%q %q %q}, want {%q %q %q}",
				c.raw, v.Query, v.Operator, v.Expected, c.query, c.op, c.expected)
		}
	}
}

// TestParseVerifyRejectsANonLiteralExpectation. The right-hand side has to be
// something to compare against. Without this the split silently eats the last
// predicate of a WHERE clause and sends a truncated query to the runtime.
func TestParseVerifyRejectsANonLiteralExpectation(t *testing.T) {
	for _, raw := range []string{
		"select count(*) from Mod.E where Value = SomeColumn",
		"select count(*) from Mod.E = ",
	} {
		if _, err := ParseVerify(raw); err == nil {
			t.Errorf("ParseVerify(%q) accepted a non-literal expectation", raw)
		}
	}
}

// TestVerifyComparesScalars pins the comparison itself, including the thing that
// makes it subtle: the runtime is asked for numbers as strings, so "81" coming
// back has to compare numerically against 81 rather than by text.
func TestVerifyComparesScalars(t *testing.T) {
	cases := []struct {
		raw    string
		actual any
		ok     bool
	}{
		{"select count(*) from Mod.E = 81", "81", true},
		{"select count(*) from Mod.E = 81", "27", false},
		{"select count(*) from Mod.E = 81", float64(81), true},
		{"select count(*) from Mod.E > 0", "1", true},
		{"select count(*) from Mod.E > 0", "0", false},
		{"select count(*) from Mod.E != 0", "5", true},
		{"select count(*) from Mod.E != 0", "0", false},
		{"select Name from Mod.E = 'Widget'", "Widget", true},
		{"select Name from Mod.E = 'Widget'", "Gadget", false},
		{"select IsDone from Mod.E = true", true, true},
		{"select IsDone from Mod.E = true", false, false},
	}
	for _, c := range cases {
		v, err := ParseVerify(c.raw)
		if err != nil {
			t.Fatalf("ParseVerify(%q): %v", c.raw, err)
		}
		got, _, err := v.compare(c.actual)
		if err != nil {
			t.Errorf("%q vs %v: %v", c.raw, c.actual, err)
			continue
		}
		if got != c.ok {
			t.Errorf("%q vs %v = %v, want %v", c.raw, c.actual, got, c.ok)
		}
	}
}

// TestVerifyRequiresAScalarResult. A query returning a table cannot be compared
// against a literal, and guessing which cell was meant is exactly the kind of
// silent wrong answer this whole area is about.
func TestVerifyRequiresAScalarResult(t *testing.T) {
	v, err := ParseVerify("select count(*) from Mod.E = 1")
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range []*oqlScalarCase{
		{name: "no rows", cols: []string{"c"}, rows: nil},
		{name: "two rows", cols: []string{"c"}, rows: [][]any{{"1"}, {"2"}}},
		{name: "two columns", cols: []string{"a", "b"}, rows: [][]any{{"1", "2"}}},
	} {
		if _, err := v.scalarOf(res.cols, res.rows); err == nil {
			t.Errorf("%s: scalarOf accepted a non-scalar result", res.name)
		}
	}
	got, err := v.scalarOf([]string{"c"}, [][]any{{"81"}})
	if err != nil {
		t.Fatalf("scalar result rejected: %v", err)
	}
	if got != "81" {
		t.Errorf("scalarOf = %v, want 81", got)
	}
}

type oqlScalarCase struct {
	name string
	cols []string
	rows [][]any
}

// TestVerifyIsRefusedUnderRollback. @verify asserts on rows the microflow wrote,
// and @cleanup rollback — the default — undoes them before anything can look.
// Running it anyway would compare against the pre-test state and report a
// confident, wrong answer.
func TestVerifyIsRefusedUnderRollback(t *testing.T) {
	doc := `/**
 * @test writes a row
 * @verify select count(*) from Mod.E = 1
 */`
	a := parseAnnotations(doc)
	if a.Cleanup != "rollback" {
		t.Fatalf("precondition: cleanup = %q, want the rollback default", a.Cleanup)
	}
	if len(a.AssertionErrors) != 1 {
		t.Fatalf("AssertionErrors = %d, want 1 — @verify ran under rollback", len(a.AssertionErrors))
	}
	if !strings.Contains(a.AssertionErrors[0], "@cleanup none") {
		t.Errorf("AssertionErrors[0] = %q, want it to name the fix", a.AssertionErrors[0])
	}

	withNone := parseAnnotations(`/**
 * @test writes a row
 * @cleanup none
 * @verify select count(*) from Mod.E = 1
 */`)
	if len(withNone.AssertionErrors) != 0 {
		t.Errorf("@cleanup none still refused: %v", withNone.AssertionErrors)
	}
	if len(withNone.Verify) != 1 {
		t.Errorf("Verify = %d, want 1", len(withNone.Verify))
	}
}

// TestVerifyCountsAsAnAssertion — it is one, now that it is evaluated.
func TestVerifyCountsAsAnAssertion(t *testing.T) {
	tc := TestCase{Verify: []Verify{{Raw: "select count(*) from Mod.E = 1"}}}
	if tc.AssertionCount() != 1 {
		t.Errorf("AssertionCount() = %d, want 1", tc.AssertionCount())
	}
}

// TestRunVerifiesDowngradesTheResult pins the three outcomes end to end against
// a stubbed OQL endpoint: a holding assertion leaves a PASS alone, a false one
// makes it FAIL with the observed value, and one that cannot be evaluated makes
// it ERROR — which FailCount counts, so the run exits non-zero.
func TestRunVerifiesDowngradesTheResult(t *testing.T) {
	cases := []struct {
		name    string
		verify  string
		reply   string
		want    TestStatus
		message string
	}{
		{"holds", "select count(*) from Mod.E = 81", `{"data":[{"c":"81"}]}`, StatusPass, ""},
		{"does not hold", "select count(*) from Mod.E = 81", `{"data":[{"c":"27"}]}`, StatusFail, "actual: 27"},
		{"bad query", "select count(*) from Mod.NoSuch = 1",
			`{"error":"Unknown entity Mod.NoSuch"}`, StatusError, "Unknown entity"},
		{"not a scalar", "select count(*) from Mod.E = 1",
			`{"data":[{"a":"1","b":"2"}]}`, StatusError, "one row and one column"},
		{"no rows", "select count(*) from Mod.E = 1", `{"data":[]}`, StatusError, "no rows"},
	}

	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, c.reply)
		}))

		v, err := ParseVerify(c.verify)
		if err != nil {
			srv.Close()
			t.Errorf("%s: ParseVerify: %v", c.name, err)
			continue
		}
		tc := TestCase{ID: "test_1", Name: c.name, Cleanup: "none", Verify: []Verify{v}}
		res := newResult(tc)
		res.Status = StatusPass
		runVerifies(&res, tc, adminOptionsForURL(srv.URL), "")
		srv.Close()

		if res.Status != c.want {
			t.Errorf("%s: status = %v (%q), want %v", c.name, res.Status, res.Message, c.want)
			continue
		}
		if c.message != "" && !strings.Contains(res.Message, c.message) {
			t.Errorf("%s: message = %q, want it to mention %q", c.name, res.Message, c.message)
		}
	}
}

// adminOptionsForURL points the admin client at a stub server.
func adminOptionsForURL(rawURL string) docker.M2EEOptions {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	port, _ := strconv.Atoi(u.Port())
	return docker.M2EEOptions{Host: u.Hostname(), Port: port, Direct: true}
}

// TestVerifyFailureAlwaysCarriesTheActualValue. A FAIL that reports only the
// expectation is the message this change set out to improve, so every comparison
// path has to thread the observed value through — including the boolean and null
// ones, which have no ordering and took a separate route.
func TestVerifyFailureAlwaysCarriesTheActualValue(t *testing.T) {
	cases := []struct {
		raw    string
		actual any
		want   string
	}{
		{"select n as n from Mod.E = 81", "27", "27"},
		{"select f as f from Mod.E = true", false, "false"},
		{"select s as s from Mod.E = 'Widget'", "Gadget", "Gadget"},
		{"select s as s from Mod.E != empty", nil, "NULL"},
	}
	for _, c := range cases {
		v, err := ParseVerify(c.raw)
		if err != nil {
			t.Fatalf("ParseVerify(%q): %v", c.raw, err)
		}
		_, shown, err := v.compare(c.actual)
		if err != nil {
			t.Errorf("%q: %v", c.raw, err)
			continue
		}
		if shown != c.want {
			t.Errorf("%q vs %v: shown = %q, want %q", c.raw, c.actual, shown, c.want)
		}
	}
}
