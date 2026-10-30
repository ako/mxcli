// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#972 item 1: `mxcli fix hashes` on a real MPR v2 project.

func copyPedApp(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skip("testdata/pedapp not available")
	}
	dst := t.TempDir()
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dst, "PedApp.mpr")
}

// revertOneUnit overwrites one .mxunit with different bytes, as `git checkout`
// of an older revision would, and returns its path.
func revertOneUnit(t *testing.T, mprPath string) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(mprPath), "mprcontents", "*", "*", "*.mxunit"))
	if len(files) == 0 {
		t.Fatal("no .mxunit files in the copy")
	}
	f := files[len(files)/2]
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	// Same content plus a trailing byte: different hash, same unit.
	if err := os.WriteFile(f, append(b, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFixHashes_UntouchedProjectIsClean(t *testing.T) {
	mprPath := copyPedApp(t)
	var out bytes.Buffer
	if err := fixHashes(mprPath, false, &out); err != nil {
		t.Fatalf("control: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "matches every unit file") {
		t.Fatalf("control not reported clean:\n%s", out.String())
	}
	if w := contentsHashDriftWarning(mprPath); w != "" {
		t.Fatalf("control warned: %s", w)
	}
}

func TestFixHashes_DetectsAndRepairsRevertedUnit(t *testing.T) {
	mprPath := copyPedApp(t)
	f := revertOneUnit(t, mprPath)
	id := strings.TrimSuffix(filepath.Base(f), ".mxunit")

	var out bytes.Buffer
	err := fixHashes(mprPath, false, &out)
	if !errors.Is(err, errHashIssuesRemain) {
		t.Fatalf("verify: want errHashIssuesRemain, got %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "MISMATCH  "+id) || !strings.Contains(out.String(), "1 mismatch(es)") {
		t.Fatalf("mismatch not reported:\n%s", out.String())
	}
	if w := contentsHashDriftWarning(mprPath); !strings.Contains(w, "1 unit(s)") || !strings.Contains(w, "mxcli fix hashes") {
		t.Fatalf("drift warning = %q", w)
	}

	out.Reset()
	if err := fixHashes(mprPath, true, &out); err != nil {
		t.Fatalf("repair: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Repaired 1 ContentsHash") {
		t.Fatalf("repair not reported:\n%s", out.String())
	}

	out.Reset()
	if err := fixHashes(mprPath, false, &out); err != nil {
		t.Fatalf("after repair: %v\n%s", err, out.String())
	}
	if w := contentsHashDriftWarning(mprPath); w != "" {
		t.Fatalf("warned after repair: %s", w)
	}
}

// ako/mxcli#1159: on MPR v1 the hash sits beside the contents in the same row,
// and every mxcli before the fix rewrote Contents without it. `fix hashes`
// compares against the Contents column there.
func TestFixHashes_V1ComparesAgainstContents(t *testing.T) {
	src := filepath.Join("..", "..", "modelsdk", "mpr", "testdata", "v1-project")
	dst := t.TempDir()
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	mprPath := filepath.Join(dst, "App.mpr")

	// Control: the Studio Pro-authored fixture is clean.
	var out bytes.Buffer
	if err := fixHashes(mprPath, false, &out); err != nil {
		t.Fatalf("v1 control: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "MPR v1") || !strings.Contains(out.String(), "matches its stored contents") {
		t.Fatalf("v1 control not reported clean:\n%s", out.String())
	}

	// What an mxcli before #1159 left behind: new Contents, old hash.
	db, err := sql.Open("sqlite", mprPath)
	if err != nil {
		t.Fatal(err)
	}
	var id []byte
	var contents []byte
	if err := db.QueryRow(`SELECT UnitID, Contents FROM Unit ORDER BY UnitID LIMIT 1`).Scan(&id, &contents); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE Unit SET Contents = ? WHERE UnitID = ?`, append(contents, 0), id); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	out.Reset()
	err = fixHashes(mprPath, false, &out)
	if !errors.Is(err, errHashIssuesRemain) {
		t.Fatalf("v1 verify: want errHashIssuesRemain, got %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "MISMATCH") || !strings.Contains(out.String(), "1 mismatch(es)") {
		t.Fatalf("v1 mismatch not reported:\n%s", out.String())
	}
	// exec and docker check stay silent on v1: the warning targets files
	// restored with git, and would read every blob of the .mpr on every run.
	if w := contentsHashDriftWarning(mprPath); w != "" {
		t.Fatalf("v1 warned: %s", w)
	}

	out.Reset()
	if err := fixHashes(mprPath, true, &out); err != nil {
		t.Fatalf("v1 repair: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Repaired 1 ContentsHash") {
		t.Fatalf("v1 repair not reported:\n%s", out.String())
	}
	out.Reset()
	if err := fixHashes(mprPath, false, &out); err != nil {
		t.Fatalf("v1 after repair: %v\n%s", err, out.String())
	}
}
