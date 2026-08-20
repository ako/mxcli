// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// stubWidgetDeps serializes a widget to a marker document, so these tests assert
// the Templates plumbing without pulling in an engine's serializer.
type stubWidgetDeps struct {
	Deps
}

func (d *stubWidgetDeps) SerializeWidget(w pages.Widget) bson.D {
	return bson.D{{Key: "$Type", Value: "Forms$TextBox"}, {Key: "Name", Value: w.GetName()}}
}

func listViewDoc(entities ...string) bson.D {
	templates := bson.A{int32(3)}
	if len(entities) > 0 {
		templates = bson.A{int32(2)}
		for _, e := range entities {
			templates = append(templates, bson.D{
				{Key: "$Type", Value: "Forms$ListViewTemplate"},
				{Key: "Entity", Value: e},
				{Key: "Widgets", Value: bson.A{int32(3)}},
			})
		}
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$ListView"},
		{Key: "Name", Value: "vehicleListView"},
		{Key: "Widgets", Value: bson.A{int32(3)}},
		{Key: "Templates", Value: templates},
	}
}

func templateEntitiesOf(t *testing.T, m *Mutator) []string {
	t.Helper()
	lv := m.widgetFinder(m.rawData, "vehicleListView")
	if lv == nil {
		t.Fatal("list view not found")
	}
	return storedTemplateEntities(lv.widget)
}

// TestInsertListViewTemplates_AppendsToTemplatesNotWidgets is the load-bearing
// assertion: a Forms$ListViewTemplate is NOT a widget, and the list view's
// Widgets array is its default body. Routing a template through the widget path
// would append a non-widget to the widget list, producing a page Studio Pro
// cannot open — the same reason DataGrid2 columns have their own path.
func TestInsertListViewTemplates_AppendsToTemplatesNotWidgets(t *testing.T) {
	m := New(makeRawPage(listViewDoc("Pages.Bus")), model.ID("u"), &stubWidgetDeps{})

	err := m.InsertListViewTemplates("vehicleListView", []*pages.ListViewTemplate{{
		Specialization: "Pages.Truck",
		Widgets:        []pages.Widget{&pages.TextBox{BaseWidget: pages.BaseWidget{Name: "truckLabel"}}},
	}})
	if err != nil {
		t.Fatalf("InsertListViewTemplates: %v", err)
	}

	if got := templateEntitiesOf(t, m); len(got) != 2 || got[0] != "Pages.Bus" || got[1] != "Pages.Truck" {
		t.Errorf("templates = %v, want [Pages.Bus Pages.Truck] — appended, in order", got)
	}

	lv := m.widgetFinder(m.rawData, "vehicleListView")
	body := bsonnav.DGetArrayElements(bsonnav.DGet(lv.widget, "Widgets"))
	if len(body) != 0 {
		t.Errorf("the list view's default body gained %d widget(s); a template must never land in Widgets", len(body))
	}

	// The new template carries its children and Studio Pro's key order.
	tplArr := bsonnav.DGetArrayElements(bsonnav.DGet(lv.widget, "Templates"))
	last, ok := tplArr[len(tplArr)-1].(bson.D)
	if !ok {
		t.Fatalf("template is %T, want bson.D", tplArr[len(tplArr)-1])
	}
	var keys []string
	for _, e := range last {
		keys = append(keys, e.Key)
	}
	if len(keys) != 4 || keys[0] != "$ID" || keys[1] != "$Type" || keys[2] != "Entity" || keys[3] != "Widgets" {
		t.Errorf("template keys = %v, want [$ID $Type Entity Widgets]", keys)
	}
	if n := len(bsonnav.DGetArrayElements(bsonnav.DGet(last, "Widgets"))); n != 1 {
		t.Errorf("template has %d child widget(s), want 1", n)
	}
}

// TestInsertListViewTemplates_FlipsTheEmptyMarker pins the list marker: Mendix
// writes 3 for an empty list and 2 for a populated one. Appending to an empty
// Templates array must flip it, or the document disagrees with itself.
func TestInsertListViewTemplates_FlipsTheEmptyMarker(t *testing.T) {
	m := New(makeRawPage(listViewDoc()), model.ID("u"), &stubWidgetDeps{})

	before := bsonnav.ToBsonA(bsonnav.DGet(m.widgetFinder(m.rawData, "vehicleListView").widget, "Templates"))
	if len(before) != 1 || before[0] != int32(3) {
		t.Fatalf("fixture is not an empty list: %v", before)
	}

	if err := m.InsertListViewTemplates("vehicleListView", []*pages.ListViewTemplate{{Specialization: "Pages.Bus"}}); err != nil {
		t.Fatalf("InsertListViewTemplates: %v", err)
	}

	after := bsonnav.ToBsonA(bsonnav.DGet(m.widgetFinder(m.rawData, "vehicleListView").widget, "Templates"))
	if len(after) != 2 || after[0] != int32(2) {
		t.Errorf("Templates = %v, want marker 2 followed by one template", after)
	}
}

// TestDropListViewTemplate covers the removal and the two ways it can be asked
// for something that is not there. A drop that matches nothing must NOT report
// success: that is how a typo in a specialization name becomes a silent no-op.
func TestDropListViewTemplate(t *testing.T) {
	t.Run("drops one and keeps the order of the rest", func(t *testing.T) {
		m := New(makeRawPage(listViewDoc("Pages.Bus", "Pages.Truck", "Pages.Car")), model.ID("u"), &stubWidgetDeps{})
		if err := m.DropListViewTemplate("vehicleListView", "Pages.Truck"); err != nil {
			t.Fatalf("DropListViewTemplate: %v", err)
		}
		got := templateEntitiesOf(t, m)
		if len(got) != 2 || got[0] != "Pages.Bus" || got[1] != "Pages.Car" {
			t.Errorf("templates = %v, want [Pages.Bus Pages.Car]", got)
		}
	})

	t.Run("emptying the list restores the empty marker", func(t *testing.T) {
		m := New(makeRawPage(listViewDoc("Pages.Bus")), model.ID("u"), &stubWidgetDeps{})
		if err := m.DropListViewTemplate("vehicleListView", "Pages.Bus"); err != nil {
			t.Fatalf("DropListViewTemplate: %v", err)
		}
		arr := bsonnav.ToBsonA(bsonnav.DGet(m.widgetFinder(m.rawData, "vehicleListView").widget, "Templates"))
		if len(arr) != 1 || arr[0] != int32(3) {
			t.Errorf("Templates = %v, want the empty-list marker 3", arr)
		}
	})

	t.Run("no such template names the ones that are there", func(t *testing.T) {
		m := New(makeRawPage(listViewDoc("Pages.Bus", "Pages.Car")), model.ID("u"), &stubWidgetDeps{})
		err := m.DropListViewTemplate("vehicleListView", "Pages.SUV")
		if err == nil {
			t.Fatal("dropping a template that does not exist reported success")
		}
		for _, want := range []string{"Pages.SUV", "Pages.Bus", "Pages.Car"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err.Error(), want)
			}
		}
	})

	t.Run("a list view with no templates says so", func(t *testing.T) {
		m := New(makeRawPage(listViewDoc()), model.ID("u"), &stubWidgetDeps{})
		err := m.DropListViewTemplate("vehicleListView", "Pages.SUV")
		if err == nil || !strings.Contains(err.Error(), "no specialization templates") {
			t.Errorf("err = %v, want it to say the list view has no templates", err)
		}
	})
}

// TestListViewTemplateOpsRefuseNonListViews pins that both operations check the
// target's type. A gallery's `template <name>` is a named content slot, not a
// per-specialization body, and must not be reachable through these.
func TestListViewTemplateOpsRefuseNonListViews(t *testing.T) {
	grid := bson.D{
		{Key: "$Type", Value: "Forms$DataGrid"},
		{Key: "Name", Value: "vehicleListView"},
	}
	m := New(makeRawPage(grid), model.ID("u"), &stubWidgetDeps{})

	if err := m.InsertListViewTemplates("vehicleListView", []*pages.ListViewTemplate{{Specialization: "Pages.Bus"}}); err == nil ||
		!strings.Contains(err.Error(), "not a list view") {
		t.Errorf("insert err = %v, want a refusal naming the widget type", err)
	}
	if err := m.DropListViewTemplate("vehicleListView", "Pages.Bus"); err == nil ||
		!strings.Contains(err.Error(), "not a list view") {
		t.Errorf("drop err = %v, want a refusal naming the widget type", err)
	}
}
