// SPDX-License-Identifier: Apache-2.0

package diaglog

import (
	"os"
	"strings"
	"testing"
)

// `mxcli diag loop-report` reports "mxcli invocations: N" as though it counted
// every run. It counted only the commands that happened to build a logged
// executor — 14 files of 53 registered commands — and only if the run survived
// long enough to reach that code (ako/mxcli#617).
//
// The fix is to initialise once per process, from PersistentPreRun, before
// argument validation. That makes Init reentrant: the commands that already
// call it must get the SAME logger back rather than opening a second session,
// or every one of them would double-count.

func sessionLines(t *testing.T, dir, msg string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read log dir: %v", err)
	}
	n := 0
	for _, e := range entries {
		b, err := os.ReadFile(dir + "/" + e.Name())
		if err != nil {
			continue
		}
		n += strings.Count(string(b), `"msg":"`+msg+`"`)
	}
	return n
}

func TestInitIsOncePerProcess(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MXCLI_LOG_DIR", dir)
	resetForTest()

	first := Init("v", "check")
	second := Init("v", "check")
	if first == nil {
		t.Fatal("Init returned nil with logging enabled")
	}
	if first != second {
		t.Error("a second Init opened a new session; every command that builds a logged " +
			"executor would then count twice")
	}
	if n := sessionLines(t, dir, "session_start"); n != 1 {
		t.Errorf("wrote %d session_start records for one process, want 1", n)
	}
}

// Close is deferred by each command that holds a logger. With one shared logger
// that must still produce exactly one session_end — and the FIRST close must not
// end the session while the command is still running.
func TestCloseWritesOneSessionEnd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MXCLI_LOG_DIR", dir)
	resetForTest()

	l := Init("v", "exec")
	_ = Init("v", "exec")
	l.Close()
	l.Close()

	if n := sessionLines(t, dir, "session_end"); n != 1 {
		t.Errorf("wrote %d session_end records, want exactly 1", n)
	}
}

// THE CONTROL. MXCLI_LOG=0 must still disable logging entirely — a singleton
// that ignored it would start writing records for users who opted out.
func TestDisabledStaysDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MXCLI_LOG_DIR", dir)
	t.Setenv("MXCLI_LOG", "0")
	resetForTest()

	if l := Init("v", "check"); l != nil {
		t.Error("Init returned a logger with MXCLI_LOG=0")
	}
	if n := sessionLines(t, dir, "session_start"); n != 0 {
		t.Errorf("wrote %d records with logging disabled, want 0", n)
	}
}
