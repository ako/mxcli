// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/version"
)

func v(s string) *version.Version { x := version.Parse(s); return &x }

func mustMarshal(t *testing.T, d bson.D) []byte {
	t.Helper()
	b, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func keysOf(t *testing.T, raw []byte, path ...string) map[string]any {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	for _, p := range path {
		var next bson.D
		for _, e := range d {
			if e.Key == p {
				next, _ = e.Value.(bson.D)
			}
		}
		if next == nil {
			t.Fatalf("no sub-document %q", p)
		}
		d = next
	}
	out := map[string]any{}
	for _, e := range d {
		out[e.Key] = e.Value
	}
	return out
}

// A loop written without Documentation is the exact element of #1373's
// microflow case: Mendix's merge throws on it once Studio Pro has saved the
// document with the property.
func loopUnit(t *testing.T, withDoc bool) []byte {
	loop := bson.D{
		{Key: "$ID", Value: freshID()},
		{Key: "$Type", Value: "Microflows$LoopedActivity"},
		{Key: "ErrorHandlingType", Value: "Rollback"},
	}
	if withDoc {
		loop = append(loop, bson.E{Key: "Documentation", Value: "keep me"})
	}
	return mustMarshal(t, bson.D{
		{Key: "$ID", Value: freshID()},
		{Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "Loop", Value: loop},
	})
}

func TestCompletePropertySets_AddsMeasuredKey(t *testing.T) {
	out := CompletePropertySets(loopUnit(t, false), v("11.14.0"))
	got := keysOf(t, out, "Loop")
	if doc, ok := got["Documentation"]; !ok || doc != "" {
		t.Fatalf("Documentation = %#v (present %v), want the measured \"\"", doc, ok)
	}
}

func TestCompletePropertySets_KeepsStatedValue(t *testing.T) {
	out := CompletePropertySets(loopUnit(t, true), v("11.14.0"))
	if got := keysOf(t, out, "Loop")["Documentation"]; got != "keep me" {
		t.Fatalf("Documentation = %#v, want the value the writer stated", got)
	}
}

// Nothing to add returns the same bytes, not a re-marshalled copy: every write
// passes through here, and an untouched unit must reach elision as it was.
func TestCompletePropertySets_NothingMissingIsIdentity(t *testing.T) {
	in := loopUnit(t, true)
	out := CompletePropertySets(in, v("11.14.0"))
	if &out[0] != &in[0] {
		t.Fatal("a complete unit was re-marshalled")
	}
}

// The version floor: a key is never added to a project older than the oldest
// version it was measured on — an undeclared key makes the document unopenable,
// which is worse than the gap.
func TestCompletePropertySets_RespectsVersionFloor(t *testing.T) {
	defs := propertyDefaults()
	var since version.Version
	for _, d := range defs["Microflows$LoopedActivity"] {
		if d.key == "Documentation" {
			since = d.since
		}
	}
	if since.IsZero() {
		t.Fatal("table has no Microflows$LoopedActivity.Documentation entry")
	}
	older := version.Version{Major: since.Major, Minor: since.Minor - 1}
	if since.Minor == 0 {
		older = version.Version{Major: since.Major - 1, Minor: 99}
	}
	in := loopUnit(t, false)
	if out := CompletePropertySets(in, &older); !bytes.Equal(out, in) {
		t.Fatalf("added a key to a %s project, measured only from %s", older, since)
	}
	if out := CompletePropertySets(in, nil); !bytes.Equal(out, in) {
		t.Fatal("added a key with no project version")
	}
}

// A default that is itself an element gets an $ID of its own — two elements
// sharing one make the project unopenable — and the SAME one every time the same
// owner is completed: a random id made every re-encoding differ, and the raw
// passthrough of `call web service raw` compares bytes, so each re-run of a
// script read as a change (TestFlowRerunProperty, 06b-soap-examples).
func TestCompletePropertySets_ElementDefaultsGetDistinctStableIDs(t *testing.T) {
	var typ, key string
	for tn, ds := range propertyDefaults() {
		for _, d := range ds {
			if sub, ok := d.value.(bson.D); ok && hasKey(sub, "$ID") {
				typ, key = tn, d.key
			}
		}
	}
	if typ == "" {
		t.Skip("table has no element-valued default")
	}
	el := func() bson.D { return bson.D{{Key: "$ID", Value: freshID()}, {Key: "$Type", Value: typ}} }
	in := mustMarshal(t, bson.D{
		{Key: "$ID", Value: freshID()},
		{Key: "$Type", Value: "Test$Holder"},
		{Key: "A", Value: el()},
		{Key: "B", Value: el()},
	})
	out := CompletePropertySets(in, v("99.0.0"))
	a, _ := keysOf(t, out, "A")[key].(bson.D)
	b, _ := keysOf(t, out, "B")[key].(bson.D)
	if a == nil || b == nil {
		t.Fatalf("%s.%s not added as an element", typ, key)
	}
	idOf := func(d bson.D) []byte {
		for _, e := range d {
			if e.Key == "$ID" {
				return e.Value.(bson.Binary).Data
			}
		}
		return nil
	}
	if bytes.Equal(idOf(a), idOf(b)) || bytes.Equal(idOf(a), make([]byte, 16)) {
		t.Fatalf("element defaults share or blank their $ID: %x / %x", idOf(a), idOf(b))
	}
	if again := CompletePropertySets(in, v("99.0.0")); !bytes.Equal(again, out) {
		t.Fatal("completing the same document twice produced different bytes")
	}
}

// The table is embedded data every write depends on; a malformed one would make
// propertyDefaults silently empty.
func TestPropertyDefaultsTableParses(t *testing.T) {
	m, err := parsePropertyDefaults(propertyDefaultsJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) == 0 {
		t.Fatal("empty table")
	}
	for typ, ds := range m {
		seen := map[string]bool{}
		for _, d := range ds {
			if seen[d.key] {
				t.Errorf("%s.%s listed twice", typ, d.key)
			}
			seen[d.key] = true
		}
	}
}

// Completing a unit decodes and re-encodes the whole document, so everything it
// does not add must come back byte for byte. Measured on every unit of the
// Studio Pro-authored fixtures: decode → encode is the identity.
func TestCompletePropertySets_ReencodeIsLossless(t *testing.T) {
	n := 0
	for _, dir := range []string{"../../testdata/pedapp/mprcontents", "../../testdata/testapp-views/mprcontents"} {
		_ = filepath.WalkDir(dir, func(p string, de fs.DirEntry, err error) error {
			if err != nil || de.IsDir() || !strings.HasSuffix(p, ".mxunit") {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			var d bson.D
			if err := bson.Unmarshal(raw, &d); err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			out, err := bson.Marshal(d)
			if err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			if !bytes.Equal(out, raw) {
				t.Errorf("%s: decode → encode changed the bytes", p)
			}
			n++
			return nil
		})
	}
	if n == 0 {
		t.Fatal("no fixture units found")
	}
}

// The upper bound: a key a newer measurement showed renamed or removed is not
// written to a project at or past its Until — ConsumedODataService's 10.24
// ConfigurationMicroflow, which 11.14 calls ConfigurationEntityMicroflow, is
// the measured case. Writing it to 11.14 was caught by re-measuring.
func TestCompletePropertySets_RespectsUntil(t *testing.T) {
	m, err := parsePropertyDefaults([]byte(`[{"type":"Test$T","key":"Old","since":"10.24.0","until":"10.25.0","value":{"v":""}}]`))
	if err != nil {
		t.Fatal(err)
	}
	d := m["Test$T"][0]
	for ver, want := range map[string]bool{"10.23.9": false, "10.24.0": true, "10.24.99": true, "10.25.0": false, "11.14.0": false} {
		if got := d.appliesTo(version.Parse(ver)); got != want {
			t.Errorf("appliesTo(%s) = %v, want %v", ver, got, want)
		}
	}
	if _, err := parsePropertyDefaults([]byte(`[{"type":"Test$T","key":"K","since":"11.0.0","until":"11.0.0","value":{"v":""}}]`)); err == nil {
		t.Error("an until not after since was accepted")
	}

	in := mustMarshal(t, bson.D{{Key: "$ID", Value: freshID()}, {Key: "$Type", Value: "Rest$ConsumedODataService"}})
	got := keysOf(t, CompletePropertySets(in, v("11.14.0")))
	if _, ok := got["ConfigurationMicroflow"]; ok {
		t.Error("wrote the 10.24 ConfigurationMicroflow to an 11.14 project")
	}
	if _, ok := got["ConfigurationEntityMicroflow"]; !ok {
		t.Error("did not write 11.14's ConfigurationEntityMicroflow")
	}
}
