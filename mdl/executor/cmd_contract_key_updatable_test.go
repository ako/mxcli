// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A writable OData entity set: the container declares Insertable=true AND
// Updatable=true, and the entity has one key property plus one ordinary one.
// Deliberately free of complex types — the key-updatability defect is
// independent of the ComplexType flattening fixed in mendixlabs/mxcli#1118 and
// reproduces on a contract with no complex type at all.
const writableSetMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Definition">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.String" Nullable="false" MaxLength="36"/>
        <Property Name="Label" Type="Edm.String" MaxLength="100"/>
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

// The same contract with the set insertable but NOT updatable — the second
// direction the report asks about.
const insertOnlySetMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Definition">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.String" Nullable="false" MaxLength="36"/>
        <Property Name="Label" Type="Edm.String" MaxLength="100"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="Definition" EntityType="App.Model.Definition">
          <Annotation Term="Org.OData.Capabilities.V1.InsertRestrictions">
            <Record><PropertyValue Property="Insertable" Bool="true"/></Record>
          </Annotation>
          <Annotation Term="Org.OData.Capabilities.V1.UpdateRestrictions">
            <Record><PropertyValue Property="Updatable" Bool="false"/></Record>
          </Annotation>
        </EntitySet>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

// The reported symptom, on mxbuild 11.12.1:
//
//	[error] [CE6630] "'DefinitionId' is marked Updatable=False in the OData
//	                  service, but True in the app."
//	        at Attribute 'MyFirstModule.Definition.DefinitionId'
//
// Mendix computes a key property as non-updatable whatever the entity set's
// UpdateRestrictions say — a key cannot be changed after creation — so an
// import that lets the key follow the set is exactly one CE6630 per key part.
//
// The key is a special case of the wider rule proved in
// TestCreateExternalEntities_TopLevelAttributesAreNeverUpdatable below — no
// attribute of a top-level entity is updatable — and it is kept as its own
// test because it is the symptom that was reported, and because `Creatable`
// going the other way on the very same attribute is the sharpest statement
// that this is not a read-only stamp.
//
// The other side is TestCreateExternalEntities_DerivedTypeKeyStaysUpdatable,
// which the live TripPin contract supplied after a blanket version of this
// rule turned one CE6630 into seven of its inverse.
func TestCreateExternalEntities_KeyAttributeIsNeverUpdatable(t *testing.T) {
	ent, _ := importOne(t, writableSetMetadata, "Definition")
	byName := attrByName(ent)

	// `Id` is a Mendix reserved word, so the import renames it — the CE6630
	// in the report names `DefinitionId` for that reason.
	key := byName["DefinitionId"]
	if key == nil {
		t.Fatalf("key attribute DefinitionId missing; got %v", attrNames(ent))
	}
	if key.Updatable {
		t.Error("key attribute is Updatable=true against a service that computes it False — CE6630 " +
			`"'DefinitionId' is marked Updatable=False in the OData service, but True in the app."`)
	}

	if !key.Creatable {
		t.Error("control failed: the key must stay Creatable — it is written once, at creation, " +
			"and clearing both capabilities is CE6630 inverted")
	}
}

// A key is NOT read-only: it is written once, at creation. The report's build
// flagged the key Updatable=False *only* — no Creatable error accompanied it —
// so the rule is "a key cannot be changed after creation", not "the entity is
// read-only". Clearing Creatable too would be the obvious over-correction and
// its own CE6630.
func TestCreateExternalEntities_KeyAttributeStaysCreatable(t *testing.T) {
	ent, _ := importOne(t, writableSetMetadata, "Definition")
	byName := attrByName(ent)

	key := byName["DefinitionId"]
	if key == nil {
		t.Fatalf("key attribute DefinitionId missing; got %v", attrNames(ent))
	}
	if !key.Creatable {
		t.Error("key attribute lost Creatable against an Insertable=true set — " +
			"the key is set at creation, so this is CE6630 the other way")
	}
}

// The second direction: a set that is insertable but not updatable. The key
// must be non-updatable here too (it already was, via the set), and Creatable
// must still follow the contract — so the key fix must not be written as
// "clear both capabilities on a key".
func TestCreateExternalEntities_InsertableButNotUpdatableSet(t *testing.T) {
	ent, _ := importOne(t, insertOnlySetMetadata, "Definition")
	byName := attrByName(ent)

	key := byName["DefinitionId"]
	if key == nil {
		t.Fatalf("key attribute DefinitionId missing; got %v", attrNames(ent))
	}
	if key.Updatable {
		t.Error("key attribute Updatable=true against a non-updatable set — CE6630")
	}
	if !key.Creatable {
		t.Error("key attribute Creatable=false against an Insertable=true set — CE6630")
	}

	label := byName["Label"]
	if label == nil {
		t.Fatalf("attribute Label missing; got %v", attrNames(ent))
	}
	if label.Updatable {
		t.Error("non-key attribute Updatable=true against a non-updatable set — CE6630")
	}
	if !label.Creatable {
		t.Error("control failed: a non-key attribute of an insertable set still follows the set's Insertable=true")
	}
}

// A base type with an entity set, plus a type derived from it that has NONE —
// TripPin's Person/Employee shape, reduced. The derived entity is reached
// through its parent's write flow, which is why its capabilities go the other
// way from a top-level entity's.
const derivedTypeMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Person">
        <Key><PropertyRef Name="UserName"/></Key>
        <Property Name="UserName" Type="Edm.String" Nullable="false" MaxLength="36"/>
        <Property Name="Label" Type="Edm.String" MaxLength="100"/>
      </EntityType>
      <EntityType Name="Employee" BaseType="App.Model.Person">
        <Property Name="Cost" Type="Edm.Decimal"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="People" EntityType="App.Model.Person">
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

// The control the live TripPin contract supplied, after a blanket "a key is
// never updatable" turned the reported CE6630 into seven of its inverse:
//
//	[error] [CE6630] "'TripId' is marked Updatable=True in the OData service,
//	                  but False in the app."
//	        at Attribute 'TripPinClient.Trip.TripId'
//
// measured on mxbuild 11.12.2 over Trip, PlanItem, Event, Flight,
// PublicTransportation, Employee and Manager — every one of them a derived or
// contained type with no entity set of its own, mutated through its parent's
// write flow. The entity sets in the same contract (Person, Airline, Airport)
// stayed silent at Updatable=false.
//
// `UserName` is the sharpest form of it: the SAME property is expected False on
// Person and True on Employee and Manager, so the split is the entity set and
// cannot be inheritance or the property itself. That is why the key rule is
// gated on isTopLevel, and why this test exists beside the top-level one — the
// pair is the two-sided control, and either alone passes against a fix that is
// wrong in the other direction.
func TestCreateExternalEntities_DerivedTypeKeyStaysUpdatable(t *testing.T) {
	all, _ := importComplexTypeContract(t, derivedTypeMetadata)

	top := all["People"]
	if top == nil {
		t.Fatalf("top-level entity People not created; got %v", entityNamesOf(all))
	}
	if k := attrByName(top)["UserName"]; k == nil {
		t.Fatal("People.UserName missing")
	} else if k.Updatable {
		t.Error("top-level key is Updatable=true — CE6630 " +
			`"'UserName' is marked Updatable=False in the OData service, but True in the app."`)
	}

	derived := all["Employee"]
	if derived == nil {
		t.Fatalf("derived entity Employee not created; got %v", entityNamesOf(all))
	}
	if k := attrByName(derived)["UserName"]; k == nil {
		t.Fatal("Employee.UserName missing")
	} else if !k.Updatable {
		t.Error("derived-type key lost Updatable — CE6630 inverted, the TripPin failure: " +
			`"'UserName' is marked Updatable=True in the OData service, but False in the app."`)
	}
}

func entityNamesOf(m map[string]*domainmodel.Entity) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// A top-level entity set that names the KEY as the only non-updatable property:
// the service is saying, by name, that `Label` IS updatable. Nothing states the
// case more plainly, and mxbuild still computes False for it.
const explicitNonUpdatableMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Definition">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.String" Nullable="false" MaxLength="36"/>
        <Property Name="Label" Type="Edm.String" MaxLength="100"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="Definition" EntityType="App.Model.Definition">
          <Annotation Term="Org.OData.Capabilities.V1.InsertRestrictions">
            <Record><PropertyValue Property="Insertable" Bool="true"/></Record>
          </Annotation>
          <Annotation Term="Org.OData.Capabilities.V1.UpdateRestrictions">
            <Record>
              <PropertyValue Property="Updatable" Bool="true"/>
              <PropertyValue Property="NonUpdatableProperties">
                <Collection><PropertyPath>Id</PropertyPath></Collection>
              </PropertyValue>
            </Record>
          </Annotation>
        </EntitySet>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

// NO attribute of a TOP-LEVEL external entity is updatable, whatever the entity
// set's UpdateRestrictions say. Following the annotation is one CE6630 per
// attribute — "'Label' is marked Updatable=False in the OData service, but True
// in the app."
//
// Measured on mxbuild 11.12.1 across TEN contract shapes, every one of them a
// top-level set mxbuild reads as updatable, every one answering False:
//
//	inline <Record>                     typed <Record Type=…>
//	UpdateMethod=PATCH                  +NonUpdatableProperties +DeleteRestrictions
//	unannotated                         external <Annotations Target=…>
//	Core.Permissions/ReadWrite          Core.OptimisticConcurrency (ETag)
//	DeepUpdateSupport/Supported=true    NonUpdatableProperties naming ONLY the key
//
// The last is the one that closes it: the service lists `Id` as the sole
// non-updatable property, so it is asserting that `Label` is updatable, and
// mxbuild still says False. The aliased-vocabulary run is the other half — with
// mxbuild reading an Updatable=true set and the app at false on every
// attribute, the build reported Creatable errors and NO Updatable error, so
// False is what it wants rather than merely what it tolerates.
//
// Two things this rule is NOT, each ruled out by its own control:
//
//   - It is not "the entity is read-only". Creatable follows Insertable and
//     stays true on the same attributes — asserted below, and the reason a
//     blanket read-only stamp cannot pass this test.
//   - It is not the model's "allow creating and changing objects locally".
//     Setting AllowCreateChangeLocally=Yes on a top-level entity left the
//     expectation at False (measured). An external entity's object can be
//     changed in memory and handed to an external action; that is governed by
//     that flag, while this one mirrors what the endpoint itself accepts.
//
// Non-top-level entities go the other way — see
// TestCreateExternalEntities_DerivedTypeKeyStaysUpdatable.
func TestCreateExternalEntities_TopLevelAttributesAreNeverUpdatable(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata string
	}{
		{"updatable set", writableSetMetadata},
		{"non-updatable properties names only the key", explicitNonUpdatableMetadata},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ent, _ := importOne(t, tc.metadata, "Definition")
			byName := attrByName(ent)

			for _, n := range []string{"DefinitionId", "Label"} {
				a := byName[n]
				if a == nil {
					t.Fatalf("attribute %s missing; got %v", n, attrNames(ent))
				}
				if a.Updatable {
					t.Errorf("%s is Updatable=true on a top-level entity — CE6630 "+
						`"'%s' is marked Updatable=False in the OData service, but True in the app."`, n, n)
				}
				// The control: Creatable goes the other way on the very same
				// attribute, so this cannot pass against a read-only stamp.
				if !a.Creatable {
					t.Errorf("%s lost Creatable against an Insertable=true set — CE6630 inverted", n)
				}
			}
		})
	}
}
