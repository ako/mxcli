// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
)

// withAuthoringLanguage runs the test under a project default language, the way
// execAlterPage publishes it before any write.
func withAuthoringLanguage(t *testing.T, lang string) {
	t.Helper()
	prev := model.AuthoringLanguage()
	model.SetAuthoringLanguage(lang)
	t.Cleanup(func() { model.SetAuthoringLanguage(prev) })
}

// translationsOf reads a Texts$Text's Items as language -> text, failing on a
// duplicate language (two entries for one language is not a shape Studio Pro writes).
func translationsOf(t *testing.T, text bson.D) map[string]string {
	t.Helper()
	got := map[string]string{}
	for _, it := range bsonnav.DGetArrayElements(bsonnav.DGet(text, "Items")) {
		d, ok := it.(bson.D)
		if !ok {
			continue
		}
		lang := bsonnav.DGetString(d, "LanguageCode")
		if _, dup := got[lang]; dup {
			t.Fatalf("two translations for %s in %v", lang, text)
		}
		got[lang] = bsonnav.DGetString(d, "Text")
	}
	if v := bsonnav.DGet(text, "Text"); v != nil {
		t.Fatalf("stray Text field %v written onto the Texts$Text", v)
	}
	return got
}

// The reported case: a project whose default language is de_DE, a tab caption
// with only an en_US translation. SET Caption writes the de_DE translation and
// keeps the en_US one — it does not overwrite another language's text, and it
// does not leave the default language empty (CE4899).
func TestTabPage_SetCaption_WritesAuthoringLanguage(t *testing.T) {
	withAuthoringLanguage(t, "de_DE")
	raw := makeTabControlPage()
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}

	if err := m.SetWidgetProperty("tabPage2", "Caption", "Benutzer"); err != nil {
		t.Fatalf("set Caption on tabPage2: %v", err)
	}
	caption := bsonnav.DGetDoc(findBsonWidget(raw, "tabPage2").widget, "Caption")
	got := translationsOf(t, caption)
	if got["de_DE"] != "Benutzer" || got["en_US"] != "Local Users" || len(got) != 2 {
		t.Fatalf("translations = %v, want de_DE=Benutzer plus the untouched en_US=Local Users", got)
	}

	// A second set updates the de_DE entry in place rather than appending another.
	if err := m.SetWidgetProperty("tabPage2", "caption", "Konten"); err != nil {
		t.Fatalf("second set Caption: %v", err)
	}
	got = translationsOf(t, caption)
	if got["de_DE"] != "Konten" || got["en_US"] != "Local Users" || len(got) != 2 {
		t.Fatalf("after second set, translations = %v", got)
	}
}

// The same language rule for every text ALTER PAGE writes. A button caption
// (Forms$ClientTemplate) used to overwrite whichever translation came first, and
// the page Title overwrote every language with the same string.
func TestSetText_TargetsAuthoringLanguage(t *testing.T) {
	withAuthoringLanguage(t, "de_DE")

	t.Run("button caption", func(t *testing.T) {
		btn := bson.D{
			{Key: "$Type", Value: "Forms$ActionButton"},
			{Key: "Name", Value: "btn"},
			{Key: "CaptionTemplate", Value: bson.D{
				{Key: "$Type", Value: "Forms$ClientTemplate"},
				{Key: "Parameters", Value: bson.A{int32(2)}},
				{Key: "Template", Value: textsText(translation("en_US", "Save"))},
			}},
		}
		raw := makeRawPage(btn)
		m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
		if err := m.SetWidgetProperty("btn", "Caption", "Speichern"); err != nil {
			t.Fatal(err)
		}
		tmpl := bsonnav.DGetDoc(bsonnav.DGetDoc(findBsonWidget(raw, "btn").widget, "CaptionTemplate"), "Template")
		if got := translationsOf(t, tmpl); got["de_DE"] != "Speichern" || got["en_US"] != "Save" {
			t.Fatalf("translations = %v", got)
		}
	})

	t.Run("page title", func(t *testing.T) {
		raw := append(makeRawPage(), bson.E{Key: "Title", Value: textsText(
			translation("en_US", "Accounts"), translation("de_DE", "Konten-alt"))})
		m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
		if err := m.SetWidgetProperty("", "Title", "Konten"); err != nil {
			t.Fatal(err)
		}
		if got := translationsOf(t, bsonnav.DGetDoc(m.rawData, "Title")); got["de_DE"] != "Konten" || got["en_US"] != "Accounts" {
			t.Fatalf("translations = %v", got)
		}
	})

	// An input widget's label is a Forms$ClientTemplate under LabelTemplate —
	// Studio Pro 11 writes no `Label` key at all. `set Label = … on textBox6`
	// looked only for `Label`, found nothing and returned nil: "Altered page",
	// label unchanged.
	t.Run("input widget label", func(t *testing.T) {
		tb := bson.D{
			{Key: "$Type", Value: "Forms$TextBox"},
			{Key: "Name", Value: "textBox6"},
			{Key: "LabelTemplate", Value: bson.D{
				{Key: "$Type", Value: "Forms$ClientTemplate"},
				{Key: "Fallback", Value: textsText()},
				{Key: "Parameters", Value: bson.A{int32(2)}},
				{Key: "Template", Value: textsText(translation("en_US", "Full name"))},
			}},
		}
		raw := makeRawPage(tb)
		m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
		if err := m.SetWidgetProperty("textBox6", "Label", "Name"); err != nil {
			t.Fatal(err)
		}
		tmpl := bsonnav.DGetDoc(bsonnav.DGetDoc(findBsonWidget(raw, "textBox6").widget, "LabelTemplate"), "Template")
		if got := translationsOf(t, tmpl); got["de_DE"] != "Name" || got["en_US"] != "Full name" {
			t.Fatalf("translations = %v", got)
		}
	})

	t.Run("unknown text shape is refused, not dropped", func(t *testing.T) {
		w := bson.D{
			{Key: "$Type", Value: "Forms$Odd"},
			{Key: "Name", Value: "odd"},
			{Key: "Caption", Value: bson.D{{Key: "$Type", Value: "Forms$Unknown"}}},
		}
		m := &Mutator{rawData: makeRawPage(w), widgetFinder: findBsonWidget}
		if err := m.SetWidgetProperty("odd", "Caption", "x"); err == nil {
			t.Fatal("set Caption on an unrecognised text shape returned nil; it stored nothing")
		}
	})
}

// mendixlabs/mxcli#1182: "SET Caption wrote Dutch". A caption first authored in
// a Dutch-default project and later translated stores nl_NL FIRST and en_US
// second; once the default is en_US, `set Caption` must write the en_US entry.
// Writing whichever translation is listed first put the English text into
// nl_NL and left en_US as it was. The other tests here have the default
// language either absent or listed first, so none of them could tell the two
// rules apart.
func TestSetCaption_DefaultLanguageListedSecond(t *testing.T) {
	withAuthoringLanguage(t, "en_US")
	btn := bson.D{
		{Key: "$Type", Value: "Forms$ActionButton"},
		{Key: "Name", Value: "btnSave"},
		{Key: "CaptionTemplate", Value: bson.D{
			{Key: "$Type", Value: "Forms$ClientTemplate"},
			{Key: "Parameters", Value: bson.A{int32(2)}},
			{Key: "Template", Value: textsText(translation("nl_NL", "Opslaan"), translation("en_US", "Save"))},
		}},
	}
	raw := makeRawPage(btn)
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("btnSave", "Caption", "Save changes"); err != nil {
		t.Fatal(err)
	}
	tmpl := bsonnav.DGetDoc(bsonnav.DGetDoc(findBsonWidget(raw, "btnSave").widget, "CaptionTemplate"), "Template")
	got := translationsOf(t, tmpl)
	if got["en_US"] != "Save changes" || got["nl_NL"] != "Opslaan" || len(got) != 2 {
		t.Fatalf("translations = %v, want en_US=Save changes and the untouched nl_NL=Opslaan", got)
	}
}
