// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `--mxbuild-path` was documented as the override for the local loop by the
// shipped run-local skill, by runlocal.go's own comment, and by two error
// messages that tell the user to pass it — while `run` never registered the
// flag, so it came back "unknown flag". On macOS that was the only advertised
// way out of a platform mismatch. (issue #1125)

// TestRunCommandHasMxBuildPathFlag pins the flag's existence. The plumbing behind
// it (LocalRunOptions.MxBuildPath -> ResolveMxBuildForLocal) already worked; only
// the registration was missing, so nothing else could reveal the gap.
func TestRunCommandHasMxBuildPathFlag(t *testing.T) {
	f := runCmd.Flags().Lookup("mxbuild-path")
	if f == nil {
		t.Fatal("run has no --mxbuild-path flag, but the run-local skill and two error messages tell users to pass it")
	}
	if f.Usage == "" {
		t.Error("the flag needs help text; it is what users are pointed at when resolution fails")
	}
}

// TestRunMxBuildPathFlagIsAccepted is the reporter's command line, resolved
// through the real command tree so that `-p` (persistent, on root) is in scope —
// parsing alone is what failed, before any project was opened.
func TestRunMxBuildPathFlagIsAccepted(t *testing.T) {
	cmd, args, err := rootCmd.Find([]string{"run", "--local", "--mxbuild-path", "/x", "-p", "app.mpr"})
	if err != nil {
		t.Fatalf("finding run: %v", err)
	}
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("mxcli run --local --mxbuild-path /x -p app.mpr: %v", err)
	}
	if got, _ := cmd.Flags().GetString("mxbuild-path"); got != "/x" {
		t.Errorf("flag parsed to %q, want /x", got)
	}
}

// TestErrorGuidanceNamesAFlagThatExists is the general form of the defect: an
// error telling the user to pass an option the command does not have is worse
// than no guidance, because it reads as the user's mistake. Every --flag any
// mxbuild-resolution message recommends must be registered on `run`.
func TestErrorGuidanceNamesAFlagThatExists(t *testing.T) {
	for _, src := range []string{
		filepath.Join("docker", "mxbuild_platform.go"),
		filepath.Join("docker", "detect.go"),
		filepath.Join("docker", "mxserve.go"),
	} {
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("reading %s: %v", src, err)
		}
		if !strings.Contains(string(b), "--mxbuild-path") {
			continue
		}
		if runCmd.Flags().Lookup("mxbuild-path") == nil {
			t.Errorf("%s tells users to pass --mxbuild-path, which `run` does not accept", src)
		}
	}
}
