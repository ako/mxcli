// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"encoding/json"
	"strings"
	"testing"
)

// An orchestrator dispatching one sub-agent per slice has to read the brain's
// output, not look at it: it decides whether to advance on what `staged` and
// `check` report. Every brain command prints for a human today, so that
// decision cannot be automated at all.
//
// AnchorState is the sharp edge. It is an int, so a plain marshal emits 0, 1
// and 2 — a contract where the meaning of "1" lives in the order of a const
// block, and where inserting a state silently reassigns every existing value.
// The three states are the whole point of the check (only the middle one is a
// failure), so they travel as names.
func TestAnchorStateTravelsAsANameNotAnOrdinal(t *testing.T) {
	for _, s := range []AnchorState{Resolved, NotFound, NotIndexable} {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		got := string(b)
		if !strings.HasPrefix(got, `"`) {
			t.Errorf("AnchorState %v marshals as %s; an ordinal reassigns itself the next time a "+
				"state is inserted, and nothing downstream would notice", s, got)
		}
		if want := `"` + s.String() + `"`; got != want {
			t.Errorf("AnchorState %v marshals as %s, want %s", s, got, want)
		}
	}
}

// The report has to survive the round trip, or a consumer cannot tell a clean
// store from one it failed to parse. In particular a failing check must be
// recognisable from the JSON alone, without re-deriving Failed()'s rules.
func TestReportRoundTripsThroughJSON(t *testing.T) {
	rep := Report{
		Shards:    []string{"Sales", PlanShard("01-accounts")},
		Entries:   3,
		Anchors:   4,
		ResolvedN: 2,
		Findings: []AnchorFinding{
			{Shard: "Sales", EntryID: "a1", Title: "why", Anchor: "@Sales.Gone", State: NotFound},
			{Shard: "Sales", EntryID: "a2", Title: "sched", Anchor: "@Sales.Nightly", State: NotIndexable},
		},
		Misfiled:  []MisfiledFinding{{Shard: "Sales", EntryID: "a3", Title: "t", Belongs: "Finance"}},
		Malformed: []string{"Sales: unreadable block"},
		Slices:    []SliceProgress{{Slice: "01-accounts", Built: 1, Planned: 2, Questions: 1, Unanchored: 0}},
		Open:      []OpenQuestion{{Shard: "Sales", EntryID: "a4", Title: "should it exist"}},
	}

	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("the report does not round-trip: %v", err)
	}

	if len(back.Findings) != 2 || back.Findings[0].State != NotFound || back.Findings[1].State != NotIndexable {
		t.Errorf("anchor states did not survive the round trip: %+v", back.Findings)
	}
	if !back.Failed() {
		t.Error("a report with a NotFound anchor and a misfiled entry did not read as failed after a round trip")
	}
	if back.Slices[0].Built != 1 || back.Slices[0].Planned != 2 || back.Slices[0].Questions != 1 {
		t.Errorf("slice progress did not survive: %+v", back.Slices[0])
	}

	// The keys are the contract. Renaming one breaks every caller silently, so
	// the names are pinned here rather than left to whatever the field is called.
	for _, key := range []string{
		`"findings"`, `"misfiled"`, `"malformed"`, `"slices"`, `"open"`,
		`"state"`, `"anchor"`, `"entry_id"`, `"shard"`, `"built"`, `"planned"`,
		`"failed":true`, // derived, so a consumer never re-implements Failed()
	} {
		if !strings.Contains(string(b), key) {
			t.Errorf("report JSON has no %s key: %s", key, b)
		}
	}
}
