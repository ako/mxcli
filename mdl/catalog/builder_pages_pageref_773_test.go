// SPDX-License-Identifier: Apache-2.0

package catalog

import "testing"

// TestScanWidgetOwnRefs_PageTarget covers mendixlabs/mxcli#773: a widget that
// navigates to a page produced no reference at all, so `show references to
// <page>` reported "(no references found)" for a page reachable from a button.
//
// The BSON shapes below are verbatim from a page mxcli wrote (Mendix 11.12): a
// show-page action nests the target under Forms$FormSettings.Form, and a
// create-object-then-show-page action carries BOTH an entity and a page. The
// scan must return the page independently of the entity — a compound action
// references both, and reporting only one is how the reference graph loses edges.
func TestScanWidgetOwnRefs_PageTarget(t *testing.T) {
	tests := []struct {
		name     string
		widget   map[string]any
		wantPage string
		wantEnt  string
	}{
		{
			name: "show_page action",
			widget: map[string]any{
				"$Type": "Forms$ActionButton",
				"Name":  "btnGo",
				"Action": map[string]any{
					"$Type": "Forms$ShowPageClientAction",
					"PageSettings": map[string]any{
						"$Type": "Forms$FormSettings",
						"Form":  "Reminders.TaskGroup_SelectCategory",
					},
				},
			},
			wantPage: "Reminders.TaskGroup_SelectCategory",
		},
		{
			name: "create_object then show_page carries both refs",
			widget: map[string]any{
				"$Type": "Forms$ActionButton",
				"Name":  "btnNew",
				"Action": map[string]any{
					"$Type": "Forms$CreateObjectClientAction",
					"EntityRef": map[string]any{
						"$Type":  "DomainModels$DirectEntityRef",
						"Entity": "Reminders.TaskGroup",
					},
					"PageSettings": map[string]any{
						"$Type": "Forms$FormSettings",
						"Form":  "Reminders.TaskGroup_SelectCategory",
					},
				},
			},
			wantPage: "Reminders.TaskGroup_SelectCategory",
			wantEnt:  "Reminders.TaskGroup",
		},
		{
			name: "container with an on-click show_page",
			widget: map[string]any{
				"$Type": "Forms$DivContainer",
				"Name":  "card",
				"Action": map[string]any{
					"$Type":        "Forms$ShowPageClientAction",
					"PageSettings": map[string]any{"Form": "Mod.Detail"},
				},
			},
			wantPage: "Mod.Detail",
		},
		{
			name: "no page target",
			widget: map[string]any{
				"$Type":  "Forms$ActionButton",
				"Action": map[string]any{"$Type": "Forms$SaveChangesClientAction"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			refs := scanWidgetOwnRefs(tc.widget)
			if refs.Page != tc.wantPage {
				t.Errorf("page = %q, want %q", refs.Page, tc.wantPage)
			}
			if refs.Entity != tc.wantEnt {
				t.Errorf("entity = %q, want %q", refs.Entity, tc.wantEnt)
			}
		})
	}
}

// TestScanWidgetOwnRefs_PageRefSkipsChildWidgets: a container must not absorb the
// page target of a button nested inside it, or every ancestor would report an edge
// the child owns. Same rule the entity/microflow scan already follows.
func TestScanWidgetOwnRefs_PageRefSkipsChildWidgets(t *testing.T) {
	container := map[string]any{
		"$Type": "Forms$DivContainer",
		"Widgets": []any{
			map[string]any{
				"$Type": "Forms$ActionButton",
				"Action": map[string]any{
					"PageSettings": map[string]any{"Form": "Mod.ChildTarget"},
				},
			},
		},
	}
	if got := scanWidgetOwnRefs(container).Page; got != "" {
		t.Errorf("container absorbed a child's page ref: %q", got)
	}
}
