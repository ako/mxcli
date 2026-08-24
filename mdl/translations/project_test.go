// SPDX-License-Identifier: Apache-2.0

package translations

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// fakeProject is a project of raw units, so the driver can be exercised without
// a .mpr. Writes land back in the map, which is what makes a re-run testable.
type fakeProject struct {
	units   map[model.ID][]byte
	order   []model.ID
	written []model.ID
	// ordinaryWrites counts calls to the path that would carry stored
	// translations back — nothing here may use it.
	ordinaryWrites int
}

func (f *fakeProject) ListUnits() ([]*types.UnitInfo, error) {
	out := make([]*types.UnitInfo, 0, len(f.order))
	for _, id := range f.order {
		out = append(out, &types.UnitInfo{ID: id})
	}
	return out, nil
}
func (f *fakeProject) GetRawUnitBytes(id model.ID) ([]byte, error) { return f.units[id], nil }
func (f *fakeProject) UpdateRawUnitOwningTranslations(id string, c []byte) error {
	f.units[model.ID(id)] = c
	f.written = append(f.written, model.ID(id))
	return nil
}

func newProject(t *testing.T, units ...[]byte) *fakeProject {
	t.Helper()
	p := &fakeProject{units: map[model.ID][]byte{}}
	for i, u := range units {
		id := model.ID(string(rune('A' + i)))
		p.units[id] = u
		p.order = append(p.order, id)
	}
	return p
}

func TestApply_WritesOnlyTheUnitsThatChanged(t *testing.T) {
	p := newProject(t,
		unit(t, text(tr("en_US", "Save"))),
		unit(t, text(tr("en_US", "Cancel"))),
	)
	stats, err := Apply(p, "en_US", "nl_NL", Dictionary{"Save": "Opslaan"}, ModeMerge, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Units != 1 || stats.Set != 1 {
		t.Fatalf("stats = %+v, want 1 unit / 1 set — a unit with nothing to change "+
			"must not be written at all", stats)
	}
	if len(p.written) != 1 || p.written[0] != "A" {
		t.Errorf("wrote %v, want only unit A", p.written)
	}
}

// The same source in several documents is translated once and lands in all of
// them. That is the point of keying on the source string.
func TestApply_OneEntryReachesEveryOccurrence(t *testing.T) {
	p := newProject(t,
		unit(t, text(tr("en_US", "Save"))),
		unit(t, text(tr("en_US", "Save")), text(tr("en_US", "Save"))),
	)
	stats, _ := Apply(p, "en_US", "nl_NL", Dictionary{"Save": "Opslaan"}, ModeMerge, nil)
	if stats.Set != 3 || stats.Units != 2 {
		t.Errorf("stats = %+v, want 3 set across 2 units", stats)
	}
}

// Re-running a file writes nothing. Idempotence has to hold at the driver, not
// only at the storage layer, or every run churns version control.
func TestApply_ReRunIsANoOp(t *testing.T) {
	p := newProject(t, unit(t, text(tr("en_US", "Save"))))
	dict := Dictionary{"Save": "Opslaan"}
	if _, err := Apply(p, "en_US", "nl_NL", dict, ModeMerge, nil); err != nil {
		t.Fatal(err)
	}
	p.written = nil
	stats, _ := Apply(p, "en_US", "nl_NL", dict, ModeMerge, nil)
	if stats.Units != 0 || len(p.written) != 0 {
		t.Errorf("re-run wrote %v (%+v), want nothing", p.written, stats)
	}
}

// A key that matches nothing is REPORTED, never skipped in silence: it means the
// file has stopped describing the project.
func TestApply_ReportsUnmatchedKeys(t *testing.T) {
	p := newProject(t, unit(t, text(tr("en_US", "Store"), tr("nl_NL", "Opslaan"))))
	stats, _ := Apply(p, "en_US", "nl_NL", Dictionary{"Save": "Opslaan"}, ModeMerge, nil)
	if len(stats.Unmatched) != 1 || stats.Unmatched[0] != "Save" {
		t.Fatalf("Unmatched = %v, want [Save]", stats.Unmatched)
	}
	entries, _, _ := Collect(p, "en_US", nil)
	drift := SuggestDrift(stats.Unmatched, Dictionary{"Save": "Opslaan"}, entries, "nl_NL")
	if drift[0].Now != "Store" {
		t.Errorf("drift did not correlate to the renamed source: %+v", drift[0])
	}
}

// Scope bounds a REPLACE. Without it, two per-module files would each wipe the
// other's work on every run — which is why `in <Module>` is a requirement and
// not a convenience.
func TestApply_ScopeBoundsTheDeletion(t *testing.T) {
	p := newProject(t,
		unit(t, text(tr("en_US", "Save"), tr("nl_NL", "Opslaan"))),     // A — in scope
		unit(t, text(tr("en_US", "Cancel"), tr("nl_NL", "Annuleren"))), // B — out of scope
	)
	onlyA := Scope(func(id model.ID) bool { return id == "A" })

	stats, _ := Apply(p, "en_US", "nl_NL", Dictionary{"Save": "Opslaan"}, ModeReplace, onlyA)
	if stats.Removed != 0 {
		t.Errorf("removed %d in scope, want 0 — 'Save' IS named", stats.Removed)
	}
	var b bson.D
	if err := bson.Unmarshal(p.units["B"], &b); err != nil {
		t.Fatal(err)
	}
	if findTexts(b)[0].byLang["nl_NL"] != "Annuleren" {
		t.Error("a REPLACE scoped to one module deleted another module's translation")
	}
}

func TestCollect_DeduplicatesAndFlagsConflicts(t *testing.T) {
	p := newProject(t,
		unit(t, text(tr("en_US", "Save"), tr("nl_NL", "Opslaan"))),
		unit(t, text(tr("en_US", "Save"), tr("nl_NL", "Bewaren"))),
		unit(t, text(tr("en_US", "Cancel"))),
	)
	entries, conflicts, err := Collect(p, "en_US", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("collected %d entries, want 2 deduplicated: %+v", len(entries), entries)
	}
	if len(conflicts) != 1 || conflicts[0] != "Save" {
		t.Errorf("conflicts = %v, want [Save] — one entry cannot describe two "+
			"different translations, and silently picking one hides that", conflicts)
	}
}
