// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package docker

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestProcessGroup_ReapsGrandchild proves setProcessGroup + killProcessGroup
// terminate a wrapper's grandchild — the real-world case where mxbuild is a
// shell-script that spawns a Temurin JVM. A single-PID signal (the old Stop
// behaviour) would leave that grandchild orphaned, still holding its port, so the
// next `mxcli run --local` refuses to start.
func TestProcessGroup_ReapsGrandchild(t *testing.T) {
	// sh (the "wrapper") backgrounds a grandchild that sleeps, prints its PID, then
	// waits so the whole group stays alive until we kill it.
	cmd := exec.Command("sh", "-c", "sleep 60 & echo $!; wait")
	setProcessGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	sc := bufio.NewScanner(stdout)
	if !sc.Scan() {
		t.Fatalf("did not read grandchild pid: %v", sc.Err())
	}
	gpid, err := strconv.Atoi(strings.TrimSpace(sc.Text()))
	if err != nil {
		t.Fatalf("parse grandchild pid %q: %v", sc.Text(), err)
	}
	if err := syscall.Kill(gpid, 0); err != nil {
		t.Fatalf("grandchild %d should be alive before the kill: %v", gpid, err)
	}

	// Kill the whole group, then reap the leader.
	if err := killProcessGroup(cmd.Process); err != nil {
		t.Fatalf("killProcessGroup: %v", err)
	}
	_ = cmd.Wait()

	// The grandchild must be gone. Poll briefly: after SIGKILL it is a zombie until
	// its reparented init reaps it, during which Kill(pid,0) still returns nil.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if err := syscall.Kill(gpid, 0); err != nil {
			return // gone — success
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(gpid, syscall.SIGKILL) // don't leak on failure
			t.Fatalf("grandchild %d survived the group kill (orphaned)", gpid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestSetProcessGroup_SetsFlag is a cheap guard that setProcessGroup requests a
// new process group (so the negative-PID kill above has a group to target).
func TestSetProcessGroup_SetsFlag(t *testing.T) {
	cmd := exec.Command("true")
	setProcessGroup(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("setProcessGroup should set SysProcAttr.Setpgid")
	}
}
