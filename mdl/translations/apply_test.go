// SPDX-License-Identifier: Apache-2.0

package translations

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func tr(lang, text string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Texts$Translation"},
		{Key: "$ID", Value: lang + "-id"},
		{Key: "LanguageCode", Value: lang},
		{Key: "Text", Value: text},
	}
}

// text builds a Texts$Text nested inside a document, the way every real one is:
// a leaf value under some property, never a document of its own.
func text(items ...bson.D) bson.D {
	arr := bson.A{itemsMarker}
	for _, it := range items {
		arr = append(arr, it)
	}
	return bson.D{
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "$ID", Value: "text-id"},
		{Key: "Items", Value: arr},
	}
}

func unit(t *testing.T, texts ...bson.D) []byte {
	t.Helper()
	widgets := bson.A{int32(3)}
	for i, tx := range texts {
		widgets = append(widgets, bson.D{
			{Key: "$Type", Value: "Forms$Button"},
			{Key: "$ID", Value: string(rune('a' + i))},
			{Key: "Caption", Value: tx},
		})
	}
	raw, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Forms$Form"},
		{Key: "Name", Value: "P"},
		{Key: "Widgets", Value: widgets},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

func langsOf(t *testing.T, raw []byte) []map[string]string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var out []map[string]string
	for _, tx := range findTexts(doc) {
		out = append(out, tx.byLang)
	}
	return out
}

func TestCollectFromUnit(t *testing.T) {
	raw := unit(t,
		text(tr("en_US", "Save"), tr("nl_NL", "Opslaan")),
		text(tr("en_US", "Cancel")),
		text(tr("nl_NL", "Alleen Nederlands")), // no source string
		text(),                                 // unset caption
	)

	got := CollectFromUnit(raw, "en_US")
	if len(got) != 2 {
		t.Fatalf("collected %d entries, want 2 — a text with no source string has "+
			"nothing to translate from and no key a dictionary could address it by: %+v", len(got), got)
	}
	if got[0].Source != "Save" || got[0].Targets["nl_NL"] != "Opslaan" {
		t.Errorf("entry 0 = %+v, want Save/Opslaan", got[0])
	}
	if got[1].Source != "Cancel" || len(got[1].Targets) != 0 {
		t.Errorf("entry 1 = %+v, want Cancel with no targets", got[1])
	}
}

func TestPatchUnit_Merge(t *testing.T) {
	raw := unit(t,
		text(tr("en_US", "Save")),
		text(tr("en_US", "Cancel"), tr("nl_NL", "Annuleren")),
	)
	out, stats := PatchUnit(raw, "en_US", "nl_NL", map[string]string{"Save": "Opslaan"}, ModeMerge)

	if stats.Set != 1 || stats.Removed != 0 {
		t.Fatalf("stats = %+v, want 1 set / 0 removed", stats)
	}
	got := langsOf(t, out)
	if got[0]["nl_NL"] != "Opslaan" {
		t.Errorf("Save's nl_NL = %q, want Opslaan", got[0]["nl_NL"])
	}
	if got[1]["nl_NL"] != "Annuleren" {
		t.Errorf("a source the file does not name lost its translation (%q) — merge "+
			"must leave it alone", got[1]["nl_NL"])
	}
}

// The point of REPLACE: the file is authoritative, so a translation the file
// does not account for is removed. That is what stops a file and a project
// drifting apart, and it is why the caller has to report what it deletes.
func TestPatchUnit_ReplaceRemovesWhatTheFileDoesNotName(t *testing.T) {
	raw := unit(t,
		text(tr("en_US", "Save"), tr("nl_NL", "Opslaan")),
		text(tr("en_US", "Cancel"), tr("nl_NL", "Annuleren")),
		text(tr("en_US", "Delete"), tr("nl_NL", "Verwijderen"), tr("de_DE", "Löschen")),
	)
	out, stats := PatchUnit(raw, "en_US", "nl_NL", map[string]string{"Save": "Opslaan"}, ModeReplace)

	if stats.Removed != 2 {
		t.Fatalf("removed %d, want 2 (Cancel and Delete are not in the file): %+v", stats.Removed, stats)
	}
	if len(stats.RemovedSources) != 2 || stats.RemovedSources[0] != "Cancel" {
		t.Errorf("RemovedSources = %v, want the source strings named so a caller can "+
			"report the work it deletes", stats.RemovedSources)
	}
	got := langsOf(t, out)
	if got[0]["nl_NL"] != "Opslaan" {
		t.Errorf("the named source lost its translation: %v", got[0])
	}
	if _, still := got[1]["nl_NL"]; still {
		t.Errorf("Cancel kept its nl_NL under replace: %v", got[1])
	}
	if got[2]["de_DE"] != "Löschen" {
		t.Errorf("replace touched a DIFFERENT language (%v) — it is scoped to the one "+
			"language the statement names", got[2])
	}
	if got[2]["en_US"] != "Delete" {
		t.Errorf("replace touched the SOURCE language: %v", got[2])
	}
}

// An empty entry and an absent entry both say the file has no translation, so
// they must behave identically. Describing an untranslated language emits empty
// entries, and those must not mean "translate this to nothing".
func TestPatchUnit_EmptyEntryMeansNoTranslation(t *testing.T) {
	raw := unit(t, text(tr("en_US", "Save"), tr("nl_NL", "Opslaan")))

	_, merge := PatchUnit(raw, "en_US", "nl_NL", map[string]string{"Save": ""}, ModeMerge)
	if merge.Set != 0 || merge.Removed != 0 {
		t.Errorf("an empty entry changed something under merge: %+v", merge)
	}

	out, repl := PatchUnit(raw, "en_US", "nl_NL", map[string]string{"Save": ""}, ModeReplace)
	if repl.Removed != 1 {
		t.Errorf("an empty entry did not remove under replace (%+v) — the file says it "+
			"has no translation, same as leaving it out", repl)
	}
	if _, still := langsOf(t, out)[0]["nl_NL"]; still {
		t.Error("nl_NL survived")
	}
}

// A no-op patch must return the input bytes untouched so the caller can skip the
// write. Writing an identical unit would churn version control for nothing.
func TestPatchUnit_NoMatchIsBytewiseUnchanged(t *testing.T) {
	raw := unit(t, text(tr("en_US", "Save"), tr("nl_NL", "Opslaan")))
	out, stats := PatchUnit(raw, "en_US", "nl_NL", map[string]string{"Save": "Opslaan"}, ModeMerge)
	if stats.touched() {
		t.Fatalf("setting a translation to what it already says counted as a change: %+v", stats)
	}
	if string(out) != string(raw) {
		t.Error("bytes changed despite nothing to do")
	}
}

// Editing an existing translation keeps the element, so its $ID survives — a
// fresh child would make the canonical form differ and defeat no-op elision.
func TestPatchUnit_KeepsTheTranslationElementWhenChangingIt(t *testing.T) {
	raw := unit(t, text(tr("en_US", "Save"), tr("nl_NL", "Opslaan")))
	out, _ := PatchUnit(raw, "en_US", "nl_NL", map[string]string{"Save": "Bewaren"}, ModeMerge)

	var doc bson.D
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	items, _ := lookup(findTexts(doc)[0].doc, "Items").(bson.A)
	for _, it := range items {
		d, ok := it.(bson.D)
		if !ok {
			continue
		}
		if l, _ := lookup(d, "LanguageCode").(string); l == "nl_NL" {
			if id, _ := lookup(d, "$ID").(string); id != "nl_NL-id" {
				t.Errorf("$ID = %q, want the stored nl_NL-id — replacing the element "+
					"instead of editing it defeats no-op elision", id)
			}
			return
		}
	}
	t.Error("nl_NL translation missing after the edit")
}

func TestLanguagesInUnit(t *testing.T) {
	raw := unit(t,
		text(tr("en_US", "Save"), tr("nl_NL", "Opslaan")),
		text(tr("en_US", "Cancel"), tr("de_DE", "Abbrechen")),
	)
	got := LanguagesInUnit(raw)
	want := []string{"de_DE", "en_US", "nl_NL"}
	if len(got) != len(want) {
		t.Fatalf("languages = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("languages = %v, want %v", got, want)
		}
	}
}

func TestPatchUnit_MalformedPassesThrough(t *testing.T) {
	bad := []byte{1, 2, 3}
	out, stats := PatchUnit(bad, "en_US", "nl_NL", map[string]string{"Save": "Opslaan"}, ModeReplace)
	if string(out) != string(bad) || stats.touched() {
		t.Error("malformed input was modified")
	}
	if CollectFromUnit(bad, "en_US") != nil {
		t.Error("malformed input yielded entries")
	}
}

// Mendix's storage reader requires $ID to be the FIRST property of every storage
// object. A translation written without one fails the build with
//
//	Expected '$ID' as the first property of a storage object, but got '$Type'
//
// which no assertion about the translation's VALUE catches — the first version
// of this package wrote 38 correct-looking translations and made the project
// unbuildable. It took running `mx check` on a patched project to find, so the
// shape is pinned here.
func TestPatchUnit_NewTranslationCarriesAnIDFirst(t *testing.T) {
	raw := unit(t, text(tr("en_US", "Save")))
	out, _ := PatchUnit(raw, "en_US", "nl_NL", map[string]string{"Save": "Opslaan"}, ModeMerge)

	var doc bson.D
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	items, _ := lookup(findTexts(doc)[0].doc, "Items").(bson.A)
	for _, it := range items {
		d, ok := it.(bson.D)
		if !ok {
			continue
		}
		if l, _ := lookup(d, "LanguageCode").(string); l != "nl_NL" {
			continue
		}
		if len(d) == 0 || d[0].Key != "$ID" {
			t.Fatalf("first property is %q, want $ID — Mendix refuses to load the "+
				"document otherwise, and the translation itself looks perfectly fine",
				firstKey(d))
		}
		bin, ok := d[0].Value.(bson.Binary)
		if !ok || len(bin.Data) != 16 {
			t.Errorf("$ID = %#v, want a 16-byte binary", d[0].Value)
		}
		return
	}
	t.Fatal("the new nl_NL translation is missing")
}

func firstKey(d bson.D) string {
	if len(d) == 0 {
		return "(none)"
	}
	return d[0].Key
}

// TestNewElementID_IsAUUIDInMendixByteOrder pins the *form* of a minted id, not
// just its presence. Mendix stores an element id as a .NET Guid — 16 bytes in
// mixed-endian order, so the RFC-4122 version nibble lands at byte 7 and the
// variant bits at byte 8. Measured on a stock 11.13.0 project: 44,002 of 44,002
// element ids are well-formed on that reading (version 4 or 5, variant 10b), as
// are all 1,650 `Texts$Translation` ids the marketplace modules ship. Raw random
// bytes satisfy it only 1 time in 64, and did: 1 of 27 written by
// `create translations`.
//
// Every other id in mxcli goes through types.GenerateID + types.UUIDToBlob and
// so has always matched; this package minted its own and was the exception.
func TestNewElementID_IsAUUIDInMendixByteOrder(t *testing.T) {
	// 200 draws makes a raw-random implementation fail with probability
	// 1-(1/64)^0 ≈ 1 — it cannot pass by luck.
	for i := 0; i < 200; i++ {
		b := newElementID()
		if len(b) != 16 {
			t.Fatalf("draw %d: len = %d, want 16", i, len(b))
		}
		if v := b[7] >> 4; v != 4 {
			t.Fatalf("draw %d: version nibble = %d, want 4 — %x is not a UUID in "+
				"Mendix's stored byte order, and Studio Pro has never been shown "+
				"one that is not", i, v, b)
		}
		if variant := b[8] >> 6; variant != 0b10 {
			t.Fatalf("draw %d: variant bits = %02b, want 10 — %x", i, variant, b)
		}
	}
}
