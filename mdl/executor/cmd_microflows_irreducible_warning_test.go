// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#923: DESCRIBE MICROFLOW rendered an un-nestable graph as
// nested IFs and produced a program with the opposite behaviour — the original
// always logged, the description never did. Until the label form lands the
// describer says so instead of handing back MDL that silently means something
// else.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// reporterCollection rebuilds the reporter's microflow from the coordinates in
// their own describe output:
//
//	split1: true → split2      false → merge1
//	split2: true → merge1      false → merge2
//	merge1 → log → merge2 → end
func reporterCollection() *microflows.MicroflowObjectCollection {
	at := func(id string, x, y int) microflows.BaseMicroflowObject {
		o := mkObj(id)
		o.Position = model.Point{X: x, Y: y}
		return o
	}
	return &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.StartEvent{BaseMicroflowObject: at("start", -450, 173)},
			&microflows.ExclusiveSplit{BaseMicroflowObject: at("split1", -335, 173),
				SplitCondition: &microflows.ExpressionSplitCondition{Expression: "not(true)"}},
			&microflows.ExclusiveSplit{BaseMicroflowObject: at("split2", -335, 336),
				SplitCondition: &microflows.ExpressionSplitCondition{Expression: "not(true)"}},
			&microflows.ExclusiveMerge{BaseMicroflowObject: at("merge1", -200, 173)},
			&microflows.ActionActivity{
				BaseActivity: microflows.BaseActivity{BaseMicroflowObject: at("log", -50, 173)},
				Action:       &microflows.LogMessageAction{LogLevel: "Info", LogNodeName: "'NODE'"}},
			&microflows.ExclusiveMerge{BaseMicroflowObject: at("merge2", 100, 173)},
			&microflows.EndEvent{BaseMicroflowObject: at("end", 200, 173)},
		},
		Flows: []*microflows.SequenceFlow{
			mkFlow("start", "split1"),
			mkBranchFlow("split1", "split2", microflows.EnumerationCase{Value: "true"}),
			mkBranchFlow("split1", "merge1", microflows.EnumerationCase{Value: "false"}),
			mkBranchFlow("split2", "merge1", microflows.EnumerationCase{Value: "true"}),
			mkBranchFlow("split2", "merge2", microflows.EnumerationCase{Value: "false"}),
			mkFlow("merge1", "log"), mkFlow("log", "merge2"), mkFlow("merge2", "end"),
		},
	}
}

func TestIrreducibleGraphWarnings_FlagsTheReporterGraph(t *testing.T) {
	got := irreducibleGraphWarnings(reporterCollection())
	if len(got) != 1 {
		t.Fatalf("want 1 warning, got %d: %v", len(got), got)
	}
	w := got[0]

	// It has to survive being pasted back as MDL, so it must be a comment.
	if !strings.HasPrefix(w, "-- WARNING:") {
		t.Errorf("warning is not an MDL comment: %q", w)
	}
	// The split's canvas position is the only way to find it in Studio Pro —
	// a merge has no name and the decision may have no caption.
	if !strings.Contains(w, "(-335, 173)") {
		t.Errorf("warning does not locate the decision: %q", w)
	}
	if !strings.Contains(w, "must not be re-executed") {
		t.Errorf("warning does not refuse the round trip, which is its whole purpose: %q", w)
	}
	if !strings.Contains(w, "recombinable") {
		t.Errorf("warning does not carry the classification: %q", w)
	}
}

// The regression that would matter far more than the bug: warning on ordinary
// microflows. Every existing describe test doubles as this control, but the
// common shapes are worth pinning explicitly.
func TestIrreducibleGraphWarnings_SilentOnNestedGraphs(t *testing.T) {
	ifElse := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.StartEvent{BaseMicroflowObject: mkObj("start")},
			&microflows.ExclusiveSplit{BaseMicroflowObject: mkObj("s"),
				SplitCondition: &microflows.ExpressionSplitCondition{Expression: "$x"}},
			&microflows.ActionActivity{
				BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("a")},
				Action:       &microflows.LogMessageAction{LogLevel: "Info", LogNodeName: "'App'"}},
			&microflows.ActionActivity{
				BaseActivity: microflows.BaseActivity{BaseMicroflowObject: mkObj("b")},
				Action:       &microflows.LogMessageAction{LogLevel: "Info", LogNodeName: "'App'"}},
			&microflows.ExclusiveMerge{BaseMicroflowObject: mkObj("m")},
			&microflows.EndEvent{BaseMicroflowObject: mkObj("end")},
		},
		Flows: []*microflows.SequenceFlow{
			mkFlow("start", "s"),
			mkBranchFlow("s", "a", microflows.EnumerationCase{Value: "true"}),
			mkBranchFlow("s", "b", microflows.EnumerationCase{Value: "false"}),
			mkFlow("a", "m"), mkFlow("b", "m"), mkFlow("m", "end"),
		},
	}

	if got := irreducibleGraphWarnings(ifElse); len(got) != 0 {
		t.Errorf("plain if/else warned: %v", got)
	}
	if got := irreducibleGraphWarnings(nil); len(got) != 0 {
		t.Errorf("nil collection warned: %v", got)
	}
}
