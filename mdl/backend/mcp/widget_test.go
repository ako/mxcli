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

func TestSetObjectList_CustomContentRejected(t *testing.T) {
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate("com.mendix.widget.web.datagrid.Datagrid", "")
	w := wb.(*mcpWidgetBuilder)
	dt := &pages.DynamicText{}
	dt.Name = "cell"
	w.SetObjectList("columns", []backend.ObjectListItemSpec{
		{ChildWidgets: map[string][]pages.Widget{"content": {dt}}},
	})
	cw := w.Finalize(model.ID("dg2"), "dg", "", "Always")
	if _, err := b.mapPageWidget(cw); err == nil {
		t.Error("a custom-content (child-widget) column should be rejected for now")
	}
}
