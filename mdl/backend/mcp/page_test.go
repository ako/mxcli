// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestMapPageWidget_ActionButton(t *testing.T) {
	b := &Backend{}
	btn := &pages.ActionButton{
		Caption:     &model.Text{Translations: map[string]string{"en_US": "Click Me"}},
		ButtonStyle: pages.ButtonStylePrimary,
		Action:      &pages.NoClientAction{},
	}
	btn.Name = "btn1"
	m, err := b.mapPageWidget(btn)
	if err != nil || m["$Type"] != "Pages$ActionButton" || m["name"] != "btn1" ||
		m["ct:caption"] != "Click Me" || m["buttonStyle"] != "Primary" {
		t.Fatalf("action button: %+v / %v", m, err)
	}
	if act, _ := m["action"].(map[string]any); act["$Type"] != "Pages$NoClientAction" {
		t.Fatalf("action: %+v", m["action"])
	}
	if ap, _ := m["appearance"].(map[string]any); ap["$Type"] != "Pages$Appearance" {
		t.Fatalf("appearance: %+v", m["appearance"])
	}
}

func TestMapPageWidget_Container(t *testing.T) {
	b := &Backend{}
	inner := &pages.ActionButton{Action: &pages.NoClientAction{}}
	inner.Name = "btn"
	c := &pages.Container{Widgets: []pages.Widget{inner}}
	c.Name = "box1"
	m, err := b.mapPageWidget(c)
	if err != nil || m["$Type"] != "Pages$DivContainer" || m["name"] != "box1" {
		t.Fatalf("container: %+v / %v", m, err)
	}
	kids, _ := m["widgets"].([]any)
	if len(kids) != 1 {
		t.Fatalf("expected 1 child widget: %+v", kids)
	}
}

func TestMapClientAction(t *testing.T) {
	none, _ := mapClientAction(nil)
	if none["$Type"] != "Pages$NoClientAction" {
		t.Errorf("nil action -> %+v", none)
	}
	mf, err := mapClientAction(&pages.MicroflowClientAction{MicroflowName: "M.ACT_Do"})
	if err != nil || mf["$Type"] != "Pages$MicroflowClientAction" || mf["microflow"] != "M.ACT_Do" {
		t.Errorf("microflow action: %+v / %v", mf, err)
	}
	pg, err := mapClientAction(&pages.PageClientAction{PageName: "M.Detail"})
	if err != nil || pg["$Type"] != "Pages$PageClientAction" {
		t.Errorf("page action: %+v / %v", pg, err)
	}
	ps, _ := pg["pageSettings"].(map[string]any)
	if ps["page"] != "M.Detail" {
		t.Errorf("pageSettings: %+v", ps)
	}
}

func TestMapPageWidget_DynamicText(t *testing.T) {
	b := &Backend{}
	dt := &pages.DynamicText{
		Content:    &pages.ClientTemplate{Template: &model.Text{Translations: map[string]string{"en_US": "$Loc/Name"}}},
		RenderMode: pages.TextRenderModeH2,
	}
	dt.Name = "txt1"
	m, err := b.mapPageWidget(dt)
	if err != nil || m["$Type"] != "Pages$DynamicText" || m["ct:content"] != "$Loc/Name" || m["renderMode"] != "H2" {
		t.Fatalf("dynamic text: %+v / %v", m, err)
	}
}

func TestMapPageWidget_DataView(t *testing.T) {
	b := &Backend{}
	inner := &pages.DynamicText{AttributePath: "$Loc/Name"}
	inner.Name = "t"
	dv := &pages.DataView{
		DataSource: &pages.DataViewSource{ParameterName: "Loc"},
		Widgets:    []pages.Widget{inner},
	}
	dv.Name = "dv1"
	m, err := b.mapPageWidget(dv)
	if err != nil || m["$Type"] != "Pages$DataView" {
		t.Fatalf("data view: %+v / %v", m, err)
	}
	src, _ := m["dataSource"].(map[string]any)
	sv, _ := src["sourceVariable"].(map[string]any)
	if sv["pageParameter"] != "Loc" {
		t.Fatalf("data view source: %+v", src)
	}
	if kids, _ := m["widgets"].([]any); len(kids) != 1 {
		t.Fatalf("data view children: %+v", m["widgets"])
	}
}

func TestMapDataViewSource(t *testing.T) {
	// page parameter -> sourceVariable
	pv, err := mapDataViewSource(&pages.DataViewSource{ParameterName: "Account"})
	if err != nil || pv["sourceVariable"] == nil {
		t.Fatalf("param source: %+v / %v", pv, err)
	}
	// direct entity -> entityRef
	er, err := mapDataViewSource(&pages.DataViewSource{EntityName: "Sales.Order"})
	if err != nil {
		t.Fatalf("entity source: %v", err)
	}
	ref, _ := er["entityRef"].(map[string]any)
	if ref["entity"] != "Sales.Order" {
		t.Fatalf("entityRef: %+v", er)
	}
	// microflow source not supported yet
	if _, err := mapDataViewSource(&pages.MicroflowSource{Microflow: "M.GetX"}); err == nil {
		t.Error("microflow data-view source should be rejected for now")
	}
}

func TestMapPageWidget_Unsupported(t *testing.T) {
	b := &Backend{}
	lv := &pages.ListView{}
	lv.Name = "lv"
	if _, err := b.mapPageWidget(lv); err == nil {
		t.Error("an unmapped widget type should error")
	}
}
