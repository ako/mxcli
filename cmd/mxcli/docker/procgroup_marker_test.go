// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"strconv"
	"strings"
	"testing"
)

// grandchildPID extracts the pid the "grandchild" helper mode announces (see
// procgroup_windows_test.go). It reports false until a COMPLETE marker line has
// arrived: the marker's only job is to prove the process holding the inherited
// pipe is running, so accepting a half-written line would give back exactly the
// false "it is up" the marker exists to rule out.
//
// It lives in an untagged file, away from its windows-only caller, so this one
// piece of parsing is covered on every platform rather than only in the Windows
// CI job.
func grandchildPID(out string) (int, bool) {
	// Whatever follows the final newline is a partial write, and a truncated pid
	// parses as a perfectly plausible one.
	end := strings.LastIndex(out, "\n")
	if end < 0 {
		return 0, false
	}
	for _, line := range strings.Split(out[:end], "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "grandchild-started ")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(rest))
		if err != nil || pid <= 0 {
			continue
		}
		return pid, true
	}
	return 0, false
}

func TestGrandchildPID(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		wantPID int
		wantOK  bool
	}{
		{"empty", "", 0, false},
		{"complete line", "grandchild-started 4242\n", 4242, true},
		{"crlf", "grandchild-started 4242\r\n", 4242, true},
		{"after other output", "noise\ngrandchild-started 7\nmore\n", 7, true},

		// The reason this function is not strings.Contains. A pipe read can stop
		// mid-line, and "grandchild-started 42" is a prefix of "...4242": taking
		// it would name a different process, and the liveness check that follows
		// would then be asserting something about a stranger.
		{"partial line is not a pid", "grandchild-started 42", 0, false},
		{"partial after complete noise", "noise\ngrandchild-started 42", 0, false},

		{"marker with no pid", "grandchild-started\n", 0, false},
		{"marker with empty pid", "grandchild-started \n", 0, false},
		{"non-numeric pid", "grandchild-started abc\n", 0, false},
		{"zero pid", "grandchild-started 0\n", 0, false},
		{"negative pid", "grandchild-started -1\n", 0, false},
		{"different marker", "grandchild-exited 4242\n", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pid, ok := grandchildPID(tt.out)
			if ok != tt.wantOK || pid != tt.wantPID {
				t.Fatalf("grandchildPID(%q) = %d, %v; want %d, %v",
					tt.out, pid, ok, tt.wantPID, tt.wantOK)
			}
		})
	}
}
