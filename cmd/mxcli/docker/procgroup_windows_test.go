// SPDX-License-Identifier: Apache-2.0

//go:build windows

package docker

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// These tests exist because `mxcli run --local` hung forever on Windows at
// "Starting mxbuild --serve...". Two POSIX assumptions were behind it, both in
// the process helpers this file exercises:
//
//  1. alive() used os.Process.Signal(syscall.Signal(0)). Go returns EWINDOWS
//     ("not supported by windows") for every signal except Kill — even for a
//     process that is very much alive — so waitReady() concluded mxbuild had
//     exited the instant it was launched.
//
//  2. Stop() waited on cmd.Wait(), and mxbuild.exe is a wrapper that launches a
//     Deno web-ext worker which inherits the stdout/stderr pipe exec.Cmd hands
//     out. Killing only the wrapper (the old killProcessGroup did p.Kill())
//     left the worker holding the pipe, so Wait() never saw EOF and blocked
//     forever.
//
// The unix implementation is covered by procgroup_unix_test.go; this file is the
// Windows half, and the CI job that runs it is what keeps it from regressing.

// TestWindowsProcessHelper is not a test of its own: it is the re-exec target the
// tests below use to get a real, controllable process tree on Windows (where
// there is no `sh -c`). It exits before the testing framework prints anything.
func TestWindowsProcessHelper(t *testing.T) {
	switch os.Getenv("MXCLI_PROC_HELPER") {
	case "":
		return // normal `go test` run: this is not a test
	case "sleep":
		time.Sleep(60 * time.Second)
		os.Exit(0)
	case "spawn":
		// A grandchild that inherits our stdout/stderr — the inherited pipe is
		// exactly what kept cmd.Wait() blocked in the field.
		//
		// Note what this mode does NOT do: announce the grandchild. Start()
		// returns as soon as the grandchild has been created, which is before it
		// is running and holding the pipe, so a marker written here says nothing
		// about the process the test is about. The grandchild announces itself
		// below instead.
		gc := exec.Command(os.Args[0], "-test.run=TestWindowsProcessHelper")
		gc.Env = append(os.Environ(), "MXCLI_PROC_HELPER=grandchild")
		gc.Stdout = os.Stdout
		gc.Stderr = os.Stderr
		if err := gc.Start(); err != nil {
			os.Exit(3)
		}
		time.Sleep(60 * time.Second)
		os.Exit(0)
	case "grandchild":
		// Announce over the INHERITED pipe, and with our own pid, so the test can
		// establish that this process — the one holding the write end — is really
		// running before it kills the tree. This mirrors the unix half, where the
		// wrapper echoes `$!` and the test checks it with kill(pid, 0).
		fmt.Fprintf(os.Stdout, "grandchild-started %d\n", os.Getpid())
		time.Sleep(60 * time.Second)
		os.Exit(0)
	}
}

// helperCmd re-execs this test binary in the requested helper mode.
func helperCmd(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestWindowsProcessHelper")
	cmd.Env = append(os.Environ(), "MXCLI_PROC_HELPER="+mode)
	return cmd
}

// processAlive must report a running process as alive. The CONTROL in the middle
// is the point: it asserts that the API processAlive replaces really is unusable
// on Windows, so nobody "simplifies" the workaround back into the bug.
func TestProcessAlive_ReportsRunningProcess(t *testing.T) {
	cmd := helperCmd(t, "sleep")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	if !processAlive(cmd.Process) {
		t.Fatal("processAlive() = false for a running process")
	}
	if err := cmd.Process.Signal(syscall.Signal(0)); err == nil {
		t.Log("note: os.Process.Signal(0) succeeded on Windows; " +
			"processAlive's WaitForSingleObject may no longer be required")
	}

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if processAlive(cmd.Process) {
		t.Fatal("processAlive() = true for an exited process")
	}
}

func TestProcessAlive_FalseWhenNilOrExited(t *testing.T) {
	if processAlive(nil) {
		t.Fatal("processAlive(nil) = true")
	}
	cmd := exec.Command("cmd", "/c", "exit", "0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	_ = cmd.Wait()
	if processAlive(cmd.Process) {
		t.Fatal("processAlive() = true for an exited process")
	}
}

// ServeServer.alive() is the exact call waitReady() makes. Before the fix it
// returned false for a live mxbuild, which is what aborted the local boot.
func TestServeServer_AliveTracksProcess(t *testing.T) {
	if (&ServeServer{}).alive() {
		t.Fatal("alive() = true with no process")
	}

	cmd := helperCmd(t, "sleep")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	s := &ServeServer{cmd: cmd, log: &syncBuffer{}}

	if !s.alive() {
		t.Fatal("ServeServer.alive() = false for a running process " +
			"(the Windows Signal(0) bug that aborted the local boot)")
	}

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if s.alive() {
		t.Fatal("ServeServer.alive() = true after the process exited")
	}
}

// LocalRuntime.alive() shares the same helper and the same fix.
func TestLocalRuntime_AliveTracksProcess(t *testing.T) {
	cmd := helperCmd(t, "sleep")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	rt := &LocalRuntime{cmd: cmd, log: &syncBuffer{}}

	if !rt.alive() {
		t.Fatal("LocalRuntime.alive() = false for a running process")
	}

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if rt.alive() {
		t.Fatal("LocalRuntime.alive() = true after the process exited")
	}
}

// TestKillProcessGroup_ReapsGrandchildAndUnblocksWait is the regression test for
// the hang. The helper spawns a grandchild that inherits the stdout pipe; the old
// single-PID kill left it alive, so cmd.Wait() blocked forever (there was no
// error, no exit — just a hung `mxcli run --local`). A tree kill closes the pipe
// and Wait() returns.
//
// It would have failed on the pre-fix code: p.Kill() terminates only the helper,
// the grandchild keeps the write end open, and Wait() never returns.
//
// The grandchild is a re-exec of this test binary rather than `cmd /c ping`, so
// that it can announce itself over the inherited pipe. That handshake is what
// makes the test deterministic: see the "spawn" mode's comment for the race the
// old marker left open, which failed this job intermittently (ako/mxcli#594).
func TestKillProcessGroup_ReapsGrandchildAndUnblocksWait(t *testing.T) {
	var log syncBuffer
	cmd := helperCmd(t, "spawn")
	cmd.Stdout = &log
	cmd.Stderr = &log
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	t.Cleanup(func() { _ = killProcessGroup(cmd.Process) })

	// Wait for the grandchild to be running and holding the inherited pipe. The
	// marker arrives over that pipe from the grandchild itself, so reading it
	// proves the pipe holder exists; waiting on anything the helper printed
	// would not (see the "spawn" mode's comment).
	deadline := time.Now().Add(15 * time.Second)
	var gpid int
	for {
		if p, ok := grandchildPID(log.String()); ok {
			gpid = p
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("grandchild never announced itself; output so far: %q", log.String())
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The CONTROL for the marker: assert the announced process really is alive
	// before the kill, so a test that goes green has actually exercised a tree
	// kill and not a race that killed a one-process tree. The unix half does the
	// same with kill(gpid, 0).
	gproc, err := os.FindProcess(gpid)
	if err != nil {
		t.Fatalf("grandchild %d should be alive before the kill: %v", gpid, err)
	}
	defer func() { _ = gproc.Release() }()
	if !processAlive(gproc) {
		t.Fatalf("grandchild %d should be alive before the kill", gpid)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	if err := killProcessGroup(cmd.Process); err != nil {
		t.Fatalf("killProcessGroup: %v", err)
	}

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("cmd.Wait() did not return after killProcessGroup: a surviving " +
			"grandchild still holds the stdout pipe (the old single-PID kill hung here)")
	}
}
