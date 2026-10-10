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

// An arity failure exits non-zero, and it now CLOSES, recording the code.
//
// It used to be the other way round: the absence of a session_end was the
// report's only evidence of a failure. That cost the run its DURATION as well
// as its verdict, so `diag loop-report` summed wall time over the runs that
// happened to succeed (PROPOSAL_agent_loop_efficiency.md item 2d). `unclosed`
// now means a process that vanished — killed, or an os.Exit inside a command —
// and a non-zero exit_code is what marks a run that ended badly.
func TestArityFailureClosesWithItsExitCode(t *testing.T) {
	recs := runMainRecords(t, "session_end", "check")
	if len(recs) != 1 {
		t.Fatalf("mxcli check (no args) wrote %d session_end records, want 1", len(recs))
	}
	if got := recs[0]["exit_code"]; got != float64(1) {
		t.Errorf("exit_code = %v, want 1 — a failed run that ends must say so", got)
	}
	// The half item 2d is about: the run's time is recorded at all.
	if _, ok := recs[0]["duration_s"]; !ok {
		t.Error("session_end carries no duration_s, so the failed run's time is still lost")
	}
}

// A command that builds a logged executor AND returns its error to cobra is
// where the close ORDERING mattered: its `defer logger.Close()` runs while the
// command is returning, before cobra hands the error back to main, so under
// diaglog's old first-wins rule the command closed the session and recorded the
// run as clean. Measured on `widget sync -p <missing>`: the pre-change binary
// wrote a session_end with no exit code, this one records 1.
//
// An arity failure cannot show this — it fails before any command builds an
// executor, so nothing competes for the close.
func TestLoggedCommandThatReturnsAnErrorRecordsTheCode(t *testing.T) {
	recs := runMainRecords(t, "session_end", "widget", "sync", "-p", "/nonexistent-xyz.mpr")
	if len(recs) != 1 {
		t.Fatalf("wrote %d session_end records, want 1", len(recs))
	}
	if got := recs[0]["exit_code"]; got != float64(1) {
		t.Errorf("exit_code = %v, want 1 — the command's own deferred Close got there first "+
			"and recorded a failed run as clean", got)
	}
}

// The success path still records no exit code, so a reader cannot mistake
// "exited cleanly" for "nobody wrote it down".
func TestSuccessfulRunRecordsNoExitCode(t *testing.T) {
	recs := runMainRecords(t, "session_end", "syntax")
	if len(recs) != 1 {
		t.Fatalf("mxcli syntax wrote %d session_end records, want 1", len(recs))
	}
	if got := recs[0]["exit_code"]; got != nil {
		t.Errorf("exit_code = %v on a successful run, want absent", got)
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
