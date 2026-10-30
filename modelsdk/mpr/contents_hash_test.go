// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// ako/mxcli#972: restoring a .mxunit with `git checkout` leaves the .mpr's
// ContentsHash describing bytes that are no longer on disk.

const hashTestUnit = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func TestVerifyContentsHashesCleanControl(t *testing.T) {
	w, _ := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	rep, err := w.ConcreteReader().VerifyContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() || rep.Units != 1 || rep.Files != 1 {
		t.Fatalf("untouched project not clean: %+v", rep)
	}
}

func TestVerifyContentsHashesDetectsRevertedFile(t *testing.T) {
	w, unitPath := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	reverted := unitDoc(t, "RevertedByGit")
	if err := os.WriteFile(unitPath, reverted, 0644); err != nil {
		t.Fatal(err)
	}
	rep, err := w.ConcreteReader().VerifyContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Mismatches) != 1 {
		t.Fatalf("want 1 mismatch, got %+v", rep)
	}
	m := rep.Mismatches[0]
	if m.UnitID != hashTestUnit || m.Path != unitPath || m.Actual != hashOf(reverted) || m.Stored == m.Actual {
		t.Fatalf("wrong mismatch: %+v", m)
	}
}

func TestVerifyContentsHashesMissingAndOrphanFiles(t *testing.T) {
	w, unitPath := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	if err := os.Remove(unitPath); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(w.ConcreteReader().ContentsDir(), "12", "34", "12345678-0000-0000-0000-000000000000.mxunit")
	if err := os.MkdirAll(filepath.Dir(orphan), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	rep, err := w.ConcreteReader().VerifyContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.MissingFiles) != 1 || rep.MissingFiles[0].UnitID != hashTestUnit {
		t.Fatalf("missing file not reported: %+v", rep)
	}
	if len(rep.OrphanFiles) != 1 || rep.OrphanFiles[0] != orphan {
		t.Fatalf("orphan file not reported: %+v", rep)
	}
	if len(rep.Mismatches) != 0 {
		t.Fatalf("unexpected mismatches: %+v", rep.Mismatches)
	}
}

func TestRepairContentsHashesRewritesIndex(t *testing.T) {
	w, unitPath := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	reverted := unitDoc(t, "RevertedByGit")
	if err := os.WriteFile(unitPath, reverted, 0644); err != nil {
		t.Fatal(err)
	}
	rep, n, err := w.RepairContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(rep.Mismatches) != 1 {
		t.Fatalf("want 1 row repaired, got %d (%+v)", n, rep)
	}
	if got := storedHash(t, w, hashTestUnit); got != hashOf(reverted) {
		t.Fatalf("ContentsHash = %q, want %q", got, hashOf(reverted))
	}
	after, err := w.ConcreteReader().VerifyContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if !after.Clean() {
		t.Fatalf("not clean after repair: %+v", after)
	}
	// The file itself is the source of truth and must be left alone.
	if b, _ := os.ReadFile(unitPath); string(b) != string(reverted) {
		t.Fatal("repair modified the unit file")
	}
}

func TestRepairContentsHashesRefusedWhileStudioProOpen(t *testing.T) {
	w, unitPath := newV2WriterForCommitTest(t, hashTestUnit, unitDoc(t, "Stored"))
	stored := storedHash(t, w, hashTestUnit)
	if err := os.WriteFile(unitPath, unitDoc(t, "RevertedByGit"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.ConcreteReader().Path()+".lock", nil, 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := w.RepairContentsHashes()
	var open *StudioProOpenError
	if !errors.As(err, &open) {
		t.Fatalf("want StudioProOpenError, got %v", err)
	}
	if got := storedHash(t, w, hashTestUnit); got != stored {
		t.Fatal("hash rewritten despite the guard")
	}
}

// ako/mxcli#1159: an MPR v1 project has no files, but it does have the
// column, and Studio Pro fills it on every unit. A writer that forgot it (every
// mxcli before the fix) left it describing the previous contents, or '' on an
// inserted unit; the comparison is against the Contents column.

// staleV1Unit replaces one Projects$Folder's Contents the way an mxcli before
// #1159 did, leaving its ContentsHash alone. Returns the unit ID and the new bytes.
func staleV1Unit(t *testing.T, w *Writer) (string, []byte) {
	t.Helper()
	id, stored := v1UnitOfType(t, w, "Projects$Folder")
	changed := withFields(t, stored, map[string]any{"Name": "WrittenByOldMxcli"})
	if _, err := w.reader.db.Exec(`UPDATE Unit SET Contents = ? WHERE UnitID = ?`, changed, uuidToBlob(id)); err != nil {
		t.Fatal(err)
	}
	return id, changed
}

func TestVerifyContentsHashesV1CleanControl(t *testing.T) {
	w := openV1FixtureWriter(t)
	rep, err := w.ConcreteReader().VerifyContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() || rep.Units != 20 || rep.Files != 0 || rep.Version != MPRVersionV1 {
		t.Fatalf("Studio Pro-authored v1 fixture not clean: %+v", rep)
	}
}

func TestVerifyContentsHashesV1DetectsStaleHash(t *testing.T) {
	w := openV1FixtureWriter(t)
	before := storedHash(t, w, v1FolderID(t, w))
	id, changed := staleV1Unit(t, w)

	rep, err := w.ConcreteReader().VerifyContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Mismatches) != 1 || len(rep.MissingFiles) != 0 || len(rep.OrphanFiles) != 0 {
		t.Fatalf("want exactly 1 mismatch, got %+v", rep)
	}
	m := rep.Mismatches[0]
	if m.UnitID != id || m.Stored != before || m.Actual != hashOf(changed) || m.Path != "" {
		t.Fatalf("wrong mismatch: %+v", m)
	}
	if m.Unit != "Projects$Folder WrittenByOldMxcli" {
		t.Fatalf("mismatch does not name its unit: %q", m.Unit)
	}
}

func TestVerifyContentsHashesV1ReportsEmptyHash(t *testing.T) {
	w := openV1FixtureWriter(t)
	id := v1FolderID(t, w)
	if _, err := w.reader.db.Exec(`UPDATE Unit SET ContentsHash = '' WHERE UnitID = ?`, uuidToBlob(id)); err != nil {
		t.Fatal(err)
	}
	rep, err := w.ConcreteReader().VerifyContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Mismatches) != 1 || rep.Mismatches[0].UnitID != id || rep.Mismatches[0].Stored != "" {
		t.Fatalf("an inserted unit's empty hash is not reported: %+v", rep)
	}
}

func TestRepairContentsHashesV1RewritesFromContents(t *testing.T) {
	w := openV1FixtureWriter(t)
	id, changed := staleV1Unit(t, w)

	rep, n, err := w.RepairContentsHashes()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(rep.Mismatches) != 1 {
		t.Fatalf("want 1 row repaired, got %d (%+v)", n, rep)
	}
	if got := storedHash(t, w, id); got != hashOf(changed) {
		t.Fatalf("ContentsHash = %q, want %q", got, hashOf(changed))
	}
	if stale := staleV1Hashes(t, w); len(stale) != 0 {
		t.Fatalf("still stale after repair: %v", stale)
	}
	// The contents are the source of truth and must be left alone.
	if got, _ := w.reader.GetRawUnitBytes(id); string(got) != string(changed) {
		t.Fatal("repair modified the unit's contents")
	}
}

func v1FolderID(t *testing.T, w *Writer) string {
	t.Helper()
	id, _ := v1UnitOfType(t, w, "Projects$Folder")
	return id
}
