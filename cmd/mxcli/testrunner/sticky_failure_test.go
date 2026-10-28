// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"
)

// twoExpectSuite is one test with two @expect lines, the shape the after-startup
// runner reports as two verdict lines: writeExpectAssertion logs PASS or FAIL
// for each assertion on its own.
func twoExpectSuite(t *testing.T) *TestSuite {
	t.Helper()
	e1, err := ParseExpect("$r = 1")
	if err != nil {
		t.Fatalf("ParseExpect: %v", err)
	}
	e2, err := ParseExpect("$r = 2")
	if err != nil {
		t.Fatalf("ParseExpect: %v", err)
	}
	tc := TestCase{ID: "test_1", Name: "two expects", MDL: "$r = 2;", Expects: []Expect{e1, e2}}
	return &TestSuite{Name: "S", Tests: []TestCase{tc}}
}

// verdictLog is a runner log for test_1 with the given verdict lines between
// its RUN line and the suite's END.
func verdictLog(lines ...string) string {
	var b strings.Builder
	b.WriteString("MXTEST: MXTEST:START:S\n")
	b.WriteString("MXTEST: MXTEST:RUN:test_1:two expects\n")
	for _, l := range lines {
		b.WriteString("MXTEST: MXTEST:" + l + "\n")
	}
	b.WriteString("MXTEST: MXTEST:END:S\n")
	return b.String()
}

// TestLaterPassDoesNotClearEarlierFailure is ako/mxcli#1753: on the after-startup
// runner a test with more than one @expect reported PASS when an earlier
// assertion failed and a later one held, and `mxcli test` exited 0. Each
// assertion logs its own verdict line, and the parser let every line overwrite
// the test's status — the last one won, and a PASS also cleared the failure
// message. The endpoint runner evaluates an assertion only while the verdict is
// still PASS, so there the first failure sticks; the log parser now does the
// same.
func TestLaterPassDoesNotClearEarlierFailure(t *testing.T) {
	const want = "Expected $r = 1, actual: 2"

	got := ParseLogResults(strings.NewReader(verdictLog("FAIL:test_1:"+want, "PASS:test_1")),
		twoExpectSuite(t), false)
	r := got.Tests[0]
	if r.Status != StatusFail {
		t.Errorf("FAIL then PASS: status = %v, want FAIL", r.Status)
	}
	if r.Message != want {
		t.Errorf("FAIL then PASS: message = %q, want %q", r.Message, want)
	}
	if got.AllPassed() {
		t.Error("FAIL then PASS: the suite passed, so `mxcli test` exits 0")
	}

	// Control: the order the old parser already got right.
	got = ParseLogResults(strings.NewReader(verdictLog("PASS:test_1", "FAIL:test_1:"+want)),
		twoExpectSuite(t), false)
	if r := got.Tests[0]; r.Status != StatusFail || r.Message != want {
		t.Errorf("control, PASS then FAIL: got %v %q, want FAIL %q", r.Status, r.Message, want)
	}

	// Control: every assertion held.
	got = ParseLogResults(strings.NewReader(verdictLog("PASS:test_1", "PASS:test_1")),
		twoExpectSuite(t), false)
	if r := got.Tests[0]; r.Status != StatusPass || r.Message != "" {
		t.Errorf("control, PASS then PASS: got %v %q, want PASS", r.Status, r.Message)
	}
}

// TestFirstFailureMessageIsReported pins which message a test that failed more
// than once reports: the first, as the endpoint runner's $Verdict does, with a
// count of the rest so the reader knows the log holds more than the summary.
func TestFirstFailureMessageIsReported(t *testing.T) {
	got := ParseLogResults(strings.NewReader(verdictLog(
		"FAIL:test_1:Expected $r = 1, actual: 2",
		"FAIL:test_1:Expected $r = 3, actual: 2",
		"PASS:test_1",
		"FAIL:test_1:Expected $r = 4, actual: 2",
	)), twoExpectSuite(t), false)
	r := got.Tests[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %v, want FAIL", r.Status)
	}
	const want = "Expected $r = 1, actual: 2 (and 2 more failures in this test)"
	if r.Message != want {
		t.Errorf("message = %q, want %q", r.Message, want)
	}
}

// TestSkippedAssertionsFailureStillWins pins the guard writeTestBlock emits after
// the assertions: when a call's own handler resumed the flow past some of them,
// the assertions that did run log PASS and the guard logs a FAIL after them
// (ako/mxcli#1448). That FAIL is the first failure, so it is the verdict.
func TestSkippedAssertionsFailureStillWins(t *testing.T) {
	got := ParseLogResults(strings.NewReader(verdictLog(
		"PASS:test_1",
		"FAIL:test_1:"+assertionsSkippedReason,
	)), twoExpectSuite(t), false)
	if r := got.Tests[0]; r.Status != StatusFail || r.Message != assertionsSkippedReason {
		t.Errorf("got %v %q, want FAIL with the skipped-assertions reason", r.Status, r.Message)
	}
}

// TestLaterPassDoesNotClearEarlierError: a setup that threw logs ERROR, and no
// verdict line after it may turn the test into a pass or a skip either.
func TestLaterPassDoesNotClearEarlierError(t *testing.T) {
	const msg = "Setup failed: Shop.Seed"
	for _, later := range []string{"PASS:test_1", "SKIP:test_1:skipped"} {
		got := ParseLogResults(strings.NewReader(verdictLog("ERROR:test_1:"+msg, later)),
			twoExpectSuite(t), false)
		if r := got.Tests[0]; r.Status != StatusError || r.Message != msg {
			t.Errorf("ERROR then %s: got %v %q, want ERROR %q", later, r.Status, r.Message, msg)
		}
	}
}

// TestRunLineStartsVerdictAfresh: a failure sticks for the run it was logged in,
// not for the rest of the log. A RUN line begins a test's verdict, so a log that
// holds the same test twice reports the later run.
func TestRunLineStartsVerdictAfresh(t *testing.T) {
	log := verdictLog("FAIL:test_1:Expected $r = 1, actual: 2", "RUN:test_1:two expects", "PASS:test_1", "PASS:test_1")
	got := ParseLogResults(strings.NewReader(log), twoExpectSuite(t), false)
	if r := got.Tests[0]; r.Status != StatusPass {
		t.Errorf("got %v %q, want the second run's PASS", r.Status, r.Message)
	}
}
