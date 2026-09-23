// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1118: "mxcli silently discards any OData property whose type is a
// ComplexType defined in a different Schema namespace within the same $metadata
// document. The attributes are not created in the Mendix entity, and no warning
// or error is emitted."
//
// Studio Pro flattens them (`MaxQty.UoMNId` → `MaxQty_UoMNId`), which is what
// the reporter's pages and microflows were written against; without them the
// build fails with CE1613 naming attributes the import never made.
const complexTypeMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="Shared.Uom" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <ComplexType Name="Quantity">
        <Property Name="UoMNId" Type="Edm.String" MaxLength="40"/>
        <Property Name="QuantityValue" Type="Edm.Decimal"/>
      </ComplexType>
    </Schema>
    <Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Definition">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.String" Nullable="false" MaxLength="36"/>
        <Property Name="MaxQty" Type="Shared.Uom.Quantity"/>
        <Property Name="MaxWeight" Type="Shared.Uom.Quantity"/>
        <Property Name="MaxVolume" Type="Shared.Uom.Quantity"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="Definition" EntityType="App.Model.Definition"/>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

// importComplexTypeContract runs CREATE OR MODIFY EXTERNAL ENTITIES FROM over
// the metadata above and returns the entity that reached the backend plus the
// executor's output.
func importComplexTypeContract(t *testing.T, metadata string) (map[string]*domainmodel.Entity, string) {
	t.Helper()
	mod := mkModule("CustomModule")
	svc := &model.ConsumedODataService{
		BaseElement:  model.BaseElement{ID: nextID("cos")},
		ContainerID:  mod.ID,
		Name:         "App",
		MetadataUrl:  "https://example.com/$metadata",
		ODataVersion: "4.0",
		Metadata:     metadata,
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	dm := &domainmodel.DomainModel{}
	dm.ID = nextID("dm")

	created := map[string]*domainmodel.Entity{}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },

		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{svc}, nil
		},
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		CreateEntityFunc: func(_ model.ID, e *domainmodel.Entity) error {
			created[e.Name] = e
			dm.Entities = append(dm.Entities, e)
			return nil
		},
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{MajorVersion: 11, MinorVersion: 12, ProductVersion: "11.12.0"}
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateExternalEntitiesStmt{
		ServiceRef:     ast.QualifiedName{Module: "CustomModule", Name: "App"},
		TargetModule:   "CustomModule",
		CreateOrModify: true,
	}
	assertNoError(t, createExternalEntities(ctx, stmt))
	if len(created) == 0 {
		t.Fatal("no entity was created")
	}
	return created, buf.String()
}

// importOne is the single-entity form most tests want.
func importOne(t *testing.T, metadata, entity string) (*domainmodel.Entity, string) {
	t.Helper()
	all, out := importComplexTypeContract(t, metadata)
	ent := all[entity]
	if ent == nil {
		names := make([]string, 0, len(all))
		for n := range all {
			names = append(names, n)
		}
		t.Fatalf("entity %q not created; got %v", entity, names)
	}
	return ent, out
}

func attrByName(e *domainmodel.Entity) map[string]*domainmodel.Attribute {
	m := map[string]*domainmodel.Attribute{}
	for _, a := range e.Attributes {
		m[a.Name] = a
	}
	return m
}

func attrNames(e *domainmodel.Entity) []string {
	var out []string
	for _, a := range e.Attributes {
		out = append(out, a.Name)
	}
	return out
}

// The reported symptom: "The imported entity contains none of these attributes."
func TestCreateExternalEntities_FlattensCrossNamespaceComplexTypes(t *testing.T) {
	ent, _ := importOne(t, complexTypeMetadata, "Definition")

	want := []string{
		"MaxQty_UoMNId", "MaxQty_QuantityValue",
		"MaxWeight_UoMNId", "MaxWeight_QuantityValue",
		"MaxVolume_UoMNId", "MaxVolume_QuantityValue",
	}
	got := attrNames(ent)
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
			}
		}
		if !found {
			t.Errorf("attribute %q missing — the ComplexType property was silently dropped (mendixlabs/mxcli#1118). got %v", w, got)
		}
	}

	// The un-flattened complex property must not survive as an attribute of its
	// own: Mendix has no complex-typed attribute, and a String(200) stand-in is
	// the lossy simplification the report calls out.
	for _, g := range got {
		if g == "MaxQty" || g == "MaxWeight" || g == "MaxVolume" {
			t.Errorf("complex property %q was kept as a scalar attribute", g)
		}
	}
}

// The leaf's own type and facets decide the Mendix attribute, not the complex
// property's. A Decimal flattened as String is a silent data-type change that
// only shows up when an expression fails to compile.
func TestCreateExternalEntities_FlattenedAttributeKeepsLeafType(t *testing.T) {
	ent, _ := importOne(t, complexTypeMetadata, "Definition")
	byName := map[string]*domainmodel.Attribute{}
	for _, a := range ent.Attributes {
		byName[a.Name] = a
	}

	qty := byName["MaxQty_QuantityValue"]
	if qty == nil {
		t.Fatal("MaxQty_QuantityValue missing")
	}
	if qty.Type == nil || qty.Type.GetTypeName() != "Decimal" {
		t.Errorf("MaxQty_QuantityValue type = %v, want Decimal", qty.Type)
	}
	if qty.RemoteType != "Edm.Decimal" {
		t.Errorf("RemoteType = %q, want Edm.Decimal", qty.RemoteType)
	}

	uom := byName["MaxQty_UoMNId"]
	if uom == nil {
		t.Fatal("MaxQty_UoMNId missing")
	}
	if uom.RemoteType != "Edm.String" {
		t.Errorf("RemoteType = %q, want Edm.String", uom.RemoteType)
	}
}

// Whatever the import cannot map, it must SAY so. The bug is not only that the
// attributes were absent — it is that `exec` reported success and printed
// nothing, so the loss was discovered at build time as CE1613 on a page.
func TestCreateExternalEntities_ReportsPropertiesItCannotMap(t *testing.T) {
	// A property typed as a complex type the document never declares cannot be
	// flattened by anyone; it is the case that must still be reported.
	const danglingComplex = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Definition">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.String" Nullable="false" MaxLength="36"/>
        <Property Name="MaxQty" Type="Absent.Namespace.Quantity"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="Definition" EntityType="App.Model.Definition"/>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

	_, out := importOne(t, danglingComplex, "Definition")
	if !strings.Contains(out, "MaxQty") {
		t.Errorf("the dropped property is not named in the output — this is the silence in mendixlabs/mxcli#1118:\n%s", out)
	}
}

// Mendix: "External entities that contain attributes of complex types can only
// be read or deleted. They cannot be created, updated, or used in external
// actions." So a flattened attribute is read-only even when the entity set's
// own contract says Insertable=true and Updatable=true.
//
// Measured on 11.12.1: following the entity set instead costs two CE6630 per
// flattened attribute — "'MaxQty_UoMNId' is marked Creatable=False in the OData
// service, but True in the app".
func TestCreateExternalEntities_FlattenedAttributesAreReadOnly(t *testing.T) {
	const writableContract = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="Shared.Uom" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <ComplexType Name="Quantity">
        <Property Name="UoMNId" Type="Edm.String" MaxLength="40"/>
      </ComplexType>
    </Schema>
    <Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Definition">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.String" Nullable="false" MaxLength="36"/>
        <Property Name="Label" Type="Edm.String" MaxLength="100"/>
        <Property Name="MaxQty" Type="Shared.Uom.Quantity"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="Definition" EntityType="App.Model.Definition">
          <Annotation Term="Org.OData.Capabilities.V1.InsertRestrictions">
            <Record><PropertyValue Property="Insertable" Bool="true"/></Record>
          </Annotation>
          <Annotation Term="Org.OData.Capabilities.V1.UpdateRestrictions">
            <Record><PropertyValue Property="Updatable" Bool="true"/></Record>
          </Annotation>
        </EntitySet>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

	ent, _ := importOne(t, writableContract, "Definition")
	byName := map[string]*domainmodel.Attribute{}
	for _, a := range ent.Attributes {
		byName[a.Name] = a
	}

	flat := byName["MaxQty_UoMNId"]
	if flat == nil {
		t.Fatal("MaxQty_UoMNId missing")
	}
	if flat.Creatable || flat.Updatable {
		t.Errorf("MaxQty_UoMNId Creatable=%v Updatable=%v, want both false — CE6630",
			flat.Creatable, flat.Updatable)
	}

	// The control: an ordinary property of the SAME writable entity set must
	// still follow the contract on CREATABLE. Without it this test passes
	// against an import that marks everything read-only.
	//
	// Updatable is deliberately NOT asserted true here, and this control used
	// to claim it was. That half was assumed rather than measured: mxbuild
	// computes every attribute of a TOP-LEVEL entity as Updatable=False, plain
	// and flattened alike, across ten contract shapes — including one whose
	// NonUpdatableProperties names only the key, i.e. asserts that `Label` is
	// updatable. See TestCreateExternalEntities_TopLevelAttributesAreNeverUpdatable.
	// So what is special about a flattened attribute is CREATABLE, not both.
	plain := byName["Label"]
	if plain == nil {
		t.Fatal("Label missing")
	}
	if !plain.Creatable {
		t.Error("Label lost Creatable — the contract says the set is insertable, and without " +
			"this the test passes against an import that marks everything read-only")
	}
	if plain.Updatable {
		t.Error("Label is Updatable=true on a top-level entity — CE6630 " +
			`"'Label' is marked Updatable=False in the OData service, but True in the app."`)
	}
}

// The remote name of a flattened attribute is the OData PATH — "MaxQty/UoMNId",
// not the underscore-joined local name.
//
// Measured on 11.12.1, three variants of the same project:
//
//	RemoteName "MaxQty/UoMNId"          -> The app contains: 0 errors.
//	RemoteName "MaxQty_UoMNId"          -> 4 x CE6615 "does not exist in the OData service"
//	RemoteName "MaxQty_ZZNOTAPATH_UoM"  -> 4 x CE6615
//
// So mxbuild resolves the path into the complex type and genuinely checks it —
// the 0-error run is evidence, not a rubber stamp. Local name and remote name
// differ by separator here, which is the detail that looks like a typo in a diff.
func TestCreateExternalEntities_FlattenedRemoteNameIsTheODataPath(t *testing.T) {
	ent, _ := importOne(t, complexTypeMetadata, "Definition")
	for _, a := range ent.Attributes {
		if a.Name == "MaxQty_UoMNId" {
			if a.RemoteName != "MaxQty/UoMNId" {
				t.Fatalf("RemoteName = %q, want %q — CE6615 otherwise", a.RemoteName, "MaxQty/UoMNId")
			}
			return
		}
	}
	t.Fatal("MaxQty_UoMNId missing")
}

// trippinShapedContract reproduces the three things the synthetic fixture above
// does not have, all of them present in the public TripPin service:
//
//	Location        — a base complex type, with a nested complex property (City)
//	AirportLocation — derived from it, adding an Edm.GeographyPoint
//	Person/Employee — a top-level entity set and a type derived from it
const trippinShapedContract = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="Trippin" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <ComplexType Name="City">
        <Property Name="Name" Type="Edm.String"/>
      </ComplexType>
      <ComplexType Name="Location">
        <Property Name="Address" Type="Edm.String"/>
        <Property Name="City" Type="Trippin.City"/>
      </ComplexType>
      <ComplexType Name="AirportLocation" BaseType="Trippin.Location">
        <Property Name="Loc" Type="Edm.GeographyPoint"/>
      </ComplexType>
      <EntityType Name="Person">
        <Key><PropertyRef Name="UserName"/></Key>
        <Property Name="UserName" Type="Edm.String" Nullable="false" MaxLength="100"/>
        <Property Name="HomeAddress" Type="Trippin.Location"/>
      </EntityType>
      <EntityType Name="Employee" BaseType="Trippin.Person">
        <Property Name="Cost" Type="Edm.Int64"/>
      </EntityType>
      <EntityType Name="Airport">
        <Key><PropertyRef Name="IcaoCode"/></Key>
        <Property Name="IcaoCode" Type="Edm.String" Nullable="false" MaxLength="10"/>
        <Property Name="Location" Type="Trippin.AirportLocation"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="People" EntityType="Trippin.Person"/>
        <EntitySet Name="Airports" EntityType="Trippin.Airport"/>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

// Mendix imports a complex type's OWN properties only — an inherited one is not
// addressable through the derived type.
//
// Measured on mxbuild 11.12.1 against TripPin: flattening `AirportLocation`'s
// inherited `Address` gives
//
//	CE6615 "Attribute 'Location_Address' of external entity 'Airports' does not
//	        exist in the OData service."
//
// while the SAME `Address` reached through `Person.HomeAddress`, typed `Location`
// directly, is accepted — so the line is inheritance, not the path syntax.
func TestCreateExternalEntities_DoesNotFlattenInheritedComplexProperties(t *testing.T) {
	airport, out := importOne(t, trippinShapedContract, "Airports")
	got := attrByName(airport)

	if _, ok := got["Location_Address"]; ok {
		t.Error("Location_Address was imported — it is inherited from Trippin.Location, which is CE6615")
	}
	// …and it is named rather than dropped in silence.
	if !strings.Contains(out, "Location/Address") {
		t.Errorf("the inherited property is not reported:\n%s", out)
	}

	// The control: the SAME leaf name reached through a non-derived complex type
	// must still be imported. Without this the test passes against a build that
	// stopped flattening altogether.
	person, _ := importOne(t, trippinShapedContract, "People")
	if _, ok := attrByName(person)["HomeAddress_Address"]; !ok {
		t.Error("HomeAddress_Address is missing — Location is not derived, so it must flatten")
	}
}

// A leaf whose type Mendix cannot represent is not an attribute. Measured on
// 11.12.1: importing TripPin's `AirportLocation.Loc` (Edm.GeographyPoint) is
//
//	CE6622 "The type of attribute 'Location_Loc' in the OData service is not
//	        supported. Please delete this attribute."
//
// `!strings.HasPrefix(t, "Edm.")` is not enough — GeographyPoint passes it.
func TestCreateExternalEntities_SkipsUnsupportedLeafTypes(t *testing.T) {
	airport, out := importOne(t, trippinShapedContract, "Airports")
	if _, ok := attrByName(airport)["Location_Loc"]; ok {
		t.Error("Location_Loc was imported as an attribute — Edm.GeographyPoint is CE6622")
	}
	if !strings.Contains(out, "Location/Loc") || !strings.Contains(out, "Edm.GeographyPoint") {
		t.Errorf("the unsupported leaf is not reported with its type:\n%s", out)
	}

	// A nested complex type is refused the same way, and named as such.
	if !strings.Contains(out, "HomeAddress/City") {
		t.Errorf("the nested complex property is not reported:\n%s", out)
	}
}

// A flattened attribute is queryable exactly where its entity is. CE6630 fires
// in BOTH directions, so neither blanket answer is right — measured on TripPin
// (11.12.1), which carries no capability annotations at all:
//
//	Person   (entity set People)      HomeAddress_Address  Filterable True
//	Employee (derived, no entity set) HomeAddress_Address  Filterable False
func TestCreateExternalEntities_FlattenedFilterabilityFollowsTheEntitySet(t *testing.T) {
	person, _ := importOne(t, trippinShapedContract, "People")
	top := attrByName(person)["HomeAddress_Address"]
	if top == nil {
		t.Fatal("People.HomeAddress_Address missing")
	}
	if !top.Filterable || !top.Sortable {
		t.Errorf("People.HomeAddress_Address Filterable=%v Sortable=%v, want both true — "+
			"the set is queryable and the contract restricts nothing (CE6630 in the other direction)",
			top.Filterable, top.Sortable)
	}

	employee, _ := importOne(t, trippinShapedContract, "Employee")
	derived := attrByName(employee)
	flat := derived["HomeAddress_Address"]
	if flat == nil {
		t.Fatal("Employee.HomeAddress_Address missing")
	}
	if flat.Filterable || flat.Sortable {
		t.Errorf("Employee.HomeAddress_Address Filterable=%v Sortable=%v, want both false — CE6630",
			flat.Filterable, flat.Sortable)
	}

	// The control: this is a property of the FLATTENING, not of derived entity
	// types. An ordinary attribute of the same derived entity stays filterable,
	// which is what it was before any of this and what mx check accepts.
	plain := derived["Cost"]
	if plain == nil {
		t.Fatal("Employee.Cost missing")
	}
	if !plain.Filterable || !plain.Sortable {
		t.Errorf("Employee.Cost Filterable=%v Sortable=%v, want both true — "+
			"derived types are not themselves unfilterable", plain.Filterable, plain.Sortable)
	}
}
