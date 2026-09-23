// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

// mendixlabs/mxcli#1118. A $metadata document that declares its complex types in a
// SEPARATE Schema from the entity types that use them — the shape the report
// carried, and the shape Mendix's own documentation uses for its example
// (`HomeAddress` of type `Lato.Address` on an entity in another namespace).
const crossNamespaceComplexTypeMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="Shared.Uom" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <ComplexType Name="Quantity">
        <Property Name="UoMNId" Type="Edm.String" MaxLength="40"/>
        <Property Name="QuantityValue" Type="Edm.Decimal"/>
      </ComplexType>
    </Schema>
    <Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <ComplexType Name="Quantity">
        <Property Name="WrongOne" Type="Edm.String"/>
      </ComplexType>
      <EntityType Name="Definition">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.String" Nullable="false" MaxLength="36"/>
        <Property Name="MaxQty" Type="Shared.Uom.Quantity"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="Definition" EntityType="App.Model.Definition"/>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

// The parser did not read <ComplexType> at all, so nothing downstream could
// distinguish "a complex type whose properties we should flatten" from "a type
// we know nothing about".
func TestParseEdmx_ParsesComplexTypes(t *testing.T) {
	doc, err := ParseEdmx(crossNamespaceComplexTypeMetadata)
	if err != nil {
		t.Fatal(err)
	}
	var shared *EdmSchema
	for _, s := range doc.Schemas {
		if s.Namespace == "Shared.Uom" {
			shared = s
		}
	}
	if shared == nil {
		t.Fatal("Shared.Uom schema missing")
	}
	if len(shared.ComplexTypes) != 1 {
		t.Fatalf("Shared.Uom has %d complex types, want 1", len(shared.ComplexTypes))
	}
	ct := shared.ComplexTypes[0]
	if ct.Name != "Quantity" {
		t.Errorf("complex type name = %q, want Quantity", ct.Name)
	}
	if len(ct.Properties) != 2 {
		t.Fatalf("Quantity has %d properties, want 2", len(ct.Properties))
	}
	if ct.Properties[0].Name != "UoMNId" || ct.Properties[0].Type != "Edm.String" {
		t.Errorf("first property = %+v", ct.Properties[0])
	}
	if ct.Properties[0].MaxLength != "40" {
		t.Errorf("MaxLength = %q, want 40 — facets must survive the flatten", ct.Properties[0].MaxLength)
	}
}

// Resolution is by QUALIFIED name. Two namespaces in one document may each
// declare `Quantity`; a short-name lookup picks whichever was parsed first, so
// the entity would silently get the other schema's properties.
func TestFindComplexType_ResolvesAcrossNamespacesByQualifiedName(t *testing.T) {
	doc, err := ParseEdmx(crossNamespaceComplexTypeMetadata)
	if err != nil {
		t.Fatal(err)
	}
	ct := doc.FindComplexType("Shared.Uom.Quantity")
	if ct == nil {
		t.Fatal("Shared.Uom.Quantity did not resolve — this is the silent drop in mendixlabs/mxcli#1118")
	}
	if len(ct.Properties) != 2 || ct.Properties[0].Name != "UoMNId" {
		t.Errorf("resolved the wrong Quantity: %+v", ct.Properties)
	}
	if other := doc.FindComplexType("App.Model.Quantity"); other == nil || other.Properties[0].Name != "WrongOne" {
		t.Errorf("App.Model.Quantity resolved to %+v, want the one declaring WrongOne", other)
	}
	if doc.FindComplexType("Nope.Missing") != nil {
		t.Error("an unknown qualified name must resolve to nil, not to a near match")
	}
}

// The flatten is what Studio Pro does: one attribute per leaf, named
// `Outer_Inner`, reached over the OData path `Outer/Inner`.
func TestFlattenProperties_ExpandsComplexTypeProperties(t *testing.T) {
	doc, err := ParseEdmx(crossNamespaceComplexTypeMetadata)
	if err != nil {
		t.Fatal(err)
	}
	et := doc.FindEntityType("App.Model.Definition")
	if et == nil {
		t.Fatal("Definition not found")
	}
	flat, unsupported := doc.FlattenProperties(et.Properties)
	if len(unsupported) != 0 {
		t.Errorf("unexpected unsupported properties: %v", unsupported)
	}

	want := []struct{ name, path, typ string }{
		{"Id", "", "Edm.String"},
		{"MaxQty_UoMNId", "MaxQty/UoMNId", "Edm.String"},
		{"MaxQty_QuantityValue", "MaxQty/QuantityValue", "Edm.Decimal"},
	}
	if len(flat) != len(want) {
		t.Fatalf("got %d properties, want %d: %+v", len(flat), len(want), flat)
	}
	for i, w := range want {
		if flat[i].Name != w.name {
			t.Errorf("[%d] Name = %q, want %q", i, flat[i].Name, w.name)
		}
		if flat[i].RemotePath != w.path {
			t.Errorf("[%d] RemotePath = %q, want %q", i, flat[i].RemotePath, w.path)
		}
		if flat[i].Type != w.typ {
			t.Errorf("[%d] Type = %q, want %q", i, flat[i].Type, w.typ)
		}
	}
	// The facet has to come from the leaf, not from the complex property.
	if flat[1].MaxLength != "40" {
		t.Errorf("MaxQty_UoMNId MaxLength = %q, want 40", flat[1].MaxLength)
	}
}
