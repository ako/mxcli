// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1305: activities_for() returned every call and delete with
// its target empty —
//
//	ℹ Act_Loop MicroflowCallAction: action_ref="" entity_ref="" service_ref="" caption="" loop_depth=1 [DUMP001]
//	ℹ Act_Loop DeleteObjectAction: action_ref="" entity_ref="" service_ref="" caption="" loop_depth=1 [DUMP001]
//
// although refs_from() named both targets. A loop-scoped rule could not follow
// a call out of the loop, nor tell a call that runs in a task queue (and so
// does not run in the loop's transaction) from one that does.
func TestActivityCallAndDeleteTargets(t *testing.T) {
	loop := loopObj("loop", &microflows.LoopedActivity{
		LoopSource: &microflows.IterableList{ListVariableName: "Sales", VariableName: "Sale"},
		ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
			loopAction("call", &microflows.MicroflowCallAction{
				MicroflowCall: &microflows.MicroflowCall{Microflow: "Loops.Sub_Process"},
			}),
			loopAction("queued", &microflows.MicroflowCallAction{
				MicroflowCall: &microflows.MicroflowCall{
					Microflow:     "Loops.Sub_Process",
					QueueSettings: &microflows.QueueSettings{Queue: "Loops.SaleQueue"},
				},
			}),
			loopAction("java", &microflows.JavaActionCallAction{
				JavaAction:    "Loops.JA_Export",
				QueueSettings: &microflows.QueueSettings{Queue: "Loops.SaleQueue"},
			}),
			loopAction("delete", &microflows.DeleteObjectAction{DeleteVariable: "Sale"}),
			// A variable the flow cannot type: no guess.
			loopAction("delete-unknown", &microflows.DeleteObjectAction{DeleteVariable: "Unknown"}),
		}},
	})
	mf := &microflows.Microflow{ContainerID: loopTestModule, Name: "Act_Loop",
		Parameters: []*microflows.MicroflowParameter{
			{Name: "Sales", Type: &microflows.ListType{EntityQualifiedName: "Loops.Sale"}},
		},
		ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{loop}},
	}
	mf.ID = "mf-act"

	nfLoop := loopObj("nf-loop", &microflows.LoopedActivity{
		LoopSource: &microflows.IterableList{ListVariableName: "Sales", VariableName: "Sale"},
		ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
			loopAction("nf-call", &microflows.NanoflowCallAction{
				NanoflowCall: &microflows.NanoflowCall{Nanoflow: "Loops.Nav_Sub"},
			}),
			loopAction("js-call", &microflows.JavaScriptActionCallAction{
				JavaScriptAction: "NanoflowCommons.ClearLocalStorage",
			}),
		}},
	})
	nf := &microflows.Nanoflow{ContainerID: loopTestModule, Name: "Nav_Loop",
		Parameters: []*microflows.MicroflowParameter{
			{Name: "Sales", Type: &microflows.ListType{EntityQualifiedName: "Loops.Sale"}},
		},
		ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{nfLoop}},
	}
	nf.ID = "nf-nav"

	cat := buildFlowsForTest(t, []*microflows.Microflow{mf}, []*microflows.Nanoflow{nf}, nil)

	res, err := cat.Query(`SELECT Id, ActionRef, QueueRef, EntityRef, ServiceRef, LoopDepth
		FROM activities WHERE ActivityType = 'ActionActivity'`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	got := map[string][]any{}
	for _, r := range res.Rows {
		got[r[0].(string)] = r[1:]
	}
	want := map[string][]any{
		"call":           {"Loops.Sub_Process", "", "", "", int64(1)},
		"queued":         {"Loops.Sub_Process", "Loops.SaleQueue", "", "", int64(1)},
		"java":           {"Loops.JA_Export", "Loops.SaleQueue", "", "", int64(1)},
		"delete":         {"", "", "Loops.Sale", "", int64(1)},
		"delete-unknown": {"", "", "", "", int64(1)},
		"nf-call":        {"Loops.Nav_Sub", "", "", "", int64(1)},
		"js-call":        {"NanoflowCommons.ClearLocalStorage", "", "", "", int64(1)},
	}
	cols := []string{"ActionRef", "QueueRef", "EntityRef", "ServiceRef", "LoopDepth"}
	for id, w := range want {
		g, ok := got[id]
		if !ok {
			t.Errorf("no row for %s", id)
			continue
		}
		for i := range w {
			if g[i] != w[i] {
				t.Errorf("%s.%s = %#v, want %#v", id, cols[i], g[i], w[i])
			}
		}
	}
}
