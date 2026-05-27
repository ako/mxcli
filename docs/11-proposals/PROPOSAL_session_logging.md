---
title: Session Logging & Diagnostics for mxcli
status: proposed
---

# Proposal: Session Logging & Diagnostics for mxcli

## Problem

mxcli is being distributed to users for testing. When bugs are reported, we currently have no way to determine what the user was doing — no logs, no session traces, no error history. Bug reports require manual reproduction, which is slow and often impossible without exact steps.

## Goal

Add a lightweight session logging system so that when users report bugs, they can attach a log file showing what happened — version, commands executed, errors, timing.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Logger library | `log/slog` (stdlib) | Go 1.24 project; no new deps; structured JSON output |
| Log location | `~/.mxcli/logs/` | Single collection point; parallels `~/.mxcli_history` |
| Log format | JSON Lines | Machine-parseable, `jq`-friendly, `slog.JSONHandler` native |
| Integration | Wrap `execute()` | One change covers all 40+ command handlers — no `cmd_*.go` edits |
| Default state | Enabled | Testing tool — logs should be there when bugs are reported |
| Rotation | 7-day age-based cleanup on startup | Simple; bounded disk usage (~1MB max) |
| Disable | `MXCLI_LOG=0` env var | Consistent with existing `MXCLI_QUIET` pattern |
| Failure mode | Silent no-op | If log dir unwritable, tool works normally |

## What Gets Logged

**Session header** (on startup):
```json
{"time":"2026-02-05T10:15:30Z","level":"info","msg":"session_start","version":"0.1.0","go":"go1.24.3","os":"linux","arch":"arm64","mode":"repl","args":["mxcli","-p","app.mpr"],"pid":12345}
```

**Connect events:**
```json
{"time":"...","level":"info","msg":"connect","mpr_path":"/path/to/app.mpr","mendix_version":"11.6.3","mpr_format":2}
```

**Command execution** (every statement):
```json
{"time":"...","level":"info","msg":"execute","stmt_type":"CreateEntityStmt","stmt_summary":"create entity MyModule.Customer","duration_ms":45}
```

**Errors:**
```json
{"time":"...","level":"error","msg":"execute_error","stmt_type":"CreatePageStmtV3","stmt_summary":"create page MyModule.Edit","error":"widget template not found","duration_ms":12}
```

**Parse errors:**
```json
{"time":"...","level":"WARN","msg":"parse_error","input_preview":"CERATE entity ...","errors":["line 1:0 mismatched input 'CERATE'"]}
```

**Session end:**
```json
{"time":"...","level":"info","msg":"session_end","commands_executed":15,"errors_count":2,"duration_s":120}
```

**NOT logged:** data content, BSON payloads, stdout/stderr output.

## Architecture

### New package: `mdl/diaglog/diaglog.go` (~180 lines)

Thin wrapper around `log/slog` with session tracking:

```go
package diaglog

type Logger struct {
    slog      *slog.Logger
    file      *os.File
    cmdCount  int
    errCount  int
    startTime time.Time
}

// Init opens ~/.mxcli/logs/mxcli-YYYY-MM-DD.log (append mode).
// returns no-op logger if MXCLI_LOG=0 or file creation fails.
func Init(version, mode string) *Logger

func (l *Logger) close()                                                             // writes session_end, closes file
func (l *Logger) Command(stmtType, summary string, duration time.Duration, err error) // logs execution
func (l *Logger) connect(mprPath, mendixVersion string, formatVersion int)            // logs connection
func (l *Logger) ParseError(inputPreview string, errs []error)                        // logs parse failures
func (l *Logger) info(msg string, args ...any)                                        // general-purpose
func (l *Logger) Warn(msg string, args ...any)
func (l *Logger) error(msg string, args ...any)
```

`Init()` also runs `cleanOldLogs()` to delete files older than 7 days.

### Integration: Wrap `execute()` in executor

The key insight: instead of modifying 40+ `cmd_*.go` handler files, we wrap the single `execute()` dispatch method:

```go
// executor.go - rename current execute() to executeInner()
func (e *Executor) execute(stmt ast.Statement) error {
    start := time.Now()
    err := e.executeInner(stmt)
    if e.logger != nil {
        e.logger.Command(stmtTypeName(stmt), stmtSummary(stmt), time.Since(start), err)
    }
    return err
}
```

This captures every command type automatically — current and future.

### New subcommand: `mxcli diag`

```bash
mxcli diag              # show version, platform, log dir, recent errors
mxcli diag --bundle     # Create tar.gz with logs + system info for bug report
mxcli diag --log-path   # Print log directory path
mxcli diag --tail 50    # Show last 50 log entries
```

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| `mdl/diaglog/diaglog.go` | Create | Logger package (~180 lines) |
| `cmd/mxcli/diag.go` | Create | `mxcli diag` subcommand (~100 lines) |
| `mdl/executor/executor.go` | Modify | Add logger field, wrap Execute(), add stmtSummary() |
| `mdl/repl/repl.go` | Modify | Add logger field, log parse errors |
| `cmd/mxcli/main.go` | Modify | Init logger in root/exec/subcommand entry points |
| `CLAUDE.md` | Modify | Document log location, disable, diag command |

**Not changed:** None of the 40+ `cmd_*.go` files. Not `lsp.go` (has its own zap logger). Not `go.mod` (slog is stdlib).

## Log File Management

- **Location:** `~/.mxcli/logs/mxcli-YYYY-MM-DD.log`
- **One file per day**, multiple sessions append with session headers
- **Auto-cleanup:** Files older than 7 days deleted on startup
- **Typical size:** 10-100KB per day (50-200 commands)
- **Total footprint:** Under 1MB for 7 days of logs

## User Experience

```bash
# Normal usage — logging happens silently in background
mxcli -p app.mpr -c "show entities"

# Disable logging for this invocation
MXCLI_LOG=0 mxcli -p app.mpr -c "show entities"

# check diagnostics after hitting a bug
mxcli diag
# Output:
#   mxcli diagnostics
#     version:     0.1.0-abc1234
#     Go:          go1.24.3 linux/arm64
#     log dir:     /home/user/.mxcli/logs/
#     log files:   3 files (142 KB)
#     Sessions:    12 (last 7 days)
#     Errors:      5 (last 7 days)
#
#   Recent errors:
#     [2026-02-05 10:15] create page MyModule.Edit: widget template not found
#     [2026-02-04 14:22] connect: failed to open MPR: database is locked

# Bundle logs for a bug report
mxcli diag --bundle
# Creates: mxcli-diag-20260205-101530.tar.gz

# view recent log entries
mxcli diag --tail 20
```

## Verification Plan

```bash
# build and test basic logging
make build
MXCLI_QUIET=1 bin/mxcli -p app.mpr -c "show entities"
cat ~/.mxcli/logs/mxcli-$(date +%Y-%m-%d).log | jq .

# Test disable
MXCLI_LOG=0 bin/mxcli -p app.mpr -c "show entities"

# Test error logging
bin/mxcli -p app.mpr -c "describe entity Nonexistent.Thing" 2>/dev/null
cat ~/.mxcli/logs/mxcli-$(date +%Y-%m-%d).log | jq 'select(.level == "ERROR")'

# Test diag command
bin/mxcli diag
bin/mxcli diag --bundle
bin/mxcli diag --tail 10

# run tests
make test
```
