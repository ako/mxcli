// SPDX-License-Identifier: Apache-2.0

// The image widgets, pinned to the shape Mendix stores.
//
// This is the legacy half of the pair; the modelsdk half is
// mdl/backend/modelsdk/widget_write_legacy_gaps_test.go and asserts the same
// things about the same widgets. Keeping both is the point: the two engines had
// silently drifted apart here, and only one of them was right.
//
// Ground truth is the three Studio-Pro-authored Forms$StaticImageViewer widgets
// ako/TestApp inherits from FeedbackModule, plus generated/metamodel (the
// arbiter per CLAUDE.md) for Forms$ImageViewer, which no reference project in
// reach carries.

package mpr

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"

	"go.mongodb.org/mongo-driver/bson"
)

func imgKeys(doc bson.D) string {
	out := make([]string, 0, len(doc))
	for _, e := range doc {
		out = append(out, e.Key)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// TestStaticImageMatchesStudioPro — legacy used to omit AlternativeText, which
// generated/metamodel declares without omitempty and all three references carry,
// and to write BSON null for the unset Image. An unset by-name reference is the
// empty string: measured 0 nulls against 4,400+ empty strings over 40
// (type, property) pairs in ako/TestApp.
func TestStaticImageMatchesStudioPro(t *testing.T) {
	img := &pages.StaticImage{Responsive: true}
	img.Name = "i1"
	doc := serializeStaticImage(img)

	want := "$ID,$Type,AlternativeText,Appearance,ClickAction," +
		"ConditionalVisibilitySettings,Height,HeightUnit,Image,Name," +
		"NativeAccessibilitySettings,Responsive,TabIndex,Width,WidthUnit"
	if got := imgKeys(doc); got != want {
		t.Errorf("keys\n got  %s\n want %s", got, want)
	}
	if got := bsonLookup(doc, "Image"); got != "" {
		t.Errorf("Image = %#v, want the empty string", got)
	}
	assertEmptyClientTemplateBSON(t, doc, "AlternativeText")
}

// TestDynamicImageMatchesMetamodel — legacy's AlternativeText here was
// hand-rolled and carried a FallbackValue key that Forms$ClientTemplate does not
// have, four lines after a comment in the shared serializer saying exactly that
// ("Must be Fallback object, not FallbackValue string").
func TestDynamicImageMatchesMetamodel(t *testing.T) {
	img := &pages.DynamicImage{Responsive: true}
	img.Name = "i2"
	doc := serializeDynamicImage(img)

	want := "$ID,$Type,AlternativeText,Appearance,ClickAction," +
		"ConditionalVisibilitySettings,DataSource,DefaultImage,Height,HeightUnit," +
		"Name,NativeAccessibilitySettings,OnClickEnlarge,Responsive," +
		"ShowAsThumbnail,TabIndex,Width,WidthUnit"
	if got := imgKeys(doc); got != want {
		t.Errorf("keys\n got  %s\n want %s", got, want)
	}
	if got := bsonLookup(doc, "DefaultImage"); got != "" {
		t.Errorf("DefaultImage = %#v, want the empty string", got)
	}
	assertEmptyClientTemplateBSON(t, doc, "AlternativeText")
}

func assertEmptyClientTemplateBSON(t *testing.T, parent bson.D, key string) {
	t.Helper()
	ct := bsonSubDoc(t, parent, key)
	if got := bsonLookup(ct, "$Type"); got != "Forms$ClientTemplate" {
		t.Errorf("%s.$Type = %v", key, got)
	}
	if bsonLookup(ct, "FallbackValue") != nil {
		t.Errorf("%s carries a FallbackValue; Forms$ClientTemplate has no such property "+
			"(metamodel: Fallback / Parameters / Template). Studio Pro refuses to open a "+
			"document with an unknown property; mxbuild builds it at 0 errors", key)
	}
	for _, sub := range []string{"Fallback", "Template"} {
		txt := bsonSubDoc(t, ct, sub)
		if got := bsonLookup(txt, "$Type"); got != "Texts$Text" {
			t.Errorf("%s.%s.$Type = %v", key, sub, got)
		}
		items, ok := bsonLookup(txt, "Items").(bson.A)
		if !ok || len(items) != 1 || items[0] != int32(3) {
			t.Errorf("%s.%s.Items = %#v, want [3]", key, sub, bsonLookup(txt, "Items"))
		}
	}
	params, ok := bsonLookup(ct, "Parameters").(bson.A)
	if !ok || len(params) != 1 || params[0] != int32(2) {
		t.Errorf("%s.Parameters = %#v, want [2]", key, bsonLookup(ct, "Parameters"))
	}
}
