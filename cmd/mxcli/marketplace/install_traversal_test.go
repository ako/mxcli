package marketplace

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func buildMPK(t *testing.T, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "probe.mpk")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInstallPackageFilesRefusesPathTraversal(t *testing.T) {
	for _, name := range []string{
		"../../evil.txt",
		"../evil.txt",
		"a/../../evil.txt",
		"/tmp/mxcli-zipslip-evil.txt",
		"..\\..\\evil.txt",
		"./../../evil.txt",
		"a/b/../../../evil.txt",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			proj := filepath.Join(root, "project")
			if err := os.MkdirAll(proj, 0o755); err != nil {
				t.Fatal(err)
			}
			mpk := buildMPK(t, map[string]string{name: "PWNED"})
			_, _, err := InstallPackageFiles(mpk, proj)
			t.Logf("err = %v", err)

			// Did anything land outside the project dir?
			var escaped []string
			_ = filepath.Walk(root, func(p string, fi os.FileInfo, e error) error {
				if e != nil || fi.IsDir() {
					return nil
				}
				rel, _ := filepath.Rel(proj, p)
				if len(rel) >= 2 && rel[:2] == ".." {
					escaped = append(escaped, p)
				}
				return nil
			})
			if _, e := os.Stat("/tmp/mxcli-zipslip-evil.txt"); e == nil {
				escaped = append(escaped, "/tmp/mxcli-zipslip-evil.txt")
				_ = os.Remove("/tmp/mxcli-zipslip-evil.txt")
			}
			if len(escaped) > 0 {
				t.Errorf("ESCAPED: %v", escaped)
			}
		})
	}

	// Control: a legitimate entry must actually be written, so a test that
	// "nothing escaped" is not passing because nothing was extracted at all.
	t.Run("control/legit entry is written", func(t *testing.T) {
		proj := t.TempDir()
		mpk := buildMPK(t, map[string]string{"themesource/x/web/main.scss": "OK"})
		written, _, err := InstallPackageFiles(mpk, proj)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(written) == 0 {
			t.Fatal("control failed: no files written, so the escape assertions prove nothing")
		}
		b, err := os.ReadFile(filepath.Join(proj, "themesource/x/web/main.scss"))
		if err != nil || string(b) != "OK" {
			t.Fatalf("control failed: %v %q", err, b)
		}
		t.Logf("control OK: wrote %v", written)
	})
}
