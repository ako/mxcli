// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
)

// Real TypePointers are BSON binary UUIDs, not strings — the setter reads them
// through ExtractBinaryIDFromDoc, so the fixture has to use the same encoding or
// it tests nothing.
var (
	idClass    = "11111111-1111-1111-1111-111111111111"
	idSortable = "22222222-2222-2222-2222-222222222222"
	idAttr     = "33333333-3333-3333-3333-333333333333"
)

// columnFixture builds a column document with three properties whose schema
// kinds differ: an Expression, a primitive, and a TextTemplate.
func columnFixture() (bson.D, map[string]string, map[string]string) {
	value := func() bson.D {
		// A WidgetValue always carries every field at once — that is why the
		// value's shape cannot be inferred from which keys are present.
		return bson.D{
			{Key: "Expression", Value: ""},
			{Key: "PrimitiveValue", Value: ""},
			{Key: "TextTemplate", Value: nil},
			{Key: "AttributeRef", Value: nil},
		}
	}
	prop := func(id string) bson.D {
		return bson.D{
			{Key: "TypePointer", Value: bsonutil.IDToBsonBinary(id)},
			{Key: "Value", Value: value()},
		}
	}
	col := bson.D{{Key: "Properties", Value: bson.A{
		prop(idClass), prop(idSortable), prop(idAttr),
	}}}
	keys := map[string]string{
		idClass: "columnClass", idSortable: "sortable", idAttr: "attribute",
	}
	kinds := map[string]string{
		idClass: "Expression", idSortable: "Boolean", idAttr: "Attribute",
	}
	return col, keys, kinds
}

func fieldOf(t *testing.T, col bson.D, typePointer, field string) any {
	t.Helper()
	for _, p := range col {
		if p.Key != "Properties" {
			continue
		}
		for _, item := range p.Value.(bson.A) {
			doc := item.(bson.D)
			var tp string
			var val bson.D
			for _, e := range doc {
				switch e.Key {
				case "TypePointer":
					tp = bsonnav.ExtractBinaryIDFromDoc(e.Value)
				case "Value":
					val, _ = e.Value.(bson.D)
				}
			}
			if tp != typePointer {
				continue
			}
			for _, e := range val {
				if e.Key == field {
					return e.Value
				}
			}
		}
	}
	return nil
}

// TestSetColumnPropertyResolvesTheCreatePathAlias is the regression test for
// mendixlabs/mxcli#919. The create path aliased DynamicCellClass → columnClass
// while the ALTER path kept its own hand-written table that did not, so a
// property mxcli could write it could not then edit.
func TestSetColumnPropertyResolvesTheCreatePathAlias(t *testing.T) {
	col, keys, kinds := columnFixture()
	if err := setColumnPropertyMut(col, keys, kinds, "DynamicCellClass", "my-class"); err != nil {
		t.Fatalf("DynamicCellClass rejected: %v", err)
	}
	if got := fieldOf(t, col, idClass, "Expression"); got != "my-class" {
		t.Errorf("Expression = %v, want my-class", got)
	}
}

// TestSetColumnPropertyIsCaseInsensitiveOnSchemaKeys — a property whose MDL name
// differs from the schema key only by case needs no alias entry, which is what
// keeps the shared table down to genuine renames.
func TestSetColumnPropertyIsCaseInsensitiveOnSchemaKeys(t *testing.T) {
	for _, name := range []string{"Sortable", "sortable", "SORTABLE"} {
		col, keys, kinds := columnFixture()
		if err := setColumnPropertyMut(col, keys, kinds, name, "true"); err != nil {
			t.Errorf("%s rejected: %v", name, err)
			continue
		}
		if got := fieldOf(t, col, idSortable, "PrimitiveValue"); got != "true" {
			t.Errorf("%s: PrimitiveValue = %v, want true", name, got)
		}
	}
}

// TestSetColumnPropertyWritesTheFieldTheSchemaDeclares is the second defect: the
// setter always wrote PrimitiveValue, so an Expression-valued property got its
// value in a field Studio Pro does not read. It reported success, did not survive
// a DESCRIBE round trip, and left mx check at 0 errors.
func TestSetColumnPropertyWritesTheFieldTheSchemaDeclares(t *testing.T) {
	col, keys, kinds := columnFixture()
	if err := setColumnPropertyMut(col, keys, kinds, "columnClass", "cls"); err != nil {
		t.Fatalf("columnClass rejected: %v", err)
	}
	if got := fieldOf(t, col, idClass, "Expression"); got != "cls" {
		t.Errorf("Expression = %v, want cls", got)
	}
	if got := fieldOf(t, col, idClass, "PrimitiveValue"); got != "" {
		t.Errorf("PrimitiveValue = %v, want it left empty — the value belongs in Expression", got)
	}
}

// TestSetColumnPropertyRefusesAStructuredValue. An attribute-valued property
// needs a structured AttributeRef, not a string. Writing a plausible-looking
// string into it is the same silent corruption in a different field.
func TestSetColumnPropertyRefusesAStructuredValue(t *testing.T) {
	col, keys, kinds := columnFixture()
	err := setColumnPropertyMut(col, keys, kinds, "attribute", "Mod.Ent.Name")
	if err == nil {
		t.Fatal("an Attribute-valued property was set from a plain string")
	}
	if !strings.Contains(err.Error(), "CREATE OR REPLACE PAGE") {
		t.Errorf("error = %q, want it to point at the supported path", err)
	}
}

// TestSetColumnPropertyErrorListsWhatIsSettable — the old message named only the
// property that failed, which is no help when the name is nearly right.
func TestSetColumnPropertyErrorListsWhatIsSettable(t *testing.T) {
	col, keys, kinds := columnFixture()
	err := setColumnPropertyMut(col, keys, kinds, "NoSuchProperty", "x")
	if err == nil {
		t.Fatal("an unknown property was accepted")
	}
	for _, want := range []string{"columnClass", "sortable", "attribute"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not list %q", err, want)
		}
	}
}
