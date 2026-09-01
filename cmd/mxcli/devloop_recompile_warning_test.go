// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// mxcli-formula1 FINDINGS §81 — the half of the shared deployment tree the
// web-bundle fix does not cover.
//
// A local test run recompiles the project's Java into `deployment/run/bin`,
// which is the classpath a live `mxcli run --local` is holding open. Measured on
// a real 11.13 project: after one `mxcli test --local`, all 134 class files have
// NEW INODES and byte-identical content — every one deleted and rewritten. A JVM
// loads classes lazily, so one it has not reached yet can fail afterwards:
//
//	java.lang.NoClassDefFoundError: odatapushdown/QueryObject
//
// What that costs is diagnosis, not the breakage. The microflows behind it then
// answer **HTTP 200 with a zero-byte body** — not a 500, not an error page —
// while source-backed resources keep working, so half the app is fine and half
// returns nothing. In the reporting project it surfaced as 21 of 34 tests
// failing in a DIFFERENT app, and 108 log lines went by before anyone connected
// it to the test run.
//
// mxcli cannot prevent the rewrite: mxbuild's Gradle pass owns the compile, and
// the deployment directory cannot be moved (mxcli-ledger §150). It can say so,
// to someone who can act on it.
//
// The fact needed — is a dev loop serving this project? — was ALREADY published:
// `mxcli run --local` writes devLoopHandshake to .mxcli/run-local.json for
// `mxcli constant set --apply`, with the pid liveness check and the
// project-identity field this needs. A second state file was written before that
// was noticed, and it would have CLOBBERED this one, dropping the admin password
// and boot config that `--apply` and `--attach` depend on.

func TestRecompileWarning_SaysWhatWillHappenAndWhatToDo(t *testing.T) {
	msg := recompileWarning(devLoopHandshake{PID: 4242, AppPort: 8080})

	for _, want := range []string{"8080", "recompil", "restart"} {
		if !strings.Contains(strings.ToLower(msg), want) {
			t.Errorf("the warning should mention %q: %s", want, msg)
		}
	}
	// The symptom is the part nobody guesses, because it does not look like a
	// failure at all.
	if !strings.Contains(msg, "200") {
		t.Errorf("the warning should name the symptom (HTTP 200, empty body): %s", msg)
	}
}

func TestWarnIfDevLoopServing_FiresForALiveDevLoop(t *testing.T) {
	p := handshakeProject(t)
	if err := writeDevLoopHandshake(p, devLoopHandshake{
		Project: p, PID: os.Getpid(), AppPort: 8080, AdminPort: 8090, Started: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	if !warnIfDevLoopServing(p, &buf) {
		t.Fatal("a live dev loop on the same project must be reported")
	}
	if !strings.Contains(buf.String(), "8080") {
		t.Errorf("the warning did not reach the writer: %q", buf.String())
	}
}

// CONTROL 1: silent when nothing is running. Every project that has never used
// the warm loop runs its tests through this path, and a warning there is noise
// on every run.
func TestWarnIfDevLoopServing_SilentWithNoDevLoop(t *testing.T) {
	var buf strings.Builder
	if warnIfDevLoopServing(handshakeProject(t), &buf) {
		t.Error("warned with no dev loop running")
	}
	if buf.String() != "" {
		t.Errorf("wrote %q when there was nothing to say", buf.String())
	}
}

// CONTROL 2 — the one that decides whether this is usable at all. A `run --local`
// that was killed, crashed, or died with its development licence (§60, measured
// lifetimes under six hours) leaves the file behind. Warning on that would fire
// forever, and a warning that is always wrong teaches the reader to skip it.
//
// readDevLoopHandshake already refuses a dead pid; this pins that the warning
// inherits it rather than reading the file itself.
func TestWarnIfDevLoopServing_SilentForAStaleHandshake(t *testing.T) {
	p := handshakeProject(t)
	if err := writeDevLoopHandshake(p, devLoopHandshake{
		Project: p, PID: 999999, AppPort: 8080,
	}); err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	if warnIfDevLoopServing(p, &buf) {
		t.Errorf("warned about a dev loop whose process is gone: %q", buf.String())
	}
}
