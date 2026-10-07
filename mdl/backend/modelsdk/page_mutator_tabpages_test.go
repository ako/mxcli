// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1215: InsertTabPages serializes the new pages through the
// codec by wrapping them in a tab container. Through the real serializer, the
// stored page must be a Forms$TabPage with its name, caption and children, and
// keep the $ID the builder gave it (registered for duplicate-name checks).
func TestInsertTabPages_CodecSerializer(t *testing.T) {
	existing := bson.D{
		{Key: "$ID", Value: "tp1"},
		{Key: "$Type", Value: "Forms$TabPage"},
		{Key: "Name", Value: "tpOne"},
		{Key: "Widgets", Value: bson.A{int32(2)}},
	}
	page := bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "FormCall", Value: bson.D{
			{Key: "$Type", Value: "Forms$LayoutCall"},
			{Key: "Arguments", Value: bson.A{int32(2), bson.D{
				{Key: "$Type", Value: "Forms$FormCallArgument"},
				{Key: "Widgets", Value: bson.A{int32(2), bson.D{
					{Key: "$Type", Value: "Forms$TabControl"},
					{Key: "Name", Value: "tabsMain"},
					{Key: "DefaultPagePointer", Value: "tp1"},
					{Key: "TabPages", Value: bson.A{int32(3), existing}},
				}}},
			}}},
		}},
	}
	m := pagemutator.New(page, model.ID("page-1"), codecPageDeps{})

	id := types.GenerateID()
	tp := &pages.TabPage{
		BaseElement: model.BaseElement{ID: model.ID(id), TypeName: "Forms$TabPage"},
		Name:        "tpTwo",
		Caption:     &model.Text{Translations: map[string]string{"en_US": "Two"}},
		Widgets: []pages.Widget{&pages.DynamicText{BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID()), TypeName: "Forms$DynamicText"},
			Name:        "txtTwo",
		}}},
	}
	if err := m.InsertTabPages("tpOne", "after", []*pages.TabPage{tp}); err != nil {
		t.Fatalf("insert after tpOne: %v", err)
	}

	ctl := bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetArrayElements(bsonnav.DGet(
		bsonnav.DGetDoc(page, "FormCall"), "Arguments"))[0].(bson.D), "Widgets"))[0].(bson.D)
	tabs := bsonnav.DGetArrayElements(bsonnav.DGet(ctl, "TabPages"))
	if len(tabs) != 2 {
		t.Fatalf("TabPages has %d entries, want 2", len(tabs))
	}
	added := tabs[1].(bson.D)
	if got := bsonnav.DGetString(added, "$Type"); got != "Forms$TabPage" {
		t.Errorf("$Type = %q, want Forms$TabPage", got)
	}
	if got := bsonnav.DGetString(added, "Name"); got != "tpTwo" {
		t.Errorf("Name = %q, want tpTwo", got)
	}
	if bsonnav.DGetDoc(added, "Caption") == nil {
		t.Error("Caption missing")
	}
	kids := bsonnav.DGetArrayElements(bsonnav.DGet(added, "Widgets"))
	if len(kids) != 1 || bsonnav.DGetString(kids[0].(bson.D), "Name") != "txtTwo" {
		t.Errorf("Widgets = %v, want txtTwo", kids)
	}
	if bsonnav.DGetString(ctl, "DefaultPagePointer") != "tp1" {
		t.Error("DefaultPagePointer moved off the existing default page")
	}
}
