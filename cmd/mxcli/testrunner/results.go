// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

// TestResult represents the outcome of a single test case.
type TestResult struct {
	ID       string        // Test ID
	Name     string        // Test name
	Status   TestStatus    // Pass, Fail, Skip, Error
	Message  string        // Failure/skip message
	Duration time.Duration // Execution time
}

// TestStatus represents the outcome status of a test.
type TestStatus int

const (
	StatusPass TestStatus = iota
	StatusFail
	StatusSkip
	StatusError
)

func (s TestStatus) String() string {
	switch s {
	case StatusPass:
		return "PASS"
	case StatusFail:
		return "FAIL"
	case StatusSkip:
		return "SKIP"
	case StatusError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// SuiteResult holds the results of an entire test suite execution.
type SuiteResult struct {
	Name     string
	Tests    []TestResult
	Duration time.Duration
	Started  time.Time
}

// PassCount returns the number of passing tests.
func (sr *SuiteResult) PassCount() int {
	n := 0
	for _, t := range sr.Tests {
		if t.Status == StatusPass {
			n++
		}
	}
	return n
}

// FailCount returns the number of failing tests.
func (sr *SuiteResult) FailCount() int {
	n := 0
	for _, t := range sr.Tests {
		if t.Status == StatusFail || t.Status == StatusError {
			n++
		}
	}
	return n
}

// ErrorCount returns the number of tests that did not reach a verdict — an
// uncompilable @expect, a missing microflow, a failed request.
//
// It is reported separately from FailCount in the summary line. A suite whose
// output cannot distinguish "this assertion is false" from "this assertion was
// never evaluated" is how the silent-pass defect stayed invisible for weeks.
func (sr *SuiteResult) ErrorCount() int {
	n := 0
	for _, t := range sr.Tests {
		if t.Status == StatusError {
			n++
		}
	}
	return n
}

// SkipCount returns the number of skipped tests.
func (sr *SuiteResult) SkipCount() int {
	n := 0
	for _, t := range sr.Tests {
		if t.Status == StatusSkip {
			n++
		}
	}
	return n
}

// AllPassed returns true if all tests passed.
func (sr *SuiteResult) AllPassed() bool {
	return sr.FailCount() == 0
}

// ParseLogResults parses structured MXTEST: log lines from runtime output
// and matches them to the test suite's test cases.
func ParseLogResults(logReader io.Reader, suite *TestSuite) *SuiteResult {
	result := &SuiteResult{
		Name:    suite.Name,
		Started: time.Now(),
	}

	// Build a map of test cases by ID for quick lookup
	testMap := make(map[string]*TestCase)
	for i := range suite.Tests {
		testMap[suite.Tests[i].ID] = &suite.Tests[i]
	}

	// Track which tests we've seen results for
	resultMap := make(map[string]*TestResult)
	runTimes := make(map[string]time.Time)

	scanner := bufio.NewScanner(logReader)
	for scanner.Scan() {
		line := scanner.Text()

		// Find MXTEST: protocol lines.
		// Runtime logs look like: "MXTEST: MXTEST:PASS:test_1" where the first
		// "MXTEST:" is the log node and the second is our protocol prefix.
		// We search for specific protocol actions to avoid matching the log node.
		protocol := ""
		for _, action := range []string{"MXTEST:START:", "MXTEST:RUN:", "MXTEST:PASS:", "MXTEST:FAIL:", "MXTEST:SKIP:", "MXTEST:END:"} {
			if idx := strings.Index(line, action); idx >= 0 {
				protocol = line[idx:]
				break
			}
		}
		if protocol == "" {
			continue
		}

		parts := strings.SplitN(protocol, ":", 4) // MXTEST:TYPE:id[:message]

		if len(parts) < 3 {
			continue
		}

		action := parts[1]
		id := parts[2]

		switch action {
		case "START":
			result.Started = time.Now()

		case "RUN":
			runTimes[id] = time.Now()
			// Extract name from RUN line: MXTEST:RUN:id:name
			name := id
			if len(parts) >= 4 {
				name = parts[3]
			}
			resultMap[id] = &TestResult{
				ID:   id,
				Name: name,
			}

		case "PASS":
			if r, ok := resultMap[id]; ok {
				r.Status = StatusPass
				if t, ok := runTimes[id]; ok {
					r.Duration = time.Since(t)
				}
			} else {
				resultMap[id] = &TestResult{
					ID:     id,
					Name:   id,
					Status: StatusPass,
				}
			}

		case "FAIL":
			msg := ""
			if len(parts) >= 4 {
				msg = parts[3]
			}
			if r, ok := resultMap[id]; ok {
				r.Status = StatusFail
				r.Message = msg
				if t, ok := runTimes[id]; ok {
					r.Duration = time.Since(t)
				}
			} else {
				resultMap[id] = &TestResult{
					ID:      id,
					Name:    id,
					Status:  StatusFail,
					Message: msg,
				}
			}

		case "SKIP":
			msg := ""
			if len(parts) >= 4 {
				msg = parts[3]
			}
			if r, ok := resultMap[id]; ok {
				r.Status = StatusSkip
				r.Message = msg
			} else {
				resultMap[id] = &TestResult{
					ID:      id,
					Name:    id,
					Status:  StatusSkip,
					Message: msg,
				}
			}

		case "END":
			result.Duration = time.Since(result.Started)
		}
	}

	// Collect results in test order
	for _, tc := range suite.Tests {
		// A test whose @expect did not compile was never generated, so the log
		// has nothing to say about it. Report the parse error rather than the
		// generic "not executed".
		if res, bad := expectErrorResult(tc); bad {
			result.Tests = append(result.Tests, res)
			continue
		}
		if r, ok := resultMap[tc.ID]; ok {
			// Use the original test name if available
			if tc.Name != "" {
				r.Name = tc.Name
			}
			result.Tests = append(result.Tests, *r)
		} else {
			// Test was not executed — mark as error
			result.Tests = append(result.Tests, TestResult{
				ID:      tc.ID,
				Name:    tc.Name,
				Status:  StatusError,
				Message: "Test was not executed (runtime may have crashed before reaching it)",
			})
		}
	}

	return result
}

// PrintResults writes a human-readable summary to the writer.
func PrintResults(w io.Writer, result *SuiteResult, color bool) {
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Test Results: %s\n", result.Name)
	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 60))

	for _, t := range result.Tests {
		var statusStr string
		if color {
			switch t.Status {
			case StatusPass:
				statusStr = "\033[32mPASS\033[0m"
			case StatusFail:
				statusStr = "\033[31mFAIL\033[0m"
			case StatusSkip:
				statusStr = "\033[33mSKIP\033[0m"
			case StatusError:
				statusStr = "\033[31mERROR\033[0m"
			}
		} else {
			statusStr = t.Status.String()
		}

		fmt.Fprintf(w, "  %s  %s", statusStr, t.Name)
		if t.Duration > 0 {
			fmt.Fprintf(w, " (%s)", t.Duration.Round(time.Millisecond))
		}
		fmt.Fprintln(w)

		if t.Message != "" && t.Status != StatusPass {
			fmt.Fprintf(w, "         %s\n", t.Message)
		}
	}

	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 60))
	errors := result.ErrorCount()
	fmt.Fprintf(w, "Total: %d  Passed: %d  Failed: %d",
		len(result.Tests), result.PassCount(), result.FailCount()-errors)
	if errors > 0 {
		fmt.Fprintf(w, "  Errors: %d", errors)
	}
	fmt.Fprintf(w, "  Skipped: %d", result.SkipCount())
	if result.Duration > 0 {
		fmt.Fprintf(w, "  Time: %s", result.Duration.Round(time.Millisecond))
	}
	fmt.Fprintln(w)

	if result.AllPassed() {
		if color {
			fmt.Fprintf(w, "\033[32mAll tests passed.\033[0m\n")
		} else {
			fmt.Fprintf(w, "All tests passed.\n")
		}
	} else {
		if color {
			fmt.Fprintf(w, "\033[31mSome tests failed.\033[0m\n")
		} else {
			fmt.Fprintf(w, "Some tests failed.\n")
		}
	}
}

// expectErrorResult turns a test whose @expect could not be compiled into an
// ERROR result.
//
// This is the fail-closed rule the whole @expect pipeline is built around: an
// assertion the runner cannot evaluate must never be able to report a pass. The
// previous implementation dropped such a line during parsing, and a test with no
// assertions left passes as long as it does not throw — so a suite could report
// green while asserting nothing. ERROR is counted with the failures, so the run's
// exit code is non-zero.
func expectErrorResult(tc TestCase) (TestResult, bool) {
	if len(tc.ExpectErrors) == 0 {
		return TestResult{}, false
	}
	return TestResult{
		ID:      tc.ID,
		Name:    tc.Name,
		Status:  StatusError,
		Message: strings.Join(tc.ExpectErrors, "; "),
	}, true
}
