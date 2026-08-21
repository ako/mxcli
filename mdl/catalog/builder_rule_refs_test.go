// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A rule is not an activity: Mendix can only call one from a decision's
// condition, so the action walk that feeds CATALOG.REFS never saw it and a rule
// called from a decision had no callers at all. #939's reporter read that as
// "the reference never resolves" — a second symptom with its own cause, which
// survived fixing the write path.
func TestCollectRuleCalls(t *testing.T) {
	split := &microflows.ExclusiveSplit{
		SplitCondition: &microflows.RuleSplitCondition{RuleQualifiedName: "Sample.Rule_IsActive"},
	}
	nested := &microflows.ExclusiveSplit{
		SplitCondition: &microflows.RuleSplitCondition{RuleQualifiedName: "Sample.Rule_InLoop"},
	}
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			split,
			&microflows.LoopedActivity{
				ObjectCollection: &microflows.MicroflowObjectCollection{
					Objects: []microflows.MicroflowObject{nested},
				},
			},
			// An expression split contributes nothing — it names no document.
			&microflows.ExclusiveSplit{
				SplitCondition: &microflows.ExpressionSplitCondition{Expression: "$x = 1"},
			},
		},
	}

	got := collectRuleCalls(oc)
	want := []string{"Sample.Rule_IsActive", "Sample.Rule_InLoop"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q — loop bodies are walked like collectActionActivities does", i, got[i], want[i])
		}
	}

	if refs := collectRuleCalls(nil); refs != nil {
		t.Errorf("nil collection = %v, want nil", refs)
	}
}
