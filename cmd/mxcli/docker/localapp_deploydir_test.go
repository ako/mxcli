// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"path/filepath"
	"strings"
	"testing"
)

// mxcli-ledger FINDINGS §150: `mxcli test --local` could not start a runtime at
// all.
//
//	Error: local runtime: runtime admin API did not come up: runtime process
//	exited during startup
//	java.lang.IllegalArgumentException: Path '…/.mxcli/deployment-test/model/bundles'
//	cannot be resolved in base path '…/.mxcli/deployment-test'
//
// The tree was empty of everything that matters — no model/ at all — because
// **mxbuild's deploy target writes to `<app dir>/deployment` and takes no option
// to change it.** Measured directly, not inferred: `mxbuild --target=deploy` on a
// project whose deployment/ had just been deleted recreated it there, and
// `mxbuild --help` lists no deployment-path flag (`-o/--output` names the .mda
// for target=package). BuildRequest has no such field either — the serve API
// exposes target, project path, mda path and the loose version check.
//
// So DeployDir decides where the RUNTIME reads and nothing decides where the
// BUILD writes. Setting them apart pointed the runtime at a directory nothing
// populates. That is now refused with the reason instead of failing inside the
// JVM against a path the user never chose.
//
// The fix this replaces was tested only on the option value — four tests
// asserting DeployDir was set, under the project, per-project, and not the dev
// loop's — and every one passed against a build that could not start. The
// checklist item is "verified at the layer the symptom lives in": the symptom
// lives in what is on disk after mxbuild runs.

func TestDeployDirMustBeWhereTheBuildWrites(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "App.mpr")

	err := checkDeployDirIsBuildable(LocalAppOptions{
		ProjectPath: project,
		DeployDir:   filepath.Join(dir, ".mxcli", "deployment-test"),
	})
	if err == nil {
		t.Fatal("a DeployDir the build cannot write to must be refused, not booted against")
	}
	// The message has to name mxbuild as the constraint, or the reader goes
	// looking for the bug in mxcli's own path handling.
	for _, want := range []string{"mxbuild", "deployment-test", filepath.Join(dir, "deployment")} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the message should name %q: %v", want, err)
		}
	}
}

// CONTROL 1: the directory mxbuild actually writes to is accepted. Without this
// the guard would refuse every local boot.
func TestDeployDirDefaultIsAccepted(t *testing.T) {
	dir := t.TempDir()
	opts := LocalAppOptions{ProjectPath: filepath.Join(dir, "App.mpr")}
	opts.applyDefaults()

	if err := checkDeployDirIsBuildable(opts); err != nil {
		t.Errorf("the default deployment directory must be buildable: %v", err)
	}
}

// CONTROL 2: with SkipBuild there is no build to disagree with, so any tree the
// caller already populated is theirs to boot against. A guard that fired here
// would forbid a legitimate use.
func TestDeployDirIsFreeWhenNothingIsBuilt(t *testing.T) {
	dir := t.TempDir()
	if err := checkDeployDirIsBuildable(LocalAppOptions{
		ProjectPath: filepath.Join(dir, "App.mpr"),
		DeployDir:   filepath.Join(dir, "somewhere-else"),
		SkipBuild:   true,
	}); err != nil {
		t.Errorf("--skip-build boots against whatever the caller points at: %v", err)
	}
}

// An unset DeployDir is the default and must pass before applyDefaults has run —
// the guard is called from StartLocalApp after defaults, but a caller reading
// this must not have to know the order.
func TestDeployDirUnsetIsAccepted(t *testing.T) {
	dir := t.TempDir()
	if err := checkDeployDirIsBuildable(LocalAppOptions{ProjectPath: filepath.Join(dir, "App.mpr")}); err != nil {
		t.Errorf("an unset DeployDir is the build's own directory: %v", err)
	}
}
