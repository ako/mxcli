// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	_ "modernc.org/sqlite"
)

// WriteTransaction.WriteUnit reconciles the new contents with the stored ones
// before writing. On MPR v1 the stored contents are in the database, and the
// reader's pool holds one connection (reader.go, SetMaxOpenConns(1)) — the one
// the open transaction owns. Reading them through the reader instead of the
// transaction waited for that connection forever, so codec.Store.SaveUnit and
// FlushUnits never returned on a v1 project. v2 reads the stored bytes from
// mprcontents/ and never touched the pool, which is why no v2 test saw it.

// newV1ProjectFile writes a minimal MPR v1 project — the Unit table with its
// Contents column, and the metadata the reader detects the version from — and
// returns its path. It is opened afterwards through NewWriter, the path every
// real write takes, so the pool is the real one-connection pool.
func newV1ProjectFile(t *testing.T, unitID string, stored []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.mpr")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE _MetaData (_ProductVersion TEXT, _BuildVersion TEXT, _SchemaHash TEXT)`,
		`INSERT INTO _MetaData VALUES ('9.24.30', '9.24.30.51827', '')`,
		`CREATE TABLE Unit (UnitID BLOB PRIMARY KEY NOT NULL, ContainerID BLOB, ContainmentName TEXT,
			TreeConflict LONG, ContentsHash TEXT, ContentsConflicts TEXT, Contents BLOB)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO Unit (UnitID, ContainerID, ContainmentName, Contents) VALUES (?, ?, ?, ?)`,
		uuidToBlob(unitID), uuidToBlob(v1UnitID(0)), "Documents", stored); err != nil {
		t.Fatal(err)
	}
	return path
}

func v1TestUnit(t *testing.T, name string) []byte {
	t.Helper()
	b, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: uuidToBlob(v1UnitID(1))}},
		{Key: "$Type", Value: "Test$Unit"},
		{Key: "Name", Value: name},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// writeInTransaction runs Begin → WriteUnit → Commit the way codec.Store.SaveUnit
// does, and fails the test if it does not return in time instead of hanging the
// whole package. On timeout the writer is deliberately left open: closing it
// would wait on the connection the stuck transaction holds.
func writeInTransaction(t *testing.T, w *Writer, unitID string, contents []byte) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		wt, err := w.BeginWriteTransaction()
		if err != nil {
			done <- err
			return
		}
		if err := wt.WriteUnit(unitID, contents); err != nil {
			_ = wt.Rollback()
			done <- err
			return
		}
		done <- wt.Commit()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("write in transaction: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("WriteTransaction.WriteUnit on an MPR v1 project did not return within 10s: " +
			"the reconcile read waits for the pool's only connection, which the transaction holds")
	}
}

func TestWriteTransactionWriteUnitV1DoesNotDeadlock(t *testing.T) {
	id := v1UnitID(1)
	path := newV1ProjectFile(t, id, v1TestUnit(t, "Before"))
	w, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}

	writeInTransaction(t, w, id, v1TestUnit(t, "After"))
	got, err := w.reader.GetRawUnitBytes(id)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("After")) {
		t.Fatalf("stored contents were not updated: %q", got)
	}

	// The unchanged write is elided, which takes the same reconcile read.
	writeInTransaction(t, w, id, v1TestUnit(t, "After"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}
