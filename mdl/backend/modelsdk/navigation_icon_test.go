// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"go.mongodb.org/mongo-driver/bson"
	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"
)

func navIconEntry(d bson.D, key string) (interface{}, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// Both engines write the same document, so the storage name has to agree. A
// divergence here means MXCLI_ENGINE silently changes what lands in the .mpr —
// and only one of the two would open in Studio Pro.
func TestNavMenuIconBson_MatchesTheMprEngine(t *testing.T) {
	d, ok := navMenuIconBson(types.NavMenuItemSpec{Icon: "Atlas_Core.Atlas.align-center"}).(bson.D)
	if !ok {
		t.Fatal("expected a bson.D")
	}
	if typ, _ := navIconEntry(d, "$Type"); typ != "Forms$IconCollectionIcon" {
		t.Errorf("$Type = %v, want Forms$IconCollectionIcon", typ)
	}
	if img, _ := navIconEntry(d, "Image"); img != "Atlas_Core.Atlas.align-center" {
		t.Errorf("Image = %v", img)
	}
	if len(d) != 3 {
		t.Errorf("icon has %d keys, want exactly $ID/$Type/Image: %v", len(d), d)
	}
}

func TestNavMenuIconBson_EmptyNameStaysNull(t *testing.T) {
	if got := navMenuIconBson(types.NavMenuItemSpec{}); got != nil {
		t.Errorf("navMenuIconBson(empty spec) = %v, want nil", got)
	}
}

// The Icon must survive navMenuItemBson, not just the helper.
func TestNavMenuItemBson_CarriesTheIconThrough(t *testing.T) {
	item := navMenuItemBson(types.NavMenuItemSpec{
		Caption: "Dashboard", Page: "M.Dash", Icon: "Atlas_Core.Atlas.align-center",
	})
	icon, present := navIconEntry(item, "Icon")
	if !present {
		t.Fatal("the Icon key must always be written")
	}
	d, ok := icon.(bson.D)
	if !ok {
		t.Fatalf("Icon = %#v; the authored icon was dropped", icon)
	}
	if img, _ := navIconEntry(d, "Image"); img != "Atlas_Core.Atlas.align-center" {
		t.Errorf("Image = %v", img)
	}
}

// Studio Pro stores the absence of a page-title override as an explicit null.
// An empty TextTemplate is a real override to "" and raises CW0263.
func TestNavFormSettingsBson_NoTitleOverrideStaysNull(t *testing.T) {
	settings := navFormSettingsBson("M.Dash")
	title, present := navIconEntry(settings, "TitleOverride")
	if !present {
		t.Fatal("TitleOverride key missing; Studio Pro writes an explicit null")
	}
	if title != nil {
		t.Fatalf("TitleOverride = %#v, want nil", title)
	}
}

func TestMenuIconOf_NilIconYieldsNothing(t *testing.T) {
	typeName, image, code := menuIconOf(nil)
	if typeName != "" || image != "" || code != 0 {
		t.Errorf("menuIconOf(nil) = (%q, %q, %d), want empty", typeName, image, code)
	}
}

// decodeIcon puts an icon document through the real codec, which is the only
// way to get the element shape menuIconOf actually receives.
func decodeIcon(t *testing.T, typeName string, extra bsonv2.E) element.Element {
	t.Helper()
	doc := bsonv2.D{{Key: "$Type", Value: typeName}, extra}
	raw, err := bsonv2.Marshal(bsonv2.D{{Key: "Icon", Value: doc}})
	if err != nil {
		t.Fatalf("marshalling the fixture: %v", err)
	}
	el, err := codec.DecodeChild(bsonv2.Raw(raw), "Icon")
	if err != nil {
		t.Fatalf("decoding the icon: %v", err)
	}
	return el
}

// menuIconOf must read the name off a REGISTERED variant.
//
// This is the case a nil-only test cannot reach. The codec returns a bare
// *element.Base for an unregistered type but a generated struct — which merely
// embeds Base — for a registered one. An assertion on *element.Base therefore
// fails for precisely the variants that exist in real projects, and DESCRIBE
// dropped every Atlas icon under this engine while the legacy engine printed
// them. Caught only by running against a real project, not by a unit test on a
// hand-built value.
func TestMenuIconOf_ReadsTheNameOffARegisteredVariant(t *testing.T) {
	for _, tc := range []struct{ typeName, image string }{
		{"Forms$IconCollectionIcon", "Atlas_Core.Atlas.align-center"},
		{"Forms$ImageIcon", "System.Images.Close"},
	} {
		t.Run(tc.typeName, func(t *testing.T) {
			el := decodeIcon(t, tc.typeName, bsonv2.E{Key: "Image", Value: tc.image})
			gotType, gotImage, _ := menuIconOf(el)
			if gotType != tc.typeName {
				t.Errorf("type = %q, want %q", gotType, tc.typeName)
			}
			if gotImage != tc.image {
				t.Errorf("image = %q, want %q — the name was dropped", gotImage, tc.image)
			}
		})
	}
}

// A glyph carries a Code and no name; reporting an empty image is what stops
// DESCRIBE from emitting a lossy ICON clause for it.
func TestMenuIconOf_GlyphHasNoName(t *testing.T) {
	el := decodeIcon(t, "Forms$GlyphIcon", bsonv2.E{Key: "Code", Value: int32(9999)})
	gotType, gotImage, _ := menuIconOf(el)
	if gotType != "Forms$GlyphIcon" {
		t.Errorf("type = %q", gotType)
	}
	if gotImage != "" {
		t.Errorf("image = %q, want empty: a glyph has no qualified name", gotImage)
	}
}

// The glyph's Code is the only thing that says WHICH glyph. Reading the $Type
// alone left a caller knowing one was there and nothing more, so DESCRIBE could
// not re-emit it and a rewrite replaced it with nothing.
func TestMenuIconOf_ReadsTheGlyphCode(t *testing.T) {
	el := decodeIcon(t, "Forms$GlyphIcon", bsonv2.E{Key: "Code", Value: int32(57345)})
	gotType, gotImage, gotCode := menuIconOf(el)
	if gotType != "Forms$GlyphIcon" {
		t.Errorf("type = %q, want Forms$GlyphIcon", gotType)
	}
	if gotImage != "" {
		t.Errorf("image = %q, want empty — a glyph carries no qualified name", gotImage)
	}
	if gotCode != 57345 {
		t.Errorf("code = %d, want 57345", gotCode)
	}
}

// The control: a collection icon must NOT pick up a code, or "has a code" stops
// distinguishing a glyph from anything else.
func TestMenuIconOf_CollectionIconHasNoCode(t *testing.T) {
	el := decodeIcon(t, "Forms$IconCollectionIcon", bsonv2.E{Key: "Image", Value: "Atlas_Core.Atlas.home"})
	_, gotImage, gotCode := menuIconOf(el)
	if gotImage != "Atlas_Core.Atlas.home" {
		t.Errorf("image = %q", gotImage)
	}
	if gotCode != 0 {
		t.Errorf("code = %d, want 0", gotCode)
	}
}
