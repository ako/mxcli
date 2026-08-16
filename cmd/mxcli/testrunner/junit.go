// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// JUnit XML types for output.

type junitTestSuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	XMLName  xml.Name        `xml:"testsuite"`
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Errors   int             `xml:"errors,attr"`
	Skipped  int             `xml:"skipped,attr"`
	Time     string          `xml:"time,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	XMLName    xml.Name         `xml:"testcase"`
	Name       string           `xml:"name,attr"`
	ClassName  string           `xml:"classname,attr"`
	File       string           `xml:"file,attr,omitempty"`
	Time       string           `xml:"time,attr"`
	Properties *junitProperties `xml:"properties,omitempty"`
	Failure    *junitFailure    `xml:"failure,omitempty"`
	Error      *junitError      `xml:"error,omitempty"`
	Skipped    *junitSkipped    `xml:"skipped,omitempty"`
}

type junitProperties struct {
	Properties []junitProperty `xml:"property"`
}

type junitProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Content string `xml:",chardata"`
}

type junitError struct {
	Message string `xml:"message,attr"`
	Content string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr,omitempty"`
}

// WriteJUnitXML writes test results in JUnit XML format.
func WriteJUnitXML(w io.Writer, result *SuiteResult) error {
	suites := junitTestSuites{
		Suites: []junitTestSuite{
			convertToJUnitSuite(result),
		},
	}

	fmt.Fprintf(w, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	if err := encoder.Encode(suites); err != nil {
		return fmt.Errorf("encoding JUnit XML: %w", err)
	}
	fmt.Fprintln(w)
	return nil
}

func convertToJUnitSuite(result *SuiteResult) junitTestSuite {
	suite := junitTestSuite{
		Name:     result.Name,
		Tests:    len(result.Tests),
		Failures: 0,
		Errors:   0,
		Skipped:  0,
		Time:     formatDuration(result.Duration),
	}

	for _, t := range result.Tests {
		tc := junitTestCase{
			Name:      t.Name,
			ClassName: junitClassName(t, result.Name),
			File:      t.SourceFile,
			Time:      formatDuration(t.Duration),
			// The assertion count rides along as a property so a CI report can
			// show what a passing test actually checked. Without it, "0
			// assertions" and "6 assertions" are the same green tick.
			Properties: &junitProperties{Properties: []junitProperty{
				{Name: "assertions", Value: strconv.Itoa(t.Assertions)},
			}},
		}

		switch t.Status {
		case StatusFail:
			suite.Failures++
			tc.Failure = &junitFailure{
				Message: t.Message,
				Content: t.Message,
			}
		case StatusError:
			suite.Errors++
			tc.Error = &junitError{
				Message: t.Message,
				Content: t.Message,
			}
		case StatusSkip:
			suite.Skipped++
			tc.Skipped = &junitSkipped{
				Message: t.Message,
			}
		}

		suite.Cases = append(suite.Cases, tc)
	}

	return suite
}

// junitClassName derives a JUnit class name from the test's source file.
//
// Every case used to be stamped with the suite name, so a `tests/` directory
// collapsed into one class and a CI failure could not say which file it came
// from. JUnit consumers treat the class name as a dotted path, so the extension
// is dropped and the separators converted.
func junitClassName(t TestResult, fallback string) string {
	if t.SourceFile == "" {
		return fallback
	}
	name := filepath.ToSlash(t.SourceFile)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = strings.TrimSuffix(name, ".test")
	return strings.ReplaceAll(name, "/", ".")
}

func formatDuration(d time.Duration) string {
	return fmt.Sprintf("%.3f", d.Seconds())
}
