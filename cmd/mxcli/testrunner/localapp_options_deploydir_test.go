// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"path/filepath"
	"strings"
	"testing"
)

// mxcli-ledger FINDINGS §150: `mxcli test --local` could not start a runtime for
// any project.
//
//	java.lang.IllegalArgumentException: Path '…/.mxcli/deployment-test/model/bundles'
//	cannot be resolved in base path '…/.mxcli/deployment-test'
//
// The tree had no model/ in it, because mxbuild writes the deployment to
// `<app dir>/deployment` and has no option to move it. Giving the test boot a
// deployment directory of its own moved where the RUNTIME reads and not where
// the BUILD writes.
//
// What this file used to assert is the lesson. Four tests checked that DeployDir
// was set, was under .mxcli/, was not the dev loop's, and differed per project —
// and all four passed against a build that could not start, because every one of
// them was about the option value rather than about what ends up on disk. The
// symptom lived one layer down, and nothing here looked there.
//
// So the option is gone and the invariant is asserted instead: the test boot
// must leave DeployDir at the build's own directory. The docker package refuses
// anything else outright (checkDeployDirIsBuildable), which is where the
// constraint belongs — it is mxbuild's, not the test runner's.

func TestLocalTestBoot_UsesTheDirectoryTheBuildWritesTo(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "App.mpr")
	opts := localAppOptions(RunOptions{ProjectPath: projectPath}, "", nil, nil)

	if opts.DeployDir != "" {
		t.Errorf("DeployDir = %q, want it unset so the boot uses %s — the only directory "+
			"mxbuild will populate", opts.DeployDir, filepath.Join(filepath.Dir(projectPath), "deployment"))
	}
}

// CONTROL: the separation that IS achievable must still hold. Sharing the
// deployment directory is forced by mxbuild; sharing the ports or the database
// is not, and a fix that gave up on those would trade one collision for another.
func TestLocalTestBoot_StillSeparatesPortsAndDatabase(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "App.mpr")
	opts := localAppOptions(RunOptions{ProjectPath: projectPath}, "", nil, nil)

	if opts.AppPort == 8080 || opts.AdminPort == 8090 {
		t.Errorf("the test boot took the dev loop's ports: app=%d admin=%d", opts.AppPort, opts.AdminPort)
	}
	if !strings.HasSuffix(opts.DB.Name, localTestDBSuffix) {
		t.Errorf("DB = %q, want the scratch database suffix %q", opts.DB.Name, localTestDBSuffix)
	}
}

// `--skip-build` means what it always meant — reuse the deployment the last
// build wrote — and must not be refused. It was, for as long as the test run was
// expected to have a tree of its own that no first run could have created.
func TestValidateOptions_SkipBuildIsAllowed(t *testing.T) {
	dir := t.TempDir()
	if err := validateOptions(RunOptions{
		ProjectPath: filepath.Join(dir, "App.mpr"),
		Local:       true,
		SkipBuild:   true,
	}); err != nil {
		t.Errorf("--skip-build reuses the project's deployment/ and must be allowed: %v", err)
	}
}

// CONTROL: the --skip-build refusals that are real are unaffected — they are
// about combinations that cannot work, not about what is on disk.
func TestValidateOptions_SkipBuildStillRefusedWithWatch(t *testing.T) {
	dir := t.TempDir()
	err := validateOptions(RunOptions{
		ProjectPath: filepath.Join(dir, "App.mpr"),
		Local:       true,
		SkipBuild:   true,
		Watch:       true,
	})
	if err == nil {
		t.Fatal("--watch with --skip-build must still be refused")
	}
	if !strings.Contains(err.Error(), "--watch") {
		t.Errorf("the message should name --watch: %v", err)
	}
}
