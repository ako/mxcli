// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMpk builds a minimal .mpk (zip) containing the given package.xml body.
func writeMpk(t *testing.T, packageXML string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.mpk")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	if packageXML != "" {
		w, err := zw.Create("package.xml")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(packageXML)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestModuleNameFromMpk_Module(t *testing.T) {
	const moduleXML = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.mendix.com/package/1.0/">
  <modelerProject xmlns="http://www.mendix.com/modelerProject/1.0/">
    <module name="DatabaseConnector" />
    <projectFile path="project.mpr" />
  </modelerProject>
</package>`
	name, err := moduleNameFromMpk(writeMpk(t, moduleXML))
	if err != nil {
		t.Fatal(err)
	}
	if name != "DatabaseConnector" {
		t.Errorf("name = %q, want DatabaseConnector", name)
	}
}

func TestModuleNameFromMpk_Widget(t *testing.T) {
	const widgetXML = `<?xml version="1.0" encoding="utf-8" ?>
<package xmlns="http://www.mendix.com/package/1.0/">
    <clientModule name="Badge" version="3.2.2" xmlns="http://www.mendix.com/clientModule/1.0/">
        <widgetFiles><widgetFile path="Badge.xml" /></widgetFiles>
    </clientModule>
</package>`
	name, err := moduleNameFromMpk(writeMpk(t, widgetXML))
	if err != nil {
		t.Fatal(err)
	}
	if name != "Badge" {
		t.Errorf("name = %q, want Badge", name)
	}
}

func TestModuleNameFromMpk_NoPackageXML(t *testing.T) {
	if _, err := moduleNameFromMpk(writeMpk(t, "")); err == nil {
		t.Fatal("expected error when package.xml is absent")
	}
}

// TestCheckStorageFormatPreserved_RefusesV2 is the guard for the silent MPR
// v2→v1 collapse.
//
// Still enforced, but no longer on the default path: a module install now copies
// units with mxcli's own writer and preserves the format, so this guards only
// the legacy --allow-format-change route through `mx module-import`.
//
// `mx module-import` rewrites a v2 project as v1: measured on a blank Mendix
// 11.12.1 app, one import turned a 69 KB .mpr plus 341 .mxunit files into a
// single 14 MB blob with no mprcontents/. That destroys the per-document files
// `mxcli diff-local` and git review depend on, it is one-way, and it happens to
// the user's real project. Refusing is the floor.
func TestCheckStorageFormatPreserved_RefusesV2(t *testing.T) {
	mpr := v2Project(t)

	err := checkStorageFormatPreserved(mpr, false)
	if err == nil {
		t.Fatal("importing into an MPR v2 project must be refused: the import would rewrite it as v1")
	}
	// The message has to name the consequence and the way out, or the user is
	// simply blocked with no path forward.
	for _, want := range []string{"MPR v2", "diff-local", "Studio Pro", "--allow-format-change"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should mention %q so the user can act on it; got:\n%s", want, err)
		}
	}
}

// TestCheckStorageFormatPreserved_AllowsOptIn — the conversion is a legitimate
// choice for a project that is not kept in git. It has to be chosen explicitly.
func TestCheckStorageFormatPreserved_AllowsOptIn(t *testing.T) {
	if err := checkStorageFormatPreserved(v2Project(t), true); err != nil {
		t.Errorf("--allow-format-change must permit the import; got: %v", err)
	}
}

// TestCheckStorageFormatPreserved_PassesV1 — a v1 project has no format to lose,
// so the guard must not block it. Without this the guard could be "refuse
// everything" and still pass the test above.
func TestCheckStorageFormatPreserved_PassesV1(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("not a real project"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkStorageFormatPreserved(mpr, false); err != nil {
		t.Errorf("an MPR v1 project has no mprcontents/ to lose and must import freely; got: %v", err)
	}
}

// v2Project builds the on-disk shape of an MPR v2 project: an .mpr beside an
// mprcontents/ tree. That directory is what the readers key on, so this is the
// same signal the guard reads.
func v2Project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("metadata"), 0o600); err != nil {
		t.Fatal(err)
	}
	unit := filepath.Join(dir, "mprcontents", "ab", "cd")
	if err := os.MkdirAll(unit, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unit, "abcd.mxunit"), []byte("bson"), 0o600); err != nil {
		t.Fatal(err)
	}
	return mpr
}
