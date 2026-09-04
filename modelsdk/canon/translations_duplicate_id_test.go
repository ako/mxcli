// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// CapTrackV2 FINDINGS §30/§17 — carrying translations gave several elements ONE
// element id, and mxcli's own duplicate-GUID guard then refused every in-place
// edit of the document:
//
//	refusing to write unit …: 1 element id(s) are used more than once
//	  c540fbf0-… held by [Texts$Translation ×8]
//
// Eight translations of the literal '{1}' on one page shared an id. `GRANT VIEW
// ON PAGE …` failed, and so did `ALTER PAGE … INSERT` (§17, the same root
// cause); a full `CREATE OR REPLACE PAGE` still worked because it rewrites the
// unit rather than patching it. So the page looked correct and only UPDATES were
// blocked — which is why it took enabling a second language to surface at all.
//
// The mechanism is in the source-string pairing. `matchBySource` hands back the
// stored translation set for a source string, and mergeText appends those stored
// elements VERBATIM — deliberately, because keeping the stored $ID is what lets
// no-op elision fire. When two rebuilt texts share a source string (eight copies
// of '{1}' is entirely ordinary), both resolve to the SAME stored set, and both
// get the same element, id included.
//
// The positional branch cannot do this: each path maps to its own stored text.
// Only the by-source branch can hand one element to many.

// twoTextDoc builds a document with two Texts$Text elements at distinct paths,
// each carrying the languages given.
func twoTextDoc(t *testing.T, extraPath bool, first, second []bson.D) []byte {
	t.Helper()
	mk := func(id byte, items []bson.D) bson.D {
		arr := bson.A{int32(3)}
		for _, it := range items {
			arr = append(arr, it)
		}
		return bson.D{
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "$ID", Value: bin(id)},
			{Key: "Items", Value: arr},
		}
	}
	doc := bson.D{
		{Key: "$Type", Value: "Forms$Form"},
		{Key: "$ID", Value: bin(1)},
		{Key: "Name", Value: "Page"},
		{Key: "Title", Value: mk(10, first)},
		{Key: "Caption", Value: mk(11, second)},
	}
	if extraPath {
		// A third text makes the path SETS differ, which is what sends
		// CarryTranslations down the by-source branch instead of the positional
		// one. Without it the two documents pair by path and each text gets its
		// own stored element.
		doc = append(doc, bson.E{Key: "Footer", Value: mk(12, first)})
	}
	return marshal(t, doc)
}

// collectTranslationIDs returns every Texts$Translation $ID in the document, in
// document order, so duplicates are visible.
func collectTranslationIDs(t *testing.T, raw []byte) []string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch n := v.(type) {
		case bson.D:
			if ty, _ := docLookup(n, "$Type").(string); ty == "Texts$Translation" {
				if b, ok := docLookup(n, "$ID").(bson.Binary); ok {
					out = append(out, string(b.Data))
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

// The reported case: two texts with the SAME source string, carried from a
// stored document that has a translation for it. Both must end up with a
// translation, and the two must not share an element id.
func TestCarryTranslations_DoesNotReuseOneElementIDTwice(t *testing.T) {
	de := tr(20, "de_DE", "Übersetzt")
	en := tr(21, "en_US", "{1}")
	stored := twoTextDoc(t, true, []bson.D{en, de}, []bson.D{en, de})
	// The rebuild carries one language, and a different path set, so the carry
	// runs by source string.
	contents := twoTextDoc(t, false, []bson.D{tr(30, "en_US", "{1}")}, []bson.D{tr(31, "en_US", "{1}")})

	out := CarryTranslations(contents, stored)

	ids := collectTranslationIDs(t, out)
	seen := map[string]int{}
	for _, id := range ids {
		seen[id]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("element id %q is used %d times — mxcli's own duplicate-GUID guard "+
				"refuses this document, blocking every in-place edit of it (§30)", id, n)
		}
	}
	// CONTROL within the same test: the carry must still have happened. A fix
	// that stopped carrying anything would satisfy the assertion above and
	// silently reintroduce the translation loss this function exists to prevent.
	if len(ids) != 4 {
		t.Errorf("got %d translations, want 4 (two texts × en_US + carried de_DE): %v", len(ids), ids)
	}
}

// CONTROL: one text, one stored set — the ordinary case — must still keep the
// STORED id. That is what makes an unchanged document compare equal, so no-op
// elision can fire; minting a fresh id for every carry would write on every run.
func TestCarryTranslations_SingleUseKeepsTheStoredID(t *testing.T) {
	de := tr(20, "de_DE", "Übersetzt")
	stored := twoTextDoc(t, true,
		[]bson.D{tr(21, "en_US", "one"), de},
		[]bson.D{tr(22, "en_US", "two"), tr(23, "de_DE", "Zwei")})
	contents := twoTextDoc(t, false,
		[]bson.D{tr(30, "en_US", "one")},
		[]bson.D{tr(31, "en_US", "two")})

	ids := collectTranslationIDs(t, CarryTranslations(contents, stored))

	want := string(bin(20).Data)
	found := false
	for _, id := range ids {
		if id == want {
			found = true
		}
	}
	if !found {
		t.Errorf("the carried translation did not keep its stored id; "+
			"an unchanged document then never equals itself and every run writes. ids=%v", ids)
	}
}

// The same inputs must produce the same bytes, or elision cannot fire and the
// document churns on every run. Map iteration order decides which text is
// visited first, so this is not automatic.
func TestCarryTranslations_IsDeterministic(t *testing.T) {
	de := tr(20, "de_DE", "Übersetzt")
	en := tr(21, "en_US", "{1}")
	stored := twoTextDoc(t, true, []bson.D{en, de}, []bson.D{en, de})

	var first []byte
	for i := 0; i < 20; i++ {
		contents := twoTextDoc(t, false,
			[]bson.D{tr(30, "en_US", "{1}")}, []bson.D{tr(31, "en_US", "{1}")})
		out := CarryTranslations(contents, stored)
		if first == nil {
			first = out
			continue
		}
		if string(out) != string(first) {
			t.Fatalf("run %d produced different bytes — the document churns on every write", i)
		}
	}
}
