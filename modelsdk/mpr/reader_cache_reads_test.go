// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"testing"
)

// mendixlabs/mxcli#1272: check -p re-read every unit file of a type from disk
// on each listing, so a pass that looks a flow up per statement cost
// statements × units file reads. CacheUnitReads holds the contents for the
// span of a pass that writes nothing.

const cacheTestUnit = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

// newListableV2Writer is newV2WriterForCommitTest with the Unit row's
// container columns filled in, which a listing reads.
func newListableV2Writer(t *testing.T, stored []byte) *Writer {
	t.Helper()
	w, _ := newV2WriterForCommitTest(t, cacheTestUnit, stored)
	if _, err := w.reader.db.Exec(`UPDATE Unit SET ContainerID = ?, ContainmentName = 'Folders'`,
		uuidToBlob("11111111-2222-3333-4444-555555555555")); err != nil {
		t.Fatalf("fill unit row: %v", err)
	}
	return w
}

func listTwice(t *testing.T, r *Reader) {
	t.Helper()
	for range 2 {
		units, err := r.ListUnitsByType("Projects$Folder")
		if err != nil || len(units) != 1 {
			t.Fatalf("list: %d units, %v", len(units), err)
		}
	}
}

func TestCacheUnitReads_ReadsEachFileOnce(t *testing.T) {
	w := newListableV2Writer(t, unitDoc(t, "Stored"))
	r := w.reader

	// Control: without the cache every listing reads the file again (the type
	// index reads it once more on the first listing).
	before := r.fileReads.Load()
	listTwice(t, r)
	if n := r.fileReads.Load() - before; n < 2 {
		t.Fatalf("control: two uncached listings read the file %d time(s), want >= 2", n)
	}

	r.InvalidateCache()
	release := r.CacheUnitReads()
	before = r.fileReads.Load()
	listTwice(t, r)
	if n := r.fileReads.Load() - before; n != 1 {
		t.Errorf("cached: two listings read the file %d time(s), want 1", n)
	}

	release()
	before = r.fileReads.Load()
	listTwice(t, r)
	if n := r.fileReads.Load() - before; n < 2 {
		t.Errorf("released: two listings read the file %d time(s), want >= 2 (the cache must not outlive the pass)", n)
	}
}

func TestCacheUnitReads_NestedReleaseKeepsOuterScope(t *testing.T) {
	w := newListableV2Writer(t, unitDoc(t, "Stored"))
	r := w.reader
	outer := r.CacheUnitReads()
	defer outer()
	r.CacheUnitReads()() // an inner pass ends inside the outer one

	before := r.fileReads.Load()
	listTwice(t, r)
	if n := r.fileReads.Load() - before; n > 1 {
		t.Errorf("an inner release ended the outer cache: %d reads, want <= 1", n)
	}
}

// A caller that edits the bytes it was handed must not change what the next
// reader sees: os.ReadFile gave every caller its own slice, and so must a hit.
func TestCacheUnitReads_CallerCannotCorruptCache(t *testing.T) {
	stored := unitDoc(t, "Stored")
	w := newListableV2Writer(t, stored)
	r := w.reader
	defer r.CacheUnitReads()()

	for range 2 {
		units, err := r.ListUnitsByType("Projects$Folder")
		if err != nil || len(units) != 1 {
			t.Fatalf("list: %v", err)
		}
		if !bytes.Equal(units[0].Contents, stored) {
			t.Fatalf("listing returned bytes that differ from the stored unit")
		}
		for i := range units[0].Contents {
			units[0].Contents[i] = 0
		}
	}
}

// A write while the cache is held is seen by the next read.
func TestCacheUnitReads_WriteIsSeen(t *testing.T) {
	w := newListableV2Writer(t, unitDoc(t, "Stored"))
	r := w.reader
	defer r.CacheUnitReads()()
	listTwice(t, r)

	written := unitDoc(t, "Written")
	if err := w.UpdateRawUnit(cacheTestUnit, written); err != nil {
		t.Fatalf("update: %v", err)
	}
	units, err := r.ListUnitsByType("Projects$Folder")
	if err != nil || len(units) != 1 {
		t.Fatalf("list: %v", err)
	}
	if !bytes.Equal(units[0].Contents, written) {
		t.Errorf("a read after a write returned the cached bytes from before it")
	}
}
