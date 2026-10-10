// SPDX-License-Identifier: Apache-2.0

package propertysets

import (
	"os"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func writeUnit(t *testing.T, dir, rel string, d bson.D) {
	t.Helper()
	b, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func id(b byte) bson.Binary {
	return bson.Binary{Subtype: 0, Data: []byte{b, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}}
}

// Measure pairs elements by $ID: a key only the converted side has is a gap
// with Mendix's value, a key only the written side has is an extra, and a unit
// mxcli did not change (per the snapshot) is not judged.
func TestMeasureUnits(t *testing.T) {
	w, c := t.TempDir(), t.TempDir()
	writeUnit(t, w, "a/a/one.mxunit", bson.D{{Key: "$ID", Value: id(1)}, {Key: "$Type", Value: "T$X"}, {Key: "Old", Value: int32(1)}})
	writeUnit(t, c, "a/a/one.mxunit", bson.D{{Key: "$ID", Value: id(1)}, {Key: "$Type", Value: "T$X"}, {Key: "New", Value: ""}})
	writeUnit(t, w, "b/b/two.mxunit", bson.D{{Key: "$ID", Value: id(2)}, {Key: "$Type", Value: "T$Y"}})
	writeUnit(t, c, "b/b/two.mxunit", bson.D{{Key: "$ID", Value: id(2)}, {Key: "$Type", Value: "T$Y"}, {Key: "Pre", Value: true}})

	before, err := Snapshot(w)
	if err != nil {
		t.Fatal(err)
	}
	// one.mxunit is rewritten by "mxcli" after the snapshot; two.mxunit is not.
	writeUnit(t, w, "a/a/one.mxunit", bson.D{{Key: "$ID", Value: id(1)}, {Key: "$Type", Value: "T$X"}, {Key: "Old", Value: int32(2)}})

	res, err := MeasureUnits(w, c, before)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Gaps) != 1 || res.Gaps[0].Type != "T$X" || res.Gaps[0].Key != "New" || res.Gaps[0].Values[0] != `{"v":""}` {
		t.Errorf("gaps = %+v, want only T$X.New = \"\"", res.Gaps)
	}
	if len(res.Extras) != 1 || res.Extras[0].Key != "Old" {
		t.Errorf("extras = %+v, want only T$X.Old", res.Extras)
	}
	if !res.Declared["T$X"]["New"] || res.Declared["T$Y"] != nil {
		t.Errorf("declared = %v", res.Declared)
	}
}

func TestTable(t *testing.T) {
	r1024 := &Result{
		Gaps: []Gap{
			{Type: "T$X", Key: "Both", Values: []string{`{"v":""}`}},
			{Type: "T$X", Key: "Renamed", Values: []string{`{"v":""}`}},
			{Type: "T$X", Key: "Varies", Values: []string{`{"v":"a"}`}},
		},
		Declared: map[string]map[string]bool{"T$X": {"Both": true, "Renamed": true, "Varies": true}},
	}
	r1114 := &Result{
		Gaps: []Gap{
			{Type: "T$X", Key: "Both", Values: []string{`{"v":""}`}},
			{Type: "T$X", Key: "Varies", Values: []string{`{"v":"b"}`}},
			{Type: "T$X", Key: "NewOnly", Values: []string{`{"v":true}`}},
		},
		Declared: map[string]map[string]bool{"T$X": {"Both": true, "Varies": true, "NewOnly": true}},
	}
	entries, varying := Table(map[string]*Result{"10.24.24": r1024, "11.14.0": r1114})
	got := map[string]Entry{}
	for _, e := range entries {
		got[e.Key] = e
	}
	for key, want := range map[string][2]string{
		"Both":    {"10.24.24", ""},
		"NewOnly": {"11.14.0", ""},
		"Renamed": {"10.24.24", "10.25.0"},
	} {
		e, ok := got[key]
		if !ok {
			t.Errorf("%s: missing", key)
			continue
		}
		if e.Since != want[0] || e.Until != want[1] {
			t.Errorf("%s: since %q until %q, want %q %q", key, e.Since, e.Until, want[0], want[1])
		}
	}
	if len(varying) != 1 || varying[0] != "T$X.Varies" {
		t.Errorf("varying = %v, want [T$X.Varies]", varying)
	}
	if _, ok := got["Varies"]; ok {
		t.Error("a value that differs between versions became a constant")
	}
}

// Merging into the committed table: an older measurement lowers Since, a value
// that contradicts the table changes nothing and is reported, and a key seen
// again past its Until moves the bound rather than dropping it.
func TestMerge_IntoExistingTable(t *testing.T) {
	existing := []Entry{
		{Type: "T$X", Key: "Lower", Since: "11.14.0", Value: []byte(`{"v":""}`)},
		{Type: "T$X", Key: "Clash", Since: "11.14.0", Value: []byte(`{"v":"a"}`)},
		{Type: "T$X", Key: "Bounded", Since: "10.24.0", Until: "10.25.0", Value: []byte(`{"v":null}`)},
	}
	r116 := &Result{
		Gaps: []Gap{
			{Type: "T$X", Key: "Lower", Values: []string{`{"v":""}`}},
			{Type: "T$X", Key: "Clash", Values: []string{`{"v":"b"}`}},
			{Type: "T$X", Key: "Bounded", Values: []string{`{"v":null}`}},
		},
		Declared: map[string]map[string]bool{"T$X": {"Lower": true, "Clash": true, "Bounded": true}},
	}
	entries, varying := Merge(existing, map[string]*Result{"11.6.0": r116})
	got := map[string]Entry{}
	for _, e := range entries {
		got[e.Key] = e
	}
	if got["Lower"].Since != "11.6.0" {
		t.Errorf("Lower: since %q, want lowered to 11.6.0", got["Lower"].Since)
	}
	if string(got["Clash"].Value) != `{"v":"a"}` || len(varying) != 1 || varying[0] != "T$X.Clash" {
		t.Errorf("Clash: value %s, varying %v — a contradicting measurement must change nothing and be reported", got["Clash"].Value, varying)
	}
	if got["Bounded"].Until != "11.7.0" {
		t.Errorf("Bounded: until %q, want moved just past the new sighting on 11.6.0 (11.7.0) — "+
			"the version that set the bound is not in the table, so it must not disappear", got["Bounded"].Until)
	}
}
