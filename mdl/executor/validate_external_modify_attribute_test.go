// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// `alter entity E modify attribute A <type>` on an external entity's mapped
// attribute must keep the type the OData service publishes. Measured on MxBuild
// 11.16.1 against a contract Product(Code Edm.String MaxLength 36 key, Label
// Edm.String MaxLength 100, Note Edm.String, Qty Edm.Int32), one project per case:
//
//	Label: string(100), Qty: integer, Qty: long          clean   (Integer and Long are one class)
//	Label: string(200), string(unlimited), Note: string(50), Code: string(50)
//	                                                       CE6621 "… has max length '200' which is not the same as max length in the OData service which is '100'"
//	Qty: decimal / boolean / datetime, Label: integer / enumeration
//	                                                       CE6616 "… is of type 'Decimal' which is not the same as property type in the OData service …"
//
// check and exec accepted every rejected case.
const externalModifyContract = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Product">
        <Key><PropertyRef Name="Code"/></Key>
        <Property Name="Code" Type="Edm.String" Nullable="false" MaxLength="36"/>
        <Property Name="Label" Type="Edm.String" MaxLength="100"/>
        <Property Name="Note" Type="Edm.String"/>
        <Property Name="Qty" Type="Edm.Int32"/>
      </EntityType>
      <EntityContainer Name="Container"><EntitySet Name="Products" EntityType="App.Model.Product"/></EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

func externalModifyBackend() func() backend.FullBackend {
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-pj"}, Name: "Pj"}
	ent := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: "e1"}, Name: "Product", Persistable: true,
		Source: externalEntitySourceType, RemoteServiceName: "Pj.Client", RemoteEntitySet: "Products", RemoteEntityName: "Product",
		Attributes: []*domainmodel.Attribute{
			{Name: "Code", RemoteName: "Code", RemoteType: "Edm.String", Type: &domainmodel.StringAttributeType{Length: 36}},
			{Name: "Label", RemoteName: "Label", RemoteType: "Edm.String", Type: &domainmodel.StringAttributeType{Length: 100}},
			{Name: "Note", RemoteName: "Note", RemoteType: "Edm.String", Type: &domainmodel.StringAttributeType{}},
			{Name: "Qty", RemoteName: "Qty", RemoteType: "Edm.Int32", Type: &domainmodel.IntegerAttributeType{}},
		},
	}
	local := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e2"}, Name: "Local",
		Attributes: []*domainmodel.Attribute{{Name: "Label", Type: &domainmodel.StringAttributeType{Length: 100}}}}
	dm := &domainmodel.DomainModel{BaseElement: model.BaseElement{ID: "dm"}, ContainerID: mod.ID, Entities: []*domainmodel.Entity{ent, local}}
	svc := &model.ConsumedODataService{ContainerID: mod.ID, Name: "Client", Metadata: externalModifyContract}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "Pj" {
				return mod, nil
			}
			return nil, nil
		},
		GetDomainModelFunc:            func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) { return []*model.ConsumedODataService{svc}, nil },
	}
	return func() backend.FullBackend { return mb }
}

func TestValidateExternalModifyAttribute(t *testing.T) {
	open := externalModifyBackend()
	cases := []struct {
		name, stmt, want string // want: "", "CE6621" or "CE6616"
	}{
		{"the published length (control)", "alter entity Pj.Product modify attribute Label: string(100);", ""},
		{"Integer kept (control)", "alter entity Pj.Product modify attribute Qty: integer;", ""},
		{"Long for an Edm.Int32 (control)", "alter entity Pj.Product modify attribute Qty: long;", ""},
		{"a longer string", "alter entity Pj.Product modify attribute Label: string(200);", "CE6621"},
		{"unlimited for a bounded property", "alter entity Pj.Product modify attribute Label: string(unlimited);", "CE6621"},
		{"bounded for an unlimited property", "alter entity Pj.Product modify attribute Note: string(50);", "CE6621"},
		{"the key's length", "alter entity Pj.Product modify attribute Code: string(50);", "CE6621"},
		{"decimal for an Edm.Int32", "alter entity Pj.Product modify attribute Qty: decimal;", "CE6616"},
		{"boolean for an Edm.Int32", "alter entity Pj.Product modify attribute Qty: boolean;", "CE6616"},
		{"integer for an Edm.String", "alter entity Pj.Product modify attribute Label: integer;", "CE6616"},
		{"a local entity (control)", "alter entity Pj.Local modify attribute Label: string(200);", ""},
		{"an attribute the entity does not have (control)", "alter entity Pj.Product modify attribute Nope: string(5);", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ValidateExternalModifyAttribute(mustBuild(t, "mdl 1;\n"+c.stmt), open)
			if c.want == "" {
				if len(got) != 0 {
					t.Fatalf("want none, got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].RuleID != externalModifyRule || !strings.Contains(got[0].Message, c.want) {
				t.Fatalf("want one %s, got %+v", c.want, got)
			}
		})
	}
}

// Not judged: no project; an entity the script itself creates; a service the
// script rewrites.
func TestValidateExternalModifyAttribute_NotJudged(t *testing.T) {
	stmt := "alter entity Pj.Product modify attribute Label: string(200);"
	if got := ValidateExternalModifyAttribute(mustBuild(t, "mdl 1;\n"+stmt), nil); len(got) != 0 {
		t.Errorf("no project: want none, got %+v", got)
	}
	open := externalModifyBackend()
	recreated := "mdl 1;\ncreate or modify persistent entity Pj.Product (Label: string(50));\n" + stmt
	if got := ValidateExternalModifyAttribute(mustBuild(t, recreated), open); len(got) != 0 {
		t.Errorf("an entity the script rewrites: want none, got %+v", got)
	}
	reimported := "mdl 1;\ncreate or modify consumed odata service Pj.Client (ODataVersion: OData4, MetadataUrl: './x.xml');\n" + stmt
	if got := ValidateExternalModifyAttribute(mustBuild(t, reimported), open); len(got) != 0 {
		t.Errorf("a service the script rewrites: want none, got %+v", got)
	}
}
