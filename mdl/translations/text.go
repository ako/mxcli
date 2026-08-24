// SPDX-License-Identifier: Apache-2.0

// Package translations reads and writes the translatable text of a Mendix
// project.
//
// A Mendix text is a `Texts$Text` holding one `Texts$Translation` child per
// language:
//
//	{ "$Type": "Texts$Text", "Items": [ 3,
//	    { "$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": "Save" },
//	    { "$Type": "Texts$Translation", "LanguageCode": "nl_NL", "Text": "Opslaan" } ] }
//
// It is a leaf *value*, never a document of its own — it appears wherever a
// caption lives: page titles, widget captions, enum captions, log and validation
// messages, workflow task names, menu items. That is what makes this package
// type-agnostic: one traversal covers every document type mxcli knows and every
// one it does not.
//
// Everything here operates on a unit's raw BSON, deliberately. Going through the
// document rebuild path would put translations back in reach of the very code
// that used to drop them, and would make writing a translation depend on mxcli
// being able to round-trip whatever document it happened to live in.
package translations

import (
	"crypto/rand"
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The leading element of a Texts$Text `Items` array is a version marker, not a
// translation. Observed as 3 on all 3299 texts of a real 11.13.0 project, and it
// is the value modelsdk/widgets writes.
const itemsMarker = int32(3)

// Entry is one translatable text as it stands in the project: the string in the
// source language, and what each other language says.
type Entry struct {
	Source  string
	Targets map[string]string
}

// Text is a Texts$Text found in a document, with the live sub-document so a
// caller can mutate it in place.
type Text struct {
	doc    bson.D
	byLang map[string]string
}

// Source returns the text in the given language, or "" when it has none.
func (t Text) Source(lang string) string { return t.byLang[lang] }

// Languages returns every language this text carries, sorted.
func (t Text) Languages() []string {
	out := make([]string, 0, len(t.byLang))
	for l := range t.byLang {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// findTexts returns every Texts$Text in a decoded document. The bson.D values
// are live, so mutating one mutates the document it came from.
func findTexts(doc bson.D) []Text {
	var out []Text
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case bson.D:
			if ty, _ := lookup(n, "$Type").(string); ty == "Texts$Text" {
				out = append(out, Text{doc: n, byLang: translationsOf(n)})
				return
			}
			for _, e := range n {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range n {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}

func translationsOf(text bson.D) map[string]string {
	items, ok := lookup(text, "Items").(bson.A)
	if !ok {
		return map[string]string{}
	}
	out := make(map[string]string, len(items))
	for _, it := range items {
		d, ok := it.(bson.D)
		if !ok {
			continue // the leading version marker
		}
		if lang, _ := lookup(d, "LanguageCode").(string); lang != "" {
			txt, _ := lookup(d, "Text").(string)
			out[lang] = txt
		}
	}
	return out
}

func lookup(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// setTranslation adds or replaces one language on a text, reporting whether the
// document changed. An existing child's Text is edited in place so its $ID
// survives — a fresh child would make the canonical form differ and defeat
// no-op elision (ADR-0008).
func (t Text) setTranslation(lang, text string) bool {
	if t.byLang[lang] == text {
		return false
	}
	items, ok := lookup(t.doc, "Items").(bson.A)
	if !ok {
		items = bson.A{itemsMarker}
	}
	for i, it := range items {
		d, ok := it.(bson.D)
		if !ok {
			continue
		}
		if l, _ := lookup(d, "LanguageCode").(string); l == lang {
			for j, f := range d {
				if f.Key == "Text" {
					d[j].Value = text
				}
			}
			items[i] = d
			t.byLang[lang] = text
			setItems(t.doc, items)
			return true
		}
	}
	items = append(items, newTranslation(lang, text))
	t.byLang[lang] = text
	setItems(t.doc, items)
	return true
}

// removeTranslation drops one language from a text, reporting whether the
// document changed.
func (t Text) removeTranslation(lang string) bool {
	if _, ok := t.byLang[lang]; !ok {
		return false
	}
	items, ok := lookup(t.doc, "Items").(bson.A)
	if !ok {
		return false
	}
	out := make(bson.A, 0, len(items))
	for _, it := range items {
		if d, ok := it.(bson.D); ok {
			if l, _ := lookup(d, "LanguageCode").(string); l == lang {
				continue
			}
		}
		out = append(out, it)
	}
	delete(t.byLang, lang)
	setItems(t.doc, out)
	return true
}

func setItems(text bson.D, items bson.A) {
	for i, e := range text {
		if e.Key == "Items" {
			text[i].Value = items
			return
		}
	}
}

// newTranslation builds a Texts$Translation element.
//
// $ID comes FIRST and is a real 16-byte binary. Mendix's storage reader requires
// it: a translation written without one fails the build with "Expected '$ID' as
// the first property of a storage object, but got '$Type'", which no unit test
// on the BSON shape catches — it took running `mx check` on a patched project.
//
// The id is fresh because the element is new. Slice 1's CarryTranslations is the
// other case: it re-attaches a translation that already existed, so it appends
// the stored element verbatim and keeps the identity it had on disk.
func newTranslation(lang, text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bson.Binary{Subtype: 0, Data: newElementID()}},
		{Key: "$Type", Value: "Texts$Translation"},
		{Key: "LanguageCode", Value: lang},
		{Key: "Text", Value: text},
	}
}

func newElementID() []byte {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail in practice; a zero id is still a valid
		// 16-byte binary, and the write is rejected downstream if it collides.
		return make([]byte, 16)
	}
	return b
}
