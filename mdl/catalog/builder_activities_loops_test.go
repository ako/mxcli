// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1266: the activities table walked the top level of a flow
// only, so nothing inside a loop — nested loops included — was catalogued, and
// a Starlark rule could not see a retrieve, commit or delete in a loop. The
// reader loads the loop body (LoopedActivity.ObjectCollection); the builder's
// three near-copy loops (microflow, nanoflow, rule) never recursed into it.

const loopTestModule = model.ID("mod-loops")

func loopObj[T microflows.MicroflowObject](id string, o T) T {
	switch v := any(o).(type) {
	case *microflows.ActionActivity:
		v.ID = model.ID(id)
	case *microflows.LoopedActivity:
		v.ID = model.ID(id)
	case *microflows.StartEvent:
		v.ID = model.ID(id)
	case *microflows.EndEvent:
		v.ID = model.ID(id)
	case *microflows.ExclusiveSplit:
		v.ID = model.ID(id)
	case *microflows.Annotation:
		v.ID = model.ID(id)
	case *microflows.ExclusiveMerge:
		v.ID = model.ID(id)
	}
	return o
}

func loopAction(id string, a microflows.MicroflowAction) *microflows.ActionActivity {
	return loopObj(id, &microflows.ActionActivity{Action: a})
}

// nestedLoopBody is
//
//	start
//	loop outer
//	    change
//	    loop inner
//	        commit
//	    delete
//	end
func nestedLoopBody(prefix string) *microflows.MicroflowObjectCollection {
	inner := loopObj(prefix+"inner", &microflows.LoopedActivity{
		ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
			loopAction(prefix+"commit", &microflows.CommitObjectsAction{}),
		}},
	})
	outer := loopObj(prefix+"outer", &microflows.LoopedActivity{
		ObjectCollection: &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
			loopAction(prefix+"change", &microflows.ChangeObjectAction{}),
			inner,
			loopAction(prefix+"delete", &microflows.DeleteObjectAction{}),
		}},
	})
	return &microflows.MicroflowObjectCollection{Objects: []microflows.MicroflowObject{
		loopObj(prefix+"start", &microflows.StartEvent{}),
		outer,
		loopObj(prefix+"end", &microflows.EndEvent{}),
	}}
}

// buildFlowsForTest runs buildMicroflows in full mode over hand-built flows.
func buildFlowsForTest(t *testing.T, mfs []*microflows.Microflow, nfs []*microflows.Nanoflow, rules []*microflows.Rule) *Catalog {
	t.Helper()
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	if mfs == nil {
		mfs = []*microflows.Microflow{}
	}
	if nfs == nil {
		nfs = []*microflows.Nanoflow{}
	}
	if rules == nil {
		rules = []*microflows.Rule{}
	}
	b := &Builder{
		catalog:        cat,
		snapshot:       &Snapshot{ID: "snap"},
		hierarchy:      &hierarchy{moduleIDs: map[model.ID]bool{loopTestModule: true}, moduleNames: map[model.ID]string{loopTestModule: "Loops"}},
		fullMode:       true,
		microflowCache: mfs,
		nanoflowCache:  nfs,
		ruleCache:      rules,
		// Set so the builder never reaches for a reader.
		domainModelCache: []*domainmodel.DomainModel{},
	}
	tx, err := cat.CatalogDB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	b.tx = tx
	if err := b.buildMicroflows(); err != nil {
		t.Fatalf("buildMicroflows: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return cat
}

type loopRow struct {
	id, actionType, parent string
	depth, seq             int64
}

func loopRows(t *testing.T, cat *Catalog, flow string) []loopRow {
	t.Helper()
	res, err := cat.Query(`SELECT Id, ActionType, ParentLoopId, LoopDepth, Sequence
		FROM activities WHERE MicroflowQualifiedName = '` + flow + `' ORDER BY Sequence`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	var out []loopRow
	for _, r := range res.Rows {
		var lr loopRow
		lr.id, _ = r[0].(string)
		lr.actionType, _ = r[1].(string)
		lr.parent, _ = r[2].(string)
		lr.depth, _ = r[3].(int64)
		lr.seq, _ = r[4].(int64)
		out = append(out, lr)
	}
	return out
}

func assertNestedLoopRows(t *testing.T, cat *Catalog, flow, p string) {
	t.Helper()
	want := []loopRow{
		{p + "start", "", "", 0, 1},
		{p + "outer", "", "", 0, 2},
		{p + "change", "ChangeObjectAction", p + "outer", 1, 3},
		{p + "inner", "", p + "outer", 1, 4},
		{p + "commit", "CommitObjectsAction", p + "inner", 2, 5},
		{p + "delete", "DeleteObjectAction", p + "outer", 1, 6},
		{p + "end", "", "", 0, 7},
	}
	got := loopRows(t, cat, flow)
	if len(got) != len(want) {
		t.Fatalf("%s: got %d activity rows, want %d -- the loop bodies are not walked: %+v", flow, len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s row %d = %+v, want %+v", flow, i, got[i], want[i])
		}
	}
}

func TestActivitiesIncludeLoopBodies_Microflow(t *testing.T) {
	mf := &microflows.Microflow{ContainerID: loopTestModule, Name: "MF_Nested", ObjectCollection: nestedLoopBody("m-")}
	mf.ID = "mf-nested"
	cat := buildFlowsForTest(t, []*microflows.Microflow{mf}, nil, nil)
	assertNestedLoopRows(t, cat, "Loops.MF_Nested", "m-")
}

func TestActivitiesIncludeLoopBodies_Nanoflow(t *testing.T) {
	nf := &microflows.Nanoflow{ContainerID: loopTestModule, Name: "NF_Nested", ObjectCollection: nestedLoopBody("n-")}
	nf.ID = "nf-nested"
	cat := buildFlowsForTest(t, nil, []*microflows.Nanoflow{nf}, nil)
	assertNestedLoopRows(t, cat, "Loops.NF_Nested", "n-")
}

func TestActivitiesIncludeLoopBodies_Rule(t *testing.T) {
	rule := &microflows.Rule{ContainerID: loopTestModule, Name: "R_Nested", ObjectCollection: nestedLoopBody("r-")}
	rule.ID = "rule-nested"
	cat := buildFlowsForTest(t, nil, nil, []*microflows.Rule{rule})
	assertNestedLoopRows(t, cat, "Loops.R_Nested", "r-")
}

// ActivityCount keeps its top-level meaning — the bundled size rules are
// calibrated on it — and TotalActivityCount adds the loop bodies.
func TestTotalActivityCountIncludesLoopBodies(t *testing.T) {
	mf := &microflows.Microflow{ContainerID: loopTestModule, Name: "MF_Nested", ObjectCollection: nestedLoopBody("m-")}
	mf.ID = "mf-nested"
	nf := &microflows.Nanoflow{ContainerID: loopTestModule, Name: "NF_Nested", ObjectCollection: nestedLoopBody("n-")}
	nf.ID = "nf-nested"
	rule := &microflows.Rule{ContainerID: loopTestModule, Name: "R_Nested", ObjectCollection: nestedLoopBody("r-")}
	rule.ID = "rule-nested"
	cat := buildFlowsForTest(t, []*microflows.Microflow{mf}, []*microflows.Nanoflow{nf}, []*microflows.Rule{rule})

	res, err := cat.Query(`SELECT Name, ActivityCount, TotalActivityCount FROM microflows ORDER BY Name`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 3 {
		t.Fatalf("got %d flows, want 3", res.Count)
	}
	for _, r := range res.Rows {
		top, _ := r[1].(int64)
		total, _ := r[2].(int64)
		// Top level: the outer loop. Total: outer, change, inner, commit, delete.
		if top != 1 || total != 5 {
			t.Errorf("%v: ActivityCount=%d TotalActivityCount=%d, want 1 and 5", r[0], top, total)
		}
	}
}

// A stored action the reader does not model is labelled by its storage type.
// TestApp has three: Microflows$GenerateJumpToOptionsAction, which the catalog
// reported as "UnsupportedAction" — the name of mxcli's placeholder Go type.
func TestUnsupportedActionIsLabelledByStorageType(t *testing.T) {
	if got := getMicroflowActionType(&microflows.UnsupportedAction{StorageType: "Microflows$GenerateJumpToOptionsAction"}); got != "GenerateJumpToOptionsAction" {
		t.Errorf("label = %q, want GenerateJumpToOptionsAction", got)
	}
	if got := getMicroflowActionType(&microflows.UnsupportedAction{}); got != "UnsupportedAction" {
		t.Errorf("label without a storage type = %q, want UnsupportedAction", got)
	}
}
