// SPDX-License-Identifier: Apache-2.0

//go:build linux

package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// portowner_linux.go answers "which process is holding this port?" so the warm
// loop's port guard can name it.
//
// The guard itself is correct and stays detection-only — reaping someone else's
// process is the user's call. What was missing is the diagnosis. `mxcli run
// --local` reaps its children on SIGINT/SIGTERM (see procgroup_unix.go), so a
// held port after a *graceful* stop is someone else's process; but a `kill -9`,
// a crash, or a reaped container runs no handler at all, and then the offender
// is the previous run's orphaned mxbuild/JVM. Both cases print the same message
// today, and it tells the user to go hunting with pgrep — three commands, one of
// which (`pgrep -f 'mxcli run'`) matches the shell you typed it in.
//
// Resolution is via /proc, not lsof/ss: those are frequently absent from slim
// containers, and shelling out to find out why a boot failed is its own failure
// mode. /proc/net/tcp gives the socket inode of the listener; scanning
// /proc/<pid>/fd for a link to that inode gives the owner.

// portOwner describes the process listening on a local port.
type portOwner struct {
	PID     int
	Cmdline string // argv joined with spaces, truncated
	Ours    bool   // command line looks like a process mxcli spawns
}

// listenerOnPort identifies the process listening on 127.0.0.1:port (or on
// 0.0.0.0/[::]:port). Returns ok=false when nothing can be resolved — a listener
// owned by another user is the common case, since /proc/<pid>/fd is unreadable
// then. Callers must degrade to the generic message rather than assert.
func listenerOnPort(port int) (portOwner, bool) {
	inodes := listeningInodes(port)
	if len(inodes) == 0 {
		return portOwner{}, false
	}
	pid, ok := pidForSocketInode(inodes)
	if !ok {
		return portOwner{}, false
	}
	cmd := processCmdline(pid)
	return portOwner{PID: pid, Cmdline: cmd, Ours: looksLikeWarmLoopChild(cmd)}, true
}

// listeningInodes returns the socket inodes of every LISTEN socket bound to
// port, across IPv4 and IPv6. Both are checked because a JVM binding "localhost"
// usually lands on [::1] or a dual-stack [::], not 127.0.0.1.
// procNetTCPFiles are the kernel tables listeningInodes reads. A var, not a
// literal, so a test can prove BOTH are consulted — parsing tcp6 correctly is
// worth nothing if the file is never opened, and a test that calls the parser
// directly cannot tell the difference.
var procNetTCPFiles = []string{"/proc/net/tcp", "/proc/net/tcp6"}

func listeningInodes(port int) map[string]bool {
	out := map[string]bool{}
	for _, f := range procNetTCPFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for ino := range parseListeningInodes(string(data), port) {
			out[ino] = true
		}
	}
	return out
}

// parseListeningInodes extracts the socket inodes of LISTEN sockets on port from
// the contents of /proc/net/tcp or /proc/net/tcp6.
//
// Split out from the file reading so both address families can be tested
// wherever the suite runs: this container has no IPv6, and a test that only ever
// skips proves nothing about the tcp6 path — which is the one that matters, since
// a JVM binding "localhost" usually lands on [::1] or a dual-stack [::].
func parseListeningInodes(contents string, port int) map[string]bool {
	out := map[string]bool{}
	lines := strings.Split(contents, "\n")
	if len(lines) > 0 {
		lines = lines[1:] // header
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		// sl(0) local_address(1) remote_address(2) st(3) … inode(9)
		if len(fields) < 10 {
			continue
		}
		// 0A is TCP_LISTEN. An established connection *to* the port is not the
		// owner of it, and counting one would name a client as the culprit.
		if fields[3] != "0A" {
			continue
		}
		if p, ok := hexPort(fields[1]); !ok || p != port {
			continue
		}
		out[fields[9]] = true
	}
	return out
}

// hexPort extracts the port from a /proc/net/tcp local_address ("0100007F:1F90").
//
// The bit size is 16 because a TCP port is 16 bits — the kernel writes the field
// as %04X. Parsing at 32 and narrowing to int is only correct where int is 64
// bits: on a 32-bit build (386, arm) "FFFFFFFF" parses fine and converts to -1,
// so an out-of-range field would be reported as a port instead of rejected.
// Nothing the kernel writes reaches that, but the comparison in the caller is
// then against a number that is not a port, and the whole point of this file is
// to name the right process. Parsing at the type's real width makes the range
// check the parser's job. (CodeQL go/incorrect-integer-conversion, alert 8.)
func hexPort(addr string) (int, bool) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return 0, false
	}
	n, err := strconv.ParseUint(addr[i+1:], 16, 16)
	if err != nil {
		return 0, false
	}
	return int(n), true
}

// pidForSocketInode finds the process holding any of the given socket inodes.
//
// A socket may be shared by several processes (a forked pre-forking server), and
// any of them will do for the message — the user needs a thread to pull, not a
// census. The lowest PID is chosen so repeated runs name the same one.
func pidForSocketInode(inodes map[string]bool) (int, bool) {
	want := map[string]bool{}
	for ino := range inodes {
		want["socket:["+ino+"]"] = true
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, false
	}
	best := 0
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // not a pid directory
		}
		fds, err := os.ReadDir(filepath.Join("/proc", e.Name(), "fd"))
		if err != nil {
			continue // another user's process, or it exited mid-scan
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join("/proc", e.Name(), "fd", fd.Name()))
			if err != nil {
				continue
			}
			if want[link] {
				if best == 0 || pid < best {
					best = pid
				}
				break
			}
		}
	}
	return best, best != 0
}

// processCmdline reads a process's argv, NUL-separated in /proc, and renders it
// on one line. Long Java command lines are truncated: the JVM's is thousands of
// characters of classpath, and printing it buries the error it is attached to.
func processCmdline(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return ""
	}
	args := strings.FieldsFunc(string(data), func(r rune) bool { return r == 0 })
	line := strings.Join(args, " ")
	const max = 120
	if len(line) > max {
		line = line[:max] + "…"
	}
	return line
}

// looksLikeWarmLoopChild reports whether a command line matches something the
// warm loop starts. This decides which advice to print, and the two are
// genuinely different: an orphan of a previous run is safe to kill, while a
// foreign listener means the port is simply taken and the right move is
// --app-port.
func looksLikeWarmLoopChild(cmdline string) bool {
	if cmdline == "" {
		return false
	}
	for _, sig := range []string{
		"mxbuild",         // the serve process (a wrapper script, or its JVM)
		"runtimelauncher", // the standalone runtime
		"RuntimeLauncher",
		"mxcli", // a previous run still alive
	} {
		if strings.Contains(cmdline, sig) {
			return true
		}
	}
	return false
}
