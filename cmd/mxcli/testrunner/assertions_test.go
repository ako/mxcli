// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
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
		// @verify is parsed and never executed, so it asserts nothing and must
		// not be counted as if it did.
		{"verify does not count", TestCase{Verify: []string{"select 1 = 1"}}, 0},
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

// TestVerifyIsRejectedRatherThanIgnored. @verify is documented in the skill's
// annotation table as an OQL post-condition, is parsed into the TestCase, and is
// then read by nothing but `--list`. That is exactly the shape of the defect
// this whole change exists to remove: an annotation that looks like an assertion
// and asserts nothing. Until it is implemented it must be an error.
func TestVerifyIsRejectedRatherThanIgnored(t *testing.T) {
	doc := `/**
 * @test writes a row
 * @verify select count(*) from Mod.E where Code = 'X' = 1
 */`
	a := parseAnnotations(doc)
	if len(a.AssertionErrors) != 1 {
		t.Fatalf("AssertionErrors: got %d, want 1 — @verify was silently ignored", len(a.AssertionErrors))
	}
	if !strings.Contains(a.AssertionErrors[0], "@verify") {
		t.Errorf("AssertionErrors[0] = %q, want it to name @verify", a.AssertionErrors[0])
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
