// SPDX-License-Identifier: Apache-2.0

// Package diaglog provides lightweight session logging for mxcli diagnostics.
// Logs are written as JSON Lines to ~/.mxcli/logs/mxcli-YYYY-MM-DD.log.
// Logging is enabled by default and can be disabled with MXCLI_LOG=0.
// If the log directory cannot be created, logging silently degrades to a no-op.
package diaglog

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Logger wraps slog.Logger with session tracking and convenience methods.
// A nil Logger is safe to use — all methods are no-ops on nil receivers.
type Logger struct {
	slog      *slog.Logger
	file      *os.File
	pid       int
	cmdCount  int
	errCount  int
	startTime time.Time
	closed    bool
}

// parentPIDEnv names the mxcli process that spawned this one, when one did.
// Set by Init and inherited by every child, so a self-spawned run is
// distinguishable from a call the user or agent made (ako/mxcli#629).
const parentPIDEnv = "MXCLI_SESSION_PID"

// One session per process.
//
// `diag loop-report` counts session records, so a process that opened two would
// count twice and one that opened none would be invisible. Init is called from
// mxcli's main() for every command and again by each command that builds a
// logged executor; the second call has to return the SAME logger (ako/mxcli#617).
var (
	mu      sync.Mutex
	current *Logger
)

// CloseCurrent ends the process session, if one was started. It is called from
// main() only when the command returned normally —
// so a run that exits through os.Exit leaves no session_end, and that absence is
// what `diag loop-report` reads as a non-zero exit.
func CloseCurrent() {
	mu.Lock()
	l := current
	mu.Unlock()
	l.Close()
}

// resetForTest drops the process logger so a test can start a fresh session.
func resetForTest() {
	mu.Lock()
	defer mu.Unlock()
	if current != nil && current.file != nil {
		current.file.Close()
	}
	current = nil
}

// Init creates the daily log file and writes a session header.
// Returns nil if logging is disabled (MXCLI_LOG=0) or the log file cannot be created.
func Init(version, mode string) *Logger {
	if isDisabled() {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if current != nil {
		return current
	}

	logDir := logDirectory()
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil
	}

	// Clean old logs (best-effort, errors ignored)
	cleanOldLogs(logDir, 7*24*time.Hour)

	filename := fmt.Sprintf("mxcli-%s.log", time.Now().Format("2006-01-02"))
	logPath := filepath.Join(logDir, filename)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil
	}

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	l := &Logger{
		slog:      slog.New(handler),
		file:      f,
		startTime: time.Now(),
	}

	current = l
	l.pid = os.Getpid()

	// mxcli spawns mxcli: `test` runs `-c DESCRIBE SETTINGS`, `-c SHOW MODULES`
	// and an `exec` of the generated runner before a single test executes, and
	// `new`, `eval`, `tui` and the LSP do the same. Each child logs a session of
	// its own, so without a marker the report counts them as calls the agent made
	// — 3 phantom calls per `mxcli test` — and shows the parent as never having
	// closed, because a session_start used to end the open invocation.
	//
	// The marker rides the ENVIRONMENT rather than each spawn site: a child
	// started with exec.Command inherits this process's environment (explicitly
	// via os.Environ(), or implicitly when Cmd.Env is nil), so setting it once
	// here covers all six self-spawn sites and any added later without touching
	// them (ako/mxcli#629).
	parent := os.Getenv(parentPIDEnv)
	os.Setenv(parentPIDEnv, strconv.Itoa(l.pid))

	// Write session header
	l.slog.Info("session_start",
		"version", version,
		"go", runtime.Version(),
		"os", runtime.GOOS,
		"arch", runtime.GOARCH,
		"mode", mode,
		"args", os.Args,
		"pid", l.pid,
		"parent_pid", parent,
	)

	return l
}

// Close writes a session summary and closes the log file.
func (l *Logger) Close() {
	if l == nil {
		return
	}
	// Each command that holds the process logger defers Close; only the first
	// may end the session, or a run would be recorded as finishing early.
	mu.Lock()
	defer mu.Unlock()
	if l.closed {
		return
	}
	l.closed = true
	l.slog.Info("session_end",
		"commands_executed", l.cmdCount,
		"errors_count", l.errCount,
		"duration_s", int(time.Since(l.startTime).Seconds()),
		// The pid is what pairs this with its session_start. Without it a reader
		// can only guess by position, which is wrong the moment one mxcli runs
		// another — and mxcli runs itself routinely (ako/mxcli#629).
		"pid", l.pid,
	)
	l.file.Close()
}

// Command logs a statement execution with timing and optional error.
func (l *Logger) Command(stmtType, summary string, duration time.Duration, err error) {
	if l == nil {
		return
	}
	l.cmdCount++
	if err != nil {
		l.errCount++
		l.slog.Error("execute_error",
			"stmt_type", stmtType,
			"stmt_summary", summary,
			"error", err.Error(),
			"duration_ms", duration.Milliseconds(),
		)
	} else {
		l.slog.Info("execute",
			"stmt_type", stmtType,
			"stmt_summary", summary,
			"duration_ms", duration.Milliseconds(),
		)
	}
}

// Connect logs a project connection event.
func (l *Logger) Connect(mprPath, mendixVersion string, formatVersion int) {
	if l == nil {
		return
	}
	l.slog.Info("connect",
		"mpr_path", mprPath,
		"mendix_version", mendixVersion,
		"mpr_format", formatVersion,
	)
}

// ParseError logs parse failures with a truncated input preview.
func (l *Logger) ParseError(inputPreview string, errs []error) {
	if l == nil {
		return
	}
	l.errCount++
	errStrings := make([]string, len(errs))
	for i, e := range errs {
		errStrings[i] = e.Error()
	}
	l.slog.Warn("parse_error",
		"input_preview", truncate(inputPreview, 200),
		"errors", errStrings,
	)
}

// Info logs a general informational message.
func (l *Logger) Info(msg string, args ...any) {
	if l == nil {
		return
	}
	l.slog.Info(msg, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, args ...any) {
	if l == nil {
		return
	}
	l.slog.Warn(msg, args...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, args ...any) {
	if l == nil {
		return
	}
	l.errCount++
	l.slog.Error(msg, args...)
}

// LogDir returns the log directory path.
func LogDir() string {
	return logDirectory()
}

// isDisabled checks if logging is disabled via environment variable.
func isDisabled() bool {
	v := os.Getenv("MXCLI_LOG")
	switch strings.ToLower(v) {
	case "0", "false", "off", "no":
		return true
	}
	return false
}

// logDirectory returns ~/.mxcli/logs/.
func logDirectory() string {
	// MXCLI_LOG_DIR keeps a test out of the user's real log directory.
	if d := os.Getenv("MXCLI_LOG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".mxcli", "logs")
}

// cleanOldLogs removes log files older than maxAge from the log directory.
func cleanOldLogs(logDir string, maxAge time.Duration) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "mxcli-") || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(logDir, entry.Name()))
		}
	}
}

// truncate shortens a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
