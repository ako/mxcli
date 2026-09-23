// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// An entity INDEX carries a GUID of its own (RegisterTypeDefaults registers
// EmitGUID for DomainModels$EntityIndex), and entityToGen rebuilds it from the
// semantic model exactly as it rebuilds an attribute — so it has the same #1119
// exposure, and carrying it is load-bearing for a second reason: without the
// carry, the storage-GUID guard would refuse every ALTER on an indexed entity.
//
// The fixture holds no Studio Pro-authored index, and one mxcli creates has
// GUID == $ID legitimately, which cannot detect the conflation. So the stored
// state is seeded the way Studio Pro leaves it — the index's GUID made to differ
// from its $ID — by patching the unit on disk, below the writer.
func TestIssue1119_AlterPreservesIndexGUIDs(t *testing.T) {
	proj := copyFixture(t)

	// 1. Create an entity with an index through the ordinary path.
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	dmID, _ := richestEntity(t, b)
	const entName = "Issue1119Indexed"
	ent := &domainmodel.Entity{
		Name:        entName,
		Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{Name: "Code", Type: &domainmodel.StringAttributeType{Length: 50}},
			{Name: "Label", Type: &domainmodel.StringAttributeType{Length: 50}},
		},
	}
	if err := b.CreateEntity(dmID, ent); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	// Re-read to learn the attribute IDs the index must point at.
	stored := mustFindEntity(t, b, dmID, entName)
	stored.Indexes = []*domainmodel.Index{{
		Attributes: []*domainmodel.IndexAttribute{
			{AttributeID: stored.Attributes[0].ID, Ascending: true},
		},
	}}
	if err := b.UpdateEntity(dmID, stored); err != nil {
		t.Fatalf("UpdateEntity (add index): %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	// 2. Make the index's GUID differ from its $ID, as a Studio Pro-authored one
	//    does. This goes straight at the stored bytes: the writer would (rightly)
	//    refuse a write that moves a GUID.
	seededGUID := patchIndexGUID(t, proj, dmID, entName)

	// 3. ALTER the entity. Nothing about the index is mentioned.
	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	target := mustFindEntity(t, b2, dmID, entName)
	target.Documentation = "issue 1119 index arm"
	if err := b2.UpdateEntity(dmID, target); err != nil {
		// A refusal here is the guard firing because the carry did not happen —
		// which is the failure this test exists to catch, not an unrelated error.
		t.Fatalf("UpdateEntity on an indexed entity: %v", err)
	}
	if err := b2.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	// 4. The index kept the GUID it was seeded with.
	b3 := New()
	if err := b3.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b3.Disconnect() })

	got := indexGUIDs(t, b3, dmID, entName)
	if len(got) != 1 {
		t.Fatalf("expected 1 index after the ALTER, got %d", len(got))
	}
	if got[0] != seededGUID {
		t.Errorf("index storage GUID changed on ALTER: seeded %s, now %s", seededGUID, got[0])
	}
}

func mustFindEntity(t *testing.T, b *Backend, dmID model.ID, name string) *domainmodel.Entity {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range dms {
		if d.ID != dmID {
			continue
		}
		for _, e := range d.Entities {
			if e.Name == name {
				return e
			}
		}
	}
	t.Fatalf("entity %s not found in domain model %s", name, dmID)
	return nil
}

func indexGUIDs(t *testing.T, b *Backend, dmID model.ID, entityName string) []string {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	var out []string
	for _, el := range gdm.EntitiesItems() {
		ge, ok := el.(*genDm.Entity)
		if !ok || ge.Name() != entityName {
			continue
		}
		for _, iel := range ge.IndexesItems() {
			if idx, ok := iel.(*genDm.Index); ok {
				out = append(out, rawKeyHex(t, idx.Raw(), "GUID"))
			}
		}
	}
	return out
}

// patchIndexGUID rewrites the stored unit so every DomainModels$EntityIndex
// carries a GUID that differs from its $ID, and returns the hex it wrote. It
// edits the file (and the .mpr's ContentsHash) directly, below the writer.
func patchIndexGUID(t *testing.T, proj string, dmID model.ID, entityName string) string {
	t.Helper()
	path := unitFilePath(t, proj, entityName)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal unit: %v", err)
	}

	seeded := make([]byte, 16)
	for i := range seeded {
		seeded[i] = 0xAB
	}
	patched := 0
	var walk func(any)
	walk = func(v any) {
		switch t2 := v.(type) {
		case bson.D:
			isIndex := false
			for _, e := range t2 {
				if e.Key == "$Type" {
					if s, ok := e.Value.(string); ok && s == "DomainModels$EntityIndex" {
						isIndex = true
					}
				}
			}
			for i, e := range t2 {
				if isIndex && e.Key == "GUID" {
					if bin, ok := e.Value.(bson.Binary); ok {
						t2[i].Value = bson.Binary{Subtype: bin.Subtype, Data: append([]byte{}, seeded...)}
						patched++
					}
				}
				walk(e.Value)
			}
		case bson.A:
			for _, e := range t2 {
				walk(e)
			}
		}
	}
	walk(doc)
	if patched == 0 {
		t.Fatal("no DomainModels$EntityIndex GUID found to seed; the index was not written")
	}

	out, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal patched unit: %v", err)
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		t.Fatalf("write patched unit: %v", err)
	}

	// The .mpr keeps a hash of each v2 unit file. Leave it consistent, so the
	// reader is not handed a unit the project believes is something else — and so
	// the seeding is not itself what the ALTER reacts to.
	sum := sha256.Sum256(out)
	db, err := sql.Open("sqlite", proj)
	if err != nil {
		t.Fatalf("open mpr: %v", err)
	}
	defer db.Close()
	res, err := db.Exec(
		"UPDATE Unit SET ContentsHash = ? WHERE lower(hex(UnitID)) = ?",
		base64.StdEncoding.EncodeToString(sum[:]), storedUnitIDHex(t, string(dmID)),
	)
	if err != nil {
		t.Fatalf("update ContentsHash: %v", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		t.Fatalf("RowsAffected: %v", err)
	}
	if n != 1 {
		t.Fatalf("ContentsHash update matched %d rows, want 1", n)
	}
	return "abababababababababababababababab"
}

// storedUnitIDHex renders a unit id the way the .mpr's Unit.UnitID blob holds
// it: .NET GUID layout, so the first three fields are little-endian. Both
// spellings of the id are in play around a v2 project — neither keying on the
// plain id nor on the file's base name matches this column, and both fail
// SILENTLY as a zero-row UPDATE.
func storedUnitIDHex(t *testing.T, id string) string {
	t.Helper()
	raw, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	if err != nil || len(raw) != 16 {
		t.Fatalf("not a uuid: %q (%v)", id, err)
	}
	out := make([]byte, 16)
	copy(out, raw)
	for _, f := range [][2]int{{0, 4}, {4, 6}, {6, 8}} {
		lo, hi := f[0], f[1]
		for i, j := lo, hi-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return hex.EncodeToString(out)
}

// unitFilePath finds the domain-model unit's file by its CONTENT rather than by
// deriving the on-disk name, which uses the .NET-swapped byte order: the unit
// wanted is the one whose $Type is DomainModels$DomainModel and which holds the
// entity just created.
func unitFilePath(t *testing.T, proj, entityName string) string {
	t.Helper()
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(proj), "mprcontents", "*", "*", "*.mxunit"))
	var found []string
	for _, m := range matches {
		raw, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var doc bson.D
		if bson.Unmarshal(raw, &doc) != nil {
			continue
		}
		isDM := false
		for _, e := range doc {
			if e.Key == "$Type" {
				if s, ok := e.Value.(string); ok && s == "DomainModels$DomainModel" {
					isDM = true
				}
			}
		}
		if isDM && bytes.Contains(raw, []byte(entityName)) {
			found = append(found, m)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly one domain-model unit holding %s, found %v", entityName, found)
	}
	return found[0]
}
