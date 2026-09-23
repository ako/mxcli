// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A command called with the wrong number of arguments wrote no session record
// at all (ako/mxcli#633). Cobra runs ValidateArgs BEFORE PersistentPreRun, so
// with diaglog.Init in PersistentPreRun an arity failure returned before
// anything was logged, and `diag loop-report` never saw the wasted call.
//
// These tests run the real main() in a child process: the failure path ends in
// os.Exit, and the point is what reaches the log file before it does.

const runMainEnv = "MXCLI_TEST_RUN_MAIN"

// TestRunMainHelper is not a test: it is main() for the child process.
func TestRunMainHelper(t *testing.T) {
	raw := os.Getenv(runMainEnv)
	if raw == "" {
		t.Skip("helper process only")
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		t.Fatalf("decode args: %v", err)
	}
	os.Args = append([]string{"mxcli"}, args...)
	main()
	os.Exit(0)
}

// runMain runs mxcli with args in a child process, logging into a fresh
// directory, and returns the session_start records it wrote.
func runMain(t *testing.T, args ...string) []map[string]any {
	t.Helper()
	return runMainRecords(t, "session_start", args...)
}

// runMainRecords is runMain for any record type.
func runMainRecords(t *testing.T, msg string, args ...string) []map[string]any {
	t.Helper()
	logDir := t.TempDir()
	enc, _ := json.Marshal(args)
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunMainHelper$")
	cmd.Dir = t.TempDir() // no .mpr to auto-discover
	cmd.Env = append(os.Environ(),
		runMainEnv+"="+string(enc),
		"MXCLI_LOG_DIR="+logDir,
		"MXCLI_LOG=1",
		"MXCLI_QUIET=1",
	)
	cmd.Stdin = strings.NewReader("")
	_ = cmd.Run() // most probes exit non-zero; the log is what is asserted

	var recs []map[string]any
	files, _ := filepath.Glob(filepath.Join(logDir, "*.log"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read log: %v", err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			var r map[string]any
			if json.Unmarshal([]byte(line), &r) == nil && r["msg"] == msg {
				recs = append(recs, r)
			}
		}
	}
	return recs
}

func TestArityFailureWritesSessionStart(t *testing.T) {
	for _, tc := range []struct {
		args []string
		mode string
	}{
		{[]string{"test"}, "mxcli test"},
		{[]string{"check"}, "mxcli check"},
		{[]string{"exec"}, "mxcli exec"},
		// Control: a file error already reached PersistentPreRun and was logged.
		{[]string{"check", "/nonexistent.mdl"}, "mxcli check"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			starts := runMain(t, tc.args...)
			if len(starts) != 1 {
				t.Fatalf("mxcli %s: want 1 session_start record, got %d",
					strings.Join(tc.args, " "), len(starts))
			}
			if got := starts[0]["mode"]; got != tc.mode {
				t.Errorf("mode = %v, want %q", got, tc.mode)
			}
		})
	}
}

// The root command is the -c one-shot or the REPL, and those callers pass
// "batch" / "repl" to the singleton. Starting the session earlier must not
// replace that with a generic name, since invocationVerb falls back to mode
// exactly when argv names no subcommand.
func TestRootSessionKeepsBatchAndReplMode(t *testing.T) {
	for _, tc := range []struct {
		args []string
		mode string
	}{
		{[]string{"-c", "show version"}, "batch"},
		{[]string{"--command=show version"}, "batch"},
		{nil, "repl"},
	} {
		t.Run(tc.mode+" "+strings.Join(tc.args, " "), func(t *testing.T) {
			starts := runMain(t, tc.args...)
			if len(starts) != 1 {
				t.Fatalf("want 1 session_start record, got %d", len(starts))
			}
			if got := starts[0]["mode"]; got != tc.mode {
				t.Errorf("mode = %v, want %q", got, tc.mode)
			}
		})
	}
}

// An arity failure exits non-zero, so it must stay unclosed — that absence is
// what the report reads as a failed run.
func TestArityFailureLeavesSessionUnclosed(t *testing.T) {
	if n := len(runMainRecords(t, "session_end", "check")); n != 0 {
		t.Fatalf("mxcli check (no args) wrote %d session_end records, want 0", n)
	}
}

// --help and --version succeed without reaching any cobra hook. With the
// session opened before Execute, a close left in PersistentPostRun would never
// run for them, and every help lookup would read as a failed run.
func TestHelpAndVersionCloseTheirSession(t *testing.T) {
	for _, tc := range []struct {
		args []string
		mode string
	}{
		{[]string{"check", "--help"}, "mxcli check"},
		{[]string{"--help"}, "help"},
		{[]string{"--version"}, "version"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			starts := runMain(t, tc.args...)
			if len(starts) != 1 {
				t.Fatalf("want 1 session_start record, got %d", len(starts))
			}
			if got := starts[0]["mode"]; got != tc.mode {
				t.Errorf("mode = %v, want %q", got, tc.mode)
			}
			if n := len(runMainRecords(t, "session_end", tc.args...)); n != 1 {
				t.Errorf("want 1 session_end record, got %d", n)
			}
		})
	}
}

// diag stays excluded: a report that counted its own runs would climb every
// time it was read.
func TestDiagWritesNoSessionStart(t *testing.T) {
	if n := len(runMain(t, "diag", "loop-report")); n != 0 {
		t.Fatalf("diag loop-report wrote %d session_start records, want 0", n)
	}
}
