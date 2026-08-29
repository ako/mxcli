// SPDX-License-Identifier: Apache-2.0

//go:build linux

package docker

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// listenOnLoopback binds an ephemeral port and returns it, closing on cleanup.
func listenOnLoopback(t *testing.T, network, addr string) int {
	t.Helper()
	ln, err := net.Listen(network, addr)
	if err != nil {
		t.Skipf("cannot bind %s %s here: %v", network, addr, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("splitting %s: %v", ln.Addr(), err)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("parsing port %q: %v", portStr, err)
	}
	return port
}

// The whole point of the lookup is to name a real pid, so the test holds the
// port itself and demands its own — an assertion that cannot pass by accident.
//
// mxcli-formula1 suggested issue 8: the guard was right to refuse, but told the
// user to go hunting with pgrep, and one of the suggested patterns
// (`pgrep -f 'mxcli run'`) matches the shell it is typed into.
func TestListenerOnPort_IdentifiesThisProcess(t *testing.T) {
	port := listenOnLoopback(t, "tcp", "127.0.0.1:0")

	owner, ok := listenerOnPort(port)
	if !ok {
		t.Fatalf("port %d is held by this test process but no owner was resolved", port)
	}
	if owner.PID != os.Getpid() {
		t.Errorf("owner pid = %d, want this process (%d)", owner.PID, os.Getpid())
	}
	if owner.Cmdline == "" {
		t.Error("no command line resolved; the message would name a bare pid")
	}
}

// A Mendix runtime binding "localhost" lands on IPv6 as often as not, so
// /proc/net/tcp6 has to be parsed too — and its local_address is a 128-bit hex
// blob, not the 32-bit one in /proc/net/tcp. Both are asserted against captured
// kernel output rather than a live socket, because this container has no IPv6
// and a test that only skips proves nothing about the path it names.
//
// 8090 is 0x1F9A; 6543 is 0x198F.
func TestParseListeningInodes_BothAddressFamilies(t *testing.T) {
	const tcp4 = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:198F 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 4242 1 0000000000000000 100 0 0 10 0
`
	const tcp6 = `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:1F9A 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 9999 1 0000000000000000 100 0 0 10 0
`
	if got := parseListeningInodes(tcp4, 6543); !got["4242"] {
		t.Errorf("IPv4 listener on 6543 not found, got %v", got)
	}
	if got := parseListeningInodes(tcp6, 8090); !got["9999"] {
		t.Errorf("IPv6 listener on 8090 not found — the 128-bit local_address is not being parsed, got %v", got)
	}
}

// An established connection to a port is not the owner of it. Without the
// TCP_LISTEN filter, a client of the app would be named as the culprit and the
// advice would tell the user to kill their own browser.
func TestParseListeningInodes_IgnoresEstablishedConnections(t *testing.T) {
	// 01 is TCP_ESTABLISHED. Same port (0x1F90 = 8080), different state.
	const established = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1F90 0100007F:C001 01 00000000:00000000 00:00000000 00000000     0        0 7777 1 0000000000000000 100 0 0 10 0
`
	if got := parseListeningInodes(established, 8080); len(got) != 0 {
		t.Errorf("an ESTABLISHED socket was treated as the port's owner: %v", got)
	}
}

// Parsing tcp6 correctly is worth nothing if the file is never opened, and the
// parser tests above cannot tell the difference — dropping "/proc/net/tcp6" from
// the list left every one of them green. This points the reader at fixtures and
// asserts an inode is picked up from EACH table.
func TestListeningInodes_ReadsBothKernelTables(t *testing.T) {
	dir := t.TempDir()
	v4 := filepath.Join(dir, "tcp")
	v6 := filepath.Join(dir, "tcp6")
	// Same port (0x198F = 6543) in both tables, distinct inodes.
	if err := os.WriteFile(v4, []byte(
		"  sl  local_address rem_address   st … inode\n"+
			"   0: 0100007F:198F 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 4242 1 0 100 0 0 10 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v6, []byte(
		"  sl  local_address                         remote_address                        st … inode\n"+
			"   0: 00000000000000000000000000000000:198F 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 9999 1 0 100 0 0 10 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	orig := procNetTCPFiles
	// Overriding the list is what makes the rest of this testable, and it is also
	// what stops the override from proving anything about the SHIPPED list — so
	// the default is asserted first. Dropping "/proc/net/tcp6" from it left every
	// other test here green.
	for _, want := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		found := false
		for _, f := range orig {
			if f == want {
				found = true
			}
		}
		if !found {
			t.Errorf("procNetTCPFiles does not include %s, so that family is never read: %v", want, orig)
		}
	}

	procNetTCPFiles = []string{v4, v6}
	t.Cleanup(func() { procNetTCPFiles = orig })

	got := listeningInodes(6543)
	if !got["4242"] {
		t.Errorf("/proc/net/tcp is not being read, got %v", got)
	}
	if !got["9999"] {
		t.Errorf("/proc/net/tcp6 is not being read — an IPv6-bound runtime would go unnamed, got %v", got)
	}
}

// A live IPv6 listener, where the environment has one. Skipped here (no IPv6),
// which is why the parser and wiring tests above carry the real weight.
func TestListenerOnPort_FindsAnIPv6Listener(t *testing.T) {
	port := listenOnLoopback(t, "tcp6", "[::1]:0")

	owner, ok := listenerOnPort(port)
	if !ok {
		t.Fatalf("IPv6 listener on %d not resolved — /proc/net/tcp6 is not being read", port)
	}
	if owner.PID != os.Getpid() {
		t.Errorf("owner pid = %d, want this process (%d)", owner.PID, os.Getpid())
	}
}

// Nothing listening must resolve to nothing. If any socket state matched, a free
// port would name some unrelated process and the advice would be worse than the
// generic message it replaced.
func TestListenerOnPort_QuietWhenNothingIsListening(t *testing.T) {
	port := listenOnLoopback(t, "tcp", "127.0.0.1:0")
	// Re-resolve after the listener is closed: the port is now free.
	owner, ok := listenerOnPort(port)
	if ok {
		// Only fail if it resolved to something while the port is still ours to
		// claim — a racing process could legitimately take it.
		if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
			_ = ln.Close()
			t.Errorf("free port %d resolved to pid %d (%s)", port, owner.PID, owner.Cmdline)
		}
	}
}

func TestLooksLikeWarmLoopChild(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cmdline string
		want    bool
	}{
		{"mxbuild serve", "/root/.mxcli/mxbuild/11.13.0/modeler/mxbuild --serve", true},
		{"runtime launcher", "java -cp … com.mendix.runtimelauncher.RuntimeLauncher", true},
		{"a previous mxcli", "mxcli run --local -p app.mpr", true},
		{"someone else's server", "python3 -m http.server 8080", false},
		{"unresolvable", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeWarmLoopChild(tc.cmdline); got != tc.want {
				t.Errorf("looksLikeWarmLoopChild(%q) = %v, want %v", tc.cmdline, got, tc.want)
			}
		})
	}
}

// The advice is the deliverable, not the lookup, so it is asserted through the
// same function the error message calls.
func TestPortCulpritAdvice_NamesThePidAndGivesOneCommand(t *testing.T) {
	port := listenOnLoopback(t, "tcp", "127.0.0.1:0")

	advice := portCulpritAdvice(port, "127.0.0.1", port)
	if !strings.Contains(advice, fmt.Sprintf("pid %d", os.Getpid())) {
		t.Errorf("advice does not name the holding pid %d:\n%s", os.Getpid(), advice)
	}
	if strings.Contains(advice, "pgrep") {
		t.Errorf("the pid is known, so the pgrep hunt must be gone:\n%s", advice)
	}
}

// A foreign listener and a leftover child need opposite remedies — kill it, or
// leave it alone and move ports — so the advice must not offer `kill` for a
// process mxcli did not start.
func TestPortCulpritAdvice_DoesNotOfferToKillAForeignProcess(t *testing.T) {
	port := listenOnLoopback(t, "tcp", "127.0.0.1:0")
	owner, ok := listenerOnPort(port)
	if !ok {
		t.Skip("owner not resolvable here")
	}
	if owner.Ours {
		// The test binary happens to match the warm-loop signature (it can, when
		// run from a path containing "mxcli"); this case cannot be exercised.
		t.Skipf("test process looks like a warm-loop child: %s", owner.Cmdline)
	}
	advice := portCulpritAdvice(port, "127.0.0.1", port)
	if strings.Contains(advice, fmt.Sprintf("kill %d", os.Getpid())) {
		t.Errorf("offered to kill a foreign process:\n%s", advice)
	}
	if !strings.Contains(advice, "not a process mxcli started") {
		t.Errorf("advice does not say the holder is foreign:\n%s", advice)
	}
}

// End to end through the real guard: the error a user sees carries the pid.
func TestCheckTargetPortsFree_ErrorNamesTheHolder(t *testing.T) {
	port := listenOnLoopback(t, "tcp", "127.0.0.1:0")

	err := checkTargetPortsFree(LocalRunOptions{AppPort: port, AdminPort: 0, ServePort: 0})
	if err == nil {
		t.Fatal("a held app port must be refused")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("pid %d", os.Getpid())) {
		t.Errorf("the refusal does not name the holder:\n%v", err)
	}
}

// A port field wider than 16 bits is not a port, and must be refused rather than
// narrowed. Parsing at bit size 32 accepted it: on a 64-bit build "FFFFFFFF"
// came back as 4294967295, and on a 32-bit one (386, arm — measured) as -1 with
// ok=true, because int is 32 bits there and the conversion silently wraps. The
// kernel never writes such a field, so nothing here is reachable from a hostile
// input; what the old code gave up was the parser doing the range check, which
// is the only thing standing between the caller's `p != port` comparison and a
// number that is not a port at all. This is CodeQL alert 8
// (go/incorrect-integer-conversion).
//
// Both halves matter: the reject case fails against the old parser, and the
// 8080 case is the control that the narrower parse still accepts a real port.
func TestHexPort_RefusesAValueWiderThanAPort(t *testing.T) {
	if p, ok := hexPort("0100007F:1F90"); !ok || p != 8080 {
		t.Fatalf("control failed: a real port no longer parses, got %d,%v", p, ok)
	}
	for _, addr := range []string{"0100007F:FFFFFFFF", "0100007F:80000000", "0100007F:10000"} {
		if p, ok := hexPort(addr); ok {
			t.Errorf("%s: accepted as port %d — a port is 16 bits", addr, p)
		}
	}
}

// End to end through the parser: a row whose port field is out of range must not
// be matched against the port being looked up, whatever it narrows to.
func TestParseListeningInodes_IgnoresAnOutOfRangePortField(t *testing.T) {
	const bogus = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:FFFFFFFF 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 5555 1 0000000000000000 100 0 0 10 0
`
	// int(^uint32(0)) is whatever 0xFFFFFFFF narrows to on this build — -1 where
	// int is 32 bits, 4294967295 where it is 64 — so the row is asked for under
	// the value the old parser would have produced, on either width. Written as
	// an expression rather than a literal because 4294967295 is not a valid int
	// constant on a 32-bit build.
	for _, port := range []int{-1, 65535, int(^uint32(0))} {
		if got := parseListeningInodes(bogus, port); len(got) != 0 {
			t.Errorf("port %d matched a row whose port field is not a port: %v", port, got)
		}
	}
}
