// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1629: `mxcli docker --help` listed "Mendix 11.6.1 or later"
// as a requirement of the whole command group. The floor is docker build's —
// a Portable App Distribution needs 11.6.1, and Build refuses anything older —
// while docker check has none: measured on a 10.24.18 project, it reports
// "The app contains: 0 errors." An agent takes the help's word for it, so a
// group-wide floor is a check it never tries.
func TestDockerHelp_VersionFloorIsBuildsNotChecks(t *testing.T) {
	for _, line := range strings.Split(dockerCmd.Long, "\n") {
		if strings.Contains(line, "11.6.1") && !strings.Contains(line, "build") {
			t.Errorf("docker --help states the 11.6.1 floor without scoping it to build: %q", line)
		}
	}
	for _, want := range []string{
		"Mendix 10",     // check has no version floor
		"exit status 4", // what a version-mismatched update-widgets looks like
	} {
		if !strings.Contains(dockerCheckCmd.Long, want) {
			t.Errorf("docker check --help does not mention %q", want)
		}
	}
}

// ako/mxcli#568, #646: docker check's help said update-widgets ran first "to
// prevent false CE0463 errors" — the CE0463s it cleared were real, and failed
// the build of the stored project. The default now checks the project as
// stored; the help has to say so, name the flag that restores the old
// behaviour, and keep accepting --no-update-widgets.
func TestDockerCheckHelp_StoredProjectIsTheDefault(t *testing.T) {
	long := flat(dockerCheckCmd.Long)
	for _, want := range []string{
		"It checks the project as stored",
		"the exit status is the stored project's",
		"stale widget definition — `mxcli fix widgets` fixes it",
		"persists after update-widgets",
		"--update-widgets",
		"--no-update-widgets",
	} {
		if !strings.Contains(long, want) {
			t.Errorf("docker check --help does not say %q", want)
		}
	}
	if strings.Contains(long, "false CE0463") {
		t.Error("docker check --help still calls the CE0463s update-widgets clears false")
	}
	for _, name := range []string{"update-widgets", "no-update-widgets"} {
		if dockerCheckCmd.Flags().Lookup(name) == nil {
			t.Errorf("docker check has no --%s flag", name)
		}
	}
	// Both at once is a contradiction cobra refuses.
	cmd := dockerCheckCmd
	t.Cleanup(func() {
		for _, name := range []string{"update-widgets", "no-update-widgets"} {
			f := cmd.Flags().Lookup(name)
			f.Value.Set("false")
			f.Changed = false
		}
	})
	cmd.Flags().Set("update-widgets", "true")
	cmd.Flags().Set("no-update-widgets", "true")
	if err := cmd.ValidateFlagGroups(); err == nil {
		t.Error("docker check accepts --update-widgets together with --no-update-widgets")
	}
}
