// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// ako/mxcli#2652: the holes #2644 closed for git merge, in marketplace package.

// An installed mx of the project's release is used before an mx on PATH.
// ResolveMxForVersion tries PATH first, so with 11.12.5 cached and 11.12.1 on
// PATH the package was refused although the right toolchain was installed.
func TestPackage_ExactInstallBeatsPath(t *testing.T) {
	f := &packageFake{modules: []string{"PbxExport"}, version: "11.12.5", mxVersion: "11.12.1",
		localMx: "/cache/mxbuild/11.12.5/modeler/mx", entries: pkgOwnEntries}
	installPackageFake(t, f)
	_, stderr, _, err := runPackageForTest(t, packageOptions{ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("refused although Mendix 11.12.5's mx is installed: %v", err)
	}
	if f.ranMx != f.localMx {
		t.Errorf("package made with %s, want the exact-version install %s", f.ranMx, f.localMx)
	}
	if strings.Contains(stderr, "warning") {
		t.Errorf("warned about an exact match:\n%s", stderr)
	}
}

// --mxbuild-path overrides the version check only for an mx that folder holds.
// A folder without one made ResolveMxForVersion fall back to PATH, and the
// override then exported with an mx nobody named, with only a warning.
func TestPackage_MxbuildPathWithoutAnMxIsAnError(t *testing.T) {
	f := &packageFake{modules: []string{"PbxExport"}, mxVersion: "11.12.1", resolveOutside: true, entries: pkgOwnEntries}
	installPackageFake(t, f)
	dir := t.TempDir()
	_, _, _, err := runPackageForTest(t, packageOptions{
		ProjectPath: pkgPlaceholderProject(t), Module: "PbxExport", OutputDir: t.TempDir(), MxBuildPath: dir})
	if err == nil || !strings.Contains(err.Error(), "no mx in "+dir) {
		t.Fatalf("want a refusal naming %s, got %v", dir, err)
	}
	if len(f.calls) != 0 {
		t.Error("mx ran although --mxbuild-path holds none")
	}
}
