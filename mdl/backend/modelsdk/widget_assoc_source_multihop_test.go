// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// entityRefSteps returns every DomainModels$EntityRefStep document of an
// encoded IndirectEntityRef, in order.
func entityRefSteps(t *testing.T, src bsonv1.D) []bsonv1.D {
	t.Helper()
	entityRef, ok := docGet(src, "EntityRef").(bsonv1.D)
	if !ok {
		t.Fatalf("EntityRef not a document (got %T)", docGet(src, "EntityRef"))
	}
	arr, ok := docGet(entityRef, "Steps").(bsonv1.A)
	if !ok {
		t.Fatalf("Steps not a list (got %T)", docGet(entityRef, "Steps"))
	}
	var out []bsonv1.D
	for _, e := range arr {
		switch v := e.(type) {
		case bsonv1.D:
			out = append(out, v)
		case bsonv1.A:
			for _, inner := range v {
				if d, ok := inner.(bsonv1.D); ok {
					out = append(out, d)
				}
			}
		}
	}
	return out
}

// mendixlabs/mxcli#1467. A two-hop association source reached the writer as
// "A1/E1/A2/E2" and was split at the first `/` into ONE step whose
// DestinationEntity was the literal "E1/A2/E2" — a by-name reference Mendix
// cannot resolve, so the loader threw setting 'DestinationEntity' of an Entity
// ref step and the project would not open. Every hop is its own step.
func TestAssociationSource_MultiHop_OneStepPerHop(t *testing.T) {
	const path = "Mod.Order_Account/Mod.Account/Mod.Account_Flag/Mod.Flag"
	want := [][2]string{
		{"Mod.Order_Account", "Mod.Account"},
		{"Mod.Account_Flag", "Mod.Flag"},
	}

	cases := []struct {
		name     string
		widget   func() pages.Widget
		wantType string
	}{
		{"data view", func() pages.Widget {
			dv := &pages.DataView{DataSource: &pages.AssociationSource{
				BaseElement:     model.BaseElement{TypeName: "Forms$AssociationSource"},
				EntityPath:      path,
				ContextVariable: "Order",
			}}
			dv.Name = "dvFlag"
			return dv
		}, "Forms$DataViewSource"},
		{"list view", func() pages.Widget {
			lv := &pages.ListView{DataSource: &pages.AssociationSource{
				BaseElement:     model.BaseElement{TypeName: "Forms$AssociationSource"},
				EntityPath:      path,
				ContextVariable: "Order",
			}}
			lv.Name = "lvFlag"
			return lv
		}, "Forms$AssociationSource"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := encodeWidget(t, tc.widget())
			src, ok := docGet(doc, "DataSource").(bsonv1.D)
			if !ok {
				t.Fatalf("DataSource not serialized (got %T)", docGet(doc, "DataSource"))
			}
			if got := docGet(src, "$Type"); got != tc.wantType {
				t.Fatalf("$Type = %v, want %v", got, tc.wantType)
			}
			steps := entityRefSteps(t, src)
			if len(steps) != len(want) {
				t.Fatalf("got %d EntityRefSteps, want %d (one per hop): %v", len(steps), len(want), steps)
			}
			for i, w := range want {
				if got := docGet(steps[i], "$Type"); got != "DomainModels$EntityRefStep" {
					t.Errorf("step %d $Type = %v", i, got)
				}
				if got := docGet(steps[i], "Association"); got != w[0] {
					t.Errorf("step %d Association = %v, want %v", i, got, w[0])
				}
				if got := docGet(steps[i], "DestinationEntity"); got != w[1] {
					t.Errorf("step %d DestinationEntity = %v, want %v", i, got, w[1])
				}
			}
		})
	}
}
