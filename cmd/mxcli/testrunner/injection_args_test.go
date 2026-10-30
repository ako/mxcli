// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"slices"
	"testing"
)

// The runner injects a Java action (the test endpoint) with `mxcli exec` and
// deletes its javasource/ folder again at cleanup. Since ako/mxcli#1618 `exec`
// names every file it writes beside the model and says to commit it — advice
// that is wrong for a file the runner is about to remove, so its injection runs
// turn the report off.
func TestInjectionExecTurnsOffTheSourceFileReport(t *testing.T) {
	args := injectionExecArgs("/tmp/mxtest-endpoint.mdl", "/work/App.mpr")
	want := []string{"exec", "/tmp/mxtest-endpoint.mdl", "-p", "/work/App.mpr", "--no-source-file-report"}
	if !slices.Equal(args, want) {
		t.Errorf("injection exec args = %q, want %q", args, want)
	}
}
