// SPDX-License-Identifier: Apache-2.0

package constantstore

import (
	"os"
	"path/filepath"
	"testing"
)

func project(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "App.mpr")
}

// A missing store is the normal case: most projects set nothing on the machine.
func TestLoad_MissingFileIsNotAnError(t *testing.T) {
	s, err := Load(project(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Constants) != 0 {
		t.Errorf("Constants = %v, want empty", s.Constants)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	p := project(t)
	if err := Save(p, &Store{Constants: map[string]string{"A.Key": "sk-123"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Constants["A.Key"] != "sk-123" {
		t.Errorf("Constants = %v", got.Constants)
	}
	if got.Version != currentVersion {
		t.Errorf("Version = %d, want %d", got.Version, currentVersion)
	}
}

// The store's whole reason for existing is holding values that must not be
// readable by anyone else on the machine.
func TestSave_Mode0600(t *testing.T) {
	p := project(t)
	if err := Save(p, &Store{Constants: map[string]string{"A.Key": "v"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(Path(p))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600 — the file can hold an API key", perm)
	}
	// And no temp file left behind carrying the same secret at another mode.
	if _, err := os.Stat(Path(p) + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("the temp file survived the write: %v", err)
	}
}

// Unsetting the last value should leave no file claiming to configure
// something. A store of {} is not the same as no store when someone reads the
// directory to see whether this machine overrides anything.
func TestSave_RemovesAnEmptyStore(t *testing.T) {
	p := project(t)
	if err := Save(p, &Store{Constants: map[string]string{"A.Key": "v"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Save(p, &Store{Constants: map[string]string{}}); err != nil {
		t.Fatalf("Save empty: %v", err)
	}
	if _, err := os.Stat(Path(p)); !os.IsNotExist(err) {
		t.Errorf("an empty store was left on disk: %v", err)
	}
	// ...and removing it again is not an error.
	if err := Save(p, &Store{Constants: map[string]string{}}); err != nil {
		t.Errorf("Save on an already-absent store: %v", err)
	}
}

// A corrupt store must be NAMED, not treated as empty: silently booting with
// different values than the author set is the failure this whole feature
// exists to remove.
func TestLoad_CorruptFileIsAnError(t *testing.T) {
	p := project(t)
	if err := os.MkdirAll(filepath.Dir(Path(p)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(p), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("a corrupt store loaded as if it were empty")
	}
}

// A store written by a newer mxcli may mean something this build would
// misread — refuse rather than apply half of it.
func TestLoad_RefusesANewerVersion(t *testing.T) {
	p := project(t)
	if err := os.MkdirAll(filepath.Dir(Path(p)), 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"version": 99, "constants": {"A.Key": "v"}}`)
	if err := os.WriteFile(Path(p), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("a store from a newer schema was accepted")
	}
}

func TestPath_LivesBesideTheProject(t *testing.T) {
	p := "/some/where/App.mpr"
	if want := "/some/where/.mxcli/" + FileName; Path(p) != want {
		t.Errorf("Path = %q, want %q", Path(p), want)
	}
}
