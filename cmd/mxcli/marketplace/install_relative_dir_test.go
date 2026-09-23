// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInstallPackageFiles_RelativeProjectDir: the containment guard must not
// refuse a legitimate entry just because the caller named the project by a
// relative path. `mxcli marketplace install ... -p app.mpr` hands this function
// filepath.Dir("app.mpr") == ".", and filepath.Join(".", "manifest.json") drops
// the dot — so the joined path "manifest.json" was compared against the prefix
// "./" and refused as "would write outside the project". Found on a real
// install (2026-09-20): the module had already been transplanted into the model
// when the bundled-file step failed, so the command reported failure over a
// half-finished install.
func TestInstallPackageFiles_RelativeProjectDir(t *testing.T) {
	mpk := buildMPK(t, map[string]string{
		"manifest.json":                   `{"name":"probe"}`,
		"themesource/probe/web/main.scss": "// probe",
	})
	proj := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	written, _, err := InstallPackageFiles(mpk, ".")
	if err != nil {
		t.Fatalf("relative project dir refused a legitimate entry: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("written = %v, want both entries", written)
	}
	for _, rel := range []string{"manifest.json", filepath.Join("themesource", "probe", "web", "main.scss")} {
		if _, err := os.Stat(filepath.Join(proj, rel)); err != nil {
			t.Errorf("%s not written under the project: %v", rel, err)
		}
	}

	// The guard itself must still hold with a relative dir.
	bad := buildMPK(t, map[string]string{"../escape.txt": "x"})
	if _, _, err := InstallPackageFiles(bad, "."); err == nil {
		t.Fatalf("a traversal entry must still be refused with a relative project dir")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(proj), "escape.txt")); err == nil {
		t.Fatalf("traversal entry escaped the project")
	}
}
