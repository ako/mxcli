// SPDX-License-Identifier: Apache-2.0

package microflowgraph

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func obj(id string) microflows.BaseMicroflowObject {
	return microflows.BaseMicroflowObject{BaseElement: model.BaseElement{ID: model.ID(id)}}
}

func split(id string) *microflows.ExclusiveSplit {
	return &microflows.ExclusiveSplit{
		BaseMicroflowObject: obj(id),
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$x"},
	}
}

func merge(id string) *microflows.ExclusiveMerge {
	return &microflows.ExclusiveMerge{BaseMicroflowObject: obj(id)}
}

func act(id string) *microflows.ActionActivity {
	return &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{BaseMicroflowObject: obj(id)},
		Action:       &microflows.LogMessageAction{LogLevel: "Info", LogNodeName: "'App'"},
	}
}

func flow(from, to string) *microflows.SequenceFlow {
	return &microflows.SequenceFlow{OriginID: model.ID(from), DestinationID: model.ID(to)}
}

func errFlow(from, to string) *microflows.SequenceFlow {
	f := flow(from, to)
	f.IsErrorHandler = true
	return f
}

func findingFor(t *testing.T, fs []Finding, splitID string) Finding {
	t.Helper()
	for _, f := range fs {
		if f.SplitID == model.ID(splitID) {
			return f
		}
	}
	t.Fatalf("no finding for split %q; got %+v", splitID, fs)
	return Finding{}
}

// The reporter's microflow (mendixlabs/mxcli#923), reconstructed from the
// coordinates in their own describe output:
//
//	split1: true → split2      false → merge1
//	split2: true → merge1      false → merge2
//	merge1 → log → merge2 → end
//
// `log` runs on ¬c1 ∨ (c1 ∧ c2); the describer rendered it as c1 ∧ c2, dropping
// the split1-false edge. split1's branches both reach merge1 before their join,
// which is what makes the graph un-nestable.
func TestAnalyze_ReporterGraphIsRecombinable(t *testing.T) {
	objects := []microflows.MicroflowObject{
		&microflows.StartEvent{BaseMicroflowObject: obj("start")},
		split("split1"), split("split2"), merge("merge1"), act("log"), merge("merge2"),
		&microflows.EndEvent{BaseMicroflowObject: obj("end")},
	}
	flows := []*microflows.SequenceFlow{
		flow("start", "split1"),
		flow("split1", "split2"), flow("split1", "merge1"),
		flow("split2", "merge1"), flow("split2", "merge2"),
		flow("merge1", "log"), flow("log", "merge2"), flow("merge2", "end"),
	}

	found := Analyze(objects, flows)
	if len(found) != 1 {
		t.Fatalf("want exactly 1 finding (split1), got %d: %+v", len(found), found)
	}
	f := findingFor(t, found, "split1")

	if f.Class != Recombinable {
		t.Errorf("Class = %q, want %q — the overlap is a single shared suffix", f.Class, Recombinable)
	}
	// merge2 is the only node on every path out of split1: the split2→merge2
	// branch bypasses merge1, so merge1 does NOT post-dominate split1. That is
	// precisely why the describer's nested rendering cannot be faithful.
	if f.JoinID != model.ID("merge2") {
		t.Errorf("JoinID = %q, want merge2", f.JoinID)
	}
	if got, want := len(f.Overlap), 2; got != want {
		t.Errorf("Overlap = %v, want merge1 and log", f.Overlap)
	}
	if len(f.Entries) != 1 || f.Entries[0] != model.ID("merge1") {
		t.Errorf("Entries = %v, want [merge1] — one entry is what makes it a shared suffix", f.Entries)
	}
}

// The negative control that matters most: ordinary nested structures must not be
// flagged. A false positive here would put a warning on almost every microflow
// in every project, which is worse than the bug.
func TestAnalyze_NestedShapesAreNotFlagged(t *testing.T) {
	cases := []struct {
		name    string
		objects []microflows.MicroflowObject
		flows   []*microflows.SequenceFlow
	}{
		{
			name: "if/else",
			objects: []microflows.MicroflowObject{
				split("s"), act("a"), act("b"), merge("m"),
				&microflows.EndEvent{BaseMicroflowObject: obj("end")},
			},
			flows: []*microflows.SequenceFlow{
				flow("s", "a"), flow("s", "b"),
				flow("a", "m"), flow("b", "m"), flow("m", "end"),
			},
		},
		{
			name: "if with no else (false goes straight to the merge)",
			objects: []microflows.MicroflowObject{
				split("s"), act("a"), merge("m"),
				&microflows.EndEvent{BaseMicroflowObject: obj("end")},
			},
			flows: []*microflows.SequenceFlow{
				flow("s", "a"), flow("s", "m"),
				flow("a", "m"), flow("m", "end"),
			},
		},
		{
			// The shape most likely to be mis-flagged: an inner split whose join
			// IS the outer split's join. Both splits share m, but no node is
			// reachable from two branches of the same split.
			name: "nested splits sharing one join",
			objects: []microflows.MicroflowObject{
				split("outer"), split("inner"), act("x"), merge("m"),
				&microflows.EndEvent{BaseMicroflowObject: obj("end")},
			},
			flows: []*microflows.SequenceFlow{
				flow("outer", "inner"), flow("outer", "m"),
				flow("inner", "x"), flow("inner", "m"),
				flow("x", "m"), flow("m", "end"),
			},
		},
		{
			// Branches that never rejoin — each returns. There is no join at all,
			// which must read as "nested", not as an error.
			name: "both branches return",
			objects: []microflows.MicroflowObject{
				split("s"), act("a"), act("b"),
				&microflows.EndEvent{BaseMicroflowObject: obj("e1")},
				&microflows.EndEvent{BaseMicroflowObject: obj("e2")},
			},
			flows: []*microflows.SequenceFlow{
				flow("s", "a"), flow("s", "b"),
				flow("a", "e1"), flow("b", "e2"),
			},
		},
		{
			name: "sequential independent splits",
			objects: []microflows.MicroflowObject{
				split("s1"), act("a"), merge("m1"), split("s2"), act("b"), merge("m2"),
				&microflows.EndEvent{BaseMicroflowObject: obj("end")},
			},
			flows: []*microflows.SequenceFlow{
				flow("s1", "a"), flow("s1", "m1"), flow("a", "m1"),
				flow("m1", "s2"),
				flow("s2", "b"), flow("s2", "m2"), flow("b", "m2"), flow("m2", "end"),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if found := Analyze(tc.objects, tc.flows); len(found) != 0 {
				t.Errorf("false positive on a properly nested graph: %+v", found)
			}
		})
	}
}

// The genuinely un-nestable shape: two branches that each reach two shared
// nodes. Structuring it needs activity duplication or a synthetic boolean, so it
// is refused rather than rewritten.
func TestAnalyze_CrossingDiamondIsInterleaved(t *testing.T) {
	objects := []microflows.MicroflowObject{
		split("a"), split("b"), split("c"), act("d"), act("e"), merge("m"),
		&microflows.EndEvent{BaseMicroflowObject: obj("end")},
	}
	flows := []*microflows.SequenceFlow{
		flow("a", "b"), flow("a", "c"),
		flow("b", "d"), flow("b", "e"),
		flow("c", "d"), flow("c", "e"),
		flow("d", "m"), flow("e", "m"), flow("m", "end"),
	}

	f := findingFor(t, Analyze(objects, flows), "a")
	if f.Class != Interleaved {
		t.Errorf("Class = %q, want %q — d and e are two separate entry points", f.Class, Interleaved)
	}
	if len(f.Entries) != 2 {
		t.Errorf("Entries = %v, want two (d and e)", f.Entries)
	}
}

// A retry loop's back edge makes the graph cyclic (issue #281). The
// post-dominator fixpoint must terminate and must not report the loop as
// irreducible — a back edge into a merge above the split is ordinary structure.
func TestAnalyze_RetryLoopBackEdgeTerminatesAndIsNotFlagged(t *testing.T) {
	objects := []microflows.MicroflowObject{
		merge("top"), act("call"), split("ok"), act("retry"),
		&microflows.EndEvent{BaseMicroflowObject: obj("end")},
	}
	flows := []*microflows.SequenceFlow{
		flow("top", "call"), flow("call", "ok"),
		flow("ok", "end"),
		flow("ok", "retry"), flow("retry", "top"), // back edge
	}

	if found := Analyze(objects, flows); len(found) != 0 {
		t.Errorf("retry loop reported as irreducible: %+v", found)
	}
}

// Error-handler flows are not branches of the condition. Counting them would
// flag every activity that has one.
func TestAnalyze_ErrorHandlerFlowsAreIgnored(t *testing.T) {
	objects := []microflows.MicroflowObject{
		split("s"), act("a"), act("handler"), merge("m"),
		&microflows.EndEvent{BaseMicroflowObject: obj("end")},
	}
	flows := []*microflows.SequenceFlow{
		flow("s", "a"), flow("s", "m"), flow("a", "m"),
		errFlow("a", "handler"), flow("handler", "m"),
		flow("m", "end"),
	}

	if found := Analyze(objects, flows); len(found) != 0 {
		t.Errorf("error-handler flow treated as a branch: %+v", found)
	}
}

// A loop body is its own graph. An irreducible shape inside one must still be
// found, and the loop must not merge its children's flows with the outer scope.
func TestAnalyze_LoopBodyIsAnalysedAsItsOwnScope(t *testing.T) {
	inner := []microflows.MicroflowObject{
		split("i1"), split("i2"), merge("im1"), act("ilog"), merge("im2"),
	}
	innerFlows := []*microflows.SequenceFlow{
		flow("i1", "i2"), flow("i1", "im1"),
		flow("i2", "im1"), flow("i2", "im2"),
		flow("im1", "ilog"), flow("ilog", "im2"),
	}
	loop := &microflows.LoopedActivity{
		BaseMicroflowObject: obj("loop"),
		ObjectCollection:    &microflows.MicroflowObjectCollection{Objects: inner, Flows: innerFlows},
	}
	objects := []microflows.MicroflowObject{
		&microflows.StartEvent{BaseMicroflowObject: obj("start")}, loop,
		&microflows.EndEvent{BaseMicroflowObject: obj("end")},
	}
	flows := []*microflows.SequenceFlow{flow("start", "loop"), flow("loop", "end")}

	f := findingFor(t, Analyze(objects, flows), "i1")
	if f.Class != Recombinable {
		t.Errorf("Class = %q, want %q", f.Class, Recombinable)
	}
}

// Determinism: the analysis walks Go maps, so without explicit sorting the
// output order (and the reported node lists) would vary run to run and make
// lint output churn.
func TestAnalyze_IsDeterministic(t *testing.T) {
	objects := []microflows.MicroflowObject{
		split("a"), split("b"), split("c"), act("d"), act("e"), merge("m"),
		&microflows.EndEvent{BaseMicroflowObject: obj("end")},
	}
	flows := []*microflows.SequenceFlow{
		flow("a", "b"), flow("a", "c"),
		flow("b", "d"), flow("b", "e"),
		flow("c", "d"), flow("c", "e"),
		flow("d", "m"), flow("e", "m"), flow("m", "end"),
	}

	first := Analyze(objects, flows)
	for i := 0; i < 25; i++ {
		got := Analyze(objects, flows)
		if len(got) != len(first) {
			t.Fatalf("finding count varies between runs: %d vs %d", len(got), len(first))
		}
		for j := range got {
			if got[j].SplitID != first[j].SplitID || got[j].Class != first[j].Class {
				t.Fatalf("order or classification varies between runs at %d", j)
			}
			if len(got[j].Overlap) != len(first[j].Overlap) {
				t.Fatalf("overlap varies between runs")
			}
			for k := range got[j].Overlap {
				if got[j].Overlap[k] != first[j].Overlap[k] {
					t.Fatalf("overlap order varies between runs")
				}
			}
		}
	}
}
