// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/mpr"
)

func txt(pairs ...string) bson.D {
	items := bson.A{int32(3)}
	for i := 0; i+1 < len(pairs); i += 2 {
		items = append(items, bson.D{
			{Key: "$Type", Value: "Texts$Translation"},
			{Key: "LanguageCode", Value: pairs[i]},
			{Key: "Text", Value: pairs[i+1]},
		})
	}
	return bson.D{{Key: "$Type", Value: "Texts$Text"}, {Key: "Items", Value: items}}
}

func marshal(t *testing.T, d bson.D) []byte {
	t.Helper()
	raw, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The gap this replaces: a widget caption is translatable text the hand-written
// extractor never reached, because it only ever read a page's Title. Measured on
// a stock project, the index held ~69 of 3265 texts.
func TestTranslatableRows_ReachesAWidgetCaptionNotJustThePageTitle(t *testing.T) {
	raw := marshal(t, bson.D{
		{Key: "$ID", Value: mpr.IDToBsonBinary("11111111-1111-1111-1111-111111111111")},
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Name", Value: "Home"},
		{Key: "Title", Value: txt("en_US", "Home")},
		{Key: "Widgets", Value: bson.A{int32(3),
			bson.D{
				{Key: "$ID", Value: mpr.IDToBsonBinary("22222222-2222-2222-2222-222222222222")},
				{Key: "$Type", Value: "Forms$ActionButton"},
				{Key: "Caption", Value: txt("en_US", "Save", "nl_NL", "Opslaan")},
			},
		}},
	})

	rows := translatableRows("Forms$Page", "MyModule.Home", "MyModule", raw)

	var caption *stringRow
	for i := range rows {
		if rows[i].StringValue == "Opslaan" {
			caption = &rows[i]
		}
	}
	if caption == nil {
		t.Fatalf("the button caption never reached the index; rows = %+v", rows)
	}
	if caption.Language != "nl_NL" {
		t.Errorf("Language = %q, want nl_NL", caption.Language)
	}
	if caption.StringContext != "Forms$ActionButton.Caption" {
		t.Errorf("StringContext = %q, want Forms$ActionButton.Caption", caption.StringContext)
	}
	if caption.ElementID != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("ElementID = %q, want the button's own", caption.ElementID)
	}
	if caption.ObjectType != "PAGE" {
		t.Errorf("ObjectType = %q, want PAGE", caption.ObjectType)
	}

	// The title must still be there — the walk replaces the typed extraction,
	// it does not trade one site for another.
	var sawTitle bool
	for _, r := range rows {
		if r.StringValue == "Home" && r.StringContext == "Forms$Page.Title" {
			sawTitle = true
		}
	}
	if !sawTitle {
		t.Errorf("page title lost; rows = %+v", rows)
	}
}

// A language reaching the index at all is what decides whether SHOW LANGUAGES
// can list it and whether QUAL005 can reason about it. Measured: the old
// extractor saw 8 of the project's 9 languages, and ar_DZ was invisible rather
// than undercounted.
func TestTranslatableRows_ALanguageOnlyOnAWidgetStillReachesTheIndex(t *testing.T) {
	raw := marshal(t, bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Title", Value: txt("en_US", "Home")},
		{Key: "Widgets", Value: bson.A{int32(3),
			bson.D{
				{Key: "$Type", Value: "Forms$Label"},
				{Key: "Caption", Value: txt("en_US", "Hello", "ar_DZ", "مرحبا")},
			},
		}},
	})

	rows := translatableRows("Forms$Page", "MyModule.Home", "MyModule", raw)

	langs := map[string]bool{}
	for _, r := range rows {
		langs[r.Language] = true
	}
	if !langs["ar_DZ"] {
		t.Fatalf("ar_DZ never reached the index, so SHOW LANGUAGES cannot list it; languages = %v", langs)
	}
}

// An empty translation is a text that exists but is not translated yet. Writing
// it as a row would make the language look present everywhere it is not, which
// is the opposite of what QUAL005 is for.
func TestTranslatableRows_AnEmptyTranslationIsNotARow(t *testing.T) {
	raw := marshal(t, bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Title", Value: txt("en_US", "Home", "nl_NL", "")},
	})

	for _, r := range translatableRows("Forms$Page", "MyModule.Home", "MyModule", raw) {
		if r.Language == "nl_NL" {
			t.Fatalf("an untranslated nl_NL was indexed as though it were translated: %+v", r)
		}
	}
}

// Atlas design templates are ~70% of a project's texts and their captions never
// render in the app, so a consumer has to be able to filter them out. They are
// indexed rather than dropped because DESCRIBE TRANSLATIONS reaches them, and a
// SHOW LANGUAGES that disagreed with it would be the same split being fixed here.
func TestCatalogObjectType_DerivesFromTheUnitTypeWithoutATable(t *testing.T) {
	for unitType, want := range map[string]string{
		"Forms$Page":                    "PAGE",
		"Forms$PageTemplate":            "PAGE_TEMPLATE",
		"Forms$BuildingBlock":           "BUILDING_BLOCK",
		"Microflows$Microflow":          "MICROFLOW",
		"Microflows$Nanoflow":           "NANOFLOW",
		"Enumerations$Enumeration":      "ENUMERATION",
		"Texts$SystemTextCollection":    "SYSTEM_TEXT_COLLECTION",
		"Navigation$NavigationDocument": "NAVIGATION_DOCUMENT",
		"Forms$Layout":                  "LAYOUT",
	} {
		if got := catalogObjectType(unitType); got != want {
			t.Errorf("catalogObjectType(%q) = %q, want %q", unitType, got, want)
		}
	}
}
