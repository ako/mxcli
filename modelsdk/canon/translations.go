// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"sort"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// CarryTranslations copies onto a rebuilt document the translations the rebuild
// could not express, taken from the stored document.
//
// # Why anything is lost
//
// A Mendix text is a `Texts$Text` holding one `Texts$Translation` child per
// language. MDL carries ONE string — `Title: 'My Account'` — so a rebuilt
// document has exactly one child, and every other language the stored document
// held is gone. Measured on 11.13.0: describing `Administration.MyAccount` and
// re-executing that description dropped "Mijn account" from the entire project,
// taking the project's nl_NL count from 17 to 16. mxbuild reports 0 errors
// before and after, because a model missing a translation is a valid model — so
// nothing downstream catches it.
//
// This is the same shape as the other carries in Reconcile: the rebuild is
// authoritative about what the statement said, and silent about everything the
// statement had no way to say.
//
// # How elements are paired
//
// Two strategies, and which one applies is decided by evidence rather than by
// preference.
//
// **By position, when the shape is provably unchanged.** Each text is addressed
// by its containment path ("Title", "Widgets/2/Caption"). If the two documents
// have the *same set of paths*, then no text was inserted, removed or moved, so
// path pairing is exact — and it is the only strategy that can carry a text
// whose source string the statement CHANGED, which is the common edit.
//
// Positional pairing is used only under that precondition, because a mis-pair
// here is not benign the way TransplantIDs' is: that function rewrites every
// reference along with the id, so a wrong pairing costs a worse diff and nothing
// else. Copying a translation onto the wrong text would inject a translation of
// some other string — silent, and plausible-looking. So if a single path differs,
// positional pairing is abandoned entirely rather than applied where it happens
// to fit.
//
// **By source string otherwise.** For each stored text, every (language, text)
// pair it carries keys that text's full translation set; a rebuilt text is
// looked up by the one pair it has. No project default language is needed —
// whichever language the rebuild wrote is the key. This is exact in practice:
// across a real project's 388 units, 34 carry translations and in NONE of them
// does one source string map to two different translations in one language.
// Where it is not exact, nothing is carried for that string.
//
// TransplantIDs' own element mapping is not reusable here: it deliberately does
// not re-marshal, patching fixed-width binaries in place, so it is unavailable to
// a pass that must change the document's length.
//
// # What it will not do
//
//   - **Overwrite a language the rebuild wrote.** The statement is the authority
//     for what it says.
//   - **Guess on an ambiguous source.** Two stored texts sharing a source string
//     but disagreeing on its translation are not resolvable this way, so that
//     string carries nothing. Guessing would silently move a translation from one
//     element to another, which is worse than leaving one behind.
//   - **Drop the other languages when the source changed.** Editing the English
//     in Studio Pro leaves the sibling translations in place, stale though they
//     become; deciding on the author's behalf that a translation is now wrong is
//     not this function's call. (`CREATE OR REPLACE TRANSLATIONS` is where a
//     stale translation is meant to be removed, deliberately and visibly.)
//
// Anything that cannot be read on either side returns the contents untouched,
// which is the behaviour that existed before this function.
func CarryTranslations(contents, stored []byte) []byte {
	if len(contents) == 0 || len(stored) == 0 {
		return contents
	}
	var storedDoc, newDoc bson.D
	if bson.Unmarshal(stored, &storedDoc) != nil {
		return contents
	}
	if bson.Unmarshal(contents, &newDoc) != nil {
		return contents
	}

	storedByPath := textsByPath(storedDoc)
	newByPath := textsByPath(newDoc)
	byPath := samePathSet(storedByPath, newByPath)

	sets := storedTranslationSets(storedDoc)
	if len(sets) == 0 && !byPath {
		return contents
	}

	changed := false
	for path, text := range newByPath {
		var want map[string]bson.D
		if byPath {
			want = textTranslationElems(storedByPath[path])
		} else {
			want = matchBySource(textTranslations(text), sets)
		}
		if len(want) == 0 {
			continue
		}
		if mergeText(text, want) {
			changed = true
		}
	}
	if !changed {
		return contents
	}
	out, err := bson.Marshal(newDoc)
	if err != nil {
		return contents
	}
	return out
}

// textsByPath addresses every Texts$Text in a document by its containment path.
// The bson.D values are the live sub-documents, so mutating one mutates the
// document it came from.
func textsByPath(doc bson.D) map[string]bson.D {
	out := map[string]bson.D{}
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch n := v.(type) {
		case bson.D:
			if ty, _ := docLookup(n, "$Type").(string); ty == "Texts$Text" {
				out[path] = n
				return
			}
			for _, e := range n {
				walk(e.Value, path+"/"+e.Key)
			}
		case bson.A:
			for i, e := range n {
				walk(e, path+"/"+strconv.Itoa(i))
			}
		}
	}
	walk(doc, "")
	return out
}

// samePathSet reports whether the two documents hold texts at exactly the same
// paths — the precondition that makes positional pairing exact.
func samePathSet(a, b map[string]bson.D) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	for p := range a {
		if _, ok := b[p]; !ok {
			return false
		}
	}
	return true
}

// matchBySource finds the stored translation set for a rebuilt text, by whichever
// (language, text) pair the rebuild wrote. Returns nil when nothing matches or
// the match is ambiguous.
func matchBySource(have map[string]string, sets map[string]*translationSet) map[string]bson.D {
	for lang, txt := range have {
		if s, ok := sets[lang+"\x00"+txt]; ok {
			if s.ambiguous {
				return nil
			}
			return s.byLang
		}
	}
	return nil
}

// ambiguous marks a key whose stored translation sets disagree.
type translationSet struct {
	byLang map[string]bson.D
	// text is the language→string view, used only to decide whether two stored
	// sets sharing a source key actually agree.
	text      map[string]string
	ambiguous bool
}

// storedTranslationSets indexes every stored text by each (language, text) pair
// it carries, so a rebuilt text can be found by whichever single language the
// rebuild happened to write.
func storedTranslationSets(doc bson.D) map[string]*translationSet {
	out := map[string]*translationSet{}
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case bson.D:
			if ty, _ := docLookup(n, "$Type").(string); ty == "Texts$Text" {
				byText := textTranslations(n)
				if len(byText) > 1 {
					set := &translationSet{byLang: textTranslationElems(n), text: byText}
					for lang, text := range byText {
						k := lang + "\x00" + text
						prev, seen := out[k]
						if !seen {
							out[k] = set
							continue
						}
						if !sameTranslations(prev.text, byText) {
							prev.ambiguous = true
						}
					}
				}
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

// mergeText appends the languages a rebuilt text is missing, reporting whether
// anything was added. A language the rebuild wrote is never overwritten: the
// statement is the authority for what it says.
func mergeText(text bson.D, want map[string]bson.D) bool {
	have := textTranslations(text)
	items, ok := docLookup(text, "Items").(bson.A)
	if !ok {
		return false
	}
	added := false
	for _, lang := range sortedLangs(want) {
		if _, exists := have[lang]; exists {
			continue
		}
		items = append(items, want[lang])
		added = true
	}
	if !added {
		return false
	}
	for i, e := range text {
		if e.Key == "Items" {
			text[i].Value = items
		}
	}
	return true
}

// sortedLangs keeps the appended order deterministic, so the same inputs produce
// the same bytes and no-op elision can fire.
func sortedLangs(m map[string]bson.D) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// textTranslationElems reads a Texts$Text's language→translation-element map.
// The whole sub-document is kept, not just the string: appending it verbatim
// carries its $ID too, so a carried translation keeps the identity it had on
// disk and the canonical form sees the same element it saw before. Appending a
// freshly built child without an $ID makes an otherwise identical document
// compare unequal, and no-op elision could never fire.
func textTranslationElems(text bson.D) map[string]bson.D {
	items, ok := docLookup(text, "Items").(bson.A)
	if !ok {
		return nil
	}
	out := map[string]bson.D{}
	for _, it := range items {
		d, ok := it.(bson.D)
		if !ok {
			continue
		}
		if lang, _ := docLookup(d, "LanguageCode").(string); lang != "" {
			out[lang] = d
		}
	}
	return out
}

// textTranslations reads a Texts$Text's language→text map.
func textTranslations(text bson.D) map[string]string {
	items, ok := docLookup(text, "Items").(bson.A)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for _, it := range items {
		d, ok := it.(bson.D)
		if !ok {
			continue
		}
		lang, _ := docLookup(d, "LanguageCode").(string)
		if lang == "" {
			continue
		}
		txt, _ := docLookup(d, "Text").(string)
		out[lang] = txt
	}
	return out
}

func sameTranslations(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func docLookup(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}
