// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"
)

// @throws replaces the test body's normal outcome, so its @expect assertions are
// never emitted into the generated microflow. Counting them as assertions while
// evaluating none of them is the same silent gap that made a dropped @expect
// look like a passing one: the test reports "2 assertions" and makes one.
func TestExpectWithThrowsIsRefused(t *testing.T) {
	a := parseAnnotations(`/**
 * @test an error is expected
 * @throws 'boom'
 * @expect $result = 'never evaluated'
 */`)

	if len(a.Expects) != 0 {
		t.Errorf("Expects = %+v, want none kept", a.Expects)
	}
	if len(a.AssertionErrors) != 1 {
		t.Fatalf("AssertionErrors = %v, want one explaining the combination", a.AssertionErrors)
	}
	msg := a.AssertionErrors[0]
	for _, want := range []string{"@throws", "$result = 'never evaluated'"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q does not mention %q", msg, want)
		}
	}
	if a.Throws != "boom" {
		t.Errorf("Throws = %q, want boom — the @throws itself still stands", a.Throws)
	}
}

// The ERROR has to reach the runner, not just the annotation struct: a test that
// cannot evaluate an assertion must not be generated or reported as a pass.
func TestExpectWithThrowsIsAnErrorEndToEnd(t *testing.T) {
	tests, err := parseMDLTests(`/**
 * @test an error is expected
 * @throws 'boom'
 * @expect $result = 'never evaluated'
 */
$result = CALL MICROFLOW M.Explode();
/
`, "throws.test.mdl")
	if err != nil {
		t.Fatalf("parseMDLTests: %v", err)
	}
	if len(tests) != 1 {
		t.Fatalf("got %d test(s), want 1", len(tests))
	}
	tc := tests[0]
	if tc.AssertionCount() != 1 {
		t.Errorf("AssertionCount = %d, want 1 — only the @throws is evaluated",
			tc.AssertionCount())
	}
	res, ok := assertionErrorResult(tc)
	if !ok || res.Status != StatusError {
		t.Errorf("result = %+v (ok=%v), want an ERROR", res, ok)
	}
	if mdl := GenerateTestFlows(&TestSuite{Name: "s", Tests: []TestCase{tc}}); strings.Contains(mdl, testFlowName(tc)) {
		t.Errorf("a test with an assertion it cannot evaluate got a microflow:\n%s", mdl)
	}
}

// Control: @throws on its own is untouched, and so is an @expect on its own.
func TestThrowsAloneAndExpectAloneAreUnaffected(t *testing.T) {
	only := parseAnnotations(`/**
 * @test just throws
 * @throws 'boom'
 */`)
	if only.Throws != "boom" || len(only.AssertionErrors) != 0 {
		t.Errorf("throws alone: %+v", only)
	}

	plain := parseAnnotations(`/**
 * @test just expects
 * @expect $result = 1
 */`)
	if len(plain.Expects) != 1 || len(plain.AssertionErrors) != 0 {
		t.Errorf("expect alone: %+v", plain)
	}
}
