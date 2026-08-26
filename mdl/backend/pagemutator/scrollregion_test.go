// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// layoutDoc is the shape a Forms$Layout is stored in: the tree hangs off
// Content, and the scroll container's children sit in five named slots.
func layoutDoc() bson.D {
	region := func(class string, widgets ...any) bson.D {
		return bson.D{
			{Key: "$Type", Value: "Forms$ScrollContainerRegion"},
			{Key: "SizeMode", Value: "Auto"},
			{Key: "Size", Value: int32(200)},
			{Key: "Appearance", Value: bson.D{{Key: "$Type", Value: "Forms$Appearance"}, {Key: "Class", Value: class}}},
			{Key: "Widgets", Value: append(bson.A{int32(2)}, widgets...)},
		}
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$Layout"},
		{Key: "Name", Value: "App_Default"},
		{Key: "Content", Value: bson.D{
			{Key: "$Type", Value: "Forms$WebLayoutContent"},
			{Key: "LayoutType", Value: "Responsive"},
			{Key: "Widgets", Value: bson.A{int32(2), bson.D{
				{Key: "$Type", Value: "Forms$ScrollContainer"},
				{Key: "Name", Value: "layoutContainer"},
				{Key: "Top", Value: region("region-topbar", bson.D{
					{Key: "$Type", Value: "Forms$DynamicText"},
					{Key: "Name", Value: "brandText"},
				})},
				{Key: "CenterRegion", Value: region("region-content", bson.D{
					{Key: "$Type", Value: "Forms$Placeholder"},
					{Key: "Name", Value: "Main"},
				})},
			}}},
		}},
	}
}

func newLayoutMutator(t *testing.T) *Mutator {
	t.Helper()
	m := New(layoutDoc(), model.ID("layout-1"), &stubWidgetDeps{})
	if m.ContainerType() != backend.ContainerLayout {
		t.Fatalf("container type = %v, want layout", m.ContainerType())
	}
	return m
}

// The page finder looks for FormCall, which a layout does not have, so before
// findBsonWidgetInLayout every widget in every layout was unreachable.
func TestLayoutMutator_FindsWidgetsThroughContent(t *testing.T) {
	m := newLayoutMutator(t)
	if !m.FindWidget("layoutContainer") {
		t.Error("scroll container not found: the layout finder is not reading through Content")
	}
	// Inside a named region — the descent the page finder never had, which is
	// why a layout's topbar and navigation were out of reach.
	if !m.FindWidget("brandText") {
		t.Error("widget inside the Top region not found")
	}
	if !m.FindWidget("Main") {
		t.Error("placeholder inside the CenterRegion not found")
	}
	if m.FindWidget("notThere") {
		t.Error("found a widget that does not exist")
	}
}

func TestInsertIntoScrollRegion_AppendsToTheNamedSlot(t *testing.T) {
	m := newLayoutMutator(t)
	err := m.InsertWidget("layoutContainer", "top", backend.InsertPosition("into"), []pages.Widget{
		&pages.DynamicText{BaseWidget: pages.BaseWidget{Name: "added"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	names := regionWidgetNames(t, m, "Top")
	if strings.Join(names, ",") != "brandText,added" {
		t.Errorf("Top widgets = %v, want brandText,added (appended, existing kept)", names)
	}
}

// The MDL slot name is `center`; the stored key is CenterRegion. Writing to a
// "Center" key would be accepted by nothing and read by nothing.
func TestInsertIntoScrollRegion_MapsCenterOntoCenterRegion(t *testing.T) {
	m := newLayoutMutator(t)
	if err := m.InsertWidget("layoutContainer", "center", backend.InsertPosition("into"), []pages.Widget{
		&pages.DynamicText{BaseWidget: pages.BaseWidget{Name: "added"}},
	}); err != nil {
		t.Fatal(err)
	}
	names := regionWidgetNames(t, m, "CenterRegion")
	if strings.Join(names, ",") != "Main,added" {
		t.Errorf("CenterRegion widgets = %v, want Main,added", names)
	}
	sc := scrollContainer(t, m)
	if bsonnav.DGetDoc(sc, "Center") != nil {
		t.Error(`wrote a "Center" key; the stored key is CenterRegion`)
	}
}

func TestInsertIntoScrollRegion_RefusesUnknownSlotAndEmptySlot(t *testing.T) {
	m := newLayoutMutator(t)
	err := m.InsertWidget("layoutContainer", "middle", backend.InsertPosition("into"), []pages.Widget{
		&pages.DynamicText{BaseWidget: pages.BaseWidget{Name: "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "no region") {
		t.Errorf("an unknown region must be refused, got %v", err)
	}

	// An unoccupied slot has no stored region document — the size, size mode and
	// class an INSERT would have to invent are exactly what CREATE LAYOUT states.
	err = m.InsertWidget("layoutContainer", "left", backend.InsertPosition("into"), []pages.Widget{
		&pages.DynamicText{BaseWidget: pages.BaseWidget{Name: "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "no left region") {
		t.Errorf("an empty slot must be refused with an actionable message, got %v", err)
	}

	// Control: the occupied slot next to it accepts the same insert.
	if err := m.InsertWidget("layoutContainer", "top", backend.InsertPosition("into"), []pages.Widget{
		&pages.DynamicText{BaseWidget: pages.BaseWidget{Name: "x"}},
	}); err != nil {
		t.Fatalf("control failed: %v", err)
	}
}

// BEFORE/AFTER position a widget among siblings. A region is not a sibling of
// anything, so treating them as INTO would put the widget somewhere the script
// did not ask for.
func TestInsertIntoScrollRegion_RefusesBeforeAndAfter(t *testing.T) {
	for _, pos := range []string{"before", "after"} {
		m := newLayoutMutator(t)
		err := m.InsertWidget("layoutContainer", "top", backend.InsertPosition(pos), []pages.Widget{
			&pages.DynamicText{BaseWidget: pages.BaseWidget{Name: "x"}},
		})
		if err == nil || !strings.Contains(err.Error(), "INSERT INTO") {
			t.Errorf("INSERT %s into a region must be refused, got %v", pos, err)
		}
	}
}

func scrollContainer(t *testing.T, m *Mutator) bson.D {
	t.Helper()
	content := bsonnav.DGetDoc(m.rawData, "Content")
	for _, e := range bsonnav.DGetArrayElements(bsonnav.DGet(content, "Widgets")) {
		if d, ok := e.(bson.D); ok && bsonnav.DGetString(d, "Name") == "layoutContainer" {
			return d
		}
	}
	t.Fatal("scroll container missing from the document")
	return nil
}

func regionWidgetNames(t *testing.T, m *Mutator, key string) []string {
	t.Helper()
	region := bsonnav.DGetDoc(scrollContainer(t, m), key)
	if region == nil {
		t.Fatalf("region %s missing", key)
	}
	var out []string
	for _, e := range bsonnav.DGetArrayElements(bsonnav.DGet(region, "Widgets")) {
		if d, ok := e.(bson.D); ok {
			out = append(out, bsonnav.DGetString(d, "Name"))
		}
	}
	return out
}

func TestBoundPlaceholders_StripsTheLayoutQualifiedName(t *testing.T) {
	page := bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "FormCall", Value: bson.D{
			{Key: "Form", Value: "Atlas_Core.Atlas_Default"},
			{Key: "Arguments", Value: bson.A{int32(2),
				bson.D{{Key: "Parameter", Value: "Atlas_Core.Atlas_Default.Main"}},
				bson.D{{Key: "Parameter", Value: "Atlas_Core.Atlas_Default.HeaderLeft"}},
			}},
		}},
	}
	m := New(page, model.ID("page-1"), nil)
	got := strings.Join(m.BoundPlaceholders(), ",")
	if got != "Main,HeaderLeft" {
		t.Errorf("bound placeholders = %s, want Main,HeaderLeft (unqualified)", got)
	}
}
