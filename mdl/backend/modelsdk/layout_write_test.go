// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// atlasLayoutKeys is the complete key set of a Studio Pro layout, measured on
// all 22 layouts Atlas ships on 11.13.0 — every one of them has exactly these
// ten, and generated/metamodel's PagesLayout declares exactly the same eight
// properties. A layout mxcli writes must match: a key Studio Pro never writes
// is the failure mode CLAUDE.md documents, where mxbuild reports 0 errors and
// Studio Pro cannot open the document.
var atlasLayoutKeys = []string{
	"$ID", "$Type", "Appearance", "CanvasHeight", "CanvasWidth",
	"Content", "Documentation", "Excluded", "ExportLevel", "Name",
}

func minimalLayout() *pages.Layout {
	return &pages.Layout{
		Name:       "App_Default",
		LayoutType: pages.LayoutTypeResponsive,
		Widgets: []pages.Widget{
			&pages.ScrollContainer{
				BaseWidget: pages.BaseWidget{Name: "layoutContainer"},
				Regions: []*pages.ScrollContainerRegion{{
					Slot:    pages.ScrollSlotCenter,
					Class:   "region-content",
					Widgets: []pages.Widget{&pages.LayoutPlaceholder{BaseWidget: pages.BaseWidget{Name: "Main"}}},
				}},
			},
		},
	}
}

func TestLayoutToGen_WritesExactlyTheKeysStudioProWrites(t *testing.T) {
	g, err := layoutToGen(minimalLayout())
	if err != nil {
		t.Fatal(err)
	}
	got := keysOf(encodeToMap(t, g))
	sort.Strings(got)
	want := append([]string(nil), atlasLayoutKeys...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("layout keys\n got: %v\nwant: %v", got, want)
	}
}

// MainPlaceholderName is the trap this test exists for. gen declares it on
// Layout (bound at index 15) so the setter compiles and mxbuild accepts the
// result — measured 0 errors — but the metamodel does not declare it and no
// Atlas layout carries it. Which placeholder is "main" is a naming convention.
func TestLayoutToGen_DoesNotWriteMainPlaceholderName(t *testing.T) {
	g, err := layoutToGen(minimalLayout())
	if err != nil {
		t.Fatal(err)
	}
	doc := encodeToMap(t, g)
	for _, k := range []string{
		"MainPlaceholderName", "AcceptPlaceholderName", "CancelPlaceholderName",
		"MainPlaceholder", "AcceptButtonPlaceholder", "CancelButtonPlaceholder",
		"UseMainPlaceholderForPopups",
	} {
		if _, ok := doc[k]; ok {
			t.Errorf("wrote %q, which Forms$Layout does not have", k)
		}
	}
}

// The tree hangs off Content, never off the layout. Reading rawData["Widget"]
// is what made DESCRIBE LAYOUT print an empty structure for an 84-element
// document, so the write side is pinned to the same shape.
func TestLayoutToGen_PutsTheTreeAndTypeOnTheContentWrapper(t *testing.T) {
	g, err := layoutToGen(minimalLayout())
	if err != nil {
		t.Fatal(err)
	}
	doc := encodeToMap(t, g)
	if _, wrong := doc["LayoutType"]; wrong {
		t.Error("LayoutType on the layout element; it belongs on the content wrapper")
	}
	content, ok := doc["Content"].(map[string]any)
	if !ok {
		t.Fatalf("Content missing; keys = %v", keysOf(doc))
	}
	if content["$Type"] != "Forms$WebLayoutContent" {
		t.Errorf("content $Type = %v, want Forms$WebLayoutContent", content["$Type"])
	}
	if content["LayoutType"] != "Responsive" {
		t.Errorf("content LayoutType = %v", content["LayoutType"])
	}
	if _, ok := content["Widgets"]; !ok {
		t.Errorf("widgets not on the content wrapper; keys = %v", keysOf(content))
	}
}

func TestLayoutToGen_NativeUsesTheNativeWrapper(t *testing.T) {
	l := minimalLayout()
	l.Native = true
	l.LayoutType = pages.LayoutTypeDefault

	g, err := layoutToGen(l)
	if err != nil {
		t.Fatal(err)
	}
	content := encodeToMap(t, g)["Content"].(map[string]any)
	if content["$Type"] != "Forms$NativeLayoutContent" {
		t.Errorf("content $Type = %v, want Forms$NativeLayoutContent", content["$Type"])
	}
}

// The two vocabularies are disjoint and neither platform reports the other's
// values, so a cross-platform type is refused rather than written.
func TestLayoutToGen_RefusesALayoutTypeFromTheOtherPlatform(t *testing.T) {
	web := minimalLayout()
	web.LayoutType = pages.LayoutTypeDefault // native-only
	if _, err := layoutToGen(web); err == nil {
		t.Error("Default on a web layout must be refused")
	}

	native := minimalLayout()
	native.Native = true
	native.LayoutType = pages.LayoutTypeResponsive // web-only
	if _, err := layoutToGen(native); err == nil {
		t.Error("Responsive on a native layout must be refused")
	}
}

// A layout with no placeholder can host no page: the page's
// Forms$FormCallArgument has nothing to name.
func TestLayoutToGen_RefusesALayoutWithNoPlaceholder(t *testing.T) {
	l := minimalLayout()
	l.Widgets = []pages.Widget{&pages.ScrollContainer{
		BaseWidget: pages.BaseWidget{Name: "layoutContainer"},
		Regions:    []*pages.ScrollContainerRegion{{Slot: pages.ScrollSlotCenter}},
	}}
	if _, err := layoutToGen(l); err == nil {
		t.Fatal("a placeholder-less layout must be refused")
	}

	// Control: the same layout with the placeholder back is accepted, so the
	// refusal is about the placeholder and not about the shape of the tree.
	if _, err := layoutToGen(minimalLayout()); err != nil {
		t.Fatalf("control failed: %v", err)
	}
}

func TestLayoutToGen_RefusesAnEmptyNameOrType(t *testing.T) {
	noName := minimalLayout()
	noName.Name = ""
	if _, err := layoutToGen(noName); err == nil {
		t.Error("a nameless layout must be refused")
	}

	noType := minimalLayout()
	noType.LayoutType = ""
	if _, err := layoutToGen(noType); err == nil {
		t.Error("a layout with no layout type must be refused")
	}
}

// The layout's own CSS class is not decoration.
//
// Atlas scopes ~24 of its layout rules to `.layout-atlas` and its variants, and
// every Atlas layout that has chrome carries one — measured on 11.13.0:
// Atlas_TopBar is 'layout-atlas layout-atlas-responsive-topbar', Atlas_Default
// is 'layout-atlas layout-atlas-responsive-default', and only PopupLayout (which
// has no chrome) is bare. A layout written without it renders with no topbar bar
// and no sidebar rail, which mx check is entirely silent about — the defect is
// only visible in a browser.
func TestLayoutToGen_CarriesTheLayoutCSSClass(t *testing.T) {
	l := minimalLayout()
	l.Class = "layout-atlas layout-atlas-responsive-topbar"
	l.Style = "min-height: 100vh;"

	g, err := layoutToGen(l)
	if err != nil {
		t.Fatal(err)
	}
	ap, ok := encodeToMap(t, g)["Appearance"].(map[string]any)
	if !ok {
		t.Fatal("Appearance missing")
	}
	if ap["Class"] != "layout-atlas layout-atlas-responsive-topbar" {
		t.Errorf("Class = %v", ap["Class"])
	}
	if ap["Style"] != "min-height: 100vh;" {
		t.Errorf("Style = %v", ap["Style"])
	}

	// Control: a layout with no class still writes an Appearance, because the
	// ten-key shape requires one — an absent Appearance is a different bug.
	bare, err := layoutToGen(minimalLayout())
	if err != nil {
		t.Fatal(err)
	}
	bareAp, ok := encodeToMap(t, bare)["Appearance"].(map[string]any)
	if !ok {
		t.Fatal("a class-less layout must still carry an Appearance")
	}
	if bareAp["Class"] != "" {
		t.Errorf("Class = %v, want empty", bareAp["Class"])
	}
}
