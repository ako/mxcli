// SPDX-License-Identifier: Apache-2.0

// An action on an object-list ITEM, over MCP (#956). The MPR path writes these
// through widgetobj/builder.go's `case "action"`; the pg builder's item switch
// had no such case, so the action fell to w.note and the widget was rejected
// rather than written — which is the right failure, but only because the case
// was missing entirely.
package mcp

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestSetObjectList_ItemActionReachesThePgObject(t *testing.T) {
	b := &Backend{}
	wb, err := b.LoadWidgetTemplate("com.mendix.widget.web.datagrid.Datagrid", "")
	if err != nil {
		t.Fatalf("LoadWidgetTemplate: %v", err)
	}
	w := wb.(*mcpWidgetBuilder)
	w.SetObjectList("columns", []backend.ObjectListItemSpec{
		{Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "attribute", Operation: "attribute", AttributePath: "M956.Thing.Name"},
			{PropertyKey: "onClick", Operation: "action",
				Action: &pages.MicroflowClientAction{MicroflowName: "M956.ACT_Clicked"}},
		}},
	})

	items, ok := w.object["columns"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("no columns on the pg object: %#v", w.object["columns"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("column is %T, want map", items[0])
	}
	act, ok := item["onClick"].(map[string]any)
	if !ok {
		t.Fatalf("no onClick on the item — the action was dropped; item keys: %v, unsupported: %v",
			keysOf(item), w.unsupported)
	}
	if act["$Type"] != "Pages$MicroflowClientAction" {
		t.Errorf("$Type = %v, want Pages$MicroflowClientAction", act["$Type"])
	}
	settings, _ := act["microflowSettings"].(map[string]any)
	if settings["microflow"] != "M956.ACT_Clicked" {
		t.Errorf("microflow = %v, want M956.ACT_Clicked", settings["microflow"])
	}
}

// PED refuses parameter mappings and every action kind but microflow and
// show-page. An item action must hit the SAME refusal the top-level slots do —
// recorded as unsupported so the widget is rejected, never written half-wired.
func TestSetObjectList_ItemActionRefusesWhatPEDCannotHold(t *testing.T) {
	b := &Backend{}
	wb, _ := b.LoadWidgetTemplate("com.mendix.widget.web.datagrid.Datagrid", "")
	w := wb.(*mcpWidgetBuilder)
	w.SetObjectList("columns", []backend.ObjectListItemSpec{
		{Properties: []backend.ObjectListItemProperty{
			{PropertyKey: "onClick", Operation: "action",
				Action: &pages.NanoflowClientAction{NanoflowName: "M956.NF"}},
		}},
	})

	item := w.object["columns"].([]any)[0].(map[string]any)
	if _, wrote := item["onClick"]; wrote {
		t.Error("a nanoflow action was written to the pg object — customWidgetClientAction refuses it")
	}
	if len(w.unsupported) == 0 {
		t.Error("the refusal was silent — it must be recorded so the widget is rejected")
	}
	if !strings.Contains(strings.Join(w.unsupported, " "), "onClick") {
		t.Errorf("unsupported notes do not name the slot: %v", w.unsupported)
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
