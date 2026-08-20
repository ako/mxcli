// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAssertionCountIsReported is the second half of the silent-pass fix. The
// first half stopped an @expect from being dropped; this one stops a test that
// carries no assertion at all from being indistinguishable, in the output, from
// one that carries six. The reporting suite's 22/22 was 6 real and 16 vacuous,
// and the cheapest way back to green after the first fix is to delete the
// @expect line — which must not look like a repair.
func TestAssertionCountIsReported(t *testing.T) {
	cases := []struct {
		name string
		tc   TestCase
		want int
	}{
		{"two expects", TestCase{Expects: []Expect{expectOf("$a = 1"), expectOf("$b = 2")}}, 2},
		{"throws is an assertion", TestCase{Throws: "boom"}, 1},
		{"nothing at all", TestCase{}, 0},
		// @verify is evaluated now, so it counts.
		{"verify counts", TestCase{Verify: []Verify{{Raw: "select count(*) from Mod.E = 1"}}}, 1},
	}
	for _, c := range cases {
		if got := c.tc.AssertionCount(); got != c.want {
			t.Errorf("%s: AssertionCount() = %d, want %d", c.name, got, c.want)
		}
	}
}

// TestZeroAssertionTestsAreVisibleInOutput pins that a smoke test says so.
func TestZeroAssertionTestsAreVisibleInOutput(t *testing.T) {
	sr := &SuiteResult{Name: "s", Tests: []TestResult{
		{ID: "1", Name: "asserts things", Status: StatusPass, Assertions: 3},
		{ID: "2", Name: "asserts nothing", Status: StatusPass, Assertions: 0},
	}}
	var b strings.Builder
	PrintResults(&b, sr, false)
	out := b.String()

	if !strings.Contains(out, "3 assertions") {
		t.Errorf("assertion count missing from the passing line:\n%s", out)
	}
	if !strings.Contains(out, "no assertions") {
		t.Errorf("a zero-assertion test is not marked:\n%s", out)
	}
	if !strings.Contains(out, `1 test(s) asserted nothing beyond "did not throw"`) {
		t.Errorf("summary does not call out the vacuous tests:\n%s", out)
	}
}

// TestRequireAssertionsMakesVacuousTestsErrors pins the opt-in enforcement. It
// is off by default because a smoke test is legitimate; it exists so a project
// that has decided otherwise can fail its CI on one.
func TestRequireAssertionsMakesVacuousTestsErrors(t *testing.T) {
	tc := TestCase{ID: "test_1", Name: "asserts nothing", SourceFile: "a.test.mdl"}

	if res, bad := vacuousResult(tc, false); bad {
		t.Errorf("a zero-assertion test errored without --require-assertions: %+v", res)
	}
	res, bad := vacuousResult(tc, true)
	if !bad {
		t.Fatal("--require-assertions did not flag a zero-assertion test")
	}
	if res.Status != StatusError {
		t.Errorf("status = %v, want ERROR", res.Status)
	}
	if !strings.Contains(res.Message, "no assertions") {
		t.Errorf("message = %q, want it to say the test asserts nothing", res.Message)
	}
}

// TestJUnitCarriesSourceFileAndAssertions pins the CI-side reporting: a failure
// in a multi-file run has to say which file it came from, and the assertion
// count has to survive into the report a CI actually renders.
func TestJUnitCarriesSourceFileAndAssertions(t *testing.T) {
	sr := &SuiteResult{Name: "mxtest", Tests: []TestResult{
		{ID: "1", Name: "a", Status: StatusPass, Assertions: 2, SourceFile: "tests/board.test.mdl"},
		{ID: "2", Name: "b", Status: StatusPass, Assertions: 0, SourceFile: "tests/mix.test.mdl"},
	}}
	var b strings.Builder
	if err := WriteJUnitXML(&b, sr); err != nil {
		t.Fatalf("WriteJUnitXML: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, `classname="tests.board"`) {
		t.Errorf("classname does not identify the source file:\n%s", out)
	}
	if !strings.Contains(out, `<property name="assertions" value="2">`) &&
		!strings.Contains(out, `<property name="assertions" value="2"></property>`) {
		t.Errorf("assertion count missing from the JUnit report:\n%s", out)
	}
}

// TestRequireAssertionsIsHonouredOnTheLogRunner is the regression test for #926.
//
// --require-assertions was registered on `mxcli test` globally but consulted in
// exactly one place — the endpoint runner's loop. The Docker path (and
// `--local --legacy-runner`) assembles its results from the runtime log in
// ParseLogResults, which had no parameter through which the flag could reach it,
// so `mxcli test tests/ -p app.mpr --require-assertions` — the command in the
// issue, note the absent --local — accepted the flag and exited 0 on a suite
// that asserted nothing.
//
// Whether a test is vacuous is a property of the *parsed* test case, known
// before any runner starts. No runner has an excuse for not knowing it, which is
// why this is implemented on the log path rather than refused there the way
// @verify is (that one genuinely cannot be evaluated during boot).
func TestRequireAssertionsIsHonouredOnTheLogRunner(t *testing.T) {
	// One test that asserts nothing, which the runtime log reports as a pass.
	newSuite := func() *TestSuite {
		return &TestSuite{Name: "s", Tests: []TestCase{
			{ID: "test_1", Name: "asserts nothing", SourceFile: "a.test.mdl"},
		}}
	}
	const log = "MXTEST: MXTEST:START:\n" +
		"MXTEST: MXTEST:RUN:test_1:asserts nothing\n" +
		"MXTEST: MXTEST:PASS:test_1\n" +
		"MXTEST: MXTEST:END:\n"

	// Off by default: a smoke test is legitimate.
	off := ParseLogResults(strings.NewReader(log), newSuite(), false)
	if got := off.Tests[0].Status; got != StatusPass {
		t.Errorf("without --require-assertions: status = %v, want PASS", got)
	}
	if !off.AllPassed() {
		t.Error("without --require-assertions the suite should still pass")
	}

	on := ParseLogResults(strings.NewReader(log), newSuite(), true)
	if got := on.Tests[0].Status; got != StatusError {
		t.Errorf("with --require-assertions: status = %v, want ERROR "+
			"(the flag is a silent no-op on this runner — issue #926)", got)
	}
	if on.ErrorCount() != 1 {
		t.Errorf("ErrorCount() = %d, want 1", on.ErrorCount())
	}
	if on.AllPassed() {
		t.Error("with --require-assertions the suite must not report all-passed")
	}
	if msg := on.Tests[0].Message; !strings.Contains(msg, "no assertions") {
		t.Errorf("message = %q, want it to say the test asserts nothing", msg)
	}
}

// TestPreRunVerdictsHaveOneCallSite is the structural half of the #926 fix.
//
// The bug was not that vacuousResult was wrong — its unit test passed all along,
// because it exercised the helper and not any runner. The bug was that one of
// the two result-assembly loops called it and the other did not. Pinning the
// helpers to a single call site is what stops a third verdict (or a third
// runner) from reintroducing the same asymmetry.
func TestPreRunVerdictsHaveOneCallSite(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, helper := range []string{"assertionErrorResult(", "vacuousResult("} {
		var callers []string
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(src), "\n") {
				if !strings.Contains(line, helper) || strings.Contains(line, "func "+helper[:len(helper)-1]) {
					continue
				}
				callers = append(callers, fmt.Sprintf("%s:%d", f, i+1))
			}
		}
		if len(callers) != 1 {
			t.Errorf("%s has %d call sites %v, want exactly 1 (preRunResult) — "+
				"a pre-run verdict reachable from one runner and not the other is issue #926",
				helper, len(callers), callers)
		}
	}
}
