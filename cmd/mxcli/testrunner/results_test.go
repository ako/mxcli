// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseLogResults(t *testing.T) {
	logs := `Starting runtime...
Core: Running after-startup-action...
MXTEST: MXTEST:START:example
MXTEST: MXTEST:RUN:test_1:Hello World
MXTEST: MXTEST:PASS:test_1
MXTEST: MXTEST:RUN:test_2:String concat
MXTEST: MXTEST:FAIL:test_2:Expected $result = 'John Doe' but got 'Jane Doe'
MXTEST: MXTEST:RUN:test_3:Entity creation
MXTEST: MXTEST:PASS:test_3
MXTEST: MXTEST:END:example
Core: Successfully ran after-startup-action.
`

	suite := &TestSuite{
		Name: "example",
		Tests: []TestCase{
			{ID: "test_1", Name: "Hello World"},
			{ID: "test_2", Name: "String concat"},
			{ID: "test_3", Name: "Entity creation"},
		},
	}

	result := ParseLogResults(strings.NewReader(logs), suite, false)

	if len(result.Tests) != 3 {
		t.Fatalf("Result count: got %d, want 3", len(result.Tests))
	}

	if result.Tests[0].Status != StatusPass {
		t.Errorf("Test 1 status: got %v, want PASS", result.Tests[0].Status)
	}
	if result.Tests[1].Status != StatusFail {
		t.Errorf("Test 2 status: got %v, want FAIL", result.Tests[1].Status)
	}
	if result.Tests[1].Message != "Expected $result = 'John Doe' but got 'Jane Doe'" {
		t.Errorf("Test 2 message: got %q", result.Tests[1].Message)
	}
	if result.Tests[2].Status != StatusPass {
		t.Errorf("Test 3 status: got %v, want PASS", result.Tests[2].Status)
	}

	if result.PassCount() != 2 {
		t.Errorf("PassCount: got %d, want 2", result.PassCount())
	}
	if result.FailCount() != 1 {
		t.Errorf("FailCount: got %d, want 1", result.FailCount())
	}
	if result.AllPassed() {
		t.Error("AllPassed should be false")
	}
}

func TestParseLogResultsMissingTest(t *testing.T) {
	// Runtime crashed before reaching test_3
	logs := `MXTEST: MXTEST:START:example
MXTEST: MXTEST:RUN:test_1:Hello World
MXTEST: MXTEST:PASS:test_1
MXTEST: MXTEST:RUN:test_2:String concat
MXTEST: MXTEST:PASS:test_2
`
	suite := &TestSuite{
		Name: "example",
		Tests: []TestCase{
			{ID: "test_1", Name: "Hello World"},
			{ID: "test_2", Name: "String concat"},
			{ID: "test_3", Name: "Entity creation"},
		},
	}

	result := ParseLogResults(strings.NewReader(logs), suite, false)

	if result.Tests[2].Status != StatusError {
		t.Errorf("Test 3 status: got %v, want ERROR (not executed)", result.Tests[2].Status)
	}
}

func TestWriteJUnitXML(t *testing.T) {
	result := &SuiteResult{
		Name: "example",
		Tests: []TestResult{
			{ID: "test_1", Name: "Hello World", Status: StatusPass},
			{ID: "test_2", Name: "String concat", Status: StatusFail, Message: "Expected 'John Doe'"},
		},
	}

	var buf bytes.Buffer
	if err := WriteJUnitXML(&buf, result); err != nil {
		t.Fatalf("WriteJUnitXML: %v", err)
	}

	xml := buf.String()
	if !strings.Contains(xml, `<?xml version="1.0"`) {
		t.Error("Missing XML declaration")
	}
	if !strings.Contains(xml, `name="example"`) {
		t.Error("Missing suite name")
	}
	if !strings.Contains(xml, `tests="2"`) {
		t.Error("Missing tests count")
	}
	if !strings.Contains(xml, `failures="1"`) {
		t.Error("Missing failures count")
	}
	if !strings.Contains(xml, `<failure`) {
		t.Error("Missing failure element")
	}
}

func TestPrintResults(t *testing.T) {
	result := &SuiteResult{
		Name: "example",
		Tests: []TestResult{
			{ID: "test_1", Name: "Hello World", Status: StatusPass},
			{ID: "test_2", Name: "Failing test", Status: StatusFail, Message: "Expected true"},
		},
	}

	var buf bytes.Buffer
	PrintResults(&buf, result, false)
	output := buf.String()

	if !strings.Contains(output, "PASS") {
		t.Error("Missing PASS in output")
	}
	if !strings.Contains(output, "FAIL") {
		t.Error("Missing FAIL in output")
	}
	if !strings.Contains(output, "Some tests failed") {
		t.Error("Missing failure summary")
	}
}
