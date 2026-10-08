// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Pairing by the NAMED ELEMENT that owns a text (ako/mxcli#705 item 1).
//
// The two strategies CarryTranslations had both fail on a real Studio Pro page:
//
//   - Positional pairing needs the whole document's text paths to be unchanged,
//     and a page rebuild almost never meets that — one DataGrid2 rebuilt from
//     its widget template reorders its properties and every path under it moves.
//   - Source pairing keys on (language, text), which cannot tell apart two texts
//     with the same source. The commonest source of all is the empty string:
//     Studio Pro stores en_US "" on every empty caption, counter message and
//     validation message, each with its own translations or none, so the key
//     (en_US, "") is ambiguous on any real page and nothing is carried. A rebuilt
//     empty text has no translation at all, so it has no key to look up either.
//
// Measured on the Blank template: describe -> exec of ShareFeedback lost a
// caption's nl_NL "Knop" and the en_US "" of a counter message, and
// Account_Overview lost nl_NL "Gebruikers" on a label whose English source,
// "Account Overview", is shared with the page title.
//
// A widget's Name is unique within its document (Studio Pro enforces it), so a
// text addressed as (element type, element name, path from that element) is
// exact wherever the path from the element contains no list index. Where it does
// — a pluggable widget's Properties list, reordered by a rebuild — this pairing
// is not used and the source pairing still applies.

func widgetText(items ...bson.D) bson.D {
	arr := bson.A{int32(3)}
	for _, it := range items {
		arr = append(arr, it)
	}
	return bson.D{{Key: "$Type", Value: "Texts$Text"}, {Key: "$ID", Value: bin(byte(90 + len(items)))}, {Key: "Items", Value: arr}}
}

func button(id byte, name string, caption bson.D) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Forms$ActionButton"},
		{Key: "$ID", Value: bin(id)},
		{Key: "Name", Value: name},
		{Key: "CaptionTemplate", Value: bson.D{
			{Key: "$Type", Value: "Forms$ClientTemplate"},
			{Key: "$ID", Value: bin(id + 1)},
			{Key: "Template", Value: caption},
		}},
	}
}

func pageOf(t *testing.T, widgets ...bson.D) []byte {
	t.Helper()
	arr := bson.A{int32(2)}
	for _, w := range widgets {
		arr = append(arr, w)
	}
	return marshal(t, bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "$ID", Value: bin(1)},
		{Key: "Name", Value: "P"},
		{Key: "Widgets", Value: arr},
	})
}

// captionOf returns lang→text for the named button's caption.
func captionOf(t *testing.T, raw []byte, name string) map[string]string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, w := range docLookup(doc, "Widgets").(bson.A)[1:] {
		wd := w.(bson.D)
		if docLookup(wd, "Name") != name {
			continue
		}
		tmpl := docLookup(docLookup(wd, "CaptionTemplate").(bson.D), "Template").(bson.D)
		return textTranslations(tmpl)
	}
	t.Fatalf("no button %q", name)
	return nil
}

// The shape differs (a button was added), so positional pairing is off; the
// stored caption is en_US "" + nl_NL "Knop", and the rebuild wrote an empty
// text. Only the owning element can pair them.
func TestCarryTranslations_PairsAnEmptySourceByItsNamedElement(t *testing.T) {
	stored := pageOf(t,
		button(10, "actionButton1", widgetText(tr(12, "en_US", ""), tr(13, "nl_NL", "Knop"))),
		button(20, "actionButton2", widgetText(tr(22, "en_US", ""))),
	)
	rebuilt := pageOf(t,
		button(30, "actionButton1", widgetText()),
		button(40, "actionButton2", widgetText()),
		button(50, "added", widgetText(tr(52, "en_US", "New"))),
	)

	out := CarryTranslations(rebuilt, stored)
	if got := captionOf(t, out, "actionButton1"); got["en_US"] != "" || got["nl_NL"] != "Knop" || len(got) != 2 {
		t.Errorf("actionButton1 caption = %v, want en_US \"\" + nl_NL Knop", got)
	}
	if got := captionOf(t, out, "actionButton2"); len(got) != 1 || got["en_US"] != "" {
		t.Errorf("actionButton2 caption = %v, want the stored en_US \"\"", got)
	}
	if got := captionOf(t, out, "added"); len(got) != 1 || got["en_US"] != "New" {
		t.Errorf("a button the stored page never had = %v, want only what the rebuild wrote", got)
	}
}

// Two elements share a source string but not its translation: the source
// pairing refuses (see TestCarryTranslations_RefusesToGuessOnAnAmbiguousSource),
// and the owning element resolves it exactly.
func TestCarryTranslations_ResolvesAnAmbiguousSourceByItsNamedElement(t *testing.T) {
	stored := pageOf(t,
		button(10, "cancelA", widgetText(tr(12, "en_US", "Cancel"), tr(13, "nl_NL", "Annuleren"))),
		button(20, "cancelB", widgetText(tr(22, "en_US", "Cancel"), tr(23, "nl_NL", "Annuleer"))),
	)
	rebuilt := pageOf(t,
		button(30, "cancelA", widgetText(tr(32, "en_US", "Cancel"))),
		button(40, "cancelB", widgetText(tr(42, "en_US", "Cancel"))),
		button(50, "added", widgetText(tr(52, "en_US", "New"))),
	)

	out := CarryTranslations(rebuilt, stored)
	if got := captionOf(t, out, "cancelA")["nl_NL"]; got != "Annuleren" {
		t.Errorf("cancelA nl_NL = %q, want Annuleren", got)
	}
	if got := captionOf(t, out, "cancelB")["nl_NL"]; got != "Annuleer" {
		t.Errorf("cancelB nl_NL = %q, want Annuleer", got)
	}
}

// The statement is still the authority: an element whose authored caption
// changed keeps its other languages (the positional rule) but its own language
// is what the statement wrote.
func TestCarryTranslations_NamedElementNeverOverwritesTheRebuild(t *testing.T) {
	stored := pageOf(t,
		button(10, "save", widgetText(tr(12, "en_US", "Save"), tr(13, "nl_NL", "Opslaan"))),
	)
	rebuilt := pageOf(t,
		button(30, "save", widgetText(tr(32, "en_US", "Store"))),
		button(50, "added", widgetText(tr(52, "en_US", "New"))),
	)
	got := captionOf(t, CarryTranslations(rebuilt, stored), "save")
	if got["en_US"] != "Store" || got["nl_NL"] != "Opslaan" {
		t.Errorf("save caption = %v, want the authored en_US Store and the stored nl_NL", got)
	}
}

// A list index between the element and its text is not a stable address — a
// rebuilt pluggable widget reorders its Properties — so the element pairing
// must not be used there. Here it would pair the two properties crosswise.
func TestCarryTranslations_NamedElementPairingSkipsIndexedPaths(t *testing.T) {
	prop := func(id byte, text bson.D) bson.D {
		return bson.D{{Key: "$Type", Value: "CustomWidgets$WidgetProperty"}, {Key: "$ID", Value: bin(id)}, {Key: "Value", Value: text}}
	}
	grid := func(id byte, props ...bson.D) bson.D {
		arr := bson.A{int32(2)}
		for _, p := range props {
			arr = append(arr, p)
		}
		return bson.D{{Key: "$Type", Value: "CustomWidgets$CustomWidget"}, {Key: "$ID", Value: bin(id)}, {Key: "Name", Value: "grid"}, {Key: "Properties", Value: arr}}
	}
	stored := pageOf(t, grid(10,
		prop(11, widgetText(tr(12, "en_US", "Export"), tr(13, "nl_NL", "Exporteren"))),
		prop(14, widgetText(tr(15, "en_US", "Cancel"), tr(16, "nl_NL", "Annuleren"))),
	))
	// Reordered, as a template rebuild does.
	rebuilt := pageOf(t, grid(30,
		prop(31, widgetText(tr(32, "en_US", "Cancel"))),
		prop(34, widgetText(tr(35, "en_US", "Export"))),
	), button(50, "added", widgetText()))

	out := CarryTranslations(rebuilt, stored)
	var doc bson.D
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	g := docLookup(doc, "Widgets").(bson.A)[1].(bson.D)
	for _, p := range docLookup(g, "Properties").(bson.A)[1:] {
		tt := textTranslations(docLookup(p.(bson.D), "Value").(bson.D))
		want := map[string]string{"Cancel": "Annuleren", "Export": "Exporteren"}[tt["en_US"]]
		if tt["nl_NL"] != want {
			t.Errorf("%q got nl_NL %q, want %q — paired by list position across a reorder", tt["en_US"], tt["nl_NL"], want)
		}
	}
}

// mendixlabs/mxcli#1182: "restating the snippet writes English correctly but
// drops the other languages". The restatement changes the English AND the shape
// (a widget is inserted ahead of the button), which rules out both older
// pairings at once: the text paths moved, so positional pairing is off, and the
// new source "Save all changes" matches no stored (language, text) pair, so
// source pairing finds nothing. Only the owning button can carry nl_NL — and
// the stored order (nl_NL listed before the default language) must not matter.
func TestCarryTranslations_ChangedSourceAndShapeKeepTheOtherLanguages(t *testing.T) {
	stored := pageOf(t,
		button(10, "btnSave", widgetText(tr(12, "nl_NL", "Opslaan"), tr(13, "en_US", "Save"))),
	)
	rebuilt := pageOf(t,
		button(30, "intro", widgetText(tr(32, "en_US", "Edit below"))),
		button(40, "btnSave", widgetText(tr(42, "en_US", "Save all changes"))),
	)

	out := CarryTranslations(rebuilt, stored)
	if got := captionOf(t, out, "btnSave"); got["en_US"] != "Save all changes" || got["nl_NL"] != "Opslaan" || len(got) != 2 {
		t.Errorf("btnSave caption = %v, want the restated en_US plus the stored nl_NL Opslaan", got)
	}
	if got := captionOf(t, out, "intro"); len(got) != 1 || got["en_US"] != "Edit below" {
		t.Errorf("a widget the stored snippet never had = %v, want only what the rebuild wrote", got)
	}
}
