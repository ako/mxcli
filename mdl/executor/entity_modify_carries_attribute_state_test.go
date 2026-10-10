// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// `create or modify external entity` rebuilt each attribute from the statement,
// and the statement has no words for an attribute's OData mapping or for
// DateTime's LocalizeDate. Running an external entity's own describe output,
// unchanged, on the Studio Pro-authored ako/TestApp turned every attribute's
// Rest$ODataMappedValue into a DomainModels$StoredValue (RemoteName, RemoteType,
// Filterable, ... gone) and dropped LocalizeDate (#743). The attribute SET is
// still the statement's to declare; what it cannot spell is carried by name.

func storedExternalEntity() *domainmodel.Entity {
	return &domainmodel.Entity{
		BaseElement:       model.BaseElement{ID: "stored-id"},
		Name:              "Orders",
		Source:            "Rest$ODataRemoteEntitySource",
		RemoteServiceName: "Clients.OrderODataClient",
		Attributes: []*domainmodel.Attribute{
			{
				BaseElement: model.BaseElement{ID: "attr-date"},
				Name:        "OrderDate",
				Type:        &domainmodel.DateTimeAttributeType{LocalizeDate: false},
				RemoteName:  "OrderDate", RemoteType: "Edm.DateTimeOffset",
				Filterable: true, Sortable: true, Creatable: true, Updatable: true,
			},
			{
				BaseElement: model.BaseElement{ID: "attr-id"},
				Name:        "OrderId",
				Type:        &domainmodel.LongAttributeType{},
				RemoteName:  "OrderId", RemoteType: "Edm.Int64",
				Filterable: true,
			},
		},
	}
}

func TestMergeDeclaredOntoStoredEntity_CarriesODataMappingAndLocalizeDate(t *testing.T) {
	declared := &domainmodel.Entity{Name: "Orders", Attributes: []*domainmodel.Attribute{
		// What the executor builds from `OrderDate: DateTime, OrderId: Long`.
		{Name: "OrderDate", Type: &domainmodel.DateTimeAttributeType{LocalizeDate: true}},
		{Name: "OrderId", Type: &domainmodel.LongAttributeType{}},
		{Name: "Added", Type: &domainmodel.LongAttributeType{}},
	}}
	merged := mergeDeclaredOntoStoredEntity(storedExternalEntity(), declared, &ast.CreateEntityStmt{})

	date := merged.Attributes[0]
	if date.RemoteName != "OrderDate" || date.RemoteType != "Edm.DateTimeOffset" ||
		!date.Filterable || !date.Sortable || !date.Creatable || !date.Updatable {
		t.Errorf("OrderDate lost its OData mapping — the writer falls back to a StoredValue: %+v", date)
	}
	if dt, ok := date.Type.(*domainmodel.DateTimeAttributeType); !ok || dt.LocalizeDate {
		t.Errorf("OrderDate LocalizeDate = %v, want the stored false", date.Type)
	}
	if id := merged.Attributes[1]; id.RemoteName != "OrderId" || id.RemoteType != "Edm.Int64" || !id.Filterable {
		t.Errorf("OrderId lost its OData mapping: %+v", id)
	}
	// Control: an attribute the stored entity does not have gets nothing carried.
	if added := merged.Attributes[2]; added.RemoteName != "" {
		t.Errorf("a new attribute was given a mapping it never had: %+v", added)
	}
}

// A type the statement changes is the statement's to decide: LocalizeDate only
// carries onto a DateTime that stays a DateTime.
func TestMergeDeclaredOntoStoredEntity_LocalizeDateOnlyCarriesOntoDateTime(t *testing.T) {
	stored := &domainmodel.Entity{Name: "E", Attributes: []*domainmodel.Attribute{
		{Name: "When", Type: &domainmodel.DateTimeAttributeType{LocalizeDate: false}},
	}}
	declared := &domainmodel.Entity{Name: "E", Attributes: []*domainmodel.Attribute{
		{Name: "When", Type: &domainmodel.StringAttributeType{Length: 20}},
	}}
	merged := mergeDeclaredOntoStoredEntity(stored, declared, &ast.CreateEntityStmt{})
	if _, ok := merged.Attributes[0].Type.(*domainmodel.StringAttributeType); !ok {
		t.Errorf("the declared type change was overridden: %T", merged.Attributes[0].Type)
	}
}

// A mapped attribute's design-time default (Studio Pro stores "false" on a
// Boolean) has no spelling in the `from odata client` form; an undeclared one
// keeps the stored value, a declared one wins.
func TestCarryStoredAttributeState_MappedDefault(t *testing.T) {
	stored := &domainmodel.Entity{Attributes: []*domainmodel.Attribute{
		{Name: "Online", Type: &domainmodel.BooleanAttributeType{}, RemoteName: "Online", Value: &domainmodel.AttributeValue{DefaultValue: "false"}},
		{Name: "Flag", Type: &domainmodel.BooleanAttributeType{}, RemoteName: "Flag", Value: &domainmodel.AttributeValue{DefaultValue: "false"}},
	}}
	declared := []*domainmodel.Attribute{
		{Name: "Online", Type: &domainmodel.BooleanAttributeType{}},
		{Name: "Flag", Type: &domainmodel.BooleanAttributeType{}, Value: &domainmodel.AttributeValue{DefaultValue: "true"}},
	}
	carryStoredAttributeState(stored, declared, nil)
	if declared[0].Value == nil || declared[0].Value.DefaultValue != "false" {
		t.Errorf("Online default = %+v, want the stored \"false\"", declared[0].Value)
	}
	if declared[1].Value.DefaultValue != "true" {
		t.Errorf("Flag's declared default was overridden: %+v", declared[1].Value)
	}
	if declared[0].Value == stored.Attributes[0].Value {
		t.Error("the carried value aliases the stored one")
	}
}

// describe external entity printed GetTypeName (`String`), which executes as
// unlimited and rewrote a Studio Pro String(36) (#743).
func TestOutputExternalEntityMDL_PrintsStringLength(t *testing.T) {
	ctx, buf := newMockCtx(t)
	e := &domainmodel.Entity{Name: "Devices", RemoteServiceName: "Odata.Client", RemoteEntitySet: "Devices",
		Attributes: []*domainmodel.Attribute{{Name: "_Id", Type: &domainmodel.StringAttributeType{Length: 36}}}}
	if err := outputExternalEntityMDL(ctx, e, "Odata"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "_Id: String(36)") {
		t.Errorf("describe lost the length:\n%s", buf.String())
	}
}
