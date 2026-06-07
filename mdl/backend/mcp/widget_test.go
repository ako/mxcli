// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// buildCombobox drives the widget builder the way the pluggable engine does for
// a combobox, then maps the resulting CustomWidget to its pg form.
func buildCombobox(t *testing.T, b *Backend, drive func(w *mcpWidgetBuilder)) map[string]any {
	t.Helper()
	wb, err := b.LoadWidgetTemplate(comboboxWidgetID, "")
	if err != nil {
		t.Fatalf("LoadWidgetTemplate: %v", err)
	}
	w := wb.(*mcpWidgetBuilder)
	drive(w)
	cw := w.Finalize(model.ID("cw1"), "cmb", "Label", "Always")
	m, err := b.mapPageWidget(cw)
	if err != nil {
		t.Fatalf("mapPageWidget: %v", err)
	}
	return m
}

func TestMapCustomWidget_EnumCombobox(t *testing.T) {
	b := &Backend{}
	m := buildCombobox(t, b, func(w *mcpWidgetBuilder) {
		// Enum mode mapping: attributeEnumeration <- Attribute.
		w.SetAttribute("attributeEnumeration", "PgTest.Order.status")
	})
	if m["$Type"] != "CustomWidgets$CustomWidget" || m["widgetId"] != comboboxWidgetID {
		t.Fatalf("custom widget: %+v", m)
	}
	ob, _ := m["object"].(map[string]any)
	// optionsSourceType must be inferred so pg keeps attributeEnumeration.
	if ob["optionsSourceType"] != "enumeration" {
		t.Fatalf("optionsSourceType not inferred: %+v", ob)
	}
	ar, _ := ob["attributeEnumeration"].(map[string]any)
	if ar["$Type"] != "DomainModels$AttributeRef" || ar["attribute"] != "PgTest.Order.status" {
		t.Fatalf("attributeEnumeration: %+v", ob["attributeEnumeration"])
	}
}

func TestMapCustomWidget_AssociationCombobox(t *testing.T) {
	b := &Backend{}
	m := buildCombobox(t, b, func(w *mcpWidgetBuilder) {
		// Association mode mappings.
		w.SetPrimitive("optionsSourceType", "association")
		w.SetDataSource("optionsSourceAssociationDataSource", &pages.DatabaseSource{EntityName: "PgTest.Customer"})
		w.SetAssociation("attributeAssociation", "PgTest.Order_Customer", "PgTest.Customer")
		w.SetAttribute("optionsSourceAssociationCaptionAttribute", "PgTest.Customer.Name")
	})
	ob, _ := m["object"].(map[string]any)
	if ob["optionsSourceType"] != "association" {
		t.Fatalf("optionsSourceType: %+v", ob)
	}
	ds, _ := ob["optionsSourceAssociationDataSource"].(map[string]any)
	if ds["$Type"] != "CustomWidgets$CustomWidgetXPathSource" {
		t.Fatalf("dataSource: %+v", ds)
	}
	assoc, _ := ob["attributeAssociation"].(map[string]any)
	steps, _ := assoc["steps"].([]any)
	step0, _ := steps[0].(map[string]any)
	if step0["association"] != "PgTest.Order_Customer" || step0["destinationEntity"] != "PgTest.Customer" {
		t.Fatalf("attributeAssociation step: %+v", step0)
	}
	cap, _ := ob["optionsSourceAssociationCaptionAttribute"].(map[string]any)
	if cap["attribute"] != "PgTest.Customer.Name" {
		t.Fatalf("captionAttribute: %+v", cap)
	}
}

func TestMapCustomWidget_UnsupportedWidgetRejected(t *testing.T) {
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate("com.mendix.widget.web.barcodescanner.BarcodeScanner", "")
	w := wb.(*mcpWidgetBuilder)
	cw := w.Finalize(model.ID("bc1"), "bc", "", "Always")
	if _, err := b.mapPageWidget(cw); err == nil {
		t.Error("a pluggable widget not in widgets.def.json should be rejected")
	}
}

func TestMapCustomWidget_UnsupportedPropertyRejected(t *testing.T) {
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate(comboboxWidgetID, "")
	w := wb.(*mcpWidgetBuilder)
	w.SetAttribute("attributeEnumeration", "M.E.Status")
	w.SetAction("onChange", nil) // records an unsupported op
	cw := w.Finalize(model.ID("cw2"), "cmb", "", "Always")
	if _, err := b.mapPageWidget(cw); err == nil {
		t.Error("combobox using an unsupported property op should be rejected")
	}
}

func TestSetObjectList_DataGridColumns(t *testing.T) {
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate("com.mendix.widget.web.datagrid.Datagrid", "")
	w := wb.(*mcpWidgetBuilder)
	// The shared engine calls SetDataSource (via auto-datasource) and SetObjectList.
	w.SetDataSource("datasource", &pages.DatabaseSource{EntityName: "PgTest.Order"})
	w.SetObjectList("columns", []backend.ObjectListItemSpec{
		{Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "attribute", Operation: "attribute", AttributePath: "PgTest.Order.OrderNumber"},
			{PropertyKey: "header", Operation: "texttemplate", TextTemplate: "Order #"},
			{PropertyKey: "showContentAs", Operation: "primitive", PrimitiveVal: "attribute"},
		}},
	})
	cw := w.Finalize(model.ID("dg1"), "dg", "", "Always")
	m, err := b.mapPageWidget(cw)
	if err != nil {
		t.Fatalf("mapPageWidget: %v", err)
	}
	ob, _ := m["object"].(map[string]any)
	ds, _ := ob["datasource"].(map[string]any)
	if ds["$Type"] != "CustomWidgets$CustomWidgetXPathSource" {
		t.Fatalf("datasource not set via PropertyTypeIDs/auto-datasource: %+v", ob["datasource"])
	}
	cols, _ := ob["columns"].([]any)
	if len(cols) != 1 {
		t.Fatalf("columns: %+v", cols)
	}
	col, _ := cols[0].(map[string]any)
	if col["$Type"] != "CustomWidgets$WidgetObject" || col["ct:header"] != "Order #" || col["showContentAs"] != "attribute" {
		t.Fatalf("column: %+v", col)
	}
	ar, _ := col["attribute"].(map[string]any)
	if ar["attribute"] != "PgTest.Order.OrderNumber" {
		t.Fatalf("column attribute: %+v", col["attribute"])
	}
}

func TestCustomWidgetXPathSource_Sort(t *testing.T) {
	src := customWidgetXPathSource(&pages.DatabaseSource{
		EntityName: "PgTest.Order",
		Sorting: []*pages.GridSort{
			{AttributePath: "PgTest.Order.OrderNumber", Direction: pages.SortDirectionAscending},
			{AttributePath: "PgTest.Order.OrderDate", Direction: pages.SortDirectionDescending},
		},
	})
	sb, _ := src["sortBar"].(map[string]any)
	if sb["$Type"] != "Pages$GridSortBar" {
		t.Fatalf("sortBar: %+v", src["sortBar"])
	}
	items, _ := sb["sortItems"].([]any)
	if len(items) != 2 {
		t.Fatalf("sortItems: %+v", items)
	}
	i0, _ := items[0].(map[string]any)
	if ar, _ := i0["attributeRef"].(map[string]any); ar["attribute"] != "PgTest.Order.OrderNumber" || i0["sortDirection"] != "Ascending" {
		t.Fatalf("sortItem[0]: %+v", i0)
	}
	i1, _ := items[1].(map[string]any)
	if i1["sortDirection"] != "Descending" {
		t.Fatalf("sortItem[1] direction: %+v", i1)
	}
	// No sort -> no sortBar key (pg fills an empty default).
	plain := customWidgetXPathSource(&pages.DatabaseSource{EntityName: "PgTest.Order"})
	if _, ok := plain["sortBar"]; ok {
		t.Fatalf("unexpected sortBar on unsorted source: %+v", plain)
	}
}

func TestCustomWidgetXPathSource_Constraint(t *testing.T) {
	src := customWidgetXPathSource(&pages.DatabaseSource{
		EntityName:      "PgTest.Order",
		XPathConstraint: "[OrderNumber > 100]",
	})
	if src["xPathConstraint"] != "[OrderNumber > 100]" {
		t.Fatalf("xPathConstraint: %+v", src["xPathConstraint"])
	}
	// No constraint -> no key.
	plain := customWidgetXPathSource(&pages.DatabaseSource{EntityName: "PgTest.Order"})
	if _, ok := plain["xPathConstraint"]; ok {
		t.Fatalf("unexpected xPathConstraint on unconstrained source: %+v", plain)
	}
}

func TestMapDataViewSource_ConstraintRejected(t *testing.T) {
	// pg's Pages$DataViewSource silently drops a constraint, so it must be rejected.
	_, err := mapDataViewSource(&pages.DatabaseSource{EntityName: "Sales.Order", XPathConstraint: "[Total > 0]"})
	if err == nil {
		t.Error("data-view database source with an XPath constraint should be rejected")
	}
}

func TestMapCustomWidget_Gallery(t *testing.T) {
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate("com.mendix.widget.web.gallery.Gallery", "")
	w := wb.(*mcpWidgetBuilder)
	// Gallery's def.json maps datasource explicitly and delivers the template
	// body via SetChildWidgets(content, ...).
	w.SetDataSource("datasource", &pages.DatabaseSource{EntityName: "PgTest.Order"})
	w.SetSelection("itemSelection", "Multi")
	row := &pages.DynamicText{Content: &pages.ClientTemplate{
		Template:   &model.Text{Translations: map[string]string{"en_US": "{1}"}},
		Parameters: []*pages.ClientTemplateParameter{{AttributeRef: "PgTest.Order.OrderNumber"}},
	}}
	row.Name = "dtNum"
	w.SetChildWidgets("content", []pages.Widget{row})
	cw := w.Finalize(model.ID("g1"), "gal", "", "Always")

	m, err := b.mapPageWidget(cw)
	if err != nil {
		t.Fatalf("mapPageWidget: %v", err)
	}
	ob, _ := m["object"].(map[string]any)
	if ob["itemSelection"] != "Multi" {
		t.Fatalf("itemSelection: %+v", ob["itemSelection"])
	}
	if ds, _ := ob["datasource"].(map[string]any); ds["$Type"] != "CustomWidgets$CustomWidgetXPathSource" {
		t.Fatalf("datasource: %+v", ob["datasource"])
	}
	content, _ := ob["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content widgets: %+v", content)
	}
	dt, _ := content[0].(map[string]any)
	if dt["$Type"] != "Pages$DynamicText" {
		t.Fatalf("content[0]: %+v", dt)
	}
	// The "{1}" binding must survive as a full ClientTemplate with a parameter.
	ct, _ := dt["ct:content"].(map[string]any)
	params, _ := ct["parameters"].([]any)
	if len(params) != 1 {
		t.Fatalf("template parameters dropped: %+v", dt["ct:content"])
	}
	p0, _ := params[0].(map[string]any)
	ar, _ := p0["attributeRef"].(map[string]any)
	if ar["attribute"] != "PgTest.Order.OrderNumber" {
		t.Fatalf("param attributeRef: %+v", p0)
	}
}

func TestSetObjectList_ColumnFilterAndCustomContent(t *testing.T) {
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate("com.mendix.widget.web.datagrid.Datagrid", "")
	w := wb.(*mcpWidgetBuilder)

	// A filter widget is itself a pluggable CustomWidget the engine builds and
	// registers; mapping the column's `filter` slot must recurse into it.
	filterWB, _ := b.LoadWidgetTemplate("com.mendix.widget.web.datagridtextfilter.DatagridTextFilter", "")
	filterW := filterWB.(*mcpWidgetBuilder)
	filterW.SetPrimitive("attrChoice", "auto")
	filter := filterW.Finalize(model.ID("tf1"), "tf", "", "Always")

	// A custom-content cell widget in the column's `content` slot.
	cell := &pages.DynamicText{Content: &pages.ClientTemplate{Template: &model.Text{Translations: map[string]string{"en_US": "x"}}}}
	cell.Name = "cell"

	w.SetObjectList("columns", []backend.ObjectListItemSpec{
		{
			Properties:   []backend.ObjectListItemProperty{{PropertyKey: "attribute", Operation: "attribute", AttributePath: "PgTest.Order.OrderNumber"}},
			ChildWidgets: map[string][]pages.Widget{"filter": {filter}, "content": {cell}},
		},
	})
	cw := w.Finalize(model.ID("dg2"), "dg", "", "Always")
	m, err := b.mapPageWidget(cw)
	if err != nil {
		t.Fatalf("mapPageWidget: %v", err)
	}
	cols, _ := m["object"].(map[string]any)["columns"].([]any)
	col, _ := cols[0].(map[string]any)
	fl, _ := col["filter"].([]any)
	if len(fl) != 1 {
		t.Fatalf("column filter slot: %+v", col["filter"])
	}
	f0, _ := fl[0].(map[string]any)
	if f0["widgetId"] != "com.mendix.widget.web.datagridtextfilter.DatagridTextFilter" {
		t.Fatalf("filter widget: %+v", f0)
	}
	if cc, _ := col["content"].([]any); len(cc) != 1 {
		t.Fatalf("column content slot: %+v", col["content"])
	}
}
