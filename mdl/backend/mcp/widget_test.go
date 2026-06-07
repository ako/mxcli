// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

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
	wb, _ := b.LoadWidgetTemplate("com.mendix.widget.web.datagrid.Datagrid", "")
	w := wb.(*mcpWidgetBuilder)
	cw := w.Finalize(model.ID("dg1"), "dg", "", "Always")
	if _, err := b.mapPageWidget(cw); err == nil {
		t.Error("non-combobox pluggable widget should be rejected")
	}
}

func TestMapCustomWidget_UnsupportedPropertyRejected(t *testing.T) {
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate(comboboxWidgetID, "")
	w := wb.(*mcpWidgetBuilder)
	w.SetAttribute("attributeEnumeration", "M.E.Status")
	w.SetObjectList("columns", nil) // records an unsupported op
	cw := w.Finalize(model.ID("cw2"), "cmb", "", "Always")
	if _, err := b.mapPageWidget(cw); err == nil {
		t.Error("combobox using an unsupported property op should be rejected")
	}
}
