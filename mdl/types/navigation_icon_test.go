// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

// A menu item's icon is one of three Mendix elements, and mxcli could name only
// one of them. MenuIconKindOf maps the storage $Type onto a vocabulary so the
// readers, the writers, DESCRIBE and MDL077 all decide the same way instead of
// each doing its own strings.HasSuffix.
func TestMenuIconKindOf(t *testing.T) {
	cases := []struct {
		iconType string
		want     MenuIconKind
	}{
		{"", MenuIconNone},
		{"Forms$IconCollectionIcon", MenuIconCollection},
		{"Forms$GlyphIcon", MenuIconGlyph},
		{"Forms$ImageIcon", MenuIconImage},
		// The metamodel spells these Pages$…; the storage name is Forms$…
		// ("Form" was the original term for "Page"). Both must map, or a reader
		// that hands over the metamodel name silently produces "no icon".
		{"Pages$IconCollectionIcon", MenuIconCollection},
		{"Pages$GlyphIcon", MenuIconGlyph},
		{"Pages$ImageIcon", MenuIconImage},
		// Anything else is unknown rather than none: "none" would make a future
		// fourth variant look like an absent icon and get silently dropped.
		{"Forms$SomethingElse", MenuIconUnknown},
	}
	for _, c := range cases {
		if got := MenuIconKindOf(c.iconType); got != c.want {
			t.Errorf("MenuIconKindOf(%q) = %q, want %q", c.iconType, got, c.want)
		}
	}
}

// The inverse, used by the writers. Round-tripping the vocabulary back to the
// storage name is what lets one kind field drive both engines.
func TestMenuIconStorageType(t *testing.T) {
	for _, k := range []MenuIconKind{MenuIconCollection, MenuIconGlyph, MenuIconImage} {
		st := MenuIconStorageType(k)
		if st == "" {
			t.Errorf("no storage type for kind %q", k)
			continue
		}
		if back := MenuIconKindOf(st); back != k {
			t.Errorf("%q -> %q -> %q, want round trip", k, st, back)
		}
	}
	if MenuIconStorageType(MenuIconNone) != "" {
		t.Error("MenuIconNone must have no storage type — it is the absence of an Icon element")
	}
}

// HasIcon is what MDL077 asks. A glyph icon carries a numeric code and NO name,
// so a check written as `Icon == ""` reports an item that plainly has an icon.
func TestNavMenuItemHasIcon(t *testing.T) {
	cases := []struct {
		name string
		item NavMenuItem
		want bool
	}{
		{"no icon", NavMenuItem{}, false},
		{"collection", NavMenuItem{IconType: "Forms$IconCollectionIcon", Icon: "Atlas_Core.Atlas.home"}, true},
		{"image", NavMenuItem{IconType: "Forms$ImageIcon", Icon: "MyMod.Images.logo"}, true},
		// The case that matters: a name-less icon that still IS an icon.
		{"glyph", NavMenuItem{IconType: "Forms$GlyphIcon", IconCode: 57345}, true},
	}
	for _, c := range cases {
		if got := c.item.HasIcon(); got != c.want {
			t.Errorf("%s: HasIcon() = %v, want %v", c.name, got, c.want)
		}
	}
}
