// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// mxcli-formula1 FINDINGS §60: a container pinned at 100% CPU on ten cores for
// eight hours.
//
//	PID     USER    %CPU    TIME+     COMMAND
//	197669  vscode  999.9   8,05      mxcli
//
//	$ ps -o pid,ppid,stat -p 198010
//	    PID    PPID STAT
//	 198010  197669 Z        <- the runtime, a zombie, never reaped
//
//	$ curl -o /dev/null -w '%{http_code}' http://localhost:8180/
//	000
//
// The runtime had shut ITSELF down hours earlier — the local standalone runtime
// runs on a developer licence with a maximum run time, and says so at every boot:
//
//	LicenseService: Maximum run time exceeded, framework is now terminating
//
// mxcli neither reaped it nor noticed. The hub tunnel kept answering, so the
// preview URL still responded while the app behind it was gone, and the sync
// simply stopped writing rows.
//
// Two mechanisms, and both are in this file's scope:
//
//  1. Nothing ever called Wait() on the runtime process, so an exited JVM stayed
//     a zombie — and alive() asked Signal(0), which SUCCEEDS on a zombie. Measured
//     (Linux 6.18, go1.26): proc state `Z`, Signal(0) returns nil; after Wait()
//     it returns "process already finished". So the liveness check reported the
//     runtime alive for as long as the process ran.
//
//  2. After boot, `run` waited on a signal and nothing else, so even a correct
//     liveness answer had no one asking. It now waits on the signal OR the
//     runtime exiting.
//
// The CPU spin itself was not reproduced here and is not claimed to be fixed by
// name: it lived in the tunnel client, under a supervisor that could not see its
// child was gone. What is fixed is the state it occurred in — mxcli now exits
// when the runtime does, which tears the tunnel down with it.

// startedRuntime is a LocalRuntime around a real short-lived child, which is what
// makes the zombie observable at all: the defect is about a process that has
// exited and not been reaped, so a fake cannot show it.
func startedRuntime(t *testing.T, script string) *LocalRuntime {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	rt := &LocalRuntime{log: &syncBuffer{}}
	cmd.Stdout = rt.log
	cmd.Stderr = rt.log
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the stand-in runtime: %v", err)
	}
	rt.cmd = cmd
	rt.watchExit()
	t.Cleanup(func() { _ = rt.stopProcess() })
	return rt
}

// The reported case: a runtime that has exited must not read as alive.
func TestLocalRuntime_ExitedRuntimeIsNotAlive(t *testing.T) {
	rt := startedRuntime(t, "exit 0")

	select {
	case <-rt.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the runtime's exit was never observed — nothing is reaping the child")
	}
	if rt.alive() {
		t.Error("alive() reports a dead runtime as alive; Signal(0) succeeds on an unreaped " +
			"zombie, which is exactly the eight-hour case")
	}
}

// CONTROL: a runtime that is still running must still read as alive, or the fix
// is just a liveness check that always says no.
func TestLocalRuntime_RunningRuntimeIsAlive(t *testing.T) {
	rt := startedRuntime(t, "sleep 30")

	if !rt.alive() {
		t.Fatal("a running runtime must read as alive")
	}
	select {
	case <-rt.Exited():
		t.Error("Exited() fired for a runtime that is still running")
	case <-time.After(200 * time.Millisecond):
	}
}

// The exit status is carried, so a caller can say more than "it stopped".
func TestLocalRuntime_ExitErrorIsAvailable(t *testing.T) {
	rt := startedRuntime(t, "exit 7")

	select {
	case <-rt.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("exit not observed")
	}
	err := rt.ExitErr()
	if err == nil {
		t.Fatal("a non-zero exit must be reported as an error")
	}
	if !strings.Contains(err.Error(), "7") {
		t.Errorf("the exit status should be recoverable from the error, got: %v", err)
	}
}

// Stop() must still work after the child has already gone — the reaper has taken
// the wait, so a second Wait() in stopProcess would block forever.
func TestLocalRuntime_StopAfterTheChildAlreadyExited(t *testing.T) {
	rt := startedRuntime(t, "exit 0")
	select {
	case <-rt.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("exit not observed")
	}

	done := make(chan error, 1)
	go func() { done <- rt.stopProcess() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("stopProcess after an exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stopProcess hung on an already-reaped child — two waiters on one process")
	}
}

// The reason the runtime stopped is in its own log, and it is the reason this
// finding exists. A run that dies at 4am should say why, not only that it did.
func TestRuntimeExitReason_NamesTheTrialLicence(t *testing.T) {
	log := strings.Join([]string{
		"INFO  - M2EE: (JVM) Started",
		"WARNING - LicenseService: The runtime has been started using a trial licence,",
		"          the framework will be terminated when the maximum time is exceeded!",
		"INFO  - Core: Some ordinary line",
		"LicenseService: Maximum run time exceeded, framework is now terminating",
	}, "\n")

	reason := runtimeExitReason(log)
	if reason == "" {
		t.Fatal("the licence termination is in the log verbatim and must be surfaced")
	}
	if !strings.Contains(strings.ToLower(reason), "licence") && !strings.Contains(strings.ToLower(reason), "license") {
		t.Errorf("the reason should name the licence, got: %s", reason)
	}
}

// CONTROL: an ordinary log yields no reason rather than a guess. Inventing a
// cause for a crash is worse than reporting the exit alone.
func TestRuntimeExitReason_SilentWhenItCannotTell(t *testing.T) {
	if r := runtimeExitReason("INFO - Core: nothing unusual here\nINFO - Done"); r != "" {
		t.Errorf("got %q, want no reason", r)
	}
}

// The second half of §60: a correct liveness answer is worth nothing if nobody
// asks. After boot, `run` waited on a signal and nothing else, so the runtime
// terminating left mxcli up — with the hub tunnel still answering, which is why
// the preview URL responded for hours while the app behind it was gone.
//
// waitForInterruptOrExit is what `run` waits on now. These cases are about which
// of the two wakes it, and what it reports.

func TestWaitForInterruptOrExit_ReturnsWhenTheRuntimeStops(t *testing.T) {
	exited := make(chan struct{})
	go func() { time.Sleep(50 * time.Millisecond); close(exited) }()

	done := make(chan bool, 1)
	go func() { done <- waitForInterruptOrExit(exited) }()

	select {
	case runtimeStopped := <-done:
		if !runtimeStopped {
			t.Error("the wait returned but did not report the runtime as the cause")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a runtime exit did not wake the run loop — this is the eight-hour case")
	}
}

// CONTROL: with the runtime up, the wait must still block. A wait that returns
// immediately turns `run` into a command that exits on boot.
func TestWaitForInterruptOrExit_BlocksWhileTheRuntimeRuns(t *testing.T) {
	done := make(chan bool, 1)
	go func() { done <- waitForInterruptOrExit(make(chan struct{})) }()

	select {
	case <-done:
		t.Fatal("returned while the runtime was still running")
	case <-time.After(300 * time.Millisecond):
	}
}

// A runtime that was never started hands back a nil channel; the wait must treat
// that as "nothing to watch" rather than as an immediate exit.
func TestWaitForInterruptOrExit_NilChannelIsNotAnExit(t *testing.T) {
	done := make(chan bool, 1)
	go func() { done <- waitForInterruptOrExit(nil) }()

	select {
	case <-done:
		t.Fatal("a nil exit channel was read as an exit")
	case <-time.After(300 * time.Millisecond):
	}
}
