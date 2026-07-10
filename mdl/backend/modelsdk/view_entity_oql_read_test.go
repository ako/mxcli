// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// On Mendix 11.0+ a view entity's OQL query is stored ONLY in its separate
// DomainModels$ViewEntitySourceDocument — the inline OqlViewEntitySource.Oql
// field was removed. The modelsdk read must follow the SourceDocumentRef and
// repopulate OqlQuery; otherwise DESCRIBE VIEW ENTITY drops the `as (…)` clause
// and the describe→exec round-trip loses the query. (The fixture is 11.6.6.)
func TestGetDomainModel_ViewEntityOqlFromSourceDocument(t *testing.T) {
	proj := copyFixture(t)
	const oql = "select s.Name as Name from MyFirstModule.Account as s"

	// Author a view entity: source document + entity referencing it.
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v %v", mod, err)
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	if _, err := b.CreateViewEntitySourceDocument(mod.ID, "MyFirstModule", "VOql", oql, ""); err != nil {
		t.Fatalf("CreateViewEntitySourceDocument: %v", err)
	}
	ent := &domainmodel.Entity{
		Name:              "VOql",
		Persistable:       true,
		Source:            "DomainModels$OqlViewEntitySource",
		SourceDocumentRef: "MyFirstModule.VOql",
		OqlQuery:          oql,
		Attributes: []*domainmodel.Attribute{{
			Name:  "Name",
			Type:  &domainmodel.StringAttributeType{Length: 200},
			Value: &domainmodel.AttributeValue{ViewReference: "Name"},
		}},
	}
	if err := b.CreateEntity(dm.ID, ent); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	// Reopen (fresh read, no write-side state) and confirm OqlQuery round-trips
	// via both GetDomainModel and ListDomainModels.
	readBack := func(t *testing.T, get func(*Backend) (*domainmodel.DomainModel, error)) {
		t.Helper()
		b2 := New()
		if err := b2.Connect(proj); err != nil {
			t.Fatalf("reconnect: %v", err)
		}
		defer b2.Disconnect()
		dm2, err := get(b2)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var got string
		found := false
		for _, e := range dm2.Entities {
			if e.Name == "VOql" {
				found, got = true, e.OqlQuery
			}
		}
		if !found {
			t.Fatal("view entity VOql not found on read-back")
		}
		if got != oql {
			t.Errorf("OqlQuery = %q, want %q (read must follow SourceDocumentRef on 11.x)", got, oql)
		}
	}

	t.Run("GetDomainModel", func(t *testing.T) {
		readBack(t, func(b *Backend) (*domainmodel.DomainModel, error) { return b.GetDomainModel(mod.ID) })
	})
	t.Run("ListDomainModels", func(t *testing.T) {
		readBack(t, func(b *Backend) (*domainmodel.DomainModel, error) {
			dms, err := b.ListDomainModels()
			if err != nil {
				return nil, err
			}
			for _, d := range dms {
				if d.ContainerID == mod.ID {
					return d, nil
				}
			}
			t.Fatal("domain model not found")
			return nil, nil
		})
	})
}
