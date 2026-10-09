// SPDX-License-Identifier: Apache-2.0

package executor

// Mode 2: the labelling half. The emission half is pinned end-to-end by
// mdl-examples/bug-tests/923-crossed-branches, because the thing that matters is
// whether the emitted MDL rebuilds the graph it came from.

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// crossedFixture builds mendixlabs/mxcli#923's graph:
//
//	split1 : true → split2      false → merge1
//	split2 : true → merge1      false → merge2
//	merge1 → act → merge2 → end
//
// merge1 is the shared suffix: reached from split1's false arm AND split2's true
// arm. No nesting of if/then/else places it.
func crossedFixture() *rejoinFixture {
	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("split1", &microflows.ExclusiveSplit{
		BaseMicroflowObject: f.base(100),
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$A"},
	})
	f.add("split2", &microflows.ExclusiveSplit{
		BaseMicroflowObject: f.base(200),
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$B"},
	})
	f.add("merge1", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(300)})
	f.add("act", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(350)}})
	f.add("merge2", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(400)})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(500)})
	f.edge("start", "split1", false)
	f.branch("split1", "split2", true)
	f.branch("split1", "merge1", false)
	f.branch("split2", "merge1", true)
	f.branch("split2", "merge2", false)
	f.edge("merge1", "act", false)
	f.edge("act", "merge2", false)
	f.edge("merge2", "end", false)
	return f
}

func TestLabelCrossedMerges_LabelsTheSharedSuffix(t *testing.T) {
	f := crossedFixture()
	labels := labelCrossedMerges(f.col)

	// Both the shared entry and the post-dominator are labelled, so every branch
	// can end in an explicit `join`. Labelling only the entry leaves split2's
	// EMPTY false arm falling through into it, which describes a different graph.
	if labels.len() != 2 {
		t.Fatalf("labelled %d merges, want 2 (the shared suffix and the join): %v", labels.len(), labels.byID)
	}
	if _, ok := labels.of(f.ids["merge1"]); !ok {
		t.Error("merge1 is the shared suffix and was not labelled")
	}
	if _, ok := labels.of(f.ids["merge2"]); !ok {
		t.Error("merge2 is the post-dominator; without a label the empty branch has nothing to join")
	}
	if !labels.isCrossed(f.ids["merge1"]) {
		t.Error("merge1 is not in the crossed set, so branches would fall into it instead of joining it")
	}
}

// THE regression control for Mode 2. A properly nested graph must gain no
// labels, so its description is byte-identical to today's. Every microflow with
// an ordinary decision is this shape.
func TestLabelCrossedMerges_NestedGraphGetsNoLabels(t *testing.T) {
	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("split", &microflows.ExclusiveSplit{
		BaseMicroflowObject: f.base(100),
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$X"},
	})
	f.add("then", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(150)}})
	f.add("else", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(160)}})
	f.add("merge", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(200)})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(300)})
	f.edge("start", "split", false)
	f.branch("split", "then", true)
	f.branch("split", "else", false)
	f.edge("then", "merge", false)
	f.edge("else", "merge", false)
	f.edge("merge", "end", false)

	labels := labelCrossedMerges(f.col)
	if labels.len() != 0 || len(labels.crossed) != 0 {
		t.Errorf("a properly nested graph was labelled: %v / %v", labels.byID, labels.crossed)
	}
}

// An interleaved `if` overlaps at more than one entry. When every entry is a
// merge, merge/join spells it — each arm joins the region it reaches, each
// region is printed once (TestDescribeInterleavedIf_*) — so the entries are
// labelled. Evora's OIDC.GetLoginEndpoint is this shape.
func TestLabelCrossedMerges_LabelsAnInterleavedIfWhoseEntriesAreMerges(t *testing.T) {
	f := interleavedFixture(func(f *rejoinFixture, id string, x int) {
		f.add(id, &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(x)})
	})
	labels := labelCrossedMerges(f.col)
	for _, id := range []string{"d", "e"} {
		if !labels.isCrossed(f.ids[id]) {
			t.Errorf("entry %s of an interleaved if is not labelled crossed", id)
		}
	}
}

// Control: an entry that is an activity has no merge to name, so the overlap
// is not describable with merge/join and keeps MDL-FLOW01.
func TestLabelCrossedMerges_IgnoresAnInterleavedIfWithAnActivityEntry(t *testing.T) {
	f := interleavedFixture(func(f *rejoinFixture, id string, x int) {
		f.add(id, &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(x)}})
	})
	labels := labelCrossedMerges(f.col)
	if labels.len() != 0 {
		t.Errorf("labelled an interleaved overlap entered at an activity: %v", labels.byID)
	}
}

// interleavedFixture is A → {B, C}; B → {D, E}; C → {D, E}: D and E are both
// entries. entry adds D and E.
func interleavedFixture(entry func(f *rejoinFixture, id string, x int)) *rejoinFixture {
	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("a", &microflows.ExclusiveSplit{
		BaseMicroflowObject: f.base(100),
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$A"},
	})
	f.add("b", &microflows.ExclusiveSplit{
		BaseMicroflowObject: f.base(200),
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$B"},
	})
	f.add("c", &microflows.ExclusiveSplit{
		BaseMicroflowObject: f.base(210),
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$C"},
	})
	entry(f, "d", 300)
	entry(f, "e", 310)
	f.add("tail", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(400)})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(500)})
	f.edge("start", "a", false)
	f.branch("a", "b", true)
	f.branch("a", "c", false)
	f.branch("b", "d", true)
	f.branch("b", "e", false)
	f.branch("c", "d", true)
	f.branch("c", "e", false)
	f.edge("d", "tail", false)
	f.edge("e", "tail", false)
	f.edge("tail", "end", false)

	return f
}

// Labels must not depend on map iteration, or every re-describe is a diff.
func TestLabelCrossedMerges_IsDeterministic(t *testing.T) {
	first := ""
	for i := 0; i < 50; i++ {
		f := crossedFixture()
		labels := labelCrossedMerges(f.col)
		l, _ := labels.of(f.ids["merge1"])
		if i == 0 {
			first = l
			continue
		}
		if l != first {
			t.Fatalf("run %d labelled the shared suffix %q, first run gave %q", i, l, first)
		}
	}
}

// A rejoin label wins over a crossed one for the same merge, but the merge still
// joins the crossed set: the label says what to call it, the set says how to
// emit it.
func TestMergeAllLabels_RejoinLabelWins(t *testing.T) {
	rejoin := mergeLabels{byID: map[model.ID]string{"m": "rejoin1"}}
	crossed := mergeLabels{
		byID:    map[model.ID]string{"m": "shared1", "n": "shared2"},
		crossed: map[model.ID]bool{"m": true, "n": true},
	}
	got := mergeAllLabels(rejoin, crossed)
	if l, _ := got.of("m"); l != "rejoin1" {
		t.Errorf("merge m labelled %q, want rejoin1 — Phase E's names are already in saved scripts", l)
	}
	if l, _ := got.of("n"); l != "shared2" {
		t.Errorf("merge n labelled %q, want shared2", l)
	}
	// The label says what to call it; the set says how to emit it.
	if !got.isCrossed("m") {
		t.Error("merge m kept its rejoin NAME but must still be emitted as a crossed merge")
	}
}
