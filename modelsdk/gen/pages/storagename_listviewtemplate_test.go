// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestListViewTemplateUsesStorageName pins the BSON key of a List View
// specialization template's entity.
//
// The generator that produced this package bound the property by its SDK name,
// "Specialization". Mendix stores it as "Entity". Both the in-repo generator and
// real documents agree:
//
//	generated/metamodel/types.go
//	    Specialization model.QualifiedName `json:"entity"`
//	                 ^ SDK name                    ^ storage name
//
//	ako/TestApp, Pages.Vehicle_Overview — all four Studio Pro authored templates
//	    {$ID, $Type, Entity, Widgets}
//
// Encode and decode are separate literals and have drifted before, so both are
// pinned here. Getting this wrong is silent in the usual way: a template written
// under "Specialization" carries a property Mendix does not have, and a reader
// keyed on it returns an empty entity for every real document — a template that
// matches nothing, with `mx check` still reporting 0 errors.
func TestListViewTemplateUsesStorageName(t *testing.T) {
	const storageKey = "Entity"

	t.Run("encode", func(t *testing.T) {
		o := NewListViewTemplate()
		o.SetSpecializationQualifiedName("Pages.Bus")

		var found bool
		for _, p := range o.Properties() {
			if p.Name() == storageKey {
				found = true
			}
			if p.Name() == "Specialization" {
				t.Errorf("property is bound as %q — that is the SDK name, not the key on disk", p.Name())
			}
		}
		if !found {
			t.Errorf("no %q property; this key is written to the .mxunit", storageKey)
		}
	})

	t.Run("decode", func(t *testing.T) {
		raw, err := bson.Marshal(bson.D{
			{Key: "$Type", Value: "Forms$ListViewTemplate"},
			{Key: storageKey, Value: "Pages.Bus"},
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		o := initListViewTemplate()
		o.InitFromRaw(bson.Raw(raw))

		if got := o.SpecializationQualifiedName(); got != "Pages.Bus" {
			t.Errorf("decoded %q, want Pages.Bus — InitFromRaw is reading the wrong key, so "+
				"every stored template reads back with no specialization", got)
		}
	})
}
