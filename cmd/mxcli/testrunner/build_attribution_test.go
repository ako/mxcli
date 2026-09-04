// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// problem builds one MxBuild problem located in a document.
func problem(code, msg, module, document, element string) docker.BuildProblem {
	return docker.BuildProblem{
		Severity:  "Error",
		ErrorCode: code,
		Message:   msg,
		Locations: []docker.BuildLocation{{Module: module, Document: document, Element: element}},
	}
}

func suiteOf(ids ...string) *TestSuite {
	s := &TestSuite{Name: "suite"}
	for _, id := range ids {
		s.Tests = append(s.Tests, TestCase{ID: id, Name: "name of " + id})
	}
	return s
}

// TestAttributeBuildProblems covers the mapping from an MxBuild location back to
// the test whose generated microflow it names.
//
// The shape is measured, not assumed: on Mendix 11.13 a serve build reports the
// document WITHOUT its module — `Microflow 'Test_test_3'` — with the module in
// the same location object.
func TestAttributeBuildProblems(t *testing.T) {
	suite := suiteOf("test_1", "test_3")

	t.Run("an error in a generated test microflow is attributed to it", func(t *testing.T) {
		p := problem("CE0117", "Error(s) in expression.", "MxTest", "Microflow 'Test_test_3'", "End event")
		byTest, other := attributeBuildProblems([]docker.BuildProblem{p}, suite)
		if len(other) != 0 {
			t.Fatalf("unattributed: %v", other)
		}
		if got := byTest["test_3"]; len(got) != 1 {
			t.Fatalf("byTest[test_3] = %v, want 1 problem", got)
		}
	})

	t.Run("an error in the user's own model is not blamed on a test", func(t *testing.T) {
		// The whole point of the split: reporting this as a test failure sends
		// the reader to the wrong file.
		p := problem("CE0109", "Undefined variable 'x'.", "Sudoku", "Microflow 'SUB_Deal'", "End event")
		byTest, other := attributeBuildProblems([]docker.BuildProblem{p}, suite)
		if len(byTest) != 0 {
			t.Errorf("must not attribute a project error to a test: %v", byTest)
		}
		if len(other) != 1 {
			t.Errorf("other = %v, want the problem", other)
		}
	})

	t.Run("a microflow in MxTest that is not a generated test is not attributed", func(t *testing.T) {
		// MxTest also holds the endpoint registration flow.
		p := problem("CE0109", "Undefined variable 'x'.", "MxTest", "Microflow 'RegisterEndpoint'", "End event")
		byTest, other := attributeBuildProblems([]docker.BuildProblem{p}, suite)
		if len(byTest) != 0 || len(other) != 1 {
			t.Errorf("byTest=%v other=%v — a non-test MxTest document must stay unattributed", byTest, other)
		}
	})

	t.Run("a generated name from a different run is not attributed", func(t *testing.T) {
		// test_9 is not in this suite; claiming it would invent a result.
		p := problem("CE0117", "Error(s) in expression.", "MxTest", "Microflow 'Test_test_9'", "End event")
		byTest, other := attributeBuildProblems([]docker.BuildProblem{p}, suite)
		if len(byTest) != 0 || len(other) != 1 {
			t.Errorf("byTest=%v other=%v — an unknown test id must stay unattributed", byTest, other)
		}
	})
}

// TestResultsFromFailedBuild covers what a run reports when the build fails: the
// blamed test is an ERROR and every other test is a SKIP.
//
// No test may be reported as passing. A build that never produced a runtime
// evaluated nothing, and a suite that reports green for something it did not
// evaluate is the defect this whole area exists to prevent.
func TestResultsFromFailedBuild(t *testing.T) {
	suite := suiteOf("test_1", "test_2", "test_3")
	p := problem("CE0117", "Error(s) in expression.", "MxTest", "Microflow 'Test_test_2'", "End event")

	results := resultsFromFailedBuild([]docker.BuildProblem{p}, suite)
	if len(results) != 3 {
		t.Fatalf("got %d results, want one per test", len(results))
	}

	byID := map[string]TestResult{}
	for _, r := range results {
		byID[r.ID] = r
		if r.Status == StatusPass {
			t.Errorf("%s reported PASS after a failed build — nothing ran", r.ID)
		}
	}

	if got := byID["test_2"]; got.Status != StatusError {
		t.Errorf("test_2 status = %v, want ERROR", got.Status)
	} else {
		for _, want := range []string{"CE0117", "Error(s) in expression"} {
			if !strings.Contains(got.Message, want) {
				t.Errorf("test_2 message %q does not carry %q", got.Message, want)
			}
		}
	}
	for _, id := range []string{"test_1", "test_3"} {
		if got := byID[id]; got.Status != StatusSkip {
			t.Errorf("%s status = %v, want SKIP", id, got.Status)
		}
	}
}

// TestResultsFromFailedBuildDeclinesUnattributableErrors is the control on the
// other side: when the failure is in the user's model, the runner must NOT
// manufacture per-test rows. Without this, every build error in the project
// would be reported as a suite of skipped tests and the real cause would vanish.
func TestResultsFromFailedBuildDeclinesUnattributableErrors(t *testing.T) {
	suite := suiteOf("test_1")
	p := problem("CE0109", "Undefined variable 'x'.", "Sudoku", "Microflow 'SUB_Deal'", "End event")

	if results := resultsFromFailedBuild([]docker.BuildProblem{p}, suite); results != nil {
		t.Fatalf("expected no results for a project-level failure, got %v", results)
	}

	_, other := attributeBuildProblems([]docker.BuildProblem{p}, suite)
	hint := buildFailureHint(other)
	for _, want := range []string{"in the project, not in the tests", "CE0109", "SUB_Deal"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint %q does not mention %q", hint, want)
		}
	}
}
