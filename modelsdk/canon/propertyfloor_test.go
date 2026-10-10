// SPDX-License-Identifier: Apache-2.0

package canon_test

import (
	"bytes"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/version"
)

func ver(s string) *version.Version { v := version.Parse(s); return &v }

func id(b byte) bson.Binary {
	return bson.Binary{Subtype: 0, Data: []byte{b, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}}
}

// A consumed REST operation as the writer emits it on every version: an empty
// QueryParameters list, which Rest$RestOperation declares from 11.0.0 (the
// generated version data; measured on 10.24.24, where `mx convert` strips it).
func restOperation(t *testing.T, queryParams bson.A) []byte {
	t.Helper()
	b, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: id(1)},
		{Key: "$Type", Value: "Rest$ConsumedRestService"},
		{Key: "Operations", Value: bson.A{int32(2), bson.D{
			{Key: "$ID", Value: id(2)},
			{Key: "$Type", Value: "Rest$RestOperation"},
			{Key: "Name", Value: "AddItem"},
			{Key: "QueryParameters", Value: queryParams},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func hasKeyAnywhere(t *testing.T, raw []byte, key string) bool {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	var found bool
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			for _, e := range x {
				if e.Key == key {
					found = true
				}
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(d)
	return found
}

func TestStripUndeclared_EmptyKeyBelowItsVersionIsDropped(t *testing.T) {
	out, err := canon.StripUndeclaredProperties(restOperation(t, bson.A{int32(2)}), ver("10.24.24"))
	if err != nil {
		t.Fatal(err)
	}
	if hasKeyAnywhere(t, out, "QueryParameters") {
		t.Error("an empty QueryParameters stayed in a 10.24 document; Mendix 10 does not declare it")
	}
	if !hasKeyAnywhere(t, out, "Name") {
		t.Error("a declared key was dropped with it")
	}
}

// Control: from the version that declares it, the key stays and the bytes are
// returned untouched.
func TestStripUndeclared_KeyFromItsVersionStays(t *testing.T) {
	in := restOperation(t, bson.A{int32(2)})
	out, err := canon.StripUndeclaredProperties(in, ver("11.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, in) {
		t.Error("a document the version fully declares was changed")
	}
	if out, _ := canon.StripUndeclaredProperties(in, nil); !bytes.Equal(out, in) {
		t.Error("stripped with no project version")
	}
}

// A value the version cannot store is refused, not dropped: dropping it would
// write a different model than the statement said.
func TestStripUndeclared_ValueBelowItsVersionIsRefused(t *testing.T) {
	param := bson.D{{Key: "$ID", Value: id(3)}, {Key: "$Type", Value: "Rest$QueryParameter"}, {Key: "Name", Value: "q"}}
	_, err := canon.StripUndeclaredProperties(restOperation(t, bson.A{int32(2), param}), ver("10.24.24"))
	if err == nil {
		t.Fatal("a query parameter was accepted into a Mendix 10 document")
	}
	for _, want := range []string{"Rest$RestOperation.QueryParameters", "11.0.0", "10.24.24"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %q: %v", want, err)
		}
	}
}

// The measured floors cover keys the generated data has none for.
func TestStripUndeclared_MeasuredFloor(t *testing.T) {
	in, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: id(4)},
		{Key: "$Type", Value: "Navigation$NavigationProfile"},
		{Key: "ThrowPartialSyncError", Value: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := canon.StripUndeclaredProperties(in, ver("11.6.8"))
	if err != nil {
		t.Fatal(err)
	}
	if hasKeyAnywhere(t, out, "ThrowPartialSyncError") {
		t.Error("ThrowPartialSyncError stayed in an 11.6.8 document; it was measured absent there")
	}
}

func TestPropertyIntroduced_FromGeneratedData(t *testing.T) {
	v, ok := version.PropertyIntroduced("Rest$RestOperation", "QueryParameters")
	if !ok || v.String() != "11.0.0" {
		t.Errorf("Rest$RestOperation.QueryParameters introduced %v (%v), want 11.0.0", v, ok)
	}
	if _, ok := version.PropertyIntroduced("Rest$RestOperation", "Name"); ok {
		t.Error("a property with no Introduced reported a floor")
	}
}
