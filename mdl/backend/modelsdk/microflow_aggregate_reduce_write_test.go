// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

// TestReduceFoldReachesStorage proves the half of #1004 that no amount of
// grammar work would fix: Mendix stores a Reduce's seed and result type in two
// properties the semantic model had no room for, so mxcli read them as nothing
// and wrote them back as nothing. The grammar gap made `reduce(...)` fail
// loudly; this one would have failed silently, deleting the user's fold from a
// microflow that still passed `mx check`.
//
// The expected values are measured, not assumed. Studio Pro writes both keys on
// every AggregateAction, not only on Reduce: in ako/TestApp's MicroflowReduce
// (Mendix 11.14) the All and Any activities each carry an empty initial value
// and a Boolean return type, even though Mendix's reference guide says a return
// type is "not applicable" to them. Attribute is likewise written as "" when
// unused, which is why it is asserted here rather than left absent.
func TestReduceFoldReachesStorage(t *testing.T) {
	proj := copyFixture(t)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	mf := &microflows.Microflow{
		ContainerID: mod.ID,
		Name:        "ZZ_ReduceFold",
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{
				actionActivity(&microflows.AggregateListAction{
					InputVariable:      "CarList",
					OutputVariable:     "Total",
					Function:           microflows.AggregateFunctionReduce,
					UseExpression:      true,
					Expression:         "$currentResult + 1",
					ReduceInitialValue: "0",
					ReduceReturnType:   &microflows.DecimalType{},
				}),
				actionActivity(&microflows.AggregateListAction{
					InputVariable:    "CarList",
					OutputVariable:   "AllMatch",
					Function:         microflows.AggregateFunctionAll,
					UseExpression:    true,
					Expression:       "true",
					ReduceReturnType: &microflows.BooleanType{},
				}),
			},
		},
	}
	if err := b.CreateMicroflow(mf); err != nil {
		t.Fatalf("CreateMicroflow: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	got := collectAggregateProps(t, readUnitBytes(t, proj, string(mf.ID)))
	if len(got) != 2 {
		t.Fatalf("stored %d aggregate actions, want 2: %#v", len(got), got)
	}

	want := []map[string]any{
		{
			"AggregateFunction":            "Reduce",
			"ReduceInitialValueExpression": "0",
			"Attribute":                    "",
		},
		{
			// ALL folds a Boolean and takes no seed, so the seed is stored empty
			// rather than omitted — matching the reference document.
			"AggregateFunction":            "All",
			"ReduceInitialValueExpression": "",
			"Attribute":                    "",
		},
	}
	wantReturnType := []string{"DataTypes$DecimalType", "DataTypes$BooleanType"}

	for i, w := range want {
		for k, v := range w {
			stored, ok := got[i][k]
			if !ok {
				t.Errorf("aggregate %d (%s): %s missing from the stored document", i, w["AggregateFunction"], k)
				continue
			}
			if stored != v {
				t.Errorf("aggregate %d (%s): %s = %#v, want %#v", i, w["AggregateFunction"], k, stored, v)
			}
		}
		rt, ok := got[i]["ReduceReturnDataType"].(bson.M)
		if !ok {
			t.Errorf("aggregate %d (%s): ReduceReturnDataType missing or not a document: %#v",
				i, w["AggregateFunction"], got[i]["ReduceReturnDataType"])
			continue
		}
		typeName, _ := rt["$Type"].(string)
		if typeName != wantReturnType[i] {
			t.Errorf("aggregate %d (%s): return type = %q, want %q",
				i, w["AggregateFunction"], typeName, wantReturnType[i])
		}
	}
}

// collectAggregateProps returns one flat map per stored aggregate activity, in
// document order.
func collectAggregateProps(t *testing.T, raw []byte) []map[string]any {
	t.Helper()

	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal unit: %v", err)
	}

	var out []map[string]any
	var walk func(any)
	walk = func(v any) {
		switch n := v.(type) {
		case bson.M:
			if typ, _ := n["$Type"].(string); typ == "Microflows$AggregateAction" {
				out = append(out, map[string]any(n))
			}
			for _, child := range n {
				walk(child)
			}
		case bson.D:
			m := bson.M{}
			for _, e := range n {
				m[e.Key] = e.Value
			}
			walk(m)
		case bson.A:
			for _, child := range n {
				walk(child)
			}
		}
	}
	walk(doc)
	return out
}
