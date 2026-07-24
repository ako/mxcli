// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package docker

import (
	"os"
	"os/exec"
	"syscall"
)

// procgroup_unix.go isolates each long-lived child of the warm loop (mxbuild
// --serve, the runtime JVM, the rollup bundler) into its own process group, so
// teardown can signal the WHOLE group rather than just the direct child. This
// matters because mxbuild is a shell-script wrapper that spawns a Temurin JVM
// grandchild: signalling only the wrapper PID leaves the JVM orphaned, still
// holding its port (6543/8080/8090), so the next `mxcli run --local` refuses to
// start ("port already in use"). Killing the group reaps the grandchild too.

// setProcessGroup makes cmd the leader of a new process group (so its PGID equals
// its PID once started). Call before cmd.Start(). It preserves any SysProcAttr a
// caller already set (e.g. none of ours do today, but PrepareMxCommand might).
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// signalProcessGroup sends sig to the whole process group led by p (started via
// setProcessGroup). A negative PID targets the group. If the group can't be
// resolved (p was not made a leader, or it already exited), it falls back to
// signalling p alone so behaviour is never worse than before.
func signalProcessGroup(p *os.Process, sig syscall.Signal) error {
	if p == nil {
		return nil
	}
	if err := syscall.Kill(-p.Pid, sig); err != nil {
		return p.Signal(sig)
	}
	return nil
}

// killProcessGroup force-terminates (SIGKILL) the whole group led by p.
func killProcessGroup(p *os.Process) error {
	return signalProcessGroup(p, syscall.SIGKILL)
}
