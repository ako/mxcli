// SPDX-License-Identifier: Apache-2.0

//go:build linux

package docker

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// session_linux.go finds every process a detached `mxcli run --local` ever
// started, so `mxcli run stop` can prove none is left behind.
//
// A detached run leads its own session (setsid). Every child the loop starts —
// mxbuild's wrapper and its JVM, the runtime JVM, the rollup bundler — is put in
// a process group of its own (procgroup_unix.go) but never in a session of its
// own, so they all keep the leader's session id. That survives the leader: a run
// killed with -9, which runs no teardown at all, leaves its orphans reparented to
// init but still in session <pid>. Matching on the session is therefore the one
// handle that finds an orphaned runtimelauncher without guessing from command
// lines — the guess `pkill -f` makes, which also matches the shell it runs in.

// clockTicks is USER_HZ, the unit of /proc/<pid>/stat's starttime. It is 100 on
// every Linux architecture Go supports; reading it needs sysconf (cgo).
const clockTicks = 100

// SessionMembers returns the pids of live processes in session sid that started
// no earlier than notBefore. The time filter guards against pid reuse: a session
// id is a pid, and long after the run has gone an unrelated session could carry
// the same number. A zero notBefore disables the filter.
func SessionMembers(sid int, notBefore time.Time) []int {
	if sid <= 0 {
		return nil
	}
	boot := bootTime()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	self := os.Getpid()
	var out []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		st, ok := parseProcStat(string(data))
		if !ok || st.session != sid || st.state == "Z" {
			continue
		}
		if !notBefore.IsZero() && !boot.IsZero() && startedBefore(boot, st.startTicks, notBefore) {
			continue
		}
		out = append(out, pid)
	}
	return out
}

// startSlack is how far before notBefore a rebuilt start time may fall and
// still count. The rebuild is btime + starttime: btime is whole seconds (up to
// 1s early) and starttime whole ticks (up to 10ms early), and the kernel's
// boot-time arithmetic and Go's wall clock can disagree by a little more. The
// previous 1s covered the first alone, so on a machine whose boot fell late in
// a second a run's own leader read as older than the run and was dropped:
// `run stop` saw nothing to stop. Fixed per machine, not per call — such a CI
// VM failed every session test in the run. Pid reuse, which this guards
// against, comes from a session that started long before, never seconds.
const startSlack = 2 * time.Second

// startedBefore reports whether a process whose stat starttime is startTicks
// began before notBefore, beyond startSlack.
func startedBefore(boot time.Time, startTicks int64, notBefore time.Time) bool {
	started := boot.Add(time.Duration(startTicks) * time.Second / clockTicks)
	return started.Before(notBefore.Add(-startSlack))
}

// procStat is the part of /proc/<pid>/stat SessionMembers needs.
type procStat struct {
	state      string
	session    int
	startTicks int64
}

// parseProcStat parses /proc/<pid>/stat. The command name (field 2) is in
// parentheses and may itself contain spaces and parentheses, so the fields are
// counted from the LAST ')'.
func parseProcStat(s string) (procStat, bool) {
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return procStat{}, false
	}
	f := strings.Fields(s[i+1:])
	// f[0] is field 3 (state); session is field 6, starttime field 22.
	if len(f) < 20 {
		return procStat{}, false
	}
	session, err := strconv.Atoi(f[3])
	if err != nil {
		return procStat{}, false
	}
	start, err := strconv.ParseInt(f[19], 10, 64)
	if err != nil {
		return procStat{}, false
	}
	return procStat{state: f[0], session: session, startTicks: start}, true
}

// bootTime is the kernel's boot time from /proc/stat ("btime").
func bootTime() time.Time {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, "btime "); ok {
			if n, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 64); err == nil {
				return time.Unix(n, 0)
			}
		}
	}
	return time.Time{}
}

// ProcessCmdline is a process's command line, truncated (for messages).
func ProcessCmdline(pid int) string { return processCmdline(pid) }

// PidAlive reports whether pid is a live process. Unlike signal 0, it does not
// count a zombie as alive: a detached run's leader is reparented when the shell
// that started it exits, and until that parent reaps it an exited run would
// otherwise read as still running — `run stop` would wait out its whole grace
// period on a process that is already gone.
func PidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	st, ok := parseProcStat(string(data))
	return ok && st.state != "Z" && st.state != "X"
}
