// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package docker

import (
	"time"

	"github.com/mendixlabs/mxcli/internal/procalive"
)

// SessionMembers is Linux-only (it reads /proc). Elsewhere `run stop` relies on
// the run's own graceful teardown, which reaps every child it started.
func SessionMembers(sid int, notBefore time.Time) []int { return nil }

// ProcessCmdline is unavailable off Linux.
func ProcessCmdline(pid int) string { return "" }

// PidAlive reports whether pid is a live process.
//
// On Windows this used to be "os.FindProcess can open it", but an exited
// process stays openable while anyone holds a handle to it (its parent,
// typically), so `run stop` kept reporting a run that had shut down as still
// alive. procalive.Alive asks whether the process has terminated instead.
func PidAlive(pid int) bool { return procalive.Alive(pid) }
