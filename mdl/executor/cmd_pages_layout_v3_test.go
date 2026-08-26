// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// layoutStmt is the shape `create layout M.App_Default (layouttype: 'Responsive')
// { scrollcontainer … }` parses to.
func layoutStmt(props map[string]any, widgets ...*ast.WidgetV3) *ast.CreateLayoutStmt {
	return &ast.CreateLayoutStmt{
		Name:       ast.QualifiedName{Module: "M", Name: "App_Default"},
		Properties: props,
		Widgets:    widgets,
	}
}

func scrollWithMain() *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type: "scrollcontainer", Name: "layoutContainer",
		Children: []*ast.WidgetV3{{
			Type: "region", Name: "center",
			Properties: map[string]any{"class": "region-content"},
			Children:   []*ast.WidgetV3{{Type: "placeholder", Name: "Main"}},
		}},
	}
}

func TestBuildLayoutV3_BuildsTheRegionTree(t *testing.T) {
	l, err := newPopupPageBuilder().buildLayoutV3(layoutStmt(
		map[string]any{"layouttype": "Responsive"},
		&ast.WidgetV3{
			Type: "scrollcontainer", Name: "layoutContainer",
			Children: []*ast.WidgetV3{
				{Type: "region", Name: "top", Properties: map[string]any{"size": 60, "sizemode": "Fixed", "class": "region-topbar"}},
				{Type: "region", Name: "center", Children: []*ast.WidgetV3{{Type: "placeholder", Name: "Main"}}},
			},
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	if l.LayoutType != pages.LayoutTypeResponsive || l.Native {
		t.Errorf("layout type = %q native = %v", l.LayoutType, l.Native)
	}
	sc, ok := l.Widgets[0].(*pages.ScrollContainer)
	if !ok {
		t.Fatalf("root widget = %T, want *pages.ScrollContainer", l.Widgets[0])
	}
	if len(sc.Regions) != 2 {
		t.Fatalf("regions = %d, want 2", len(sc.Regions))
	}
	top := sc.Regions[0]
	if top.Slot != pages.ScrollSlotTop || top.Size != 60 || top.SizeMode != "Fixed" || top.Class != "region-topbar" {
		t.Errorf("top region = %+v", top)
	}
	// Size is read case-insensitively like every other MDL property; a lookup
	// that only matched "Size" read 0 for the `size:` the script actually wrote.
	if top.Size == 0 {
		t.Error("region size read as 0 from a lowercase `size:` property")
	}
}

// The platform is inferred from the layout type because the two vocabularies
// are disjoint, so a native type must produce a native layout with no extra
// property in the script.
func TestBuildLayoutV3_InfersThePlatformFromTheLayoutType(t *testing.T) {
	for _, tc := range []struct {
		layoutType string
		native     bool
	}{
		{"Responsive", false}, {"Phone", false}, {"Tablet", false}, {"ModalPopup", false},
		{"Default", true}, {"Popup", true},
	} {
		l, err := newPopupPageBuilder().buildLayoutV3(layoutStmt(
			map[string]any{"layouttype": tc.layoutType}, scrollWithMain()))
		if err != nil {
			t.Fatalf("%s: %v", tc.layoutType, err)
		}
		if l.Native != tc.native {
			t.Errorf("%s: native = %v, want %v", tc.layoutType, l.Native, tc.native)
		}
	}
}

func TestBuildLayoutV3_RefusesAnUnknownLayoutType(t *testing.T) {
	_, err := newPopupPageBuilder().buildLayoutV3(layoutStmt(
		map[string]any{"layouttype": "Responsiv"}, scrollWithMain()))
	if err == nil {
		t.Fatal("a misspelled layout type must be refused")
	}

	// Control: the correct spelling is accepted, so the refusal is about the
	// value and not about the property being read at all.
	if _, err := newPopupPageBuilder().buildLayoutV3(layoutStmt(
		map[string]any{"layouttype": "Responsive"}, scrollWithMain())); err != nil {
		t.Fatalf("control failed: %v", err)
	}
}

func TestBuildLayoutV3_RefusesAMissingLayoutType(t *testing.T) {
	if _, err := newPopupPageBuilder().buildLayoutV3(layoutStmt(nil, scrollWithMain())); err == nil {
		t.Fatal("a layout with no layouttype must be refused")
	}
}

// `mainplaceholder:` is the obvious property to reach for — gen even declares
// it — and it writes nothing. Silently ignoring it would report acceptance for
// a layout that does not carry the value.
func TestBuildLayoutV3_RefusesAnUnknownHeaderProperty(t *testing.T) {
	_, err := newPopupPageBuilder().buildLayoutV3(layoutStmt(
		map[string]any{"layouttype": "Responsive", "mainplaceholder": "Main"}, scrollWithMain()))
	if err == nil {
		t.Fatal("an unknown layout property must be refused")
	}
	if !strings.Contains(err.Error(), "mainplaceholder") {
		t.Errorf("error = %q, want it to name the offending property", err.Error())
	}
}

func TestBuildScrollContainerV3_RefusesADuplicateOrUnknownSlot(t *testing.T) {
	twice := &ast.WidgetV3{Type: "scrollcontainer", Name: "sc", Children: []*ast.WidgetV3{
		{Type: "region", Name: "top"}, {Type: "region", Name: "top"},
	}}
	if _, err := newPopupPageBuilder().buildScrollContainerV3(twice); err == nil {
		t.Error("two regions in one slot must be refused; the second would silently win")
	}

	unknown := &ast.WidgetV3{Type: "scrollcontainer", Name: "sc", Children: []*ast.WidgetV3{
		{Type: "region", Name: "middle"},
	}}
	if _, err := newPopupPageBuilder().buildScrollContainerV3(unknown); err == nil {
		t.Error("an unknown region name must be refused")
	}

	notARegion := &ast.WidgetV3{Type: "scrollcontainer", Name: "sc", Children: []*ast.WidgetV3{
		{Type: "container", Name: "c"},
	}}
	if _, err := newPopupPageBuilder().buildScrollContainerV3(notARegion); err == nil {
		t.Error("a non-region child of a scroll container must be refused")
	}

	// Control: one valid region of each kind builds.
	ok := &ast.WidgetV3{Type: "scrollcontainer", Name: "sc", Children: []*ast.WidgetV3{
		{Type: "region", Name: "top"}, {Type: "region", Name: "center"},
	}}
	if _, err := newPopupPageBuilder().buildScrollContainerV3(ok); err != nil {
		t.Fatalf("control failed: %v", err)
	}
}

func TestBuildPlaceholderV3_RequiresAName(t *testing.T) {
	if _, err := newPopupPageBuilder().buildPlaceholderV3(&ast.WidgetV3{Type: "placeholder"}); err == nil {
		t.Fatal("an unnamed placeholder must be refused: pages bind to it by name")
	}
}

// A layout in a Marketplace module is overwritten by the next module update,
// which is exactly why Mendix's own guidance is to put custom layouts elsewhere.
//
// The signal is FromAppStore on the module. That field is only populated by
// decoding the module unit, and GetModuleByName did not do so — a guard reading
// it would have been inert for every module. The enrichment is wired into every
// module lookup for that reason, and verified end to end against a real project
// (Atlas_Core in an 11.13.0 app is refused; a hand-made module is not).
func TestExecCreateLayout_RefusesAMarketplaceModule(t *testing.T) {
	mp := &model.Module{
		BaseElement:  model.BaseElement{ID: model.ID("mod-mp")},
		Name:         "Atlas_Core",
		FromAppStore: true,
		AppStoreGuid: "5e6e6ca9-fb4c-4c9c-8d2b-1c3c5d6e7f80",
	}
	own := &model.Module{
		BaseElement: model.BaseElement{ID: model.ID("mod-own")},
		Name:        "MyModule",
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListModulesFunc:  func() ([]*model.Module, error) { return []*model.Module{mp, own}, nil },
		ListLayoutsFunc:  func() ([]*pages.Layout, error) { return nil, nil },
		CreateLayoutFunc: func(*pages.Layout) error { return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mp, own)))

	stmt := layoutStmt(map[string]any{"layouttype": "Responsive"}, scrollWithMain())
	stmt.Name = ast.QualifiedName{Module: "Atlas_Core", Name: "Hijacked"}
	if err := execCreateLayout(ctx, stmt); err == nil {
		t.Fatal("writing a layout into a marketplace module must be refused")
	} else if !strings.Contains(err.Error(), "marketplace") {
		t.Errorf("error = %q, want it to explain the marketplace problem", err.Error())
	}

	// Control: the same statement into a module the project owns is accepted,
	// so the refusal is about FromAppStore and not about the statement.
	stmt.Name = ast.QualifiedName{Module: "MyModule", Name: "App_Default"}
	if err := execCreateLayout(ctx, stmt); err != nil {
		t.Fatalf("control failed: %v", err)
	}
}
