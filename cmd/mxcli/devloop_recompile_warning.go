// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
)

// warnIfDevLoopServing tells a `mxcli test --local` run that the dev loop is
// serving the same project, and reports whether it did.
//
// A test run recompiles the project's Java into `deployment/run/bin`, which is
// the classpath that running app's JVM is holding open. mxcli cannot avoid it:
// mxbuild's Gradle pass owns the compile, and the deployment directory cannot be
// moved — mxbuild writes it to `<app dir>/deployment` and takes no option to
// change it (mxcli-ledger §150). So the collision is reported rather than
// prevented, which is the part that was missing: the reporting project spent 108
// log lines and a wrong hypothesis about a different app before connecting the
// two (mxcli-formula1 §81).
//
// It reads the handshake `mxcli run --local` already publishes for `mxcli
// constant set --apply`, rather than a second file of its own: that one carries
// the admin password and boot config those commands depend on, and a competing
// writer at the same path would drop them.
//
// It warns rather than refuses. The warm loop exists so an app can stay up while
// you work on it — the reporting project runs two apps that way as a matter of
// course — and refusing would break the workflow the feature is for, to prevent
// something whose remedy is one restart.
func warnIfDevLoopServing(projectPath string, w io.Writer) bool {
	if w == nil {
		return false
	}
	// A missing file, a corrupt one, and a pid that is gone all come back as an
	// error here, which is exactly the behaviour wanted: a stale handshake from a
	// `run --local` that was killed or whose development licence expired (§60)
	// must not warn, or the warning fires forever and stops being read.
	hs, err := readDevLoopHandshake(projectPath)
	if err != nil {
		return false
	}
	fmt.Fprintln(w, recompileWarning(hs))
	return true
}

// recompileWarning names the symptom, because the symptom is the part nobody
// guesses: the app does not error, it answers 200 with nothing in it, and only
// the resources backed by Java are affected — so half the app keeps working,
// which is not a shape that suggests "the test run did this".
func recompileWarning(hs devLoopHandshake) string {
	return fmt.Sprintf(
		"Warning: `mxcli run --local` is serving this project on port %d (pid %d).\n"+
			"  This test run recompiles the project's Java into deployment/run/bin, which is\n"+
			"  that app's classpath — every class file is rewritten. A class the running app\n"+
			"  has not loaded yet can then fail with NoClassDefFoundError, and the microflows\n"+
			"  behind it answer HTTP 200 with an EMPTY BODY rather than an error, so the app\n"+
			"  looks half-working (mxcli-formula1 §81).\n"+
			"  If anything it serves stops returning data, restart that app.\n"+
			"  The deployment directory cannot be separated: mxbuild always writes it to\n"+
			"  <app dir>/deployment.",
		hs.AppPort, hs.PID)
}
