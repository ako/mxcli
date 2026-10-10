// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// close page N: Mendix reads the string NumberOfPagesToClose; the int
// NumberOfPages was deleted in 8.11, so writing it closed one page and left a
// key Mendix strips.
func TestIssue1373_ClosePageWritesNumberOfPagesToClose(t *testing.T) {
	for _, tc := range []struct {
		n    int
		want string
	}{{0, ""}, {1, ""}, {2, "2"}, {5, "5"}} {
		a := &microflows.ClosePageAction{NumberOfPages: tc.n}
		a.ID = "aaaaaaaa-0000-4000-8000-000000000002"
		m := encodeToMap(t, microflowActionToGen(a))
		if got, ok := m["NumberOfPagesToClose"]; !ok || got != tc.want {
			t.Errorf("close %d: NumberOfPagesToClose = %#v (present %v), want %q", tc.n, got, ok, tc.want)
		}
		if _, ok := m["NumberOfPages"]; ok {
			t.Errorf("close %d: wrote the deleted NumberOfPages key", tc.n)
		}
	}
}

func TestIssue1373_ClosePageReadsNumberOfPagesToClose(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  bsonv2.D
		want int
	}{
		{"studio pro default", bsonv2.D{{Key: "NumberOfPagesToClose", Value: ""}}, 1},
		{"studio pro explicit", bsonv2.D{{Key: "NumberOfPagesToClose", Value: "3"}}, 3},
		{"mxcli before #1373", bsonv2.D{{Key: "NumberOfPages", Value: int32(2)}}, 2},
	} {
		doc := append(bsonv2.D{
			{Key: "$ID", Value: bsonv2.Binary{Subtype: 0, Data: make([]byte, 16)}},
			{Key: "$Type", Value: "Microflows$CloseFormAction"},
		}, tc.doc...)
		raw, err := bsonv2.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
		if err != nil {
			t.Fatalf("%s: decode: %v", tc.name, err)
		}
		cf, ok := el.(*genMf.CloseFormAction)
		if !ok {
			t.Fatalf("%s: decoded %T", tc.name, el)
		}
		if got := closePageCount(cf); got != tc.want {
			t.Errorf("%s: closePageCount = %d, want %d", tc.name, got, tc.want)
		}
	}
}
