// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// upstream #954, legacy engine. This copy of WriteTransaction has no callers
// today — the legacy write path is Writer.updateUnit, which has always failed
// hard when it could not put a unit's bytes on disk — but it is exported, so it
// is kept in step with modelsdk/mpr's copy rather than left holding the bug.
// These mirror the modelsdk tests against it.

// newV2WriterForCommitTest builds a minimal MPR v2 writer over a temp SQLite DB
// and mprcontents folder, seeded with one unit holding stored.
func newV2WriterForCommitTest(t *testing.T, unitID string, stored []byte) (*Writer, string) {
	t.Helper()

	root := t.TempDir()
	dbPath := filepath.Join(root, "app.mpr")
	contentsDir := filepath.Join(root, "mprcontents")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE Unit (
			UnitID BLOB PRIMARY KEY NOT NULL,
			ContainerID BLOB,
			ContainmentName TEXT,
			TreeConflict LONG,
			ContentsHash TEXT,
			ContentsConflicts TEXT
		)
	`); err != nil {
		t.Fatalf("create Unit table: %v", err)
	}

	blob := uuidToBlob(unitID)
	swapped := blobToUUIDSwapped(blob)
	unitPath := filepath.Join(contentsDir, swapped[0:2], swapped[2:4], swapped+".mxunit")
	if err := os.MkdirAll(filepath.Dir(unitPath), 0755); err != nil {
		t.Fatalf("mkdir unit dir: %v", err)
	}
	if err := os.WriteFile(unitPath, stored, 0644); err != nil {
		t.Fatalf("seed unit file: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO Unit (UnitID, ContentsHash) VALUES (?, ?)`, blob, contentHashBase64(stored),
	); err != nil {
		t.Fatalf("insert unit row: %v", err)
	}

	reader := &Reader{path: dbPath, db: db, version: MPRVersionV2, contentsDir: contentsDir}
	return &Writer{reader: reader}, unitPath
}

func commitTestStoredHash(t *testing.T, w *Writer, unitID string) string {
	t.Helper()
	var got string
	if err := w.reader.db.QueryRow(
		`SELECT ContentsHash FROM Unit WHERE UnitID = ?`, uuidToBlob(unitID),
	).Scan(&got); err != nil {
		t.Fatalf("read ContentsHash: %v", err)
	}
	return got
}

func TestCommitFailsWhenAUnitFileCannotBeFinalized(t *testing.T) {
	const unitID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	stored := []byte("stored bytes")
	updated := []byte("updated bytes")

	w, unitPath := newV2WriterForCommitTest(t, unitID, stored)

	wt, err := w.BeginWriteTransaction()
	if err != nil {
		t.Fatalf("begin write transaction: %v", err)
	}
	if err := wt.WriteUnit(unitID, updated); err != nil {
		t.Fatalf("write unit: %v", err)
	}

	// Stand-in for the Windows lock: make the rename fail. What fails it is not
	// the point, only that it can.
	if err := os.Remove(wt.pendingFiles[0].tempPath); err != nil {
		t.Fatalf("remove temp file: %v", err)
	}

	if err := wt.Commit(); err == nil {
		t.Fatal("Commit returned nil after failing to finalize a unit file")
	} else if !strings.Contains(err.Error(), unitID) {
		t.Errorf("Commit error %q does not name the unit %s", err, unitID)
	}

	onDisk, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit file: %v", err)
	}
	if string(onDisk) != string(stored) {
		t.Error("unit file changed despite the failed commit")
	}
	if got, want := commitTestStoredHash(t, w, unitID), contentHashBase64(stored); got != want {
		t.Errorf("ContentsHash = %q, want %q: the database describes contents "+
			"that are not on disk", got, want)
	}
}

// TestCommitSucceedsWhenEveryUnitFileIsFinalized is the false-positive control.
func TestCommitSucceedsWhenEveryUnitFileIsFinalized(t *testing.T) {
	const unitID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeef"
	stored := []byte("stored bytes")
	updated := []byte("updated bytes")

	w, unitPath := newV2WriterForCommitTest(t, unitID, stored)

	wt, err := w.BeginWriteTransaction()
	if err != nil {
		t.Fatalf("begin write transaction: %v", err)
	}
	if err := wt.WriteUnit(unitID, updated); err != nil {
		t.Fatalf("write unit: %v", err)
	}
	if err := wt.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	onDisk, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit file: %v", err)
	}
	if string(onDisk) != string(updated) {
		t.Error("unit file was not updated on the success path")
	}
	if got, want := commitTestStoredHash(t, w, unitID), contentHashBase64(updated); got != want {
		t.Errorf("ContentsHash = %q, want %q", got, want)
	}

	// No .tmp and no .bak: a successful commit leaves the folder as Mendix
	// expects to find it.
	entries, err := os.ReadDir(filepath.Dir(unitPath))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".mxunit") {
			t.Errorf("stray file left behind: %s", e.Name())
		}
	}
}
