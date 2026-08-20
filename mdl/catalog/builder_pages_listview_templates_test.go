// SPDX-License-Identifier: Apache-2.0

package catalog

import "testing"

// listViewWithTemplates builds the shape from issue #940: a List View over a
// generalization, with one template per specialization. Each template holds a
// data container over a child entity and a button calling a microflow — the
// content that reported "0 pages, 0 widgets".
//
// The List View's own Widgets array is empty on purpose. Studio Pro also
// populates it, and that content WAS indexed; leaving it empty isolates the
// templates so the test cannot pass on the strength of the sibling array.
func listViewWithTemplates(firstSpecialization string) map[string]any {
	tmpl := func(specialization, viewName, buttonName, microflow string) map[string]any {
		return map[string]any{
			"$Type":  "Forms$ListViewTemplate",
			"Entity": specialization,
			"Widgets": []any{
				map[string]any{
					"$Type": "Forms$DataView",
					"Name":  viewName,
					"DataSource": map[string]any{
						"$Type": "Forms$DataViewSource",
						"EntityRef": map[string]any{
							"$Type":  "DomainModels$DirectEntityRef",
							"Entity": "MyModule.ChildRecord",
						},
					},
					"Widgets": []any{
						map[string]any{
							"$Type": "Forms$ActionButton",
							"Name":  buttonName,
							"Action": map[string]any{
								"$Type":     "Forms$MicroflowClientAction",
								"Microflow": microflow,
							},
						},
					},
				},
			},
		}
	}
	return map[string]any{
		"$Type": "Forms$ListView",
		"Name":  "listView1",
		"DataSource": map[string]any{
			"$Type": "Forms$ListViewXPathSource",
			"EntityRef": map[string]any{
				"$Type":  "DomainModels$DirectEntityRef",
				"Entity": "MyModule.BaseItem",
			},
		},
		"Widgets": []any{},
		"Templates": []any{
			tmpl(firstSpecialization, "typeAView", "btnA", "MyModule.ACT_A_DoThing"),
			tmpl("MyModule.TypeB", "typeBView", "btnB", "MyModule.ACT_TypeB_DoThing"),
		},
	}
}

func widgetByName(rows []rawWidgetInfo, name string) *rawWidgetInfo {
	for i := range rows {
		if rows[i].Name == name {
			return &rows[i]
		}
	}
	return nil
}

// TestListViewTemplateContentsAreIndexed is the regression test for #940.
//
// extractWidgetsRecursive knew Widgets, Rows/Columns, FooterWidgets, TabPages,
// pluggable Object.Properties and NavigationList Items — but not Templates. So
// every widget inside a List View specialization template was absent from the
// widgets table, and since the refs projection is built from that table,
// `SHOW REFERENCES TO <entity used only in a template>` reported 0 pages and
// 0 widgets. Anything using reference counts to decide "unused, safe to delete"
// would delete a document in active use.
func TestListViewTemplateContentsAreIndexed(t *testing.T) {
	rows := extractWidgetsRecursive(listViewWithTemplates("MyModule.TypeA"))

	for _, name := range []string{"typeAView", "typeBView", "btnA", "btnB"} {
		if widgetByName(rows, name) == nil {
			t.Errorf("widget %q inside a list view template is missing from the index (#940)", name)
		}
	}

	if w := widgetByName(rows, "typeBView"); w != nil && w.EntityRef != "MyModule.ChildRecord" {
		t.Errorf("template data view EntityRef = %q, want MyModule.ChildRecord "+
			"(this is the ref that made SHOW REFERENCES report 0 pages)", w.EntityRef)
	}
	if w := widgetByName(rows, "btnB"); w != nil && w.MicroflowRef != "MyModule.ACT_TypeB_DoThing" {
		t.Errorf("template button MicroflowRef = %q, want MyModule.ACT_TypeB_DoThing", w.MicroflowRef)
	}

	// The specialization each template renders is itself a reference the page
	// makes. It is carried by the template's own row rather than folded into the
	// list view, which is what the next test pins.
	var specializations []string
	for _, r := range rows {
		if r.WidgetType == "Forms$ListViewTemplate" {
			specializations = append(specializations, r.EntityRef)
		}
	}
	if len(specializations) != 2 {
		t.Errorf("got %d list view template row(s) %v, want 2", len(specializations), specializations)
	}
}

// TestListViewDataSourceSurvivesATemplateEntity pins the defect the report did
// not find. Templates was missing from widgetChildKeys as well, so the list
// view's own ref scan descended into its templates, collected each one's
// Entity, and returned the lexicographically smallest of the candidates. A
// specialization that sorts before the list view's real datasource silently
// replaced it — the widgets table then recorded the wrong entity for the list
// view, decided by alphabetical accident.
func TestListViewDataSourceSurvivesATemplateEntity(t *testing.T) {
	// "MyModule.AAA_First" sorts before "MyModule.BaseItem".
	rows := extractWidgetsRecursive(listViewWithTemplates("MyModule.AAA_First"))

	lv := widgetByName(rows, "listView1")
	if lv == nil {
		t.Fatal("the list view itself is missing from the index")
	}
	if lv.EntityRef != "MyModule.BaseItem" {
		t.Errorf("list view EntityRef = %q, want MyModule.BaseItem — a template's "+
			"specialization displaced the widget's own datasource (#940)", lv.EntityRef)
	}
}
