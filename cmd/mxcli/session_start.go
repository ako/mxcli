// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/diaglog"
)

// startSession records the invocation before anything can reject it.
// `diag loop-report` reports "mxcli invocations: N", and burning calls on
// malformed invocations is exactly the agent failure it is opened to see
// (ako/mxcli#617). It runs from main() rather than PersistentPreRun because
// cobra validates Args before any hook: an arity failure used to return
// before anything was logged (ako/mxcli#633).
//
// Init is a per-process singleton, so the commands that call it later get this
// same logger — and whatever mode is chosen here is the one recorded.
func startSession(args []string) {
	if mode, ok := sessionMode(args); ok {
		_ = diaglog.Init(version, mode)
	}
}

// sessionMode names the session the way the command would once it ran: the
// subcommand's path, or for the root command the mode its own Run passes —
// "batch" for -c, "repl" otherwise — which invocationVerb falls back to when
// argv names no subcommand. ok is false for diag: a report that counted its
// own runs would climb every time it was read.
func sessionMode(args []string) (mode string, ok bool) {
	cmd, _, err := rootCmd.Find(args)
	if err != nil || cmd == nil {
		// An unknown subcommand is still a wasted call; name it by the root.
		return rootCmd.Name(), true
	}
	path := cmd.CommandPath()
	if strings.HasPrefix(path, rootCmd.Name()+" diag") {
		return "", false
	}
	if cmd != rootCmd {
		return path, true
	}
	// --help and --version are answered by cobra without reaching Run, so the
	// root is neither a one-shot nor a REPL there.
	if hasFlag(args, "-h", "--help") {
		return "help", true
	}
	if hasFlag(args, "", "--version") {
		return "version", true
	}
	if hasCommandFlag(args) {
		return "batch", true
	}
	return "repl", true
}

// hasCommandFlag reports whether args pass the root's -c / --command, in any
// of the forms pflag accepts.
func hasCommandFlag(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		switch {
		case a == "-c", a == "--command", strings.HasPrefix(a, "--command="):
			return true
		case strings.HasPrefix(a, "-c") && !strings.HasPrefix(a, "--"):
			return true // -cVALUE, -c=VALUE
		}
	}
	return false
}

// hasFlag reports whether args pass a boolean flag by its short or long name.
func hasFlag(args []string, short, long string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if (short != "" && a == short) || a == long || strings.HasPrefix(a, long+"=") {
			return true
		}
	}
	return false
}
