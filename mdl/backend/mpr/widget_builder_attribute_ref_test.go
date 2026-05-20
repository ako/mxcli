// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// TestSetAttributeRefField locks in the BSON shape for the
// pluggable-widget engine's `attribute` operation (object-list item
// sub-properties — e.g. `column colName (Attribute: NId)` inside a
// pluggablewidget 'com.mendix.widget.web.datagrid.Datagrid' block).
//
// Regression for #64: an earlier implementation wrote `$Type:
// "CustomWidgets$AttributeRef"` and only the bare attribute name. Mendix's
// type cache has no such type, so `mx update-widgets` / `mx check` failed
// with TypeCacheUnknownTypeException and Studio Pro flagged CE0463.
func TestSetAttributeRefField(t *testing.T) {
	t.Run("fully-qualified path -> DomainModels$AttributeRef", func(t *testing.T) {
		value := bson.D{
			{Key: "AttributeRef", Value: nil},
			{Key: "PrimitiveValue", Value: ""},
		}
		got := setAttributeRefField(value, "ReproUoM.UoM.NId")

		ref := findField(t, got, "AttributeRef")
		refDoc, ok := ref.(bson.D)
		if !ok {
			t.Fatalf("AttributeRef value is %T, want bson.D", ref)
		}

		gotType := findField(t, refDoc, "$Type")
		if gotType != "DomainModels$AttributeRef" {
			t.Errorf("$Type = %q, want DomainModels$AttributeRef (CustomWidgets$AttributeRef is not registered in Mendix's type cache)", gotType)
		}

		gotAttr := findField(t, refDoc, "Attribute")
		if gotAttr != "ReproUoM.UoM.NId" {
			t.Errorf("Attribute = %q, want fully-qualified ReproUoM.UoM.NId", gotAttr)
		}

		gotEntityRef := findField(t, refDoc, "EntityRef")
		if gotEntityRef != nil {
			t.Errorf("EntityRef = %v, want nil", gotEntityRef)
		}
	})

	t.Run("unqualified path -> AttributeRef nil", func(t *testing.T) {
		value := bson.D{{Key: "AttributeRef", Value: bson.D{{Key: "stale", Value: 1}}}}
		got := setAttributeRefField(value, "NId")
		if findField(t, got, "AttributeRef") != nil {
			t.Errorf("AttributeRef must be nil for unqualified path; got %v", findField(t, got, "AttributeRef"))
		}
	})
}

func findField(t *testing.T, doc bson.D, key string) any {
	t.Helper()
	for _, e := range doc {
		if e.Key == key {
			return e.Value
		}
	}
	t.Fatalf("field %q not found in BSON doc", key)
	return nil
}
