// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// writeZip builds a zip from name→content pairs.
func writeZip(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pkg.mpk")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func zipEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		_ = rc.Close()
		out[f.Name] = buf.Bytes()
	}
	return out
}

// TestMakeImportable_LeavesANonModulePackageAlone — a widget package has no
// project.mpr. Returning it untouched keeps the diagnosis with mx rather than
// inventing one here.
func TestMakeImportable_LeavesANonModulePackageAlone(t *testing.T) {
	pkg := writeZip(t, map[string][]byte{"package.xml": []byte("<package/>")})

	got, err := makeImportable(pkg, t.TempDir())
	if err != nil {
		t.Fatalf("makeImportable: %v", err)
	}
	if got != pkg {
		t.Errorf("a package with no %s should be returned as-is; got %s", packageProjectEntry, got)
	}
}

// TestMakeImportable_UnpacksAwayFromTheScratchProject guards the trap that made
// the first working version silently do nothing.
//
// An .mpr is read as MPR v2 when an mprcontents/ directory sits beside it. The
// work directory holds the scratch project mx just created, mprcontents/
// included, so unpacking the package's v1 .mpr directly into it makes the reader
// resolve unit contents against the *scratch project's* files. Everything still
// "succeeds" and the flag flip reaches nothing.
func TestMakeImportable_UnpacksAwayFromTheScratchProject(t *testing.T) {
	workDir := t.TempDir()
	// Stand in for the scratch project mx creates in this directory.
	if err := os.MkdirAll(filepath.Join(workDir, "mprcontents", "ab", "cd"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := writeZip(t, map[string][]byte{packageProjectEntry: []byte("not a real sqlite file")})

	// The unpack must not land beside mprcontents/. A corrupt .mpr is fine here:
	// the assertion is about *where* the file goes, and the function must not
	// leave it in the work directory root either way.
	_, _ = makeImportable(pkg, workDir)

	if _, err := os.Stat(filepath.Join(workDir, packageProjectEntry)); err == nil {
		t.Errorf("the package's %s was unpacked beside the scratch project's mprcontents/, "+
			"where it reads as MPR v2 and resolves against the wrong files", packageProjectEntry)
	}
}

// TestReplaceZipEntry_CarriesEverythingElseAcross — the package handed to mx must
// differ from the published one in the module document and nothing else, or the
// reference project stops being a faithful copy of the release.
func TestReplaceZipEntry_CarriesEverythingElseAcross(t *testing.T) {
	src := writeZip(t, map[string][]byte{
		packageProjectEntry:    []byte("original model"),
		"package.xml":          []byte("<package/>"),
		"themesource/a/x.scss": []byte(".a{}"),
		"widgets/W.mpk":        []byte("widget bytes"),
	})
	replacement := filepath.Join(t.TempDir(), "new.mpr")
	if err := os.WriteFile(replacement, []byte("edited model"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "out.mpk")

	if err := replaceZipEntry(src, packageProjectEntry, replacement, dst); err != nil {
		t.Fatalf("replaceZipEntry: %v", err)
	}

	before, after := zipEntries(t, src), zipEntries(t, dst)
	if len(after) != len(before) {
		t.Fatalf("entry count changed: %d → %d", len(before), len(after))
	}
	for name, body := range before {
		got, ok := after[name]
		if !ok {
			t.Errorf("entry %q was dropped", name)
			continue
		}
		if name == packageProjectEntry {
			if string(got) != "edited model" {
				t.Errorf("%s was not replaced: %q", name, got)
			}
			continue
		}
		if !bytes.Equal(got, body) {
			t.Errorf("entry %q changed but should have been carried across byte-for-byte", name)
		}
	}
}
