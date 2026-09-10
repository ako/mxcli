// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"strings"
	"testing"
)

// `mxcli rename` "renames an element and automatically updates all
// cross-references" — within the model. The brain's anchors are references to
// the same elements and were not updated, so a refactor silently invalidated
// them.
//
// `brain check` catches only half of that, and the half it misses is inherent
// to the design rather than a bug in the check: a decision's anchor points
// backward, so one that stops resolving is reported NOT FOUND; a requirement's
// points forward, so one that stops resolving just counts as PLANNED — which is
// what a forward anchor failing is SUPPOSED to mean. The observed symptom on a
// real project was the plan's progress moving from 65/65 to 63/65 with nothing
// else to see. That ambiguity is the argument for fixing it at the rename.
func TestRenameRewritesAnchorsAcrossTheStore(t *testing.T) {
	s := newTestStore(t)
	promote(t, s, mustEntry(t, "orders are committed by Finance", "@Sales.Order"))
	promote(t, s, mustEntry(t, "the status attribute drives the grid", "@Sales.Order.Status"))
	promote(t, s, mustRequirement(t, "approvals read the order", "02-approvals", "@Sales.Order", "@Sales.ACT_Approve"))

	n, err := s.RenameAnchors("Sales.Order", "Sales.PurchaseOrder")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("rewrote %d anchors, want 3", n)
	}

	for _, shard := range []string{"Sales", PlanShard("02-approvals")} {
		entries, _, err := s.LoadShard(shard)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			for _, a := range e.Anchors {
				if strings.HasPrefix(a, "@Sales.Order") {
					t.Errorf("%s: %q still names the old element", shard, a)
				}
			}
		}
	}

	// A member anchor follows its element: @Sales.Order.Status has to become
	// @Sales.PurchaseOrder.Status, not be left behind or truncated.
	if got := anchorsIn(t, s, "Sales"); !containsAnchor(got, "@Sales.PurchaseOrder.Status") {
		t.Errorf("the member anchor did not follow the rename: %v", got)
	}
}

// The sharp edge. Anchors are dotted names, so a naive prefix replace rewrites
// every element whose name merely STARTS with the renamed one — @Sales.OrderLine
// becoming @Sales.PurchaseOrderLine, which names nothing and which `brain check`
// then reports as a stale decision. The match has to end at a boundary.
func TestRenameDoesNotTouchNamesThatMerelyStartWithTheOldOne(t *testing.T) {
	s := newTestStore(t)
	promote(t, s, mustEntry(t, "orders are committed by Finance", "@Sales.Order"))
	promote(t, s, mustEntry(t, "lines are never edited after posting", "@Sales.OrderLine"))
	promote(t, s, mustEntry(t, "the archive is a separate module", "@SalesArchive.Order"))

	n, err := s.RenameAnchors("Sales.Order", "Sales.PurchaseOrder")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("rewrote %d anchors, want exactly 1", n)
	}

	got := anchorsIn(t, s, "Sales")
	if !containsAnchor(got, "@Sales.OrderLine") {
		t.Errorf("@Sales.OrderLine was rewritten by a rename of @Sales.Order: %v", got)
	}
	if a := anchorsIn(t, s, "SalesArchive"); !containsAnchor(a, "@SalesArchive.Order") {
		t.Errorf("@SalesArchive.Order was rewritten by a rename in a different module: %v", a)
	}
}

// A module rename moves every anchor in the store AND the shard file itself.
// Without the move, every entry in modules/Old.md now anchors into New and
// `brain check` reports the lot as misfiled — trading one false signal for
// another.
func TestRenamingAModuleMovesItsShard(t *testing.T) {
	s := newTestStore(t)
	promote(t, s, mustEntry(t, "orders are committed by Finance", "@Sales.Order"))
	promote(t, s, mustEntry(t, "sales is the only writer", "@Sales"))
	promote(t, s, mustRequirement(t, "approvals read the order", "02-approvals", "@Sales.Order"))

	if _, err := s.RenameAnchors("Sales", "Commerce"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(s.ShardPath("Sales")); !os.IsNotExist(err) {
		t.Error("modules/Sales.md survived the module rename; its entries now anchor into Commerce " +
			"and read as misfiled")
	}
	got := anchorsIn(t, s, "Commerce")
	if !containsAnchor(got, "@Commerce.Order") || !containsAnchor(got, "@Commerce") {
		t.Errorf("modules/Commerce.md holds %v", got)
	}
	// The plan shard's anchor moved too, but the plan shard itself does not:
	// slices span modules by design.
	if a := anchorsIn(t, s, PlanShard("02-approvals")); !containsAnchor(a, "@Commerce.Order") {
		t.Errorf("the plan shard was not rewritten: %v", a)
	}
}

// An entry's id is content-derived, and its content includes its anchors — so
// re-deriving it here would be the obvious thing and is wrong. The id is a
// handle: `brain promote <id>`, `brain resolve <id>`, and prose that cites one.
// A rename must not invalidate every reference to the entry in order to fix the
// entry's references to the model.
func TestRenameKeepsEntryIDsStable(t *testing.T) {
	s := newTestStore(t)
	e := mustEntry(t, "orders are committed by Finance", "@Sales.Order")
	promote(t, s, e)

	if _, err := s.RenameAnchors("Sales.Order", "Sales.PurchaseOrder"); err != nil {
		t.Fatal(err)
	}

	entries, _, err := s.LoadShard("Sales")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != e.ID {
		t.Fatalf("the entry's id changed from %s to %v; every reference to it now dangles",
			e.ID, idsOf(entries))
	}
}

// A rename that matches nothing must say so rather than reporting success, so
// a caller can tell "no anchors needed updating" from "the anchors were missed".
func TestRenameReportsZeroWhenNothingMatches(t *testing.T) {
	s := newTestStore(t)
	promote(t, s, mustEntry(t, "orders are committed by Finance", "@Sales.Order"))

	n, err := s.RenameAnchors("Billing.Invoice", "Billing.Bill")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("rewrote %d anchors in a store that references neither name", n)
	}
}

// The staged queue holds anchors too, and it is where an agent's captures live
// before anyone has looked at them — the ones most likely to name something
// just renamed.
func TestRenameRewritesTheStagedQueue(t *testing.T) {
	s := newTestStore(t)
	dir := strings.TrimSuffix(s.Root, "/"+StoreDir)
	q := NewQueue(dir)
	e := mustEntry(t, "orders are committed by Finance", "@Sales.Order")
	if _, err := q.Append(e); err != nil {
		t.Fatal(err)
	}

	if _, err := RenameQueueAnchors(q, "Sales.Order", "Sales.PurchaseOrder"); err != nil {
		t.Fatal(err)
	}

	got, err := q.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Anchors[0] != "@Sales.PurchaseOrder" {
		t.Fatalf("queue anchors were not rewritten: %v", got)
	}
	if got[0].ID != e.ID {
		t.Errorf("the queued entry's id changed; `brain promote %s` no longer finds it", e.ID)
	}
}

func anchorsIn(t *testing.T, s *Store, shard string) []string {
	t.Helper()
	entries, _, err := s.LoadShard(shard)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Anchors...)
	}
	return out
}

func containsAnchor(anchors []string, want string) bool {
	for _, a := range anchors {
		if a == want {
			return true
		}
	}
	return false
}

func idsOf(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.ID)
	}
	return out
}
