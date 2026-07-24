// SPDX-License-Identifier: Apache-2.0

//go:build windows

package docker

import (
	"os"
	"os/exec"
	"syscall"
)

// procgroup_windows.go provides no-op / single-process fallbacks for the
// process-group helpers. Windows has no POSIX process groups; the warm loop is a
// Linux-devcontainer feature, so this only needs to keep the package compiling
// and preserve the prior single-PID signalling behaviour.

// setProcessGroup is a no-op on Windows.
func setProcessGroup(cmd *exec.Cmd) {}

// signalProcessGroup signals the process itself (no group semantics on Windows).
func signalProcessGroup(p *os.Process, sig syscall.Signal) error {
	if p == nil {
		return nil
	}
	return p.Signal(sig)
}

// killProcessGroup force-terminates the process.
func killProcessGroup(p *os.Process) error {
	if p == nil {
		return nil
	}
	return p.Kill()
}
