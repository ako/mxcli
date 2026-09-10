// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"strings"
	"testing"
)

// The store's own README describes the scoping — "what lets a session load the
// shards for the modules it is touching instead of the whole store" — and
// nothing produces it. docs/brain/ is a directory, so a session either reads
// all of it or guesses which parts matter, and both are wrong in the same
// direction: measured on a real project, the whole store is 7,531 tokens and
// the correct pack for one slice is ~2,530.
//
// That is worth little in one long session, where the store is read once and
// then cached. It is a third of the context in the sub-agent shape, where the
// pack is re-read per slice from a cold start.
//
// Which module shards belong in the pack is DERIVED, not configured: they are
// the modules the slice's own requirements anchor into. Asking the caller which
// modules a slice touches would be asking it the thing it opened the brief to
// find out.
func TestBriefIsProjectPlusTheSlicesModulesPlusItsPlan(t *testing.T) {
	s := newTestStore(t)

	// The slice under test anchors into Sales and Planning.
	promote(t, s, mustRequirement(t, "roll up by cost centre", "07-planning", "@Planning.ACT_Rollup"))
	promote(t, s, mustRequirement(t, "orders feed the rollup", "07-planning", "@Sales.Order"))
	// Another slice, anchored into a module the first one does not touch.
	promote(t, s, mustRequirement(t, "invoices are posted nightly", "02-billing", "@Billing.ACT_Post"))

	// Decisions in each of those modules, plus a cross-cutting one.
	promote(t, s, mustEntry(t, "planning uses a snapshot, not live totals", "@Planning.Snapshot"))
	promote(t, s, mustEntry(t, "orders are committed by Finance", "@Sales.Order"))
	promote(t, s, mustEntry(t, "billing runs on its own schedule", "@Billing.ACT_Post"))
	promote(t, s, mustEntry(t, "the whole app is single-tenant"))

	b, err := s.Brief("07-planning")
	if err != nil {
		t.Fatal(err)
	}

	if got := shardNames(b); !equalStrings(got, []string{ProjectShard, "Planning", "Sales", PlanShard("07-planning")}) {
		t.Fatalf("brief holds %v, want project + the slice's two modules + its plan shard", got)
	}

	text := b.Text()
	for _, want := range []string{
		"single-tenant",          // project.md, always
		"snapshot",               // Planning: a module the slice anchors into
		"committed by Finance",   // Sales: likewise
		"roll up by cost centre", // the slice's own plan
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the brief does not contain %q", want)
		}
	}

	// The point of the pack is what it LEAVES OUT. Without this the test would
	// pass against a brief that simply concatenated the whole store.
	for _, unwanted := range []string{
		"billing runs on its own schedule", // a module this slice does not touch
		"invoices are posted nightly",      // another slice's plan
	} {
		if strings.Contains(text, unwanted) {
			t.Errorf("the brief contains %q, which belongs to a slice this one does not touch — "+
				"a pack that includes everything is the whole-store read it replaces", unwanted)
		}
	}
}

// A slice with no requirements yet still gets project.md: a session picking it
// up needs the project's decisions before it has written anything down.
func TestBriefOfAnEmptySliceIsStillTheProjectShard(t *testing.T) {
	s := newTestStore(t)
	promote(t, s, mustEntry(t, "the whole app is single-tenant"))

	b, err := s.Brief("11-nothing-yet")
	if err != nil {
		t.Fatal(err)
	}
	if got := shardNames(b); !equalStrings(got, []string{ProjectShard}) {
		t.Fatalf("brief holds %v, want just the project shard", got)
	}
	if !strings.Contains(b.Text(), "single-tenant") {
		t.Error("project.md was not included")
	}
}

// Without a slice the brief is the modules' decisions and no plan at all — for
// a session doing maintenance rather than working the roadmap.
func TestBriefForNamedModules(t *testing.T) {
	s := newTestStore(t)
	promote(t, s, mustEntry(t, "planning uses a snapshot", "@Planning.Snapshot"))
	promote(t, s, mustEntry(t, "billing runs nightly", "@Billing.ACT_Post"))
	promote(t, s, mustEntry(t, "single-tenant"))

	b, err := s.BriefForModules([]string{"Planning"})
	if err != nil {
		t.Fatal(err)
	}
	if got := shardNames(b); !equalStrings(got, []string{ProjectShard, "Planning"}) {
		t.Fatalf("brief holds %v, want project + Planning", got)
	}
	if strings.Contains(b.Text(), "billing runs nightly") {
		t.Error("a module that was not asked for is in the brief")
	}
}

// The brief reports its own size, because the reason it exists is the size of
// the alternative. A number nobody can see does not change anyone's behaviour.
func TestBriefReportsItsSizeAgainstTheWholeStore(t *testing.T) {
	s := newTestStore(t)
	promote(t, s, mustRequirement(t, "roll up by cost centre", "07-planning", "@Planning.ACT_Rollup"))
	promote(t, s, mustEntry(t, "planning uses a snapshot", "@Planning.Snapshot"))
	for _, e := range []Entry{
		mustEntry(t, "billing runs nightly and has a long tail of detail behind it", "@Billing.ACT_Post"),
		mustEntry(t, "shipping is handled by a third party with its own SLA", "@Shipping.Carrier"),
		mustRequirement(t, "invoices are posted nightly", "02-billing", "@Billing.ACT_Post"),
	} {
		promote(t, s, e)
	}

	b, err := s.Brief("07-planning")
	if err != nil {
		t.Fatal(err)
	}
	whole, err := s.Size()
	if err != nil {
		t.Fatal(err)
	}
	if b.Lines >= whole {
		t.Errorf("the brief is %d lines against a whole store of %d; it is not saving anything",
			b.Lines, whole)
	}
	if b.Lines != strings.Count(b.Text(), "\n") {
		t.Errorf("reported %d lines, text has %d", b.Lines, strings.Count(b.Text(), "\n"))
	}
}

func shardNames(b Brief) []string {
	out := make([]string, 0, len(b.Shards))
	for _, s := range b.Shards {
		out = append(out, s.Shard)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(t.TempDir())
	if _, err := s.Init(); err != nil {
		t.Fatal(err)
	}
	return s
}

func promote(t *testing.T, s *Store, e Entry) {
	t.Helper()
	if err := s.Promote(e, e.Shard()); err != nil {
		t.Fatalf("promote %q: %v", e.Title, err)
	}
}
