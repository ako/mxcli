// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// censusUnit is a unit holding one nanoflow-like element with the given keys,
// as the repair leaves it.
func censusUnit(t *testing.T, id, name string, keys ...string) PropertySetRepair {
	t.Helper()
	d := bson.D{{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: uuidToBlob(id)}}, {Key: "$Type", Value: "Microflows$Nanoflow"}}
	for _, k := range keys {
		d = append(d, bson.E{Key: k, Value: ""})
	}
	b, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return PropertySetRepair{UnitID: id, Type: "Microflows$Nanoflow", Name: name, normalized: b}
}

// ako/mxcli#1505: one nanoflow without the ExportLevel every other nanoflow of
// the project carries is reported, with that nanoflow as the example; a key the
// minority carries names the minority instead.
func TestPropertySetCensus_NamesTheElementsThatDiffer(t *testing.T) {
	c := NewPropertySetCensus()
	c.Count(censusUnit(t, "11111111-1111-4111-8111-111111111111", "A", "Name", "ExportLevel"))
	c.Count(censusUnit(t, "22222222-2222-4222-8222-222222222222", "B", "Name", "ExportLevel"))
	c.Count(censusUnit(t, "33333333-3333-4333-8333-333333333333", "ACT_Login", "Name", "Extra"))

	got := c.Mixed()
	if len(got) != 2 {
		t.Fatalf("Mixed() = %+v, want ExportLevel and Extra", got)
	}
	el, extra := got[0], got[1]
	if el.Key != "ExportLevel" || el.With != 2 || el.Elements != 3 || !el.Without() ||
		len(el.Examples) != 1 || el.Examples[0].Name != "ACT_Login" {
		t.Errorf("ExportLevel = %+v, want on 2 of 3, without it: ACT_Login", el)
	}
	if extra.Key != "Extra" || extra.With != 1 || extra.Without() ||
		len(extra.Examples) != 1 || extra.Examples[0].Name != "ACT_Login" {
		t.Errorf("Extra = %+v, want on 1 of 3, only on: ACT_Login", extra)
	}
}

// The control: elements of a type that all carry the same names report
// nothing, and a unit the repair could not normalize is not counted.
func TestPropertySetCensus_UniformTypeIsClean(t *testing.T) {
	c := NewPropertySetCensus()
	c.Count(censusUnit(t, "11111111-1111-4111-8111-111111111111", "A", "Name", "ExportLevel"))
	c.Count(censusUnit(t, "22222222-2222-4222-8222-222222222222", "B", "Name", "ExportLevel"))
	c.Count(PropertySetRepair{UnitID: "33333333-3333-4333-8333-333333333333"})
	if got := c.Mixed(); len(got) != 0 {
		t.Fatalf("Mixed() = %+v on a uniform type", got)
	}
}
