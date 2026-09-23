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

// upstream #855: `alter page … set DataSource = $Param` was refused with
// "unsupported DataSource type for alter page set: parameter", while the same
// change through REPLACE worked — REPLACE rebuilds the widget through the CREATE
// PAGE path, which has always supported every datasource type.
//
// The parameter form is a Forms$DataViewSource whose EntityRef names the
// parameter's entity and whose SourceVariable is a Forms$PageVariable pointing
// at the page parameter. The entity is not in the statement, so it is resolved
// from the page's own Parameters list.
func makeParameterisedPage(paramType string, widgets ...bson.D) bson.D {
	doc := makeRawPage(widgets...)
	return append(doc, bson.E{Key: "Parameters", Value: bson.A{
		int32(3),
		bson.D{
			{Key: "$Type", Value: paramType},
			{Key: "Name", Value: "Order"},
			{Key: "ParameterType", Value: bson.D{
				{Key: "$Type", Value: "DataTypes$ObjectType"},
				{Key: "Entity", Value: "P855.Order"},
			}},
		},
	}})
}

func TestSetWidgetDataSource_Parameter(t *testing.T) {
	dv := bson.D{
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Name", Value: "dvOrder"},
		// Starts on a microflow source — the reported starting point.
		{Key: "DataSource", Value: bson.D{
			{Key: "$Type", Value: "Forms$MicroflowSource"},
		}},
	}
	raw := makeParameterisedPage("Forms$PageParameter", dv)

	m := New(raw, model.ID("unit-1"), nil)
	if err := m.SetWidgetDataSource("dvOrder", &pages.DataViewSource{ParameterName: "Order"}); err != nil {
		t.Fatalf("SetWidgetDataSource: %v", err)
	}

	widget := findWidgetForTest(t, m.rawData, "dvOrder")
	dsDoc := bsonnav.DGetDoc(widget, "DataSource")
	if dsDoc == nil {
		t.Fatal("DataSource missing after set")
	}
	if got := bsonnav.DGetString(dsDoc, "$Type"); got != "Forms$DataViewSource" {
		t.Errorf("$Type = %q, want Forms$DataViewSource", got)
	}

	// EntityRef must name the parameter's entity: an unresolved entity ref is a
	// by-name reference Mendix resolves to null (see #854).
	entityRef := bsonnav.DGetDoc(dsDoc, "EntityRef")
	if entityRef == nil {
		t.Fatal("EntityRef missing — the DataView has no entity to bind against")
	}
	if got := bsonnav.DGetString(entityRef, "$Type"); got != "DomainModels$DirectEntityRef" {
		t.Errorf("EntityRef $Type = %q, want DomainModels$DirectEntityRef", got)
	}
	if got := bsonnav.DGetString(entityRef, "Entity"); got != "P855.Order" {
		t.Errorf("EntityRef Entity = %q, want P855.Order (resolved from the page's Parameters)", got)
	}

	sv := bsonnav.DGetDoc(dsDoc, "SourceVariable")
	if sv == nil {
		t.Fatal("SourceVariable missing — nothing ties the DataView to the parameter")
	}
	if got := bsonnav.DGetString(sv, "$Type"); got != "Forms$PageVariable" {
		t.Errorf("SourceVariable $Type = %q, want Forms$PageVariable", got)
	}
	if got := bsonnav.DGetString(sv, "PageParameter"); got != "Order" {
		t.Errorf("PageParameter = %q, want Order", got)
	}
	// The old microflow settings must be gone, not merged alongside.
	if bsonnav.DGet(dsDoc, "MicroflowSettings") != nil {
		t.Error("MicroflowSettings survived the retype — the DataSource was merged, not replaced")
	}
}

// A snippet parameter is the same shape under a different key. Mirroring the
// stored parameter's own $Type keeps the two apart without the mutator having to
// be told which container it is editing.
func TestSetWidgetDataSource_SnippetParameter(t *testing.T) {
	dv := bson.D{
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Name", Value: "dvOrder"},
		{Key: "DataSource", Value: bson.D{{Key: "$Type", Value: "Forms$MicroflowSource"}}},
	}
	raw := makeParameterisedPage("Forms$SnippetParameter", dv)

	m := New(raw, model.ID("unit-1"), nil)
	if err := m.SetWidgetDataSource("dvOrder", &pages.DataViewSource{ParameterName: "Order"}); err != nil {
		t.Fatalf("SetWidgetDataSource: %v", err)
	}
	sv := bsonnav.DGetDoc(bsonnav.DGetDoc(findWidgetForTest(t, m.rawData, "dvOrder"), "DataSource"), "SourceVariable")
	if sv == nil {
		t.Fatal("SourceVariable missing")
	}
	if got := bsonnav.DGetString(sv, "SnippetParameter"); got != "Order" {
		t.Errorf("SnippetParameter = %q, want Order", got)
	}
	if bsonnav.DGet(sv, "PageParameter") != nil {
		t.Error("PageParameter written on a snippet parameter — Mendix resolves the wrong ref to null")
	}
}

// Naming a parameter the container does not have must fail loudly rather than
// write a DataViewSource with no EntityRef, which is the #854 shape: a project
// that will not open, with no build error to say why.
func TestSetWidgetDataSource_UnknownParameterIsRefused(t *testing.T) {
	dv := bson.D{
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Name", Value: "dvOrder"},
		{Key: "DataSource", Value: bson.D{{Key: "$Type", Value: "Forms$MicroflowSource"}}},
	}
	raw := makeParameterisedPage("Forms$PageParameter", dv)

	m := New(raw, model.ID("unit-1"), nil)
	err := m.SetWidgetDataSource("dvOrder", &pages.DataViewSource{ParameterName: "Nope"})
	if err == nil {
		t.Fatal("unknown parameter accepted — an unresolved EntityRef makes the .mpr unopenable")
	}
}

func findWidgetForTest(t *testing.T, raw bson.D, name string) bson.D {
	t.Helper()
	result := findBsonWidget(raw, name)
	if result == nil {
		t.Fatalf("widget %q not found", name)
	}
	return result.widget
}

// upstream #1032: `alter page … { set DataSource = DATABASE Mod.Entity on dvCust; }`
// reported success and left the DataView with no usable datasource at all.
//
// The cause is one mapping doing duty for several: a DATABASE source has no
// single stored shape. Studio Pro writes Forms$ListViewXPathSource on a list
// view, CustomWidgets$CustomWidgetXPathSource on a pluggable widget,
// Forms$GridXPathSource on a grid — and a DATA VIEW has no database form at
// all, which is why CREATE PAGE's dataViewSourceToGen refuses one. The mutator
// instead wrote a Forms$DataViewSource — the "data from context" source — with
// the entity in EntityRef and SourceVariable left null. DESCRIBE reads that back
// as no datasource (parseContextSource needs a SourceVariable), so the write
// looked like a deletion, and the only other signal was CE7007 at build time.
func TestSetWidgetDataSource_DatabaseOnDataViewIsRefused(t *testing.T) {
	dv := bson.D{
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Name", Value: "dvCust"},
		{Key: "DataSource", Value: bson.D{
			{Key: "$Type", Value: "Forms$MicroflowSource"},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Microflow", Value: "Test.ACT_GetCustomer"},
			}},
		}},
	}
	m := New(makeRawPage(dv), model.ID("unit-1"), nil)

	err := m.SetWidgetDataSource("dvCust", &pages.DatabaseSource{EntityName: "Test.Customer"})
	if err == nil {
		t.Fatal("a database datasource on a data view was accepted — " +
			"the write leaves the widget with no usable source and surfaces only as CE7007")
	}
	// The message has to say what a data view CAN take. Pointing at REPLACE
	// here would be wrong: REPLACE rebuilds through the CREATE PAGE path, which
	// refuses this pairing too, so the author would be sent down a dead end.
	for _, want := range []string{"data view", "database"} {
		if !strings.Contains(strings.ToLower(err.Error()), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	// Guard-don't-drop: the stored source must be untouched, not half-replaced.
	dsDoc := bsonnav.DGetDoc(findWidgetForTest(t, m.rawData, "dvCust"), "DataSource")
	if dsDoc == nil {
		t.Fatal("DataSource removed by a refused set")
	}
	if got := bsonnav.DGetString(dsDoc, "$Type"); got != "Forms$MicroflowSource" {
		t.Errorf("$Type = %q, want the stored Forms$MicroflowSource to survive", got)
	}
	if bsonnav.DGetDoc(dsDoc, "MicroflowSettings") == nil {
		t.Error("MicroflowSettings gone — the refused set still mutated the widget")
	}
}

// The same wrong mapping landed on every other widget too, unreported: a list
// view legitimately takes a database source, but its stored shape is a
// Forms$ListViewXPathSource, not the Forms$DataViewSource this wrote. Here
// REPLACE genuinely is the way out — listViewSourceToGen builds the right
// element — so the message names it.
func TestSetWidgetDataSource_DatabaseOnListViewIsRefused(t *testing.T) {
	lv := bson.D{
		{Key: "$Type", Value: "Forms$ListView"},
		{Key: "Name", Value: "lvCust"},
		{Key: "DataSource", Value: bson.D{
			{Key: "$Type", Value: "Forms$ListViewXPathSource"},
			{Key: "EntityRef", Value: bson.D{
				{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
				{Key: "Entity", Value: "Test.Customer"},
			}},
		}},
	}
	m := New(makeRawPage(lv), model.ID("unit-1"), nil)

	err := m.SetWidgetDataSource("lvCust", &pages.DatabaseSource{EntityName: "Test.Customer"})
	if err == nil {
		t.Fatal("a database datasource on a list view was accepted — " +
			"the mutator writes a Forms$DataViewSource, which is not a list view source")
	}
	if !strings.Contains(err.Error(), "replace") {
		t.Errorf("error %q does not name the path that can do this", err)
	}
	dsDoc := bsonnav.DGetDoc(findWidgetForTest(t, m.rawData, "lvCust"), "DataSource")
	if got := bsonnav.DGetString(dsDoc, "$Type"); got != "Forms$ListViewXPathSource" {
		t.Errorf("$Type = %q, want the stored Forms$ListViewXPathSource to survive", got)
	}
}

// Control: the refusal is about the DATABASE form only. Every source the
// mutator can write correctly must still go through, or the fix for #1032 is a
// regression for the retype #855 added.
func TestSetWidgetDataSource_NonDatabaseSourcesStillWork(t *testing.T) {
	cases := []struct {
		name     string
		ds       pages.DataSource
		wantType string
	}{
		{"microflow", &pages.MicroflowSource{Microflow: "Test.ACT_GetCustomer"}, "Forms$MicroflowSource"},
		{"nanoflow", &pages.NanoflowSource{Nanoflow: "Test.NF_GetCustomer"}, "Forms$NanoflowSource"},
		{"selection", &pages.ListenToWidgetSource{WidgetName: "lvCust"}, "Forms$ListenTargetSource"},
		{"parameter", &pages.DataViewSource{ParameterName: "Order"}, "Forms$DataViewSource"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dv := bson.D{
				{Key: "$Type", Value: "Forms$DataView"},
				{Key: "Name", Value: "dvOrder"},
				{Key: "DataSource", Value: bson.D{{Key: "$Type", Value: "Forms$MicroflowSource"}}},
			}
			m := New(makeParameterisedPage("Forms$PageParameter", dv), model.ID("unit-1"), nil)
			if err := m.SetWidgetDataSource("dvOrder", tc.ds); err != nil {
				t.Fatalf("SetWidgetDataSource(%T): %v", tc.ds, err)
			}
			dsDoc := bsonnav.DGetDoc(findWidgetForTest(t, m.rawData, "dvOrder"), "DataSource")
			if got := bsonnav.DGetString(dsDoc, "$Type"); got != tc.wantType {
				t.Errorf("$Type = %q, want %q", got, tc.wantType)
			}
		})
	}
}
