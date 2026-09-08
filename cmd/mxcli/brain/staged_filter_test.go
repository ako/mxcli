// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"testing"
)

// A dispatcher running one sub-agent per slice needs to ask what THIS slice
// staged, so it can refuse to advance on a slice that recorded nothing. The
// queue answers "everything ever staged" and nothing else.
//
// Which filter does that has to come from what an entry actually carries, and
// the obvious one does not work. `--slice` can only match Entry.Slice, which
// is set by `capture --slice` — and that flag makes the entry a REQUIREMENT.
// A decision captured while working slice 07 carries no slice at all, so a
// slice filter silently answers a narrower question than the one asked: it
// reports the slice's planned scope, not its findings. The queue's own order
// is the honest boundary — it is append-only, so "everything after the id that
// was last there" is exactly this slice's captures, decisions included.
//
// Date cannot substitute: Entry.Date is a day, so every capture in a session
// shares one value and a boundary inside a day does not exist.
func TestSinceIDReturnsEverythingCapturedAfterTheBoundary(t *testing.T) {
	before := []Entry{mustEntry(t, "decided before the slice began"), mustEntry(t, "also before")}
	during := []Entry{
		mustEntry(t, "a decision found while building the slice"),
		mustRequirement(t, "a requirement of the slice", "07-planning"),
		mustEntry(t, "another decision, no slice on it at all"),
	}
	all := append(append([]Entry{}, before...), during...)

	got, err := FilterStaged(all, StagedFilter{SinceID: before[len(before)-1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(during) {
		t.Fatalf("got %d entries after the boundary, want %d: %v", len(got), len(during), titles(got))
	}
	for i := range during {
		if got[i].ID != during[i].ID {
			t.Errorf("position %d: got %q, want %q", i, got[i].Title, during[i].Title)
		}
	}

	// The decisions are the point. A slice filter would return one of these
	// three; the whole reason --since exists is that it returns all three.
	var decisions int
	for _, e := range got {
		if e.EntryKind() == KindDecision {
			decisions++
		}
	}
	if decisions != 2 {
		t.Errorf("got %d decisions after the boundary, want 2 — a slice's findings are mostly "+
			"decisions, and they carry no slice", decisions)
	}
}

// The boundary is exclusive, and an empty result is the answer a dispatcher
// most needs: the slice staged nothing.
func TestSinceIDIsExclusiveAndCanBeEmpty(t *testing.T) {
	all := []Entry{mustEntry(t, "first thing"), mustEntry(t, "second thing")}

	got, err := FilterStaged(all, StagedFilter{SinceID: all[len(all)-1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("the boundary entry itself came back: %v", titles(got))
	}
}

// An id that is not in the queue must be an error, not an empty result. Empty
// is the signal a dispatcher acts on ("this slice recorded nothing"), so a
// typo'd or promoted id quietly producing it would make the dispatcher abort a
// slice that had in fact done its job.
func TestUnknownSinceIDIsAnErrorNotAnEmptyResult(t *testing.T) {
	all := []Entry{mustEntry(t, "first thing")}

	if _, err := FilterStaged(all, StagedFilter{SinceID: "nope12"}); err == nil {
		t.Error("an unknown --since id returned a result instead of an error; empty is a " +
			"meaningful answer here and must not be produced by a typo")
	}
}

// --slice still earns its place for the plan-shaped question ("what scope is
// queued for this slice"), and must not quietly include decisions.
func TestSliceFilterMatchesRequirementsOfThatSliceOnly(t *testing.T) {
	all := []Entry{
		mustEntry(t, "a decision with no slice"),
		mustRequirement(t, "scope for accounts", "01-accounts"),
		mustRequirement(t, "scope for approvals", "02-approvals"),
		mustRequirement(t, "more scope for accounts", "01-accounts"),
	}

	got, err := FilterStaged(all, StagedFilter{Slice: "01-accounts"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2: %v", len(got), titles(got))
	}
	for _, e := range got {
		if e.Slice != "01-accounts" {
			t.Errorf("%q is in slice %q", e.Title, e.Slice)
		}
	}
}

// Both filters together narrow rather than widen — a dispatcher asking "what
// scope did this slice add" wants the intersection.
func TestFiltersCombineAsAnIntersection(t *testing.T) {
	old := mustRequirement(t, "scope queued before the slice ran", "01-accounts")
	all := []Entry{
		old,
		mustEntry(t, "a decision during the slice"),
		mustRequirement(t, "scope added during the slice", "01-accounts"),
		mustRequirement(t, "scope for another slice", "02-approvals"),
	}

	got, err := FilterStaged(all, StagedFilter{SinceID: old.ID, Slice: "01-accounts"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "scope added during the slice" {
		t.Fatalf("got %v, want just the one entry matching both", titles(got))
	}
}

func titles(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Title)
	}
	return out
}
