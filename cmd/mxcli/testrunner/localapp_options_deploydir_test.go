// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mxcli-formula1 FINDINGS §62: `mxcli test` blanked the running app.
//
// Both apps went black while answering HTTP 200 — 1,718 bytes, Mendix's SPA
// shell. The runtime was fine; what it served the shell FOR was gone:
//
//	ERROR - Connector: 404 - file not found for file: dist%2Findex.js
//	$ ls deployment/web/dist
//	ls: cannot access 'deployment/web/dist': No such file or directory
//
// `deployment/web/` had been rewritten by something that was not the app. The
// culprit reproduces on demand in twenty seconds:
//
//	before:            deployment/web/dist
//	after mxcli test:  MISSING
//	bundle now:        404
//
// `mxcli test --local` rebuilt the deployment directory of a project another
// `mxcli run` was serving, and its headless build does not bundle the web
// client. The tests pass, the run keeps running, and the app goes blank —
// nothing reports an error at either end.
//
// The design intent was already written down, one line above the constants this
// test is about: a local test run "deliberately does not share the dev loop's
// ports or database … a test run must neither refuse to start because of it nor
// write its fixtures into the database the developer is looking at." The
// DEPLOYMENT DIRECTORY was the one shared resource left off that list, and it is
// the one the browser reads from.
//
// Detection was not the fix. The two processes use different ports by design, so
// no port check can see the collision, and a lock file would only turn a silent
// blanking into a refusal. Giving the test boot its own deployment directory
// makes the collision impossible instead.

func TestLocalTestBoot_DoesNotShareTheDevLoopDeployment(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "App.mpr")
	opts := localAppOptions(RunOptions{ProjectPath: projectPath}, "", nil, nil)

	devLoop := filepath.Join(filepath.Dir(projectPath), "deployment")
	if opts.DeployDir == "" {
		t.Fatal("DeployDir is unset, so the boot defaults to the dev loop's deployment/ — " +
			"the directory `mxcli run --local` is serving the browser bundle from")
	}
	if opts.DeployDir == devLoop {
		t.Errorf("the test boot builds into %s, which is the running app's deployment directory",
			opts.DeployDir)
	}
}

// It must also be inside the project, and inside the directory mxcli already
// owns and gitignores — a scratch build tree at the project root would show up
// as untracked files on the developer's next `git status`.
func TestLocalTestBoot_DeploymentLivesUnderTheMxcliDir(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "App.mpr")
	opts := localAppOptions(RunOptions{ProjectPath: projectPath}, "", nil, nil)

	want := filepath.Join(filepath.Dir(projectPath), ".mxcli")
	if !strings.HasPrefix(opts.DeployDir, want+string(filepath.Separator)) {
		t.Errorf("DeployDir = %s, want it under %s (gitignored, and already where the "+
			"test runtime log lives)", opts.DeployDir, want)
	}
}

// CONTROL: the separation the finding's comment already claimed must still hold.
// A fix that moved the deployment directory but collapsed the database or the
// ports would trade one collision for another.
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

// Two projects side by side must not share the scratch tree either — the F1
// solution runs a backend and a frontend from one repository, which is how this
// class of collision was found in the first place.
func TestLocalTestBoot_DeploymentIsPerProject(t *testing.T) {
	a := localAppOptions(RunOptions{ProjectPath: filepath.Join(t.TempDir(), "Backend", "App.mpr")}, "", nil, nil)
	b := localAppOptions(RunOptions{ProjectPath: filepath.Join(t.TempDir(), "Frontend", "App.mpr")}, "", nil, nil)

	if a.DeployDir == b.DeployDir {
		t.Errorf("both projects build into %s", a.DeployDir)
	}
}

// `--skip-build` used to mean "reuse the deployment/ tree the dev loop built".
// It now reuses the test run's OWN tree, which does not exist until tests have
// been run once — so the first such run must say that, rather than failing
// somewhere inside the runtime boot with a path nobody chose.
//
// Reusing the dev loop's directory is not the alternative: booting a runtime
// against it removes the browser bundle even without a rebuild, which is the
// same §62 blanking by a different route (and was FINDINGS §35).
func TestValidateOptions_SkipBuildWithNoScratchDeployment(t *testing.T) {
	dir := t.TempDir()
	err := validateOptions(RunOptions{
		ProjectPath: filepath.Join(dir, "App.mpr"),
		Local:       true,
		SkipBuild:   true,
	})
	if err == nil {
		t.Fatal("--skip-build with no previous local test run must be refused, not attempted")
	}
	for _, want := range []string{"--skip-build", localTestDeployDir} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message should name %q: %v", want, err)
		}
	}
}

// CONTROL 1: once the scratch tree exists, --skip-build is allowed again.
func TestValidateOptions_SkipBuildWithAScratchDeployment(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "App.mpr")
	deploy := filepath.Join(dir, ".mxcli", localTestDeployDir, "model")
	if err := os.MkdirAll(deploy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateOptions(RunOptions{ProjectPath: project, Local: true, SkipBuild: true}); err != nil {
		t.Errorf("an existing scratch deployment must be reusable: %v", err)
	}
}

// CONTROL 2: without --skip-build there is nothing to check — the build creates
// the tree. A guard that fired here would refuse every first test run.
func TestValidateOptions_NoSkipBuildNeedsNothingOnDisk(t *testing.T) {
	dir := t.TempDir()
	if err := validateOptions(RunOptions{ProjectPath: filepath.Join(dir, "App.mpr"), Local: true}); err != nil {
		t.Errorf("an ordinary local run must not require an existing deployment: %v", err)
	}
}
