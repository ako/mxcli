// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A Call REST service activity carries Studio Pro's "Use a timeout" toggle and
// the seconds beside it. Mendix stores them as UseRequestTimeOut (bool) and
// TimeOutExpression (a STRING, not an int — "300", not 300), and before these
// columns existed neither reached the catalog, so the obvious rule
//
//	every RestCallAction must have a timeout
//
// could not be written at all: a rule could find the activity and learn
// nothing about how it was configured.
//
// The toggle is the part that matters. Unticking "Use a timeout" leaves the
// seconds in place, so a rule keyed on the expression being non-empty passes a
// call that has no timeout — which is why both columns are asserted here, and
// why the "off" case keeps its expression.
func TestActivitiesCarryTheRestCallTimeout(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()

	const modID = model.ID("mod-sales")

	act := func(id string, use bool, expr string) *microflows.ActionActivity {
		a := &microflows.ActionActivity{
			Action: &microflows.RestCallAction{
				UseRequestTimeOut: use,
				TimeoutExpression: expr,
			},
		}
		a.ID = model.ID(id)
		return a
	}

	mf := &microflows.Microflow{
		ContainerID: modID,
		Name:        "ACT_Rest_Calls",
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{
				act("a-on", true, "300"),
				act("a-off", false, "300"),
			},
		},
	}
	mf.ID = model.ID("mf-rest")

	b := &Builder{
		catalog:   cat,
		snapshot:  &Snapshot{ID: "snap"},
		hierarchy: &hierarchy{moduleIDs: map[model.ID]bool{modID: true}, moduleNames: map[model.ID]string{modID: "Sales"}},
		fullMode:  true,
		// Every cache is set so the builder never reaches for a reader.
		microflowCache:   []*microflows.Microflow{mf},
		nanoflowCache:    []*microflows.Nanoflow{},
		ruleCache:        []*microflows.Rule{},
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

	res, err := cat.Query(`SELECT Id, ActionType, UseRequestTimeout, TimeoutExpression
		FROM activities_data ORDER BY Id`)
	if err != nil {
		t.Fatalf("query: %v -- the columns must exist, or no rule can read a timeout at all", err)
	}
	if res.Count != 2 {
		t.Fatalf("got %d activity rows, want 2", res.Count)
	}

	type row struct {
		actionType string
		use        int64
		expr       string
	}
	got := map[string]row{}
	for _, r := range res.Rows {
		id, _ := r[0].(string)
		at, _ := r[1].(string)
		use, _ := r[2].(int64)
		expr, _ := r[3].(string)
		got[id] = row{at, use, expr}
	}

	if got["a-off"].actionType != "RestCallAction" || got["a-on"].actionType != "RestCallAction" {
		t.Fatalf("action types = %q / %q, want RestCallAction -- the switch never "+
			"matched, so the timeout could not have been read either",
			got["a-on"].actionType, got["a-off"].actionType)
	}
	if got["a-on"].use != 1 {
		t.Errorf("UseRequestTimeout for the ticked activity = %d, want 1 -- "+
			"the toggle is not reaching the row", got["a-on"].use)
	}
	if got["a-off"].use != 0 {
		t.Errorf("UseRequestTimeout for the unticked activity = %d, want 0 -- "+
			"a call with no timeout must not claim one", got["a-off"].use)
	}
	for id, want := range map[string]string{"a-on": "300", "a-off": "300"} {
		if got[id].expr != want {
			t.Errorf("TimeoutExpression for %s = %q, want %q -- Studio Pro keeps "+
				"the seconds when the toggle is off, which is exactly why a rule "+
				"must key on the toggle and not on this", id, got[id].expr, want)
		}
	}
}
