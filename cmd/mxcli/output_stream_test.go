// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"testing"
)

// `mxcli lint --format json` wrote its progress commentary — "Connected to:",
// "Loading cached catalog...", "✓ Catalog ready" — to stdout, ahead of the JSON
// payload, so stdout was never parseable:
//
//	$ mxcli lint -p app.mpr --format json 2>/dev/null | jq .
//	parse error: Invalid numeric literal at line 1, column 10
//
// The command's own help advertises `--format sarif > results.sarif`, which
// produced a corrupt file for the same reason. Diagnostics belong on stderr
// whenever stdout carries a machine-readable payload.
func TestIsMachineReadableFormat(t *testing.T) {
	tests := []struct {
		format string
		want   bool
	}{
		{"json", true},
		{"sarif", true},
		{"html", true},
		{"markdown", true},
		{"md", true},
		{"text", false},
		{"", false},
		// Case and padding must not decide whether stdout gets corrupted.
		{"JSON", true},
		{"Sarif", true},
		{" json ", true},
		// An unrecognised format is treated as human-facing: a future text-like
		// format silently losing its progress output to stderr is a smaller
		// harm than guessing wrong the other way is, but neither is silent —
		// resolveFormat validates the flag before this is consulted.
		{"not-a-format", false},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			if got := isMachineReadableFormat(tt.format); got != tt.want {
				t.Errorf("isMachineReadableFormat(%q) = %v, want %v", tt.format, got, tt.want)
			}
		})
	}
}

// progressSink is what the command hands to the executor, so this is the
// assertion that actually keeps chatter off stdout.
func TestProgressSink(t *testing.T) {
	if got := progressSink("json"); got != os.Stderr {
		t.Error("json progress must go to stderr, or the payload on stdout is unparseable")
	}
	if got := progressSink("sarif"); got != os.Stderr {
		t.Error("sarif progress must go to stderr")
	}
	if got := progressSink("text"); got != os.Stdout {
		t.Error("text progress belongs on stdout — moving it would break interactive use")
	}
}
