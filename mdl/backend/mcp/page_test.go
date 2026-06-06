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

func TestMapPageWidget_Unsupported(t *testing.T) {
	b := &Backend{}
	dt := &pages.DynamicText{}
	dt.Name = "txt"
	if _, err := b.mapPageWidget(dt); err == nil {
		t.Error("an unmapped widget type should error")
	}
}
