// SPDX-License-Identifier: Apache-2.0

//go:build windows

package docker

import (
	"syscall"
	"testing"
)

// `run stop` waits for PidAlive to turn false, then reports whatever is still
// alive. PidAlive used to say a pid was alive whenever os.FindProcess could
// open it, and Windows keeps an exited process openable for as long as anyone
// holds a handle to it — the parent that started it, typically. So a run that
// had shut down read as alive, `run stop` waited out its grace period, killed
// a process that was already gone, and still printed "failed: 1 process(es) …
// are still alive" (the same class as mendixlabs/mxcli#1284).
func TestPidAlive_ExitedProcessWithAnOpenHandleIsNotAlive(t *testing.T) {
	cmd := helperCmd(t, "sleep")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	if !PidAlive(pid) {
		t.Fatalf("PidAlive(%d) = false for a running process", pid)
	}

	// Hold a handle of our own, as the run's parent does, so the process object
	// outlives the process.
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatalf("OpenProcess: %v", err)
	}
	defer syscall.CloseHandle(h)

	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if ev, err := syscall.WaitForSingleObject(h, 10_000); err != nil || ev != syscall.WAIT_OBJECT_0 {
		t.Fatalf("process did not exit after Kill: event=%d err=%v", ev, err)
	}

	if PidAlive(pid) {
		t.Fatalf("PidAlive(%d) = true for a process that has exited (a handle to it is still open)", pid)
	}
}
