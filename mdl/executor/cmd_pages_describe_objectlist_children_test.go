// SPDX-License-Identifier: Apache-2.0

// Issue #891 (1): DESCRIBE PAGE renders an Accordion group empty.
//
// An object-list item (an Accordion `group`, a PopupMenu `basicItem`) can carry
// child WIDGETS in a Widgets-typed sub-property — the group's `content` slot.
// extractObjectListItem handled only scalar sub-properties (datasource,
// attribute, expression, text template, primitive), so those children were
// never read, and the emitter always closed the item with "\n" so they had
// nowhere to go even if they had been.
//
// The grid really is in the model — the reporter's BSON dump showed it, and so
// does a stock blank app: describing an accordion whose group holds a DataGrid2
// emits `group group1 ( ...props... )` and nothing else. Feeding that
// description back through `exec` silently deletes the grid, which is what
// makes this worth more than a cosmetic gap.
package executor

import (
	"bytes"
	"strings"
	"testing"
)

// buildAccordionWithNestedWidget mirrors the shape Mendix stores: a `groups`
// object-list whose item type declares a `header` text template and a `content`
// Widgets slot, with one child widget inside that slot.
func buildAccordionWithNestedWidget() map[string]any {
	const (
		idGroups  = "type-id-groups"
		idHeader  = "type-id-header"
		idContent = "type-id-content"
	)

	return map[string]any{
		"Name": "acc1",
		"Type": map[string]any{
			"WidgetId": "com.mendix.widget.web.accordion.Accordion",
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					map[string]any{
						"$ID": idGroups, "PropertyKey": "groups",
						"ValueType": map[string]any{
							"ObjectType": map[string]any{
								"PropertyTypes": []any{
									map[string]any{"$ID": idHeader, "PropertyKey": "header",
										"ValueType": map[string]any{"Type": "TextTemplate"}},
									map[string]any{"$ID": idContent, "PropertyKey": "content",
										"ValueType": map[string]any{"Type": "Widgets"}},
								},
							},
						},
					},
				},
			},
		},
		"Object": map[string]any{
			"Properties": []any{
				map[string]any{
					"TypePointer": idGroups,
					"Value": map[string]any{
						"Objects": []any{
							map[string]any{
								"Properties": []any{
									map[string]any{
										"TypePointer": idHeader,
										"Value": map[string]any{
											"TextTemplate": map[string]any{
												"Template": map[string]any{
													"Items": []any{
														map[string]any{"Text": "Group one"},
													},
												},
											},
										},
									},
									map[string]any{
										"TypePointer": idContent,
										"Value": map[string]any{
											"Widgets": []any{
												map[string]any{
													"$Type": "Forms$StaticText",
													"Name":  "txtInsideGroup",
													"Text": map[string]any{
														"Items": []any{
															map[string]any{"Text": "hello"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// The read half: the child widget must survive extraction.
func TestObjectListItem_KeepsNestedChildWidgets(t *testing.T) {
	lists := extractObjectLists(nil, buildAccordionWithNestedWidget())
	if len(lists) != 1 {
		t.Fatalf("expected 1 object list, got %d", len(lists))
	}
	if len(lists[0].Items) != 1 {
		t.Fatalf("expected 1 group, got %d", len(lists[0].Items))
	}
	item := lists[0].Items[0]
	if len(item.Children) == 0 {
		t.Fatal("the group's nested widget was dropped — DESCRIBE would emit an empty group " +
			"and a re-exec of that output would delete the widget (#891)")
	}
	if got := item.Children[0].Name; got != "txtInsideGroup" {
		t.Errorf("nested child Name = %q, want %q", got, "txtInsideGroup")
	}
}

// The emit half: an item carrying children must be written with a body. Testing
// through outputWidgetMDLV3 rather than a formatting helper means deleting the
// emit change fails this test — a helper-level assertion would prove the helper
// works and nothing about the wiring.
func TestObjectListItem_EmitsNestedChildWidgets(t *testing.T) {
	lists := extractObjectLists(nil, buildAccordionWithNestedWidget())
	if len(lists) == 0 || len(lists[0].Items) == 0 {
		t.Fatal("fixture produced no object-list items")
	}

	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}
	outputWidgetMDLV3(ctx, rawWidget{
		Type:        "CustomWidgets$CustomWidget",
		RenderMode:  "accordion",
		Name:        "acc1",
		WidgetID:    "com.mendix.widget.web.accordion.Accordion",
		ObjectLists: lists,
	}, 0)

	out := buf.String()
	if !strings.Contains(out, "txtInsideGroup") {
		t.Errorf("nested widget missing from DESCRIBE output:\n%s", out)
	}
	// A body, not a bare item line — otherwise the output cannot re-parse.
	if !strings.Contains(out, "group group1") || !strings.Contains(out, "{") {
		t.Errorf("group should be emitted with a body:\n%s", out)
	}
}
