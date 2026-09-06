// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// navIconEntry returns the value of a key in a bson.D and whether it was
// present. Unlike the package's bsonLookup it separates an absent key from a
// null one — the distinction a null Icon turns on.
func navIconEntry(d bson.D, key string) (interface{}, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// The storage name is the whole point of this test.
//
// The metamodel calls the element Pages$IconCollectionIcon, but Mendix stores it
// as Forms$IconCollectionIcon — the "Form was the original term for Page" rename
// CLAUDE.md documents for ShowFormAction. Getting a polymorphic child's $Type
// wrong yields a document mxbuild accepts (its deserializer tolerates unknown
// properties) and Studio Pro cannot open, so the name is pinned against a
// Studio Pro-authored reference: every menu icon in ako/mxcli-ledger's
// navigation document is Forms$IconCollectionIcon{Image: "Atlas_Core.Atlas.…"}.
func TestBuildMenuIconBson_UsesTheFormsStorageName(t *testing.T) {
	got := buildMenuIconBson(NavMenuItemSpec{Icon: "Atlas_Core.Atlas.align-center"})
	d, ok := got.(bson.D)
	if !ok {
		t.Fatalf("expected a bson.D, got %T", got)
	}
	typ, _ := navIconEntry(d, "$Type")
	if typ != "Forms$IconCollectionIcon" {
		t.Errorf("$Type = %v, want Forms$IconCollectionIcon (NOT the metamodel's Pages$…)", typ)
	}
	img, present := navIconEntry(d, "Image")
	if !present {
		t.Fatal("the icon carries its name in Image; the key is missing")
	}
	if img != "Atlas_Core.Atlas.align-center" {
		t.Errorf("Image = %v", img)
	}
	if _, present := navIconEntry(d, "$ID"); !present {
		t.Error("every stored element needs its own $ID")
	}
	// Studio Pro writes exactly these three keys. A fourth would be a property
	// the type does not declare, which is what makes a document unopenable.
	if len(d) != 3 {
		t.Errorf("icon has %d keys, want exactly $ID/$Type/Image: %v", len(d), d)
	}
}

// No icon must stay a null, not an empty element: an IconCollectionIcon with a
// blank Image is a dangling reference, where absent is the modelled default.
func TestBuildMenuIconBson_EmptyNameStaysNull(t *testing.T) {
	if got := buildMenuIconBson(NavMenuItemSpec{}); got != nil {
		t.Errorf("buildMenuIconBson(empty spec) = %v, want nil", got)
	}
}

// The regression this fixes: buildMenuItemBson hardcoded Icon to nil, so an icon
// the author wrote was dropped on the way to the file. Assert on the encoded
// item, not just the helper, so the wiring is covered too.
func TestBuildMenuItemBson_CarriesTheIconThrough(t *testing.T) {
	item := buildMenuItemBson(NavMenuItemSpec{
		Caption: "Dashboard",
		Page:    "M.Dash",
		Icon:    "Atlas_Core.Atlas.align-center",
	})
	icon, present := navIconEntry(item, "Icon")
	if !present {
		t.Fatal("the Icon key must always be written, even when null")
	}
	d, ok := icon.(bson.D)
	if !ok {
		t.Fatalf("Icon = %#v; the authored icon was dropped on the way to BSON", icon)
	}
	if img, _ := navIconEntry(d, "Image"); img != "Atlas_Core.Atlas.align-center" {
		t.Errorf("Image = %v", img)
	}
}

// A sub-menu is built by the same recursion, so its icon has to survive it.
func TestBuildMenuItemBson_CarriesTheIconThroughSubItems(t *testing.T) {
	item := buildMenuItemBson(NavMenuItemSpec{
		Caption: "Reports",
		Items: []NavMenuItemSpec{{
			Caption: "Monthly", Page: "M.Monthly", Icon: "Atlas_Core.Atlas.folder",
		}},
	})
	items, _ := navIconEntry(item, "Items")
	arr, ok := items.(bson.A)
	if !ok || len(arr) != 2 { // [list-marker, one child]
		t.Fatalf("Items = %#v", items)
	}
	sub, ok := arr[1].(bson.D)
	if !ok {
		t.Fatalf("sub-item = %#v", arr[1])
	}
	icon, _ := navIconEntry(sub, "Icon")
	d, ok := icon.(bson.D)
	if !ok {
		t.Fatalf("sub-item Icon = %#v; the recursion dropped it", icon)
	}
	if img, _ := navIconEntry(d, "Image"); img != "Atlas_Core.Atlas.folder" {
		t.Errorf("sub-item Image = %v", img)
	}
}

// An item written without an icon keeps the null it had before this change.
func TestBuildMenuItemBson_NoIconStillWritesNull(t *testing.T) {
	item := buildMenuItemBson(NavMenuItemSpec{Caption: "Dashboard", Page: "M.Dash"})
	icon, present := navIconEntry(item, "Icon")
	if !present {
		t.Fatal("the Icon key must be written even with no icon")
	}
	if icon != nil {
		t.Errorf("Icon = %#v, want nil", icon)
	}
}

// Studio Pro stores the absence of a page-title override as an explicit null.
// An empty TextTemplate is a real override to "" and raises CW0263.
func TestBuildFormSettingsBson_NoTitleOverrideStaysNull(t *testing.T) {
	settings := buildFormSettingsBson("M.Dash")
	title, present := navIconEntry(settings, "TitleOverride")
	if !present {
		t.Fatal("TitleOverride key missing; Studio Pro writes an explicit null")
	}
	if title != nil {
		t.Fatalf("TitleOverride = %#v, want nil", title)
	}
}

// The read side has to recognise all three variants, because a project authored
// in Studio Pro contains all three. The fixtures are the literal shapes dumped
// from ako/mxcli-ledger's navigation document.
func TestParseNavMenuItem_ReadsEachIconVariant(t *testing.T) {
	for _, tc := range []struct {
		name           string
		icon           map[string]any
		wantType, want string
	}{
		{
			name:     "icon collection",
			icon:     map[string]any{"$Type": "Forms$IconCollectionIcon", "Image": "Atlas_Core.Atlas.align-center"},
			wantType: "Forms$IconCollectionIcon", want: "Atlas_Core.Atlas.align-center",
		},
		{
			name:     "image",
			icon:     map[string]any{"$Type": "Forms$ImageIcon", "Image": "System.Images.Close"},
			wantType: "Forms$ImageIcon", want: "System.Images.Close",
		},
		{
			// A glyph carries a numeric Code and no name at all. Reporting an
			// empty Icon here is load-bearing: it is what stops DESCRIBE from
			// emitting an ICON clause that would convert the variant on replay.
			name:     "glyph",
			icon:     map[string]any{"$Type": "Forms$GlyphIcon", "Code": int32(9999)},
			wantType: "Forms$GlyphIcon", want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mi := parseNavMenuItem(map[string]any{
				"Caption": map[string]any{},
				"Icon":    tc.icon,
				"Action": map[string]any{
					"$Type":        "Forms$FormAction",
					"FormSettings": map[string]any{"Form": "M.Dash"},
				},
			})
			if mi == nil {
				t.Fatal("the item did not parse")
			}
			if mi.IconType != tc.wantType {
				t.Errorf("IconType = %q, want %q", mi.IconType, tc.wantType)
			}
			if mi.Icon != tc.want {
				t.Errorf("Icon = %q, want %q", mi.Icon, tc.want)
			}
		})
	}
}

// A menu item with no icon must read back as no icon, not as an empty-named one.
func TestParseNavMenuItem_NoIconReadsAsNone(t *testing.T) {
	mi := parseNavMenuItem(map[string]any{
		"Caption": map[string]any{},
		"Action": map[string]any{
			"$Type": "Forms$FormAction", "FormSettings": map[string]any{"Form": "M.Dash"},
		},
	})
	if mi == nil {
		t.Fatal("the item did not parse")
	}
	if mi.Icon != "" || mi.IconType != "" {
		t.Errorf("Icon/IconType = (%q, %q), want both empty", mi.Icon, mi.IconType)
	}
}

// All three icon elements are written now. Only the collection variant used to
// be, and because `create or replace navigation` is a full replacement, an icon
// the writer would not emit was an icon the statement DELETED — measured on
// testdata/expr-checker, exec of DESCRIBE's own output destroyed a glyph icon at
// exit 0.
func TestBuildMenuIconBson_Glyph(t *testing.T) {
	got, ok := buildMenuIconBson(NavMenuItemSpec{IconKind: types.MenuIconGlyph, IconCode: 57345}).(bson.D)
	if !ok {
		t.Fatal("a glyph icon produced no document")
	}
	m := bsonDToMap(got)
	if m["$Type"] != "Forms$GlyphIcon" {
		t.Errorf("$Type = %v, want Forms$GlyphIcon", m["$Type"])
	}
	if m["Code"] != int32(57345) {
		t.Errorf("Code = %#v, want int32(57345) — Mendix stores the character code as an int32", m["Code"])
	}
	if _, hasImage := m["Image"]; hasImage {
		t.Error("a glyph icon must not carry an Image; it has no qualified name")
	}
}

func TestBuildMenuIconBson_Image(t *testing.T) {
	got, ok := buildMenuIconBson(NavMenuItemSpec{IconKind: types.MenuIconImage, Icon: "MyMod.Images.logo"}).(bson.D)
	if !ok {
		t.Fatal("an image icon produced no document")
	}
	m := bsonDToMap(got)
	if m["$Type"] != "Forms$ImageIcon" {
		t.Errorf("$Type = %v, want Forms$ImageIcon", m["$Type"])
	}
	if m["Image"] != "MyMod.Images.logo" {
		t.Errorf("Image = %v", m["Image"])
	}
}

// The control that keeps the dispatch honest: a name with NO kind still means an
// icon-collection icon. Every script written before the kind existed carries
// exactly that, so treating it as "unknown" would silently drop every icon in
// the corpus.
func TestBuildMenuIconBson_BareNameIsStillACollectionIcon(t *testing.T) {
	got, ok := buildMenuIconBson(NavMenuItemSpec{Icon: "Atlas_Core.Atlas.home"}).(bson.D)
	if !ok {
		t.Fatal("a bare name produced no document")
	}
	if bsonDToMap(got)["$Type"] != "Forms$IconCollectionIcon" {
		t.Errorf("$Type = %v, want Forms$IconCollectionIcon", bsonDToMap(got)["$Type"])
	}
}

// A glyph with no code identifies no glyph, and an element with no Code renders
// as a blank where an icon should be. Emit nothing instead.
func TestBuildMenuIconBson_GlyphWithoutACodeIsNoIcon(t *testing.T) {
	if got := buildMenuIconBson(NavMenuItemSpec{IconKind: types.MenuIconGlyph}); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func bsonDToMap(d bson.D) map[string]any {
	m := make(map[string]any, len(d))
	for _, e := range d {
		m[e.Key] = e.Value
	}
	return m
}
