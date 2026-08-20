// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// newVehiclePB mirrors ako/TestApp: Pages.Vehicle with four specializations, plus
// an unrelated entity to test the specialization check against.
func newVehiclePB() *pageBuilder {
	const modID = model.ID("mod")
	ent := func(name, gen string) *domainmodel.Entity {
		return &domainmodel.Entity{
			BaseElement:       model.BaseElement{ID: model.ID("e-" + name)},
			Name:              name,
			GeneralizationRef: gen,
		}
	}
	return &pageBuilder{
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
		execCache: &executorCache{
			hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "Pages"}},
			domainModels: []*domainmodel.DomainModel{{
				ContainerID: modID,
				Entities: []*domainmodel.Entity{
					ent("Vehicle", ""),
					ent("Bus", "Pages.Vehicle"),
					ent("Truck", "Pages.Vehicle"),
					ent("Car", "Pages.Vehicle"),
					ent("SUV", "Pages.Vehicle"),
					ent("Unrelated", ""),
				},
			}},
		},
	}
}

func templateWidget(specialization, childName string) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type:           "template",
		Specialization: specialization,
		Properties:     map[string]any{},
		Children: []*ast.WidgetV3{{
			Type:       "dynamictext",
			Name:       childName,
			Properties: map[string]any{"Content": "x"},
		}},
	}
}

func listViewWidget(children ...*ast.WidgetV3) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type:       "listview",
		Name:       "vehicleListView",
		Properties: map[string]any{"DataSource": &ast.DataSourceV3{Type: "database", Reference: "Pages.Vehicle"}},
		Children:   children,
	}
}

// TestBuildListViewTemplates pins the split: `template for` blocks become
// Templates, everything else stays the list view's own body, and source order is
// preserved because Mendix's order is authored rather than derived.
func TestBuildListViewTemplates(t *testing.T) {
	pb := newVehiclePB()
	lv, err := pb.buildListViewV3(listViewWidget(
		&ast.WidgetV3{Type: "dynamictext", Name: "defaultVehicle", Properties: map[string]any{"Content": "v"}},
		templateWidget("Pages.Bus", "busLabel"),
		templateWidget("Pages.Truck", "truckLabel"),
		templateWidget("Pages.Car", "carLabel"),
		templateWidget("Pages.SUV", "suvLabel"),
	))
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	if len(lv.Widgets) != 1 {
		t.Errorf("list view body has %d widget(s), want 1 — templates must not land in Widgets", len(lv.Widgets))
	}
	want := []string{"Pages.Bus", "Pages.Truck", "Pages.Car", "Pages.SUV"}
	if len(lv.Templates) != len(want) {
		t.Fatalf("got %d template(s), want %d", len(lv.Templates), len(want))
	}
	for i, w := range want {
		if got := lv.Templates[i].Specialization; got != w {
			t.Errorf("template %d = %q, want %q (source order must be preserved)", i, got, w)
		}
		if lv.Templates[i].TypeName != "Forms$ListViewTemplate" {
			t.Errorf("template %d TypeName = %q", i, lv.Templates[i].TypeName)
		}
		if len(lv.Templates[i].Widgets) != 1 {
			t.Errorf("template %d has %d widget(s), want 1", i, len(lv.Templates[i].Widgets))
		}
	}
}

// TestBuildListViewTemplateRejections covers the three ways a template can be
// wrong. Each refuses rather than writing a document that cannot render: Mendix
// matches a template against the object's type, so a template for an unrelated
// entity is unreachable by construction.
func TestBuildListViewTemplateRejections(t *testing.T) {
	cases := []struct {
		name     string
		children []*ast.WidgetV3
		wantErr  string
	}{
		{
			"entity is not a specialization of the list view's entity",
			[]*ast.WidgetV3{templateWidget("Pages.Unrelated", "x")},
			"is not Pages.Vehicle or a specialization of it",
		},
		{
			"two templates for one specialization",
			[]*ast.WidgetV3{templateWidget("Pages.Bus", "a"), templateWidget("Pages.Bus", "b")},
			"more than one template for Pages.Bus",
		},
		{
			"nested template",
			func() []*ast.WidgetV3 {
				outer := templateWidget("Pages.Bus", "a")
				outer.Children = append(outer.Children, templateWidget("Pages.Truck", "b"))
				return []*ast.WidgetV3{outer}
			}(),
			"cannot nest",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pb := newVehiclePB()
			_, err := pb.buildListViewV3(listViewWidget(c.children...))
			if err == nil {
				t.Fatalf("build succeeded; want an error containing %q", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), c.wantErr)
			}
		})
	}
}

// TestBuildListViewTemplateOnTheListEntityItself is allowed: a template for the
// list view's own entity is the base case Mendix permits, and
// entityIsOrDescendsFrom returns true for the entity itself.
func TestBuildListViewTemplateOnTheListEntityItself(t *testing.T) {
	pb := newVehiclePB()
	lv, err := pb.buildListViewV3(listViewWidget(templateWidget("Pages.Vehicle", "base")))
	if err != nil {
		t.Fatalf("a template for the list view's own entity was refused: %v", err)
	}
	if len(lv.Templates) != 1 {
		t.Fatalf("got %d template(s), want 1", len(lv.Templates))
	}
}
