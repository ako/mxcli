// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// encodeToMap encodes an element the way storage does and decodes it back to a
// map, so assertions are on BSON keys rather than on gen's Go accessor names.
// The two disagree exactly where it matters: gen's setter is SetCenter and the
// key is CenterRegion, so a test written against the accessor proves nothing.
func encodeToMap(t *testing.T, e element.Element) map[string]any {
	t.Helper()
	raw, err := (&codec.Encoder{}).Encode(e)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var m map[string]any
	if err := bson.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return m
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The four element types a layout needs beyond what pages already author.
// Each assertion is pinned to Atlas_Core.Atlas_Default on 11.13.0 rather than
// to gen's Go accessor names, because the two disagree in exactly the place
// that matters: gen's setter is SetCenter and the BSON key is CenterRegion.
func TestScrollContainerToGen_FillsTheNamedSlots(t *testing.T) {
	sc := &pages.ScrollContainer{
		BaseWidget: pages.BaseWidget{Name: "layoutContainer"},
		Regions: []*pages.ScrollContainerRegion{
			{Slot: pages.ScrollSlotTop, Class: "region-topbar", Size: 60, SizeMode: "Fixed"},
			{Slot: pages.ScrollSlotLeft, Class: "region-sidebar", Size: 200},
			{Slot: pages.ScrollSlotCenter, Class: "region-content", Widgets: []pages.Widget{
				&pages.LayoutPlaceholder{BaseWidget: pages.BaseWidget{Name: "Main"}},
			}},
		},
	}

	g, err := widgetToGen(sc)
	if err != nil {
		t.Fatal(err)
	}
	// Assert on the encoded document, which is the only thing Mendix reads.
	doc := encodeToMap(t, g)
	if doc["$Type"] != "Forms$ScrollContainer" {
		t.Fatalf("$Type = %v", doc["$Type"])
	}
	for _, slot := range []string{"Top", "Left", "CenterRegion"} {
		if _, ok := doc[slot]; !ok {
			t.Errorf("slot %q missing; keys = %v", slot, keysOf(doc))
		}
	}
	// The name gen invites is "Center" and it is wrong — a document with a
	// "Center" key is one Mendix ignores.
	if _, wrong := doc["Center"]; wrong {
		t.Error(`emitted a "Center" key; the BSON key is "CenterRegion"`)
	}
	if _, empty := doc["Right"]; empty {
		t.Error("an unoccupied slot must not be emitted as an empty region")
	}
}

func TestScrollContainerToGen_RejectsAnUnknownSlot(t *testing.T) {
	sc := &pages.ScrollContainer{
		BaseWidget: pages.BaseWidget{Name: "sc"},
		Regions:    []*pages.ScrollContainerRegion{{Slot: "middle"}},
	}
	if _, err := widgetToGen(sc); err == nil {
		t.Fatal("an unknown region must be refused, not silently dropped")
	}
}

// A placeholder with no name cannot be bound to: a page references it as
// Module.Layout.<Name>, so an unnamed one is a slot nothing can fill.
func TestPlaceholderToGen_RequiresAName(t *testing.T) {
	if _, err := widgetToGen(&pages.LayoutPlaceholder{}); err == nil {
		t.Fatal("an unnamed placeholder must be refused")
	}
	g, err := widgetToGen(&pages.LayoutPlaceholder{BaseWidget: pages.BaseWidget{Name: "Main"}})
	if err != nil {
		t.Fatal(err)
	}
	doc := encodeToMap(t, g)
	if doc["$Type"] != "Forms$Placeholder" || doc["Name"] != "Main" {
		t.Errorf("placeholder = %v", doc)
	}
}

// The profile is stored inside a Forms$NavigationSource under MenuSource, not
// on the tree — measured on Atlas_Default, whose NavigationTree carries
// MenuSource → NavigationProfile "Responsive".
func TestNavigationTreeToGen_WrapsTheProfileInAMenuSource(t *testing.T) {
	g, err := widgetToGen(&pages.NavigationTree{
		BaseWidget:        pages.BaseWidget{Name: "navMenu"},
		NavigationProfile: "Responsive",
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := encodeToMap(t, g)
	if doc["$Type"] != "Forms$NavigationTree" {
		t.Fatalf("$Type = %v", doc["$Type"])
	}
	src, ok := doc["MenuSource"].(map[string]any)
	if !ok {
		t.Fatalf("MenuSource missing or not a document; keys = %v", keysOf(doc))
	}
	if src["NavigationProfile"] != "Responsive" {
		t.Errorf("NavigationProfile = %v, want Responsive", src["NavigationProfile"])
	}
}

// A menu bar is the horizontal navigation a topbar carries — the same four keys
// as a NavigationTree and the same MenuSource wrapper, measured on
// Atlas_Core.Atlas_TopBar and matching generated/metamodel's PagesMenuBar.
//
// It exists because the scaffold in `mxcli new` needs it: without a MenuBar the
// generated layout has no navigation in the topbar, which is precisely what a
// describe → exec copy of Atlas_TopBar loses.
func TestMenuBarToGen_WrapsTheProfileInAMenuSource(t *testing.T) {
	g, err := widgetToGen(&pages.MenuBar{
		BaseWidget:        pages.BaseWidget{Name: "mainMenu"},
		NavigationProfile: "Responsive",
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := encodeToMap(t, g)
	if doc["$Type"] != "Forms$MenuBar" {
		t.Fatalf("$Type = %v", doc["$Type"])
	}
	src, ok := doc["MenuSource"].(map[string]any)
	if !ok {
		t.Fatalf("MenuSource missing or not a document; keys = %v", keysOf(doc))
	}
	if src["NavigationProfile"] != "Responsive" {
		t.Errorf("NavigationProfile = %v, want Responsive", src["NavigationProfile"])
	}
	// The metamodel declares exactly Appearance, MenuSource, Name and TabIndex.
	for _, k := range keysOf(doc) {
		switch k {
		case "$ID", "$Type", "Appearance", "MenuSource", "Name", "TabIndex":
		default:
			t.Errorf("wrote %q, which Forms$MenuBar does not have", k)
		}
	}
}

// ako/mxcli#573: Atlas_Core.Phone_BottomBar's bottom bar is a
// Forms$SimpleMenuBar, and mxcli could not author one — so a phone layout
// written in MDL had no bottom bar at all.
//
// The shape is measured on that widget in a blank 11.14.0 project: Appearance,
// MenuSource, Name, Orientation, TabIndex, with the menu document named inside a
// Forms$MenuDocumentSource — not on the bar, and not as a navigation profile.
func TestSimpleMenuBarToGen_WrapsTheMenuInAMenuDocumentSource(t *testing.T) {
	g, err := widgetToGen(&pages.SimpleMenuBar{
		BaseWidget:  pages.BaseWidget{Name: "simpleMenuBar1"},
		Menu:        "Atlas_Core.Phone_Menu",
		Orientation: pages.MenuOrientationHorizontal,
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := encodeToMap(t, g)
	if doc["$Type"] != "Forms$SimpleMenuBar" {
		t.Fatalf("$Type = %v", doc["$Type"])
	}
	src, ok := doc["MenuSource"].(map[string]any)
	if !ok {
		t.Fatalf("MenuSource missing or not a document; keys = %v", keysOf(doc))
	}
	if src["$Type"] != "Forms$MenuDocumentSource" {
		t.Errorf("MenuSource $Type = %v, want Forms$MenuDocumentSource", src["$Type"])
	}
	if src["Menu"] != "Atlas_Core.Phone_Menu" {
		t.Errorf("Menu = %v, want Atlas_Core.Phone_Menu", src["Menu"])
	}
	if doc["Orientation"] != "Horizontal" {
		t.Errorf("Orientation = %v, want Horizontal", doc["Orientation"])
	}
	for _, k := range keysOf(doc) {
		switch k {
		case "$ID", "$Type", "Appearance", "MenuSource", "Name", "Orientation", "TabIndex":
		default:
			t.Errorf("wrote %q, which Forms$SimpleMenuBar does not have", k)
		}
	}
}

// Every menu widget takes its items from either a navigation profile or a menu
// document; the two are different MenuSource subtypes, and naming both is a
// contradiction rather than a preference order.
func TestMenuWidgetToGen_MenuSourceKind(t *testing.T) {
	cases := []struct {
		name     string
		w        pages.Widget
		wantType string
		wantKey  string
		wantVal  string
	}{
		{"simple bar, profile", &pages.SimpleMenuBar{BaseWidget: pages.BaseWidget{Name: "b"}, NavigationProfile: "Phone"},
			"Forms$NavigationSource", "NavigationProfile", "Phone"},
		{"menu bar, document", &pages.MenuBar{BaseWidget: pages.BaseWidget{Name: "b"}, Menu: "M.Top"},
			"Forms$MenuDocumentSource", "Menu", "M.Top"},
		{"tree, document", &pages.NavigationTree{BaseWidget: pages.BaseWidget{Name: "t"}, Menu: "M.Side"},
			"Forms$MenuDocumentSource", "Menu", "M.Side"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, err := widgetToGen(c.w)
			if err != nil {
				t.Fatal(err)
			}
			src, _ := encodeToMap(t, g)["MenuSource"].(map[string]any)
			if src["$Type"] != c.wantType || src[c.wantKey] != c.wantVal {
				t.Errorf("MenuSource = %v, want %s{%s: %s}", src, c.wantType, c.wantKey, c.wantVal)
			}
		})
	}

	_, err := widgetToGen(&pages.SimpleMenuBar{BaseWidget: pages.BaseWidget{Name: "b"}, Menu: "M.X", NavigationProfile: "Phone"})
	if err == nil {
		t.Error("a menu widget naming both a menu document and a profile was accepted")
	}
}
