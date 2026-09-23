// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// On Windows `mxcli test --local` ran the Linux mxbuild from ~/.mxcli/mxbuild and
// died with "local runtime: starting mxbuild serve: mxbuild --serve did not
// become ready", and the reporter found "no flag, environment variable, or
// mechanism to redirect mxcli to the Windows mxbuild.exe already present in the
// Studio Pro installation". `run` gained --mxbuild-path in #1125; `test` boots
// the same app through the same resolver and was left without it. (issue #1086)

// TestTestMxBuildPathFlagIsAccepted is the reporter's command line plus the
// override, resolved through the real command tree so `-p` (persistent, on
// root) is in scope.
func TestTestMxBuildPathFlagIsAccepted(t *testing.T) {
	const override = `C:\Program Files\Mendix\11.11.0\modeler\mxbuild.exe`
	cmd, args, err := rootCmd.Find([]string{"test", "tests/", "-p", "MyApp.mpr", "--local", "--mxbuild-path", override})
	if err != nil {
		t.Fatalf("finding test: %v", err)
	}
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("mxcli test tests/ -p MyApp.mpr --local --mxbuild-path …: %v", err)
	}
	if got, _ := cmd.Flags().GetString("mxbuild-path"); got != override {
		t.Errorf("flag parsed to %q, want %q", got, override)
	}
}
