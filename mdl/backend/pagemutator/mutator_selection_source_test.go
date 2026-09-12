// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"go.mongodb.org/mongo-driver/bson"
)

// ---------------------------------------------------------------------------
// mendixlabs/mxcli#1076: the entity walk and the datasource walk each knew a
// different subset of the ten Forms$*Source kinds, so ALTER PAGE built the
// replacement widget's bindings in the wrong scope:
//
//   - Forms$ListenTargetSource (a data view bound `datasource: selection lv`)
//     carries no EntityRef at all — only the listen target's NAME — so the walk
//     left the context at the OUTER data view and the binding re-scoped to it
//     (CE1613 "the selected attribute no longer exists").
//   - a PLUGGABLE list (Gallery, DataGrid 2) keeps its source under
//     Object.Properties[datasource].Value.DataSource, which the flow reader
//     never looked at, so a microflow/nanoflow-sourced gallery contributed
//     nothing and the binding was written unbound (CE0402).
//
// Both are the FINDINGS #55 / #935 failure again: the walk must resolve EVERY
// source kind, and a nearer source that resolves to nothing must SHADOW the
// outer entity rather than let it be inherited.
// ---------------------------------------------------------------------------

// selectionDataView builds `dataview <name> (datasource: selection <target>)`.
func selectionDataView(name, target string, children ...bson.D) bson.D {
	arr := bson.A{int32(2)}
	for _, c := range children {
		arr = append(arr, c)
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Name", Value: name},
		{Key: "Widgets", Value: arr},
		{Key: "DataSource", Value: bson.D{
			{Key: "$Type", Value: "Forms$ListenTargetSource"},
			{Key: "ListenTarget", Value: target},
		}},
	}
}

// dataViewOn builds an ordinary entity-bound data view.
func dataViewOn(name, entity string, children ...bson.D) bson.D {
	arr := bson.A{int32(2)}
	for _, c := range children {
		arr = append(arr, c)
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Name", Value: name},
		{Key: "Widgets", Value: arr},
		{Key: "DataSource", Value: bson.D{
			{Key: "$Type", Value: "Forms$DataViewSource"},
			{Key: "EntityRef", Value: directEntityRef(entity)},
		}},
	}
}

// pluggableList builds a Gallery-shaped CustomWidget: a `datasource` property
// holding the given DataSource document, and a `content` property holding the
// template widgets. This is where a pluggable widget keeps its source — NOT at
// the widget's top level, which is the only place the flow reader looked.
func pluggableList(name string, dataSource bson.D, content bson.A) bson.D {
	dsID := idBin(0x40)
	contentID := idBin(0x41)
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: name},
		{Key: "Type", Value: bson.D{
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{
					int32(2),
					bson.D{{Key: "$ID", Value: dsID}, {Key: "PropertyKey", Value: "datasource"}},
					bson.D{{Key: "$ID", Value: contentID}, {Key: "PropertyKey", Value: "content"}},
				}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{
				int32(2),
				bson.D{
					{Key: "TypePointer", Value: dsID},
					{Key: "Value", Value: bson.D{{Key: "DataSource", Value: dataSource}}},
				},
				bson.D{
					{Key: "TypePointer", Value: contentID},
					{Key: "Value", Value: bson.D{{Key: "Widgets", Value: content}}},
				},
			}},
		}},
	}
}

func microflowSource(qn string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Forms$MicroflowSource"},
		{Key: "MicroflowSettings", Value: bson.D{
			{Key: "$Type", Value: "Forms$MicroflowSettings"},
			{Key: "Microflow", Value: qn},
		}},
	}
}

func databaseSource(entity string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Forms$CustomWidgetXPathSource"},
		{Key: "EntityRef", Value: directEntityRef(entity)},
	}
}

// The reported repro: a selection data view nested inside a page-level data
// view. Its children belong to the LISTEN TARGET's entity (the association
// list's destination), never to the outer data view's.
func TestEnclosingEntity_SelectionSource(t *testing.T) {
	row := makeWidget("txtRow", "Forms$DynamicText")
	lv := makeAssociationListView("lvVersions", "Bug.Version_Dashboard", "Bug.DashboardVersion", row)
	sel := selectionDataView("dvSelected", "lvVersions", makeWidget("txtPreview", "Forms$DynamicText"))
	outer := dataViewOn("dvDashboard", "Bug.Dashboard", lv, sel)
	m := &Mutator{rawData: pageWith(outer), widgetFinder: findBsonWidget}

	// REPLACE / INSERT BEFORE|AFTER: the target's enclosing scope.
	if got := m.EnclosingEntity("txtPreview"); got != "Bug.DashboardVersion" {
		t.Errorf("EnclosingEntity(txtPreview) = %q, want Bug.DashboardVersion", got)
	}
	// INSERT INTO the selection data view itself.
	if got := m.EnclosingEntityForChildren("dvSelected"); got != "Bug.DashboardVersion" {
		t.Errorf("EnclosingEntityForChildren(dvSelected) = %q, want Bug.DashboardVersion", got)
	}
	// Control: the outer data view still governs its own direct children, so a
	// fix cannot pass by returning the listen target's entity everywhere.
	if got := m.EnclosingEntity("lvVersions"); got != "Bug.Dashboard" {
		t.Errorf("EnclosingEntity(lvVersions) = %q, want Bug.Dashboard", got)
	}
}

// A selection source may listen to a PLUGGABLE list, whose own source lives in
// its Object.Properties rather than at the widget's top level.
func TestEnclosingEntity_SelectionOfPluggableList(t *testing.T) {
	gallery := pluggableList("galItems", databaseSource("Bug.Item"), bson.A{
		int32(2),
		makeWidget("txtCard", "Forms$DynamicText"),
	})
	sel := selectionDataView("dvSel", "galItems", makeWidget("txtSel", "Forms$DynamicText"))
	m := &Mutator{rawData: pageWith(gallery, sel), widgetFinder: findBsonWidget}

	if got := m.EnclosingEntity("txtSel"); got != "Bug.Item" {
		t.Errorf("EnclosingEntity(txtSel) = %q, want Bug.Item", got)
	}
}

// A selection source whose target is a FLOW-sourced list has no entity anywhere
// in the page: the mutator must hand the executor the flow's qualified name so
// its RETURN type can be resolved through the model.
func TestEnclosingDataSourceFlow_SelectionOfFlowSourcedList(t *testing.T) {
	lv := makeMicroflowListView("lvRows", "Bug.DS_Rows", makeWidget("txtRow", "Forms$DynamicText"))
	sel := selectionDataView("dvSel", "lvRows", makeWidget("txtSel", "Forms$DynamicText"))
	m := &Mutator{rawData: pageWith(lv, sel), widgetFinder: findBsonWidget}

	if mf, nf := m.EnclosingDataSourceFlow("txtSel", false); mf != "Bug.DS_Rows" || nf != "" {
		t.Errorf("EnclosingDataSourceFlow(txtSel) = (%q,%q), want (Bug.DS_Rows, )", mf, nf)
	}
	if mf, _ := m.EnclosingDataSourceFlow("dvSel", true); mf != "Bug.DS_Rows" {
		t.Errorf("EnclosingDataSourceFlow(dvSel, forChildren) = %q, want Bug.DS_Rows", mf)
	}
}

// A pluggable list bound to a microflow: the flow reader looked only at the
// widget's top-level DataSource key, so a Gallery sourced by a microflow
// reported no flow and the replacement widget was written unbound.
func TestEnclosingDataSourceFlow_PluggableSource(t *testing.T) {
	gallery := pluggableList("galMf", microflowSource("Bug.DS_Items"), bson.A{
		int32(2),
		makeContainerWidget("ctnCard", makeWidget("txtName", "Forms$DynamicText")),
	})
	m := &Mutator{rawData: pageWith(gallery), widgetFinder: findBsonWidget}

	if mf, nf := m.EnclosingDataSourceFlow("txtName", false); mf != "Bug.DS_Items" || nf != "" {
		t.Errorf("EnclosingDataSourceFlow(txtName) = (%q,%q), want (Bug.DS_Items, )", mf, nf)
	}
	if mf, _ := m.EnclosingDataSourceFlow("galMf", true); mf != "Bug.DS_Items" {
		t.Errorf("EnclosingDataSourceFlow(galMf, forChildren) = %q, want Bug.DS_Items", mf)
	}
}

// The same gap one level deeper: a widget inside a DataGrid 2 customContent
// cell, under a flow-sourced grid. The datasource walk never descended into an
// object-list item's own widgets — the descent the entity walk gained in #935.
func TestEnclosingDataSourceFlow_InsideCustomContentColumn(t *testing.T) {
	grid := buildDataGridWithCustomContentColumn(nil, cellContainer())
	// Swap the column grid's datasource for a microflow source.
	grid = withPluggableDataSource(t, grid, microflowSource("Bug.DS_Orders"))
	m := &Mutator{rawData: pageWith(grid), widgetFinder: findBsonWidget}

	if mf, _ := m.EnclosingDataSourceFlow("txtCust", false); mf != "Bug.DS_Orders" {
		t.Errorf("EnclosingDataSourceFlow(txtCust) = %q, want Bug.DS_Orders", mf)
	}
}

// withPluggableDataSource replaces the `datasource` property's DataSource
// document on a pluggable widget built by buildDataGridWithCustomContentColumn.
func withPluggableDataSource(t *testing.T, widget bson.D, ds bson.D) bson.D {
	t.Helper()
	keys := buildPropKeyMap(widget)
	obj := bsonnav.DGetDoc(widget, "Object")
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties")) {
		pd, ok := p.(bson.D)
		if !ok {
			continue
		}
		if keys[bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(pd, "TypePointer"))] != "datasource" {
			continue
		}
		for i, kv := range pd {
			if kv.Key == "Value" {
				pd[i].Value = bson.D{{Key: "DataSource", Value: ds}}
			}
		}
		return widget
	}
	t.Fatal("no datasource property on the test widget")
	return widget
}

// A flow-sourced list nested inside an entity-bound data view must SHADOW the
// outer entity: inheriting it is how the wrong binding got written in the first
// place. The entity is empty and the flow is reported instead.
func TestEnclosingEntity_FlowSourceShadowsOuterEntity(t *testing.T) {
	lv := makeMicroflowListView("lvRows", "Bug.DS_Rows", makeWidget("txtRow", "Forms$DynamicText"))
	outer := dataViewOn("dvWeek", "Bug.Week", lv)
	m := &Mutator{rawData: pageWith(outer), widgetFinder: findBsonWidget}

	if got := m.EnclosingEntity("txtRow"); got != "" {
		t.Errorf("EnclosingEntity(txtRow) = %q, want \"\" (the microflow list shadows Bug.Week)", got)
	}
	if mf, _ := m.EnclosingDataSourceFlow("txtRow", false); mf != "Bug.DS_Rows" {
		t.Errorf("EnclosingDataSourceFlow(txtRow) = %q, want Bug.DS_Rows", mf)
	}
}

// Robustness controls: a selection source naming a widget that does not exist,
// and two data views listening to each other, must terminate and report no
// scope — never the outer entity, and never a stack overflow.
func TestSelectionSource_DanglingAndCyclicTargets(t *testing.T) {
	dangling := selectionDataView("dvSel", "noSuchWidget", makeWidget("txtSel", "Forms$DynamicText"))
	outer := dataViewOn("dvOuter", "Bug.Week", dangling)
	m := &Mutator{rawData: pageWith(outer), widgetFinder: findBsonWidget}
	if got := m.EnclosingEntity("txtSel"); got != "" {
		t.Errorf("dangling listen target: EnclosingEntity(txtSel) = %q, want \"\"", got)
	}

	a := selectionDataView("dvA", "dvB", makeWidget("txtA", "Forms$DynamicText"))
	b := selectionDataView("dvB", "dvA")
	m2 := &Mutator{rawData: pageWith(a, b), widgetFinder: findBsonWidget}
	if got := m2.EnclosingEntity("txtA"); got != "" {
		t.Errorf("cyclic listen targets: EnclosingEntity(txtA) = %q, want \"\"", got)
	}
}
