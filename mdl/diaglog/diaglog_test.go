// SPDX-License-Identifier: Apache-2.0

package diaglog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

func TestNilLoggerIsSafe(t *testing.T) {
	var l *Logger
	// All methods should be safe on nil
	l.Command("TestStmt", "TEST", time.Millisecond, nil)
	l.Connect("/path", "10.0", 2)
	l.ParseError("bad input", nil)
	l.Info("msg")
	l.Warn("msg")
	l.Error("msg")
	l.Close()
}

func TestInitAndClose(t *testing.T) {
	// Use a temp dir for logs
	tmpDir := t.TempDir()
	testutil.SetHome(t, tmpDir)
	// Init is a per-process singleton (ako/mxcli#617); start from a clean one so
	// this test does not inherit the previous test's open session.
	resetForTest()

	l := Init("test-version", "test")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	defer l.Close()

	// Verify log file was created
	logDir := filepath.Join(tmpDir, ".mxcli", "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("failed to read log dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 log file, got %d", len(entries))
	}

	name := entries[0].Name()
	if !strings.HasPrefix(name, "mxcli-") || !strings.HasSuffix(name, ".log") {
		t.Errorf("unexpected log file name: %s", name)
	}
}

func TestCommandLogging(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.SetHome(t, tmpDir)
	// Init is a per-process singleton (ako/mxcli#617); start from a clean one so
	// this test does not inherit the previous test's open session.
	resetForTest()

	l := Init("test", "batch")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}

	l.Command("ShowStmt", "SHOW ENTITIES", 50*time.Millisecond, nil)
	l.Command("CreateEntityStmt", "CREATE ENTITY Foo.Bar", 100*time.Millisecond, nil)
	// The PROCESS ends the session, not a command: l.Close() is a command
	// releasing its hold and writes nothing, because it runs while the command
	// is still returning and so cannot know the exit code.
	CloseCurrent()

	// Read log file and check it contains expected entries
	logDir := filepath.Join(tmpDir, ".mxcli", "logs")
	entries, _ := os.ReadDir(logDir)
	content, err := os.ReadFile(filepath.Join(logDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "session_start") {
		t.Error("missing session_start")
	}
	if !strings.Contains(s, "SHOW ENTITIES") {
		t.Error("missing SHOW ENTITIES command")
	}
	if !strings.Contains(s, "session_end") {
		t.Error("missing session_end")
	}
	if !strings.Contains(s, `"commands_executed":2`) {
		t.Error("expected commands_executed:2")
	}
}

func TestDisabledViaEnv(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.SetHome(t, tmpDir)
	// Init is a per-process singleton (ako/mxcli#617); start from a clean one so
	// this test does not inherit the previous test's open session.
	resetForTest()
	t.Setenv("MXCLI_LOG", "0")

	l := Init("test", "batch")
	if l != nil {
		t.Error("expected nil logger when MXCLI_LOG=0")
	}

	// nil logger should be safe
	l.Command("X", "X", 0, nil)
	l.Close()
}

func TestCleanOldLogs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some fake log files
	oldFile := filepath.Join(tmpDir, "mxcli-2020-01-01.log")
	newFile := filepath.Join(tmpDir, "mxcli-2099-12-31.log")
	notLog := filepath.Join(tmpDir, "other.txt")

	os.WriteFile(oldFile, []byte("old"), 0644)
	os.WriteFile(newFile, []byte("new"), 0644)
	os.WriteFile(notLog, []byte("keep"), 0644)

	// Set old file to old mtime
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(oldFile, oldTime, oldTime)

	cleanOldLogs(tmpDir, 7*24*time.Hour)

	// Old log should be deleted
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("expected old log to be deleted")
	}
	// New log should remain
	if _, err := os.Stat(newFile); err != nil {
		t.Error("expected new log to remain")
	}
	// Non-log file should remain
	if _, err := os.Stat(notLog); err != nil {
		t.Error("expected non-log file to remain")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 100); got != "short" {
		t.Errorf("expected 'short', got %q", got)
	}
	if got := truncate("this is a long string", 10); got != "this is a ..." {
		t.Errorf("expected truncated, got %q", got)
	}
}

// readSessionEnd returns the session_end record from the one log file in dir.
func readSessionEnd(t *testing.T, tmpDir string) map[string]any {
	t.Helper()
	logDir := filepath.Join(tmpDir, ".mxcli", "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("want one log file in %s, got %v (err %v)", logDir, entries, err)
	}
	data, err := os.ReadFile(filepath.Join(logDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) != nil {
			continue
		}
		if rec["msg"] == "session_end" {
			return rec
		}
	}
	t.Fatalf("no session_end record in:\n%s", data)
	return nil
}

// A command that returns an error is a run that ENDED, and main() closes the
// session on that path so the run keeps its duration. Before this, the error
// path went straight to os.Exit: no session_end, so no duration, so
// `diag loop-report` summed wall time over the runs that happened to succeed
// (PROPOSAL_agent_loop_efficiency.md item 2d).
func TestCloseWithExitRecordsTheCode(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.SetHome(t, tmpDir)
	resetForTest()

	if l := Init("test-version", "test"); l == nil {
		t.Fatal("expected non-nil logger")
	}
	CloseCurrentWithExit(1)

	rec := readSessionEnd(t, tmpDir)
	if got, ok := rec["exit_code"]; !ok || got != float64(1) {
		t.Errorf("exit_code = %v (present %v), want 1", got, ok)
	}
	// The duration is the half that item 2d is about: a failed run's time is
	// still time the loop spent.
	if _, ok := rec["duration_s"]; !ok {
		t.Error("session_end carries no duration_s, so the run's time is still lost")
	}
}

// Zero is omitted rather than written, so a reader cannot mistake "exited
// cleanly" for "nobody recorded it" — and a session_end from a log written
// before exit codes existed, where closing implied success, reads as 0 too.
func TestCleanCloseOmitsTheExitCode(t *testing.T) {
	tmpDir := t.TempDir()
	testutil.SetHome(t, tmpDir)
	resetForTest()

	if l := Init("test-version", "test"); l == nil {
		t.Fatal("expected non-nil logger")
	}
	CloseCurrent()

	if rec := readSessionEnd(t, tmpDir); rec["exit_code"] != nil {
		t.Errorf("exit_code = %v on a clean close, want absent", rec["exit_code"])
	}
}
