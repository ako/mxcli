// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#1791: naming every folder (the container hierarchy, built by
// nearly every command) or every microflow (a describe's name map) opened each
// of their files on every call, because a unit's Name is only in its BSON. The
// unit index records Name and Excluded beside the $Type, keyed by the same
// ContentsHash, so a fresh process names the units it has seen without
// opening them.

// headerDoc is a unit with a Name and, when excluded, Excluded = true.
func headerDoc(t *testing.T, typ, name string, excluded bool) []byte {
	t.Helper()
	d := bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob("11111111-1111-1111-1111-111111111111")}},
		{Key: "$Type", Value: typ},
		{Key: "Name", Value: name},
	}
	if excluded {
		d = append(d, bson.E{Key: "Excluded", Value: true})
	}
	b, err := bson.Marshal(d)
	if err != nil {
		t.Fatalf("marshal unit: %v", err)
	}
	return b
}

func TestListUnitHeaders_SecondProcessOpensNoFile(t *testing.T) {
	const n = 40
	p, _ := seedProject(t, n)
	for i := n + 1; i <= n+5; i++ {
		p.add(t, i, "Projects$Folder")
	}

	first := p.reader()
	want, err := first.ListUnitHeaders("Microflows$Microflow")
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != n {
		t.Fatalf("first reader listed %d microflow headers, want %d", len(want), n)
	}
	if got := first.UnitFileReads(); got < n {
		t.Fatalf("control: the cold listing read %d files, want >= %d (the fixture does not exercise the index)", got, n)
	}

	second := p.reader()
	got, err := second.ListUnitHeaders("Microflows$Microflow")
	if err != nil {
		t.Fatal(err)
	}
	folders, err := second.ListFolders()
	if err != nil {
		t.Fatal(err)
	}
	if reads := second.UnitFileReads(); reads != 0 {
		t.Errorf("a fresh process naming %d microflows and %d folders opened %d unit files, want 0", len(got), len(folders), reads)
	}
	if len(got) != len(want) {
		t.Fatalf("second reader listed %d headers, the first %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("header %d from the index = %+v, from the file = %+v", i, got[i], want[i])
		}
	}
	for _, f := range folders {
		if f.Name == "" {
			t.Errorf("folder %s has no name from the index", f.ID)
		}
	}
}

// What the index answers is what the files say, on both storage formats,
// including a name no line-based format could hold unescaped and the
// Excluded flag.
func TestListUnitHeaders_AgreesWithTheContents(t *testing.T) {
	awkward := "Tab\there \"quoted\"\nnewline ü"

	p := newV2Project(t)
	ids := []string{p.add(t, 1, "Microflows$Microflow")}
	id := p.add(t, 2, "Microflows$Microflow")
	p.rewrite(t, id, headerDoc(t, "Microflows$Microflow", awkward, true))
	ids = append(ids, id)
	p.add(t, 3, "Forms$Page")

	// Warm the persisted index with one reader, answer from it with another.
	if _, err := p.reader().ListUnitHeaders("Microflows$Microflow"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.reader().unitTypeIndexPath()); err != nil {
		t.Fatalf("control: the index was not persisted: %v", err)
	}
	r := p.reader()
	hs, err := r.ListUnitHeaders("Microflows$Microflow")
	if err != nil {
		t.Fatal(err)
	}
	if reads := r.UnitFileReads(); reads != 0 {
		t.Fatalf("control: the listing opened %d files, so it did not answer from the persisted index", reads)
	}
	if len(hs) != len(ids) {
		t.Fatalf("listed %d microflow headers, want %d", len(hs), len(ids))
	}
	for _, h := range hs {
		contents, err := os.ReadFile(p.unitPath(h.ID))
		if err != nil {
			t.Fatal(err)
		}
		e := unitIndexEntryOf(contents)
		if h.Name != e.Name || h.Excluded != e.Excluded {
			t.Errorf("header %s = {%q %v}, the file says {%q %v}", h.ID, h.Name, h.Excluded, e.Name, e.Excluded)
		}
	}

	v1 := newTypedListingFixture(t, []string{"Microflows$Microflow", "Forms$Page", "Microflows$Microflow"})
	units, err := v1.r.listUnitsByType("Microflows$Microflow")
	if err != nil {
		t.Fatal(err)
	}
	v1hs, err := v1.r.ListUnitHeaders("Microflows$Microflow")
	if err != nil {
		t.Fatal(err)
	}
	if len(v1hs) != len(units) || len(units) != 2 {
		t.Fatalf("v1: %d headers for %d microflows, want 2", len(v1hs), len(units))
	}
	for i, u := range units {
		e := unitIndexEntryOf(u.Contents)
		if v1hs[i].ID != u.ID || v1hs[i].Name != e.Name || v1hs[i].Excluded != e.Excluded {
			t.Errorf("v1 header %d = %+v, the blob says {%s %q %v}", i, v1hs[i], u.ID, e.Name, e.Excluded)
		}
	}
}

// An index in the previous format (type only, unit-types.idx) is neither read
// nor overwritten: an older mxcli working on the same project keeps its own.
func TestUnitIndex_PreviousFormatIsLeftAlone(t *testing.T) {
	p, _ := seedProject(t, 3)
	r := p.reader()
	dir := r.unitTypeIndexPath()[:len(r.unitTypeIndexPath())-len(unitTypeIndexName)]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := []byte("mxcli-unit-types 1\nsomehash\tMicroflows$Microflow\n")
	if err := os.WriteFile(dir+"unit-types.idx", old, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ListUnitHeaders("Microflows$Microflow"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dir + "unit-types.idx")
	if err != nil || string(got) != string(old) {
		t.Errorf("the previous format's index was changed: %q, %v", got, err)
	}
	if _, err := os.Stat(r.unitTypeIndexPath()); err != nil {
		t.Errorf("the current index was not written beside it: %v", err)
	}
}
