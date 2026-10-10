// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
)

// Running `create or modify odata service` over a Studio Pro-authored service
// (ako/TestApp: MyFirstModule.CarOdataApi, Odata.Published_OData_GraphQL_service,
// Services.OrderODataApi) dropped its ExportLevel and wrote AuthenticationTypes
// with marker 3 where Studio Pro writes 1 on all three (#743).

func encodePublishedService(t *testing.T, svc *model.PublishedODataService) bson.Raw {
	t.Helper()
	raw, err := (&codec.Encoder{}).Encode(publishedODataServiceToGen(svc, true))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return raw
}

func TestPublishedODataServiceToGen_WritesStoredExportLevel(t *testing.T) {
	raw := encodePublishedService(t, &model.PublishedODataService{Name: "CarOdataApi", ExportLevel: "Hidden"})
	v, err := raw.LookupErr("ExportLevel")
	if err != nil {
		t.Fatal("ExportLevel is absent; a rewrite of a Studio Pro service deletes it")
	}
	if s, _ := v.StringValueOK(); s != "Hidden" {
		t.Errorf("ExportLevel = %v, want Hidden", v)
	}
	// Control: a service that never had one (created by mxcli) is written as before.
	if _, err := encodePublishedService(t, &model.PublishedODataService{Name: "New"}).LookupErr("ExportLevel"); err == nil {
		t.Error("an unset ExportLevel was written")
	}
}

func TestPublishedODataServiceToGen_AuthenticationTypesMarkerIsOne(t *testing.T) {
	raw := encodePublishedService(t, &model.PublishedODataService{Name: "CarOdataApi", AuthenticationTypes: []string{"Basic"}})
	v, err := raw.LookupErr("AuthenticationTypes")
	if err != nil {
		t.Fatal("AuthenticationTypes is absent")
	}
	vals, err := v.Array().Values()
	if err != nil || len(vals) != 2 {
		t.Fatalf("AuthenticationTypes = %v, want [marker, Basic]", v)
	}
	if m, ok := vals[0].Int32OK(); !ok || m != 1 {
		t.Errorf("marker = %v, want 1 (what Studio Pro writes)", vals[0])
	}
}

// A member's stored CanBeEmpty is written back as stored; without one the
// writer derives it from the key, as before (#743).
func TestPublishedMemberCanBeEmpty(t *testing.T) {
	no, yes := false, true
	for _, c := range []struct {
		m    model.PublishedMember
		want bool
	}{
		{model.PublishedMember{Kind: "attribute", CanBeEmpty: &no}, false},
		{model.PublishedMember{Kind: "attribute", IsPartOfKey: true, CanBeEmpty: &yes}, true},
		{model.PublishedMember{Kind: "attribute"}, true},
		{model.PublishedMember{Kind: "attribute", IsPartOfKey: true}, false},
	} {
		if got := memberCanBeEmpty(&c.m); got != c.want {
			t.Errorf("memberCanBeEmpty(%+v) = %v, want %v", c.m, got, c.want)
		}
	}
	if m := publishedMemberFromRaw(map[string]any{"$Type": "ODataPublish$PublishedAttribute", "CanBeEmpty": false}); m.CanBeEmpty == nil || *m.CanBeEmpty {
		t.Errorf("stored CanBeEmpty false read as %v", m.CanBeEmpty)
	}
	if m := publishedMemberFromRaw(map[string]any{"$Type": "ODataPublish$PublishedAttribute"}); m.CanBeEmpty != nil {
		t.Errorf("absent CanBeEmpty read as %v", *m.CanBeEmpty)
	}
}
