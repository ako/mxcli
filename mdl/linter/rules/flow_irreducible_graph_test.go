// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/microflowgraph"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The rule's Check() needs a project reader, so — following the pattern the
// other rule tests use — the pure part is tested directly. That Analyze() finds
// the right graphs is covered in mdl/microflowgraph; that the rule is wired into
// `mxcli lint` at all was established with a forced-fire control against a real
// project (a rule that never runs and a rule that finds nothing look identical
// from the outside).
func TestIrreducibleFlowGraph_MessageNamesTheDecisionAndRefusesTheRoundTrip(t *testing.T) {
	r := NewIrreducibleFlowGraphRule()
	mf := testMicroflow()

	split := &microflows.ExclusiveSplit{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID("s1")},
			Position:    model.Point{X: -335, Y: 173},
		},
	}

	v := r.violation(mf, microflowgraph.Finding{
		SplitID: "s1", Split: split, Class: microflowgraph.Recombinable,
		BranchCount: 2, Overlap: []model.ID{"m1", "log"}, Entries: []model.ID{"m1"},
	})

	if v.RuleID != "MDL-FLOW01" {
		t.Errorf("RuleID = %q, want MDL-FLOW01", v.RuleID)
	}
	if v.Severity != linter.SeverityInfo {
		t.Errorf("Severity = %v, want info — the model is valid; what fails is describing it", v.Severity)
	}
	if !strings.Contains(v.Message, "(-335, 173)") {
		t.Errorf("message does not locate the decision on the canvas: %s", v.Message)
	}
	if !strings.Contains(v.Message, "MyModule.ACT_Process") {
		t.Errorf("message does not name the microflow: %s", v.Message)
	}
	// The whole point of the rule is to stop someone re-executing a description
	// that means something else, so the suggestion has to say so.
	if !strings.Contains(v.Suggestion, "round-trip") {
		t.Errorf("suggestion does not warn against the round trip: %s", v.Suggestion)
	}
	if v.Location.DocumentName != "ACT_Process" || v.Location.Module != "MyModule" {
		t.Errorf("location not set: %+v", v.Location)
	}
}

// The two classes carry different advice: a recombinable graph can usually be
// folded into one decision by hand, an interleaved one cannot be expressed at all
// without changing the model. Telling a user to "fold the condition" on a graph
// where that is impossible would send them in circles.
func TestIrreducibleFlowGraph_InterleavedAdviceDiffersFromRecombinable(t *testing.T) {
	r := NewIrreducibleFlowGraphRule()
	mf := testMicroflow()

	rec := r.violation(mf, microflowgraph.Finding{
		Class: microflowgraph.Recombinable, BranchCount: 2,
		Overlap: []model.ID{"m1"}, Entries: []model.ID{"m1"},
	})
	inter := r.violation(mf, microflowgraph.Finding{
		Class: microflowgraph.Interleaved, BranchCount: 2,
		Overlap: []model.ID{"d", "e"}, Entries: []model.ID{"d", "e"},
	})

	if !strings.Contains(rec.Suggestion, "not(a) or b") {
		t.Errorf("recombinable case should offer the folded-condition escape: %s", rec.Suggestion)
	}
	if strings.Contains(inter.Suggestion, "not(a) or b") {
		t.Errorf("interleaved case must NOT suggest folding — it is not expressible: %s", inter.Suggestion)
	}
	if !strings.Contains(inter.Message, "cross") {
		t.Errorf("interleaved message should say the branches cross: %s", inter.Message)
	}
	if !strings.Contains(inter.Suggestion, "Studio Pro") {
		t.Errorf("interleaved case should send the user to Studio Pro: %s", inter.Suggestion)
	}
}
