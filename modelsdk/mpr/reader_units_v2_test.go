// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// mendixlabs/mxcli#1603: on MPR v2 the first listing of a process opened every
// unit file in mprcontents/ to learn its $Type, and threw the bytes away —
// `describe microflow` read 393 of a blank app's 394 files — and every write
// made the next listing do it again. The type index is keyed by the Unit
// table's ContentsHash, so only a unit whose hash it has not seen is opened,
// and it is persisted under .mxcli/ so the next process starts warm.

// v2Project is a minimal MPR v2 project on disk: a Unit table plus one
// .mxunit file per unit, laid out as Studio Pro lays them out.
type v2Project struct {
	root, mprPath, contentsDir string
	db                         *sql.DB
}

func newV2Project(t *testing.T) *v2Project {
	t.Helper()
	root := t.TempDir()
	p := &v2Project{root: root, mprPath: filepath.Join(root, "app.mpr"), contentsDir: filepath.Join(root, "mprcontents")}
	db, err := sql.Open("sqlite", p.mprPath)
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
	p.db = db
	return p
}

// add stores a unit the way the reader finds it: the file under the path
// readMprContents builds from the cache's ID, the row with its ContentsHash.
func (p *v2Project) add(t *testing.T, i int, typ string) string {
	t.Helper()
	id := fmt.Sprintf("%08x-0000-0000-0000-000000000000", i+1)
	blob := uuidToBlob(id)
	doc := typedDoc(t, typ, fmt.Sprintf("Unit%d", i))
	path := p.unitPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, doc, 0o644); err != nil {
		t.Fatalf("write unit: %v", err)
	}
	if _, err := p.db.Exec(`INSERT INTO Unit (UnitID, ContainerID, ContainmentName, ContentsHash) VALUES (?, ?, 'Documents', ?)`,
		blob, uuidToBlob("11111111-2222-3333-4444-555555555555"), hashOf(doc)); err != nil {
		t.Fatalf("insert unit row: %v", err)
	}
	return id
}

// unitPath is where readMprContents looks for the unit with this ID.
func (p *v2Project) unitPath(id string) string {
	cacheID := blobToUUID(uuidToBlob(id))
	return filepath.Join(p.contentsDir, cacheID[0:2], cacheID[2:4], cacheID+".mxunit")
}

// rewrite replaces a unit's bytes and hash, as a Studio Pro save would.
func (p *v2Project) rewrite(t *testing.T, id string, doc []byte) {
	t.Helper()
	if err := os.WriteFile(p.unitPath(id), doc, 0o644); err != nil {
		t.Fatalf("rewrite unit: %v", err)
	}
	if _, err := p.db.Exec(`UPDATE Unit SET ContentsHash = ? WHERE UnitID = ?`, hashOf(doc), uuidToBlob(id)); err != nil {
		t.Fatalf("update hash: %v", err)
	}
}

// reader opens the project as a fresh process would: nothing carried over
// from any earlier reader except what is on disk.
func (p *v2Project) reader() *Reader {
	return &Reader{path: p.mprPath, db: p.db, version: MPRVersionV2, contentsDir: p.contentsDir}
}

func typedDoc(t *testing.T, typ, name string) []byte {
	t.Helper()
	b, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob("11111111-1111-1111-1111-111111111111")}},
		{Key: "$Type", Value: typ},
		{Key: "Name", Value: name},
	})
	if err != nil {
		t.Fatalf("marshal unit: %v", err)
	}
	return b
}

// seedProject is a project of n microflows and one module: the shape of the
// report, where listing the modules opened every microflow too.
func seedProject(t *testing.T, n int) (*v2Project, string) {
	t.Helper()
	p := newV2Project(t)
	module := p.add(t, 0, "Projects$ModuleImpl")
	for i := 1; i <= n; i++ {
		p.add(t, i, "Microflows$Microflow")
	}
	return p, module
}

func listModules(t *testing.T, r *Reader) []UnitRef {
	t.Helper()
	units, err := r.ListUnitsByType("Projects$ModuleImpl")
	if err != nil {
		t.Fatalf("list modules: %v", err)
	}
	return units
}

// The reported symptom: a second process listing the one module opened every
// unit file. It must open only the module's.
func TestListUnitsV2_SecondProcessOpensOnlyTheMatches(t *testing.T) {
	const n = 40
	p, _ := seedProject(t, n)

	first := p.reader()
	if got := len(listModules(t, first)); got != 1 {
		t.Fatalf("first reader listed %d modules, want 1", got)
	}
	// The cold reader has no choice: it has never seen these hashes.
	if got := first.UnitFileReads(); got < n {
		t.Fatalf("control: the cold listing read %d files, want >= %d (the fixture does not exercise the index)", got, n)
	}

	second := p.reader()
	if got := len(listModules(t, second)); got != 1 {
		t.Fatalf("second reader listed %d modules, want 1", got)
	}
	if got := second.UnitFileReads(); got != 1 {
		t.Errorf("a listing of one module in a fresh process read %d unit files, want 1 (#1603: every unit was opened to learn its $Type)", got)
	}
}

// After a write the next listing re-types only the unit that changed.
func TestListUnitsV2_RebuildAfterWriteOpensOnlyTheChange(t *testing.T) {
	const n = 40
	p, _ := seedProject(t, n)
	r := p.reader()
	listModules(t, r)

	p.add(t, n+1, "Microflows$Microflow") // a new unit
	r.InvalidateCache()
	before := r.UnitFileReads()
	listModules(t, r)
	if got := r.UnitFileReads() - before; got != 2 {
		t.Errorf("a listing after one insert read %d unit files, want 2 (the new unit to type it, the module to return it)", got)
	}
}

// The index is keyed by content, not by unit: a unit whose bytes change type
// — mxcli re-types elements in place (#503) — is listed under its new type.
func TestListUnitsV2_RetypedUnitIsSeenUnderItsNewType(t *testing.T) {
	p, _ := seedProject(t, 3)
	r := p.reader()
	listModules(t, r)

	retyped := p.add(t, 10, "Microflows$Microflow")
	r.InvalidateCache()
	if got := len(listModules(t, r)); got != 1 {
		t.Fatalf("listed %d modules before the re-type, want 1", got)
	}
	p.rewrite(t, retyped, typedDoc(t, "Projects$ModuleImpl", "Second"))
	r.InvalidateCache()
	if got := len(listModules(t, r)); got != 2 {
		t.Errorf("listed %d modules after a unit was re-typed to a module, want 2", got)
	}
	// And a fresh process, starting from the persisted index, agrees.
	if got := len(listModules(t, p.reader())); got != 2 {
		t.Errorf("a fresh reader listed %d modules after the re-type, want 2", got)
	}
}

// A unit with no ContentsHash cannot be keyed, so it is read every time — the
// pre-index behaviour, never a wrong type.
func TestListUnitsV2_UnhashedUnitIsAlwaysRead(t *testing.T) {
	p, _ := seedProject(t, 3)
	if _, err := p.db.Exec(`UPDATE Unit SET ContentsHash = ''`); err != nil {
		t.Fatal(err)
	}
	listModules(t, p.reader())
	r := p.reader()
	if got := len(listModules(t, r)); got != 1 {
		t.Fatalf("listed %d modules, want 1", got)
	}
	if got := r.UnitFileReads(); got < 4 {
		t.Errorf("unhashed units read %d times, want every unit typed from its file (>= 4)", got)
	}
}

// A corrupt index file is a cold start, not an error and not a wrong answer.
func TestListUnitsV2_CorruptIndexIsIgnored(t *testing.T) {
	p, _ := seedProject(t, 3)
	listModules(t, p.reader())
	idx := filepath.Join(p.root, ".mxcli", unitTypeIndexName)
	if _, err := os.Stat(idx); err != nil {
		t.Fatalf("the index was not persisted at %s: %v", idx, err)
	}
	if err := os.WriteFile(idx, []byte("garbage\nmore\tgarbage\tthan\tfits\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := len(listModules(t, p.reader())); got != 1 {
		t.Errorf("listed %d modules with a corrupt index, want 1", got)
	}
}

// A reader that catches a unit between a writer's file rename and its hash
// update sees new bytes under the old hash. Learning that pair would list the
// unit under the wrong type once the old bytes are back — a rolled-back write,
// a checkout — so the index learns the hash of what it read.
func TestListUnitsV2_FileAheadOfItsHashTeachesNothingWrong(t *testing.T) {
	p, _ := seedProject(t, 3)
	unit := p.add(t, 10, "Microflows$Microflow")
	stored, err := os.ReadFile(p.unitPath(unit))
	if err != nil {
		t.Fatal(err)
	}
	// The window: the file is a module now, the row still hashes the microflow.
	if err := os.WriteFile(p.unitPath(unit), typedDoc(t, "Projects$ModuleImpl", "Midway"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := len(listModules(t, p.reader())); got != 2 {
		t.Fatalf("listed %d modules mid-write, want 2 (the file is what is read)", got)
	}
	// The write is undone: file and row agree again on the microflow.
	if err := os.WriteFile(p.unitPath(unit), stored, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := len(listModules(t, p.reader())); got != 1 {
		t.Errorf("listed %d modules after the write was undone, want 1: the index learnt the stale hash with the new type", got)
	}
}
