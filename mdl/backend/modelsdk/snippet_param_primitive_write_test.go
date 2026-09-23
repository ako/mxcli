// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// mendixlabs/mxcli#1028 — shape guard on the one builder that now serves both
// Forms$PageParameter and Forms$SnippetParameter. Both declare ParameterType as
// the polymorphic DataTypes$DataType (generated/metamodel:
// PagesSnippetParameter.ParameterType *DataTypesDataType), so one builder is
// correct for both, and snippetParameterToGen no longer keeps a second copy
// that could only ever produce an ObjectType.
//
// A primitive snippet parameter is refused upstream (MDL087 — mxbuild rejects
// it with CE0046, measured), so these rows are not a supported authoring path.
// They pin the builder's contract: whatever type it is handed, it writes THAT
// type, never an ObjectType pointing at an entity named "" — which is what the
// duplicate copy produced, and the reason a wrong parameter type used to show
// up as a dangling reference rather than as anything an author could read.
func snippetParamType(t *testing.T, p *pages.SnippetParameter) bson.Raw {
	t.Helper()
	b, err := (&codec.Encoder{}).Encode(snippetParameterToGen(p))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	child, err := bson.Raw(b).LookupErr("ParameterType")
	if err != nil {
		t.Fatalf("no ParameterType in %v", bson.Raw(b))
	}
	doc, ok := child.DocumentOK()
	if !ok {
		t.Fatalf("ParameterType is not a document: %v", child)
	}
	return doc
}

func TestSnippetParameterToGen_PrimitiveWritesItsOwnDataType(t *testing.T) {
	for _, tc := range []struct{ stored, wantType string }{
		{"DataTypes$StringType", "DataTypes$StringType"},
		{"DataTypes$IntegerType", "DataTypes$IntegerType"},
		{"DataTypes$DecimalType", "DataTypes$DecimalType"},
		{"DataTypes$BooleanType", "DataTypes$BooleanType"},
		{"DataTypes$DateTimeType", "DataTypes$DateTimeType"},
	} {
		t.Run(tc.stored, func(t *testing.T) {
			doc := snippetParamType(t, &pages.SnippetParameter{Name: "P", Type: tc.stored})
			got, err := doc.LookupErr("$Type")
			if err != nil {
				t.Fatalf("no $Type: %v", doc)
			}
			if got.StringValue() != tc.wantType {
				t.Errorf("$Type = %q, want %q", got.StringValue(), tc.wantType)
			}
			// A primitive type has no Entity — writing one would be a key the
			// type does not declare.
			if _, err := doc.LookupErr("Entity"); err == nil {
				t.Errorf("primitive ParameterType carries an Entity key: %v", doc)
			}
		})
	}
}

// CONTROL: an entity parameter is unchanged — still a DataTypes$ObjectType
// naming the entity. Without this the fix could be "always write a StringType".
func TestSnippetParameterToGen_EntityStillWritesObjectType(t *testing.T) {
	doc := snippetParamType(t, &pages.SnippetParameter{Name: "C", EntityName: "Sales.Customer"})
	ty, err := doc.LookupErr("$Type")
	if err != nil || ty.StringValue() != "DataTypes$ObjectType" {
		t.Fatalf("$Type = %v, want DataTypes$ObjectType", ty)
	}
	ent, err := doc.LookupErr("Entity")
	if err != nil || ent.StringValue() != "Sales.Customer" {
		t.Errorf("Entity = %v, want Sales.Customer", ent)
	}
}
