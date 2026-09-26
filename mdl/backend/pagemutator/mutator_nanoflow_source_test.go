// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A Forms$NanoflowSource is FLAT — ForceFullObjects, Nanoflow, ParameterMappings
// directly on the source. Measured: 5 of 5 Studio Pro-authored nanoflow sources
// in Feedback v4.0.2 (Mendix 11.13.0). The nested Forms$NanoflowSettings shape
// mxcli used to write makes mxbuild report CE2633 "No nanoflow configured for
// the data source of this data view".
func TestSetWidgetDataSource_NanoflowIsFlat(t *testing.T) {
	dv := bson.D{
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Name", Value: "dv1"},
		{Key: "DataSource", Value: bson.D{{Key: "$Type", Value: "Forms$MicroflowSource"}}},
	}
	m := New(makeRawPage(dv), model.ID("unit-1"), nil)
	if err := m.SetWidgetDataSource("dv1", &pages.NanoflowSource{Nanoflow: "Mod.DS_NF"}); err != nil {
		t.Fatalf("SetWidgetDataSource: %v", err)
	}
	ds := bsonnav.DGetDoc(findWidgetForTest(t, m.rawData, "dv1"), "DataSource")
	var keys []string
	for _, e := range ds {
		keys = append(keys, e.Key)
	}
	sort.Strings(keys)
	want := "$ID,$Type,ForceFullObjects,Nanoflow,ParameterMappings"
	if got := strings.Join(keys, ","); got != want {
		t.Fatalf("keys = %s, want %s (Studio Pro's shape)", got, want)
	}
	if v := bsonnav.DGetString(ds, "Nanoflow"); v != "Mod.DS_NF" {
		t.Errorf("Nanoflow = %q", v)
	}
	if v, ok := bsonnav.DGet(ds, "ForceFullObjects").(bool); !ok || v {
		t.Errorf("ForceFullObjects = %#v, want false", bsonnav.DGet(ds, "ForceFullObjects"))
	}
	if pm, ok := bsonnav.DGet(ds, "ParameterMappings").(bson.A); !ok || len(pm) != 1 || pm[0] != int32(2) {
		t.Errorf("ParameterMappings = %#v, want [2]", bsonnav.DGet(ds, "ParameterMappings"))
	}
}

// The flow a source names must be found in BOTH shapes: Studio Pro's flat one,
// and the nested one mxcli wrote before the fix, which stays in projects.
func TestFlowFromDataSourceDoc_NanoflowBothShapes(t *testing.T) {
	flat := bson.D{
		{Key: "$Type", Value: "Forms$NanoflowSource"},
		{Key: "ForceFullObjects", Value: false},
		{Key: "Nanoflow", Value: "Mod.DS_Flat"},
		{Key: "ParameterMappings", Value: bson.A{int32(2)}},
	}
	if _, nf := flowFromDataSourceDoc(flat); nf != "Mod.DS_Flat" {
		t.Errorf("flat (Studio Pro) shape: nanoflow = %q, want Mod.DS_Flat", nf)
	}
	nested := bson.D{
		{Key: "$Type", Value: "Forms$NanoflowSource"},
		{Key: "NanoflowSettings", Value: bson.D{
			{Key: "$Type", Value: "Forms$NanoflowSettings"},
			{Key: "Nanoflow", Value: "Mod.DS_Nested"},
		}},
	}
	if _, nf := flowFromDataSourceDoc(nested); nf != "Mod.DS_Nested" {
		t.Errorf("nested (pre-fix mxcli) shape: nanoflow = %q, want Mod.DS_Nested", nf)
	}
}
