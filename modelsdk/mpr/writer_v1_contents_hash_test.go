// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1159: on an MPR v1 project (unit contents inside the .mpr, Mendix
// below 10.18) every unit write replaced Unit.Contents and left
// Unit.ContentsHash describing the old bytes; an insert stored ''. Studio Pro
// fills the column on every unit, base64(SHA-256(Contents)) — measured on a
// fresh 9.24.30 app (308 of 308 units) and on testdata/v1-project — and uses
// it to detect what changed. mx check and MxBuild do not read it, so nothing
// but this check notices.

// copyV1Fixture copies the Studio Pro-authored v1 fixture into a temp dir and
// returns the copy's path.
func copyV1Fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "v1-project", "App.mpr"))
	if err != nil {
		t.Fatalf("read v1 fixture: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "App.mpr")
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dst
}

// staleV1Hashes is the reporter's check: every Unit row whose ContentsHash is
// not base64(SHA-256(Contents)).
func staleV1Hashes(t *testing.T, w *Writer) []string {
	t.Helper()
	rows, err := w.reader.db.Query(`SELECT UnitID, COALESCE(ContentsHash, ''), Contents FROM Unit`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var stale []string
	for rows.Next() {
		var id, contents []byte
		var hash string
		if err := rows.Scan(&id, &hash, &contents); err != nil {
			t.Fatal(err)
		}
		if hash != hashOf(contents) {
			stale = append(stale, blobToUUID(id))
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return stale
}

// v1UnitOfType returns the ID and contents of the first unit of typeName.
func v1UnitOfType(t *testing.T, w *Writer, typeName string) (string, []byte) {
	t.Helper()
	rows, err := w.reader.db.Query(`SELECT UnitID, Contents FROM Unit`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, contents []byte
		if err := rows.Scan(&id, &contents); err != nil {
			t.Fatal(err)
		}
		if getTypeFromContents(contents) == typeName {
			return blobToUUID(id), contents
		}
	}
	t.Fatalf("no %s unit in the fixture", typeName)
	return "", nil
}

// withFields returns doc with the given top-level fields replaced.
func withFields(t *testing.T, doc []byte, fields map[string]any) []byte {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(doc, &d); err != nil {
		t.Fatal(err)
	}
	for i := range d {
		if v, ok := fields[d[i].Key]; ok {
			d[i].Value = v
		}
	}
	out, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func openV1FixtureWriter(t *testing.T) *Writer {
	t.Helper()
	w, err := NewWriter(copyV1Fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	if w.reader.version != MPRVersionV1 {
		t.Fatalf("fixture opened as MPR v%d, want v1", w.reader.version)
	}
	return w
}

// TestV1FixtureHashesMatchContents is the control: what Studio Pro stores is
// exactly the formula the writer must reproduce. If this fails, the formula
// is wrong, not the writer.
func TestV1FixtureHashesMatchContents(t *testing.T) {
	w := openV1FixtureWriter(t)
	if stale := staleV1Hashes(t, w); len(stale) != 0 {
		t.Fatalf("Studio Pro-authored fixture has %d unit(s) whose ContentsHash is not base64(SHA-256(Contents)): %v", len(stale), stale)
	}
}

func TestV1UpdateUnitUpdatesContentsHash(t *testing.T) {
	w := openV1FixtureWriter(t)
	id, stored := v1UnitOfType(t, w, "Projects$Folder")
	updated := withFields(t, stored, map[string]any{"Name": "RenamedBy1159"})

	if err := w.UpdateRawUnit(id, updated); err != nil {
		t.Fatalf("UpdateRawUnit: %v", err)
	}
	got, err := w.reader.GetRawUnitBytes(id)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(got, stored) {
		t.Fatal("update did not land: Contents unchanged, so the test proves nothing")
	}
	if stale := staleV1Hashes(t, w); len(stale) != 0 {
		t.Fatalf("after an update, ContentsHash is stale for %v (stored %q, want %q)",
			stale, storedHash(t, w, id), hashOf(got))
	}
}

// Through the real one-connection pool: the transaction now reads the stored
// bytes through itself (writer_v1_transaction_test.go), so it no longer needs a
// pool of its own to avoid waiting on the connection it holds.
func TestV1WriteTransactionUpdatesContentsHash(t *testing.T) {
	w := openV1FixtureWriter(t)
	id, stored := v1UnitOfType(t, w, "Projects$Folder")
	updated := withFields(t, stored, map[string]any{"Name": "RenamedInTransaction"})

	wt, err := w.BeginWriteTransaction()
	if err != nil {
		t.Fatal(err)
	}
	if err := wt.WriteUnit(id, updated); err != nil {
		_ = wt.Rollback()
		t.Fatalf("WriteUnit: %v", err)
	}
	if err := wt.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	got, err := w.reader.GetRawUnitBytes(id)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(got, stored) {
		t.Fatal("write did not land: Contents unchanged, so the test proves nothing")
	}
	if stale := staleV1Hashes(t, w); len(stale) != 0 {
		t.Fatalf("after a transactional write, ContentsHash is stale for %v (stored %q, want %q)",
			stale, storedHash(t, w, id), hashOf(got))
	}
}

func TestV1InsertUnitStoresContentsHash(t *testing.T) {
	w := openV1FixtureWriter(t)
	moduleID, _ := v1UnitOfType(t, w, "Projects$ModuleImpl")
	_, folder := v1UnitOfType(t, w, "Projects$Folder")

	newID := generateUUID()
	contents := withFields(t, folder, map[string]any{
		"$ID":  bson.Binary{Subtype: 0x00, Data: uuidToBlob(newID)},
		"Name": "InsertedBy1159",
	})
	if err := w.InsertUnit(newID, moduleID, "Folders", "Projects$Folder", contents); err != nil {
		t.Fatalf("InsertUnit: %v", err)
	}
	got, err := w.reader.GetRawUnitBytes(newID)
	if err != nil {
		t.Fatalf("inserted unit not readable: %v", err)
	}
	if h := storedHash(t, w, newID); h != hashOf(got) {
		t.Fatalf("inserted unit's ContentsHash = %q, want %q", h, hashOf(got))
	}
	if stale := staleV1Hashes(t, w); len(stale) != 0 {
		t.Fatalf("after an insert, ContentsHash is stale for %v", stale)
	}
}
