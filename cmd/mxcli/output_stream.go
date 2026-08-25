// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"os"
	"strings"
)

// machineReadableFormats are output formats whose payload is consumed by a
// program or redirected to a file, so stdout must carry the payload and nothing
// else.
var machineReadableFormats = map[string]bool{
	"json":     true,
	"sarif":    true,
	"html":     true,
	"markdown": true,
	"md":       true,
}

// isMachineReadableFormat reports whether stdout is carrying a payload that
// anything else printed there would corrupt.
//
// `mxcli lint --format json` used to print "Connected to:", "Loading cached
// catalog..." and "✓ Catalog ready" to stdout ahead of the JSON, so stdout never
// parsed — while the command's own help advertised
// `--format sarif > results.sarif`, which wrote a corrupt file for the same
// reason. An unrecognised format is treated as human-facing; the format flag is
// validated separately, so this is not where a typo should be caught.
func isMachineReadableFormat(format string) bool {
	return machineReadableFormats[strings.ToLower(strings.TrimSpace(format))]
}

// progressSink returns the stream a command should send progress and diagnostic
// output to for the given output format: stderr when stdout is carrying a
// machine-readable payload, stdout otherwise.
//
// Routing progress to stderr rather than discarding it keeps it visible on a
// terminal and in CI logs — the information is not noise, it is just on the
// wrong stream when stdout is a payload.
func progressSink(format string) io.Writer {
	if isMachineReadableFormat(format) {
		return os.Stderr
	}
	return os.Stdout
}
