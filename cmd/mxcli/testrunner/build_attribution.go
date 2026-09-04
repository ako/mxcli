// SPDX-License-Identifier: Apache-2.0

// Attributing a failed build back to the test that caused it.
//
// An @expect the runner cannot compile is already an ERROR at parse time. What
// is left is everything only MxBuild decides — an undefined variable (CE0109), a
// String compared to a number (CE0117) — where nothing upstream of the build has
// the information to object.
//
// Checking those earlier was tried and abandoned. The microflow validator tracks
// variables where they are ASSIGNED, and reusing that model to check READS
// produces false refusals: `$latestHttpResponse` is a Mendix system variable that
// no MDL statement declares, and a loop iterator is only registered when the
// list's type is known. Both refuse a microflow `mx check` accepts at 0 errors.
// The scope model is right for the bar it was built for and wrong for this one,
// so the build stays the authority and this file makes its verdict legible.
//
// When that happens the build fails, the runtime never boots, and the run
// produces no test results at all. The failure is real; what was missing is
// saying which test caused it. MxBuild locates every problem
// (`module` + `document`), and the test microflows are generated with names this
// package chose, so the mapping back is exact rather than a guess.
//
// ako/mxcli-sudoku FINDINGS #46, follow-up.
package testrunner

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// testFlowDocumentPattern matches the `document` MxBuild reports for a generated
// test microflow.
//
// MxBuild names the document WITHOUT its module — `Microflow 'Test_test_3'`,
// with the module carried separately in the same location — so this matches the
// bare name and the module is checked alongside it. Anchored on the generated
// prefix, so an error in the user's own microflow is never blamed on a test.
var testFlowDocumentPattern = regexp.MustCompile(`^Microflow '(Test_[^']*)'$`)

// attributeBuildProblems splits a failed build's errors into those belonging to
// a generated test microflow and those that do not.
//
// The second group matters as much as the first: an error in the user's own
// model also fails the build, and reporting it as a test failure would send them
// looking in the wrong place.
func attributeBuildProblems(problems []docker.BuildProblem, suite *TestSuite) (map[string][]docker.BuildProblem, []docker.BuildProblem) {
	byTest := map[string][]docker.BuildProblem{}
	var other []docker.BuildProblem

	// Keyed on the BARE document name MxBuild reports, not the qualified one.
	known := map[string]string{} // "Test_test_3" -> test ID
	if suite != nil {
		for _, tc := range suite.Tests {
			known[strings.TrimPrefix(testFlowName(tc), mxTestModule+".")] = tc.ID
		}
	}

	for _, p := range problems {
		id := ""
		for _, loc := range p.Locations {
			if !strings.EqualFold(loc.Module, mxTestModule) {
				continue
			}
			m := testFlowDocumentPattern.FindStringSubmatch(loc.Document)
			if m == nil {
				continue
			}
			if tid, ok := known[m[1]]; ok {
				id = tid
				break
			}
		}
		if id == "" {
			other = append(other, p)
			continue
		}
		byTest[id] = append(byTest[id], p)
	}
	return byTest, other
}

// buildProblemMessage renders one test's build errors for its result row.
func buildProblemMessage(problems []docker.BuildProblem) string {
	parts := make([]string, 0, len(problems))
	for _, p := range problems {
		msg := p.Message
		if p.ErrorCode != "" {
			msg = p.ErrorCode + ": " + msg
		}
		if p.Locations != nil && p.Where() != "" {
			// Only the element is worth repeating here — the module and document
			// are the test itself, which the row already names.
			if el := p.Locations[0].Element; el != "" {
				msg += " (at " + el + ")"
			}
		}
		parts = append(parts, msg)
	}
	return "the assertion could not be built: " + strings.Join(parts, "; ")
}

// resultsFromFailedBuild turns a failed build into one result per test, so a run
// that cannot boot still reports something per test instead of nothing at all.
//
// A test MxBuild blamed becomes an ERROR carrying the consistency message. Every
// other test becomes a SKIP: they were not run and must not be counted as
// passing — the whole point of this area is that a test framework never reports
// green for something it did not evaluate.
//
// Returns nil when no error could be attributed to a test, so the caller reports
// the build failure as it always did rather than inventing per-test rows for a
// problem in the user's own model.
func resultsFromFailedBuild(problems []docker.BuildProblem, suite *TestSuite) []TestResult {
	byTest, _ := attributeBuildProblems(problems, suite)
	if len(byTest) == 0 || suite == nil {
		return nil
	}

	results := make([]TestResult, 0, len(suite.Tests))
	for _, tc := range suite.Tests {
		r := newResult(tc)
		if ps, ok := byTest[tc.ID]; ok {
			r.Status = StatusError
			r.Message = buildProblemMessage(ps)
		} else {
			r.Status = StatusSkip
			r.Message = "not run: another test in this run failed to build"
		}
		results = append(results, r)
	}
	return results
}

// buildFailureHint is appended to the error when a build failure could not be
// attributed to any test, which means it is in the project rather than in the
// suite.
func buildFailureHint(other []docker.BuildProblem) string {
	if len(other) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n  The build errors are in the project, not in the tests:")
	for _, p := range other {
		b.WriteString(fmt.Sprintf("\n    %s %s", p.ErrorCode, p.Message))
		if w := p.Where(); w != "" {
			b.WriteString(" — at " + w)
		}
	}
	return b.String()
}
