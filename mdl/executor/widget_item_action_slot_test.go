// SPDX-License-Identifier: Apache-2.0

// Action slots on object-list ITEMS — the remaining half of upstream #956.
//
// The top-level slots became authorable in #230 (named action slots). An action
// on a list *item* did not: a chart series' staticOnClickAction, a popupmenu
// item's action, a maps marker's onClick, an accordion group's
// onToggleCollapsed, an htmlelement event's eventAction. Measured on one
// ordinary project: 19 such slots across 6 widgets.
//
// Everything under the executor was already in place — the .def.json carries the
// mapping (operationForType returns "action"), backend.ObjectListItemProperty
// has an Action field, and widgetobj/builder.go writes Value.Action. The gap was
// one `default: continue` in the item-property switch, carrying a
// TODO(#538 follow-up). So the failure mode was silence: the action parsed, the
// page was written, and the slot was empty — a perfectly valid document that
// `mx check` reports 0 errors on.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// An action written on an object-list item reaches the item spec. Before the
// fix the switch fell to `default: continue` and produced no property at all.
func TestBuildObjectListItem_ActionSlotReachesTheSpec(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	mapping := &ObjectListMapping{
		PropertyKey:  "basicItems",
		MDLContainer: "ITEM",
		ItemProperties: []ItemPropertyMapping{
			{PropertyKey: "action", Operation: "action"},
		},
	}
	child := &ast.WidgetV3{Name: "it", Properties: map[string]any{
		// close_page: no backend needed to resolve it, so the assertion is about
		// the switch reaching the reader, not about microflow resolution.
		"action": &ast.ActionV3{Type: "close"},
	}}

	spec, err := e.buildObjectListItem(mapping, child)
	if err != nil {
		t.Fatalf("buildObjectListItem: %v", err)
	}

	prop, ok := itemPropertyByKey(spec.Properties, "action")
	if !ok {
		t.Fatalf("no `action` property in the item spec — the action was dropped in silence; got %d properties: %v",
			len(spec.Properties), itemPropertyKeys(spec.Properties))
	}
	if prop.Operation != "action" {
		t.Errorf("operation = %q, want \"action\"", prop.Operation)
	}
	if prop.Action == nil {
		t.Fatal("item property carries no ClientAction — widgetobj/builder.go writes Value.Action only when it is non-nil, so the slot would still serialize empty")
	}
	if _, ok := prop.Action.(*pages.ClosePageClientAction); !ok {
		t.Fatalf("client action is %T, want *pages.ClosePageClientAction", prop.Action)
	}
}

// The microflow form is the one that matters in practice, and it is also the
// hard one: `microflow M.X` parses as a *DataSourceV3 because dataSourceExprV3
// wins in widgetPropertyV3. namedActionSlotValue converts it; the item path must
// go through that same reader rather than reading the AST slot directly.
func TestBuildObjectListItem_ActionSlotAcceptsTheMicroflowFormThatParsesAsADataSource(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{
		// Resolve the microflow through the created-this-session cache, so the
		// test says something about the action slot rather than about
		// hierarchy lookups.
		execCache: &executorCache{createdMicroflows: map[string]*createdMicroflowInfo{
			"M956.ACT_Clicked": {ID: "mf-1"},
		}},
	}}
	mapping := &ObjectListMapping{
		PropertyKey:  "series",
		MDLContainer: "SERIES",
		ItemProperties: []ItemPropertyMapping{
			{PropertyKey: "staticOnClickAction", Operation: "action"},
		},
	}
	child := &ast.WidgetV3{Name: "s1", Properties: map[string]any{
		"staticOnClickAction": &ast.DataSourceV3{Type: "microflow", Reference: "M956.ACT_Clicked"},
	}}

	spec, err := e.buildObjectListItem(mapping, child)
	if err != nil {
		t.Fatalf("buildObjectListItem: %v", err)
	}
	prop, ok := itemPropertyByKey(spec.Properties, "staticOnClickAction")
	if !ok {
		t.Fatalf("no `staticOnClickAction` property; got %v", itemPropertyKeys(spec.Properties))
	}
	mf, ok := prop.Action.(*pages.MicroflowClientAction)
	if !ok {
		t.Fatalf("client action is %T, want *pages.MicroflowClientAction", prop.Action)
	}
	if mf.MicroflowName != "M956.ACT_Clicked" {
		t.Errorf("microflow = %q, want \"M956.ACT_Clicked\"", mf.MicroflowName)
	}
}

// An unset action slot must produce no property, not an empty one: writing a
// NoAction over a slot the widget already had would clear it.
func TestBuildObjectListItem_UnsetActionSlotWritesNothing(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	mapping := &ObjectListMapping{
		PropertyKey:  "basicItems",
		MDLContainer: "ITEM",
		ItemProperties: []ItemPropertyMapping{
			{PropertyKey: "action", Operation: "action"},
		},
	}
	child := &ast.WidgetV3{Name: "it", Properties: map[string]any{"caption": "Hi"}}

	spec, err := e.buildObjectListItem(mapping, child)
	if err != nil {
		t.Fatalf("buildObjectListItem: %v", err)
	}
	if _, ok := itemPropertyByKey(spec.Properties, "action"); ok {
		t.Error("an unset action slot produced a property — an empty action is not the same as an unset one")
	}
}

// A datasource shape with no action form must be refused, not coerced. Same
// rule the top-level named slots follow.
func TestBuildObjectListItem_ActionSlotRejectsANonFlowDataSource(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	mapping := &ObjectListMapping{
		PropertyKey:  "basicItems",
		MDLContainer: "ITEM",
		ItemProperties: []ItemPropertyMapping{
			{PropertyKey: "action", Operation: "action"},
		},
	}
	child := &ast.WidgetV3{Name: "it", Properties: map[string]any{
		"action": &ast.DataSourceV3{Type: "database", Reference: "M956.Thing"},
	}}

	if _, err := e.buildObjectListItem(mapping, child); err == nil {
		t.Fatal("a database data source in an action slot was accepted — it names something an action cannot hold")
	}
}

type backendItemProp = backend.ObjectListItemProperty

func itemPropertyByKey(props []backendItemProp, key string) (backendItemProp, bool) {
	for _, p := range props {
		if p.PropertyKey == key {
			return p, true
		}
	}
	return backendItemProp{}, false
}

func itemPropertyKeys(props []backendItemProp) []string {
	out := make([]string, 0, len(props))
	for _, p := range props {
		out = append(out, p.PropertyKey)
	}
	return out
}

// DESCRIBE must read an item action back, or the round trip is lossy in the
// quiet way: the page keeps the action, the description does not mention it, and
// re-executing the description clears the slot.
func TestExtractObjectListItem_ReadsAnAction(t *testing.T) {
	const idAction = "id-onclick"
	nested := map[string]string{idAction: "staticOnClickAction"}
	itemObj := map[string]any{"Properties": []any{
		int32(2), // BSON non-empty-array marker
		map[string]any{"TypePointer": idAction, "Value": map[string]any{
			"Action": map[string]any{
				// Forms$MicroflowAction, not …ClientAction: the storage name is
				// not the SDK name (writer_widgets_action.go emits this one).
				"$Type": "Forms$MicroflowAction",
				"MicroflowSettings": map[string]any{
					"$Type":     "Forms$MicroflowSettings",
					"Microflow": "M956.ACT_Clicked",
				},
			},
		}},
	}}

	item := extractObjectListItem(nil, itemObj, nested)

	var got string
	for _, p := range item.Props {
		if p.Key == "StaticOnClickAction" {
			if !p.IsRef {
				t.Error("the action was quoted — `staticOnClickAction: 'microflow …'` does not parse back")
			}
			got = p.Value
		}
	}
	if got == "" {
		t.Fatalf("no action in the described item; props = %v", item.Props)
	}
	if !strings.Contains(got, "M956.ACT_Clicked") {
		t.Errorf("described action = %q, want it to name M956.ACT_Clicked", got)
	}
}

// A NoAction is the unset default. Emitting it would add a line to every
// untouched item's description and, worse, write an explicit empty action back.
func TestExtractObjectListItem_SkipsANoAction(t *testing.T) {
	const idAction = "id-onclick"
	nested := map[string]string{idAction: "staticOnClickAction"}
	itemObj := map[string]any{"Properties": []any{
		int32(2),
		map[string]any{"TypePointer": idAction, "Value": map[string]any{
			"Action": map[string]any{"$Type": "Forms$NoAction"},
		}},
	}}

	if item := extractObjectListItem(nil, itemObj, nested); len(item.Props) != 0 {
		t.Errorf("a NoAction was described as %v — an unset slot must describe as nothing", item.Props)
	}
}
