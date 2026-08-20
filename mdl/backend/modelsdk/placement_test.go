// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
)

// firstDocument returns some document unit in the module, to move around.
func firstDocument(t *testing.T, b *Backend, moduleName string) (name string, container model.ID) {
	t.Helper()
	mfs, err := b.ListMicroflows()
	if err != nil {
		t.Fatalf("ListMicroflows: %v", err)
	}
	for _, mf := range mfs {
		if b.moduleNameFor(mf.ID) == moduleName {
			return mf.Name, mf.ContainerID
		}
	}
	t.Skipf("fixture has no microflow in %s to move", moduleName)
	return "", ""
}

// TestFindDocumentUnitResolvesByNameWithoutAType is the half of #932 that makes
// a placement fix general: a document is located through the unit table, so a
// doctype nobody wrote a finder for is still reachable.
func TestFindDocumentUnitResolvesByNameWithoutAType(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	name, _ := firstDocument(t, b, "MyFirstModule")
	got, err := b.FindDocumentUnit("MyFirstModule", name)
	if err != nil {
		t.Fatalf("FindDocumentUnit: %v", err)
	}
	if got == nil {
		t.Fatalf("FindDocumentUnit(MyFirstModule, %s) found nothing", name)
	}
	if got.Name != name || got.ID == "" || got.Type == "" {
		t.Errorf("got %+v, want the named document with an id and a type", got)
	}
	if got.Kind == "" {
		t.Errorf("got no human-readable kind for %q", got.Type)
	}

	// A name that is not there must be reported as absent, not invented: the
	// MOVE handler distinguishes the two to produce a "not found" instead of
	// reparenting whatever came first.
	missing, err := b.FindDocumentUnit("MyFirstModule", "NoSuchDocument_932")
	if err != nil {
		t.Fatalf("FindDocumentUnit(missing): %v", err)
	}
	if missing != nil {
		t.Errorf("FindDocumentUnit for an absent name returned %+v", missing)
	}
}

// TestMoveDocumentPersistsAndIsIdempotent pins both halves of the reparent
// primitive against real storage: the row really changes, and moving a document
// to where it already is neither writes nor counts as a write.
//
// The write counters matter as much as the row does. A move changes no byte of
// the document, so it is invisible to content-based no-op elision — without the
// count, ReportMutation calls a real move "Unchanged", which is the same class
// of lie as the silent no-op this fixes.
func TestMoveDocumentPersistsAndIsIdempotent(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	var _ backend.DocumentPlacementBackend = b

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v, %v", mod, err)
	}
	name, origin := firstDocument(t, b, "MyFirstModule")
	doc, err := b.FindDocumentUnit("MyFirstModule", name)
	if err != nil || doc == nil {
		t.Fatalf("FindDocumentUnit(%s): %v, %v", name, doc, err)
	}

	folder := &model.Folder{Name: "Placement932", ContainerID: mod.ID}
	if err := b.CreateFolder(folder); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if folder.ID == origin {
		t.Fatal("fixture folder collided with the document's current container")
	}

	offeredBefore, writtenBefore := b.writer.WriteStats()
	if err := b.MoveDocument(doc.ID, folder.ID); err != nil {
		t.Fatalf("MoveDocument: %v", err)
	}
	offeredAfter, writtenAfter := b.writer.WriteStats()
	if writtenAfter != writtenBefore+1 || offeredAfter != offeredBefore+1 {
		t.Errorf("a real move counted %d offered / %d written, want one of each",
			offeredAfter-offeredBefore, writtenAfter-writtenBefore)
	}

	// Same move again: offered, but elided.
	if err := b.MoveDocument(doc.ID, folder.ID); err != nil {
		t.Fatalf("MoveDocument (repeat): %v", err)
	}
	offeredRepeat, writtenRepeat := b.writer.WriteStats()
	if writtenRepeat != writtenAfter {
		t.Errorf("moving a document to its current container wrote (written %d → %d)",
			writtenAfter, writtenRepeat)
	}
	if offeredRepeat != offeredAfter+1 {
		t.Errorf("the repeat move was not offered to storage, so it cannot have been judged elided")
	}

	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	// Reopen: the placement must have reached disk, not just the cache.
	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	again, err := b2.FindDocumentUnit("MyFirstModule", name)
	if err != nil || again == nil {
		t.Fatalf("FindDocumentUnit after reopen: %v, %v", again, err)
	}
	if again.ContainerID != folder.ID {
		t.Errorf("after reopen the document sits in %q, want the folder %q", again.ContainerID, folder.ID)
	}
	// Still in the module: a move that lost the module qualification would be
	// the orphaning failure #892 documents, not a move.
	if b2.moduleNameFor(again.ID) != "MyFirstModule" {
		t.Errorf("the moved document is no longer resolvable in its module")
	}
}
