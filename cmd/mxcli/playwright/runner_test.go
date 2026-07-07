// SPDX-License-Identifier: Apache-2.0

package playwright

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverScripts(t *testing.T) {
	// Create temp dir with test scripts
	dir := t.TempDir()
	for _, name := range []string{"verify-login.test.sh", "verify-crud.test.sh", "not-a-test.sh", "readme.md"} {
		os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/bash\necho ok"), 0755)
	}

	scripts, err := discoverScripts([]string{dir})
	if err != nil {
		t.Fatalf("discoverScripts: %v", err)
	}
	if len(scripts) != 2 {
		t.Fatalf("expected 2 scripts, got %d: %v", len(scripts), scripts)
	}

	// Should be sorted
	if filepath.Base(scripts[0]) != "verify-crud.test.sh" {
		t.Errorf("expected verify-crud.test.sh first, got %s", filepath.Base(scripts[0]))
	}
	if filepath.Base(scripts[1]) != "verify-login.test.sh" {
		t.Errorf("expected verify-login.test.sh second, got %s", filepath.Base(scripts[1]))
	}
}

func TestDiscoverScriptsSingleFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "verify-smoke.test.sh")
	os.WriteFile(f, []byte("#!/bin/bash\necho ok"), 0755)

	scripts, err := discoverScripts([]string{f})
	if err != nil {
		t.Fatalf("discoverScripts: %v", err)
	}
	if len(scripts) != 1 {
		t.Fatalf("expected 1 script, got %d", len(scripts))
	}
}

func TestDiscoverScriptsEmpty(t *testing.T) {
	dir := t.TempDir()
	scripts, err := discoverScripts([]string{dir})
	if err != nil {
		t.Fatalf("discoverScripts: %v", err)
	}
	if len(scripts) != 0 {
		t.Fatalf("expected 0 scripts, got %d", len(scripts))
	}
}

func TestScriptName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/tmp/tests/verify-login.test.sh", "verify-login"},
		{"verify-crud.test.sh", "verify-crud"},
		{"/a/b/c/verify-smoke.test.sh", "verify-smoke"},
	}
	for _, tt := range tests {
		got := scriptName(tt.path)
		if got != tt.want {
			t.Errorf("scriptName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestRunScriptPass(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "pass.test.sh")
	os.WriteFile(f, []byte("#!/bin/bash\necho 'PASS: smoke'\n"), 0755)

	var buf bytes.Buffer
	result := runScript(f, "", 10e9, false, &buf)
	if result.Status != StatusPass {
		t.Errorf("expected PASS, got %s: %s", result.Status, result.Message)
	}
}

func TestRunScriptFail(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "fail.test.sh")
	os.WriteFile(f, []byte("#!/bin/bash\necho 'widget not found'\nexit 1\n"), 0755)

	var buf bytes.Buffer
	result := runScript(f, "", 10e9, false, &buf)
	if result.Status != StatusFail {
		t.Errorf("expected FAIL, got %s", result.Status)
	}
	if result.Message != "widget not found" {
		t.Errorf("expected 'widget not found', got %q", result.Message)
	}
}

func TestLastNonEmptyLine(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello\nworld\n", "world"},
		{"hello\n\n\n", "hello"},
		{"\n\n", ""},
		{"single", "single"},
		{"first\nsecond\nthird\n\n", "third"},
	}
	for _, tt := range tests {
		got := lastNonEmptyLine(tt.input)
		if got != tt.want {
			t.Errorf("lastNonEmptyLine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestOriginOf(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"http://localhost:8080", "http://localhost:8080"},
		{"http://localhost:8080/", "http://localhost:8080"},
		{"http://localhost:8080/p/Customer_Overview", "http://localhost:8080"},
		{"https://app.example.com:9443/index.html", "https://app.example.com:9443"},
		{"localhost:8080", ""}, // no scheme -> not absolute
		{"", ""},
		{"not a url", ""},
	}
	for _, tt := range tests {
		if got := originOf(tt.in); got != tt.want {
			t.Errorf("originOf(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseEvalResult(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"result heading with value", "### Result\nhttp://localhost:8080/p/Home\n", "http://localhost:8080/p/Home"},
		{"quoted value under heading", "### Result\n\"http://localhost:8080\"\n", "http://localhost:8080"},
		{"heading with blank line before value", "### Result\n\n  http://localhost:8080  \n", "http://localhost:8080"},
		{"no heading falls back to first non-empty line", "http://localhost:8080\n", "http://localhost:8080"},
		{"empty", "", ""},
		{"about:blank", "### Result\nabout:blank\n", "about:blank"},
	}
	for _, tt := range tests {
		if got := parseEvalResult(tt.in); got != tt.want {
			t.Errorf("%s: parseEvalResult(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestSuiteResultCounts(t *testing.T) {
	result := &SuiteResult{
		Scripts: []ScriptResult{
			{Status: StatusPass},
			{Status: StatusPass},
			{Status: StatusFail},
			{Status: StatusError},
		},
	}

	if result.PassCount() != 2 {
		t.Errorf("PassCount = %d, want 2", result.PassCount())
	}
	if result.FailCount() != 2 {
		t.Errorf("FailCount = %d, want 2", result.FailCount())
	}
	if result.AllPassed() {
		t.Error("AllPassed should be false")
	}
}

func TestPrintResults(t *testing.T) {
	result := &SuiteResult{
		Name: "test-suite",
		Scripts: []ScriptResult{
			{Name: "smoke", Status: StatusPass},
			{Name: "crud", Status: StatusFail, Message: "widget missing"},
		},
	}

	var buf bytes.Buffer
	PrintResults(&buf, result, false)
	output := buf.String()

	if !bytes.Contains([]byte(output), []byte("PASS")) {
		t.Error("expected PASS in output")
	}
	if !bytes.Contains([]byte(output), []byte("FAIL")) {
		t.Error("expected FAIL in output")
	}
	if !bytes.Contains([]byte(output), []byte("widget missing")) {
		t.Error("expected failure message in output")
	}
}

func TestWriteJUnitXML(t *testing.T) {
	result := &SuiteResult{
		Name: "test-suite",
		Scripts: []ScriptResult{
			{Name: "smoke", Status: StatusPass},
			{Name: "crud", Status: StatusFail, Message: "error msg", Output: "full output"},
		},
	}

	var buf bytes.Buffer
	if err := WriteJUnitXML(&buf, result); err != nil {
		t.Fatalf("WriteJUnitXML: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("testsuites")) {
		t.Error("expected testsuites element")
	}
	if !bytes.Contains([]byte(output), []byte("failures=\"1\"")) {
		t.Error("expected failures=1")
	}
	if !bytes.Contains([]byte(output), []byte("error msg")) {
		t.Error("expected failure message")
	}
}
