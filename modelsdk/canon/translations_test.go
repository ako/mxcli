// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A Mendix text carries one Texts$Translation child per language. MDL carries
// ONE string, so a rebuilt document has exactly one — and every other language
// the stored document held is gone. Measured on 11.13.0 before this fix:
// describing Administration.MyAccount and re-executing that description dropped
// "Mijn account" from the entire project, taking nl_NL from 17 strings to 16.
// mxbuild reports 0 errors either way, so nothing catches it.
//
// The languages are independent siblings, which is why the rebuild loses them
// and also why they can be carried back: matching is by the source string within
// the document, which on a real project is exact — 34 units carry translations
// and in none of them does one source map to two translations in one language.
func textDoc(t *testing.T, id byte, name string, items ...bson.D) []byte {
	t.Helper()
	arr := bson.A{int32(3)}
	for _, it := range items {
		arr = append(arr, it)
	}
	return marshal(t, bson.D{
		{Key: "$Type", Value: "Forms$Form"},
		{Key: "$ID", Value: bin(1)},
		{Key: "Name", Value: name},
		{Key: "Title", Value: bson.D{
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "$ID", Value: bin(id)},
			{Key: "Items", Value: arr},
		}},
	})
}

func tr(id byte, lang, text string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Texts$Translation"},
		{Key: "$ID", Value: bin(id)},
		{Key: "LanguageCode", Value: lang},
		{Key: "Text", Value: text},
	}
}

// translationsOf returns lang→text for the first Texts$Text found.
func translationsOf(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out := map[string]string{}
	var walk func(v any) bool
	walk = func(v any) bool {
		if d, ok := asDoc(v); ok {
			if ty, _ := d["$Type"].(string); ty == "Texts$Text" {
				if items, ok := asSlice(d["Items"]); ok {
					for _, it := range items {
						if id, ok := asDoc(it); ok {
							lang, _ := id["LanguageCode"].(string)
							txt, _ := id["Text"].(string)
							if lang != "" {
								out[lang] = txt
							}
						}
					}
				}
				return true
			}
			for _, k := range sortedKeys(d) {
				if walk(d[k]) {
					return true
				}
			}
			return false
		}
		if s, ok := asSlice(v); ok {
			for _, e := range s {
				if walk(e) {
					return true
				}
			}
		}
		return false
	}
	walk(doc)
	return out
}

func TestCarryTranslations_CarriesTheLanguagesTheRebuildDropped(t *testing.T) {
	stored := textDoc(t, 10, "MyAccount",
		tr(11, "en_US", "My Account"),
		tr(12, "nl_NL", "Mijn account"),
		tr(13, "de_DE", "Mein Konto"),
	)
	// What a rebuild from MDL produces: the one string MDL carries.
	rebuilt := textDoc(t, 20, "MyAccount", tr(21, "en_US", "My Account"))

	got := translationsOf(t, CarryTranslations(rebuilt, stored))
	want := map[string]string{"en_US": "My Account", "nl_NL": "Mijn account", "de_DE": "Mein Konto"}
	if len(got) != len(want) {
		t.Fatalf("carried %v, want %v — a rebuild drops every language but the one MDL carries", got, want)
	}
	for l, v := range want {
		if got[l] != v {
			t.Errorf("%s = %q, want %q", l, got[l], v)
		}
	}
}

// The point of carrying them: a describe→exec round-trip on a translated
// document becomes a genuine no-op instead of a silent deletion.
func TestReconcile_TranslatedDocumentRoundTripsUnchanged(t *testing.T) {
	stored := textDoc(t, 10, "MyAccount",
		tr(11, "en_US", "My Account"),
		tr(12, "nl_NL", "Mijn account"),
	)
	rebuilt := textDoc(t, 20, "MyAccount", tr(21, "en_US", "My Account"))

	out, unchanged := Reconcile(rebuilt, stored)
	if !unchanged {
		t.Errorf("a rebuild that differs only by the translations it dropped was written; "+
			"after carrying them it is the same document\n got: %v", translationsOf(t, out))
	}
}

// A source string the author actually changed keeps the other languages, stale
// though they now are. That is what Studio Pro does when the English is edited —
// the other Texts$Translation siblings are untouched — so mxcli must not decide
// on the author's behalf that a translation is now wrong.
func TestCarryTranslations_KeepsOtherLanguagesWhenTheSourceChanged(t *testing.T) {
	stored := textDoc(t, 10, "MyAccount",
		tr(11, "en_US", "My Account"),
		tr(12, "nl_NL", "Mijn account"),
	)
	rebuilt := textDoc(t, 20, "MyAccount", tr(21, "en_US", "My Profile"))

	got := translationsOf(t, CarryTranslations(rebuilt, stored))
	if got["en_US"] != "My Profile" {
		t.Errorf("en_US = %q, want the authored 'My Profile' — carrying must never "+
			"overwrite what the statement said", got["en_US"])
	}
	if got["nl_NL"] != "Mijn account" {
		t.Errorf("nl_NL = %q, want 'Mijn account' kept — Studio Pro leaves the sibling "+
			"translation in place when the source is edited, and dropping it here would "+
			"delete work on the author's behalf", got["nl_NL"])
	}
}

// A language the rebuild DID write wins: the statement is the authority for what
// it says, and carrying is only for what it could not say.
func TestCarryTranslations_DoesNotOverwriteWhatTheRebuildWrote(t *testing.T) {
	stored := textDoc(t, 10, "P",
		tr(11, "en_US", "Save"),
		tr(12, "nl_NL", "Opslaan"),
	)
	rebuilt := textDoc(t, 20, "P",
		tr(21, "en_US", "Save"),
		tr(22, "nl_NL", "Bewaren"),
	)

	if got := translationsOf(t, CarryTranslations(rebuilt, stored)); got["nl_NL"] != "Bewaren" {
		t.Errorf("nl_NL = %q, want the rebuilt 'Bewaren'", got["nl_NL"])
	}
}

// Two stored texts sharing a source string but disagreeing on the translation
// are not resolvable by source matching, so nothing is carried for that string.
// Refusing to guess matters more than recovering the last translation: guessing
// would silently move a translation from one place to another.
func TestCarryTranslations_RefusesToGuessOnAnAmbiguousSource(t *testing.T) {
	stored := marshal(t, bson.D{
		{Key: "$Type", Value: "Forms$Form"},
		{Key: "$ID", Value: bin(1)},
		{Key: "A", Value: bson.D{
			{Key: "$Type", Value: "Texts$Text"}, {Key: "$ID", Value: bin(10)},
			{Key: "Items", Value: bson.A{int32(3), tr(11, "en_US", "Cancel"), tr(12, "nl_NL", "Annuleren")}},
		}},
		{Key: "B", Value: bson.D{
			{Key: "$Type", Value: "Texts$Text"}, {Key: "$ID", Value: bin(20)},
			{Key: "Items", Value: bson.A{int32(3), tr(21, "en_US", "Cancel"), tr(22, "nl_NL", "Annuleer")}},
		}},
	})
	rebuilt := textDoc(t, 30, "P", tr(31, "en_US", "Cancel"))

	if got := translationsOf(t, CarryTranslations(rebuilt, stored)); len(got) != 1 {
		t.Errorf("carried %v for a source with two different stored translations — "+
			"picking one silently moves a translation between elements", got)
	}
}

// Malformed input on either side passes through untouched, the behaviour that
// existed before this function.
func TestCarryTranslations_MalformedPassesThrough(t *testing.T) {
	good := textDoc(t, 10, "P", tr(11, "en_US", "Save"))
	for _, tc := range []struct {
		name             string
		contents, stored []byte
	}{
		{"stored malformed", good, []byte{1, 2, 3}},
		{"contents malformed", []byte{1, 2, 3}, good},
		{"stored empty", good, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := CarryTranslations(tc.contents, tc.stored)
			if string(out) != string(tc.contents) {
				t.Error("contents were modified despite unusable input")
			}
		})
	}
}
