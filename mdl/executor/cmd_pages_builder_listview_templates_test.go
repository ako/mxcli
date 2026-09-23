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
			"is not a specialization of Pages.Vehicle",
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

// ako/mxcli#514. This test used to assert the opposite — "a template for the
// list view's own entity is the base case Mendix permits" — on the strength of
// entityIsOrDescendsFrom returning true for the entity itself. That was never
// measured, and it is false. Mendix requires a STRICT specialization; the list
// view's own body is already what renders an object no template matches, so a
// template for the base entity would be a second, unreachable default.
//
// Measured on a blank Mendix 11.12.2 project, list view over MyFirstModule.Vehicle:
//
//	template for MyFirstModule.Car      -> mx check: 0 errors     (a real specialization)
//	template for MyFirstModule.Vehicle  -> mx check: 1 error
//	  [CE0543] "The entity of the list view template is 'MyFirstModule.Vehicle' and
//	           this is not a specialization of the entity of the list view."
//
// mxcli check passed and exec wrote the page in both cases, so the guard existed
// and was one case too generous — the wording "or a specialization of it" was
// itself the bug.
func TestBuildListViewTemplateOnTheListEntityItselfIsRefused(t *testing.T) {
	pb := newVehiclePB()
	_, err := pb.buildListViewV3(listViewWidget(templateWidget("Pages.Vehicle", "base")))
	if err == nil {
		t.Fatal("a template for the list view's own entity was accepted; mxbuild refuses it with CE0543")
	}
	// The old wording — "is not <listEntity> or a specialization of it" — named
	// the very case Mendix refuses as one of the accepted ones.
	if strings.Contains(err.Error(), "is not Pages.Vehicle or a specialization of it") {
		t.Errorf("the message still offers the case it now refuses: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "own entity") {
		t.Errorf("the message does not say why this one is different: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "Pages.Vehicle") {
		t.Errorf("the message does not name the entity: %q", err.Error())
	}
}

// The control, and the reason the fix is not "refuse every template": a genuine
// specialization still builds. Without this the test above passes against a
// guard that rejects everything.
func TestBuildListViewTemplateOnASpecializationIsAccepted(t *testing.T) {
	pb := newVehiclePB()
	lv, err := pb.buildListViewV3(listViewWidget(templateWidget("Pages.Bus", "bus")))
	if err != nil {
		t.Fatalf("a template for a real specialization was refused: %v", err)
	}
	if len(lv.Templates) != 1 {
		t.Fatalf("got %d template(s), want 1", len(lv.Templates))
	}
}

// A grandchild is still a specialization. entityIsOrDescendsFrom walked the
// whole chain and the strict form must keep doing so — dropping to "is my
// immediate generalization" would refuse a legal two-level hierarchy.
func TestBuildListViewTemplateOnAGrandchildIsAccepted(t *testing.T) {
	pb := newVehiclePB()
	pb.execCache.domainModels[0].Entities = append(pb.execCache.domainModels[0].Entities,
		&domainmodel.Entity{
			BaseElement:       model.BaseElement{ID: model.ID("e-Minibus")},
			Name:              "Minibus",
			GeneralizationRef: "Pages.Bus",
		})
	if _, err := pb.buildListViewV3(listViewWidget(templateWidget("Pages.Minibus", "mini"))); err != nil {
		t.Fatalf("a template for a grandchild specialization was refused: %v", err)
	}
}
