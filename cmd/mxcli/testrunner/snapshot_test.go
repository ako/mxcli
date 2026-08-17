// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"os"
	"path/filepath"
	"testing"
)

// newProjectFixture writes a project file plus a document tree beside it.
func newProjectFixture(t *testing.T, mprBytes string, units map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(path, []byte(mprBytes), 0o644); err != nil {
		t.Fatal(err)
	}
	contents := filepath.Join(dir, "mprcontents")
	if err := os.MkdirAll(contents, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range units {
		if err := os.WriteFile(filepath.Join(contents, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestSnapshotRestoresTheProjectFileByte is the regression test for the dirty
// .mpr: a run that injects and removes leaves the model identical but the file
// different, because every unit write stamps the _Transaction row and SQLite
// relays its pages. Version control compares bytes, so the run shows up as a
// modification.
func TestSnapshotRestoresTheProjectFileByte(t *testing.T) {
	path := newProjectFixture(t, "original-bytes", map[string]string{"a.mxunit": "A"})
	snap := takeProjectSnapshot(path)

	// Stand in for the run: the file changes, the document tree ends up back
	// where it started.
	if err := os.WriteFile(path, []byte("churned-by-the-run"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := snap.restore(true); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := readFile(t, path); got != "original-bytes" {
		t.Errorf("project file = %q, want the original bytes", got)
	}
}

// TestSnapshotRefusesWhenCleanupFailed. Restoring over a project that is still
// modified would replace a visible, harmless discrepancy with an invisible,
// misleading one — the tree would read as clean while the model was not.
func TestSnapshotRefusesWhenCleanupFailed(t *testing.T) {
	path := newProjectFixture(t, "original-bytes", map[string]string{"a.mxunit": "A"})
	snap := takeProjectSnapshot(path)
	if err := os.WriteFile(path, []byte("still-has-MxTest-in-it"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := snap.restore(false); err != nil {
		t.Fatalf("restore returned an error rather than declining: %v", err)
	}
	if got := readFile(t, path); got != "still-has-MxTest-in-it" {
		t.Errorf("project file = %q, want it left alone after a failed cleanup", got)
	}
}

// TestSnapshotRefusesWhenTheDocumentTreeMoved. The .mpr indexes the documents
// beside it, so putting an old one back over a changed tree would leave the
// index pointing at documents that are no longer there.
func TestSnapshotRefusesWhenTheDocumentTreeMoved(t *testing.T) {
	path := newProjectFixture(t, "original-bytes", map[string]string{"a.mxunit": "A"})
	snap := takeProjectSnapshot(path)

	if err := os.WriteFile(path, []byte("churned"), 0o644); err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(filepath.Dir(path), "mprcontents", "leftover.mxunit")
	if err := os.WriteFile(leftover, []byte("a unit cleanup missed"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := snap.restore(true)
	if err == nil {
		t.Fatal("restore overwrote the .mpr while the document tree had changed")
	}
	if got := readFile(t, path); got != "churned" {
		t.Errorf("project file = %q, want it left alone", got)
	}
}

// TestDocumentTreeDigestNoticesEveryKindOfChange — content, name and removal all
// have to register, or the interlock above is decorative.
func TestDocumentTreeDigestNoticesEveryKindOfChange(t *testing.T) {
	path := newProjectFixture(t, "x", map[string]string{"a.mxunit": "A", "b.mxunit": "B"})
	contents := filepath.Join(filepath.Dir(path), "mprcontents")

	base, err := documentTreeDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	if base == "" {
		t.Fatal("a v2 project digested to the empty v1 value")
	}

	cases := []struct {
		name   string
		mutate func()
	}{
		{"content", func() { os.WriteFile(filepath.Join(contents, "a.mxunit"), []byte("CHANGED"), 0o644) }},
		{"rename", func() {
			os.Rename(filepath.Join(contents, "a.mxunit"), filepath.Join(contents, "renamed.mxunit"))
		}},
		{"removal", func() { os.Remove(filepath.Join(contents, "b.mxunit")) }},
		{"addition", func() { os.WriteFile(filepath.Join(contents, "c.mxunit"), []byte("C"), 0o644) }},
	}
	for _, c := range cases {
		fresh := newProjectFixture(t, "x", map[string]string{"a.mxunit": "A", "b.mxunit": "B"})
		contents = filepath.Join(filepath.Dir(fresh), "mprcontents")
		before, _ := documentTreeDigest(fresh)
		c.mutate()
		after, err := documentTreeDigest(fresh)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if after == before {
			t.Errorf("%s: the digest did not change", c.name)
		}
	}
}

// TestSnapshotOfAMissingProjectDeclines — a snapshot that could not be taken
// must do nothing rather than truncate the file it never read.
func TestSnapshotOfAMissingProjectDeclines(t *testing.T) {
	snap := takeProjectSnapshot(filepath.Join(t.TempDir(), "nope.mpr"))
	if err := snap.restore(true); err != nil {
		t.Errorf("an empty snapshot errored instead of declining: %v", err)
	}
}
