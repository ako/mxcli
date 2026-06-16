// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// TestCreateConsumedODataService_RoundTrip creates a consumed OData service with
// an HTTP configuration and confirms it round-trips through the reader.
func TestCreateConsumedODataService_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	svc := &model.ConsumedODataService{
		ContainerID:            mod.ID,
		Name:                   "ZzClient",
		Version:                "1.0",
		ODataVersion:           "OData4",
		MetadataUrl:            "https://api.example.com/odata/v4/$metadata",
		ConfigurationMicroflow: "MyFirstModule.ConfigReq",
		HttpConfiguration:      &model.HttpConfiguration{HttpMethod: "Post"},
	}
	if err := b.CreateConsumedODataService(svc); err != nil {
		t.Fatalf("CreateConsumedODataService: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	all, err := b2.ListConsumedODataServices()
	if err != nil {
		t.Fatalf("ListConsumedODataServices: %v", err)
	}
	found := false
	for _, s := range all {
		if s.Name == "ZzClient" {
			found = true
			if s.MetadataUrl != "https://api.example.com/odata/v4/$metadata" {
				t.Errorf("MetadataUrl not round-tripped: %q", s.MetadataUrl)
			}
		}
	}
	if !found {
		t.Errorf("ZzClient not found after create")
	}
}

// TestCreatePublishedODataService_RoundTrip creates a published OData service with
// one entity type, members (a key attribute), and an entity set, then confirms it
// round-trips through ListPublishedODataServices.
func TestCreatePublishedODataService_RoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	svc := &model.PublishedODataService{
		ContainerID:         mod.ID,
		Name:                "ZzApi",
		Path:                "odata/zz/",
		Version:             "1.0.0",
		ODataVersion:        "OData4",
		Namespace:           "MyFirstModule.Zz",
		AuthenticationTypes: []string{"Basic"},
		EntityTypes: []*model.PublishedEntityType{{
			Entity:      "MyFirstModule.Thing",
			ExposedName: "Things",
			Members: []*model.PublishedMember{
				{Kind: "attribute", Name: "Code", ExposedName: "code", IsPartOfKey: true, Filterable: true, Sortable: true},
				{Kind: "attribute", Name: "Label", ExposedName: "label"},
			},
		}},
		EntitySets: []*model.PublishedEntitySet{{
			ExposedName: "Things", EntityTypeName: "MyFirstModule.Thing",
			ReadMode: "source", UsePaging: true, PageSize: 100,
		}},
	}
	if err := b.CreatePublishedODataService(svc); err != nil {
		t.Fatalf("CreatePublishedODataService: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	all, err := b2.ListPublishedODataServices()
	if err != nil {
		t.Fatalf("ListPublishedODataServices: %v", err)
	}
	var got *model.PublishedODataService
	for _, s := range all {
		if s.Name == "ZzApi" {
			got = s
			break
		}
	}
	if got == nil {
		t.Fatalf("ZzApi not found after create")
	}
	// Note: ListPublishedODataServices reconstructs the service header but not the
	// full EntityTypes slice (a read-side gap; the entity-count display uses a gen
	// accessor). Assert the header fields the reader does populate; the entity-tree
	// write fidelity is covered by the 10-odata-examples mx-check parity check.
	if got.Path != "odata/zz/" {
		t.Errorf("Path not round-tripped: %q", got.Path)
	}
	if got.ODataVersion != "OData4" {
		t.Errorf("ODataVersion not round-tripped: %q", got.ODataVersion)
	}
}
