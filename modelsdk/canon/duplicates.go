// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"fmt"
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A unit whose document contains two elements with the same $ID cannot be
// OPENED. Studio Pro refuses the whole project, not the one document:
//
//	InvalidOperationException: An error occurred while saving the project:
//	Duplicate Guid in unit page 'FteCapTrack.PlanningContext_Matrix'.
//	Object types: Mendix.Modeler.Texts.Translation, Mendix.Modeler.Texts.Translation.
//
// "Guid" there is the element $ID — a Texts$Translation stores exactly four
// keys ($ID, $Type, LanguageCode, Text) and has no GUID property of its own, so
// the identity Mendix is complaining about is the 16-byte $ID. Verified against
// stored translations on 11.13.
//
// This is a WRITE-TIME guard with no root cause behind it, and that is
// deliberate. The reporter (ako/mxcli-captrack #2) lost about an hour to one of
// these and could not establish what produced it: two candidate explanations
// were proposed during the build, both were tested, and both were withdrawn.
// What was established is that the corruption is STICKY — once a unit carries
// duplicate ids, every later edit inherits them, so each subsequent write looks
// like the culprit and the second experiment measures the first one's damage.
//
// A guard is worth having anyway, and arguably worth more without a root cause
// than with one: it converts an unopenable project and an hour of archaeology
// into one message naming the unit, at the moment the bytes would have landed.
// It also breaks the stickiness, since the first bad write is refused rather
// than becoming the baseline for everything after it.
//
// It is intentionally NOT a repair. Deduplicating ids here would mean choosing
// which of two elements keeps its identity, and an $ID is a pointer target —
// every reference to it elsewhere in the document resolves by that value
// (ADR-0008), so silently re-pointing half of them is how a puzzling refusal
// becomes a quietly wrong model.

// Duplicate is one $ID held by more than one element in a unit, with the
// $Types of the elements holding it — mirroring what Studio Pro's own error
// reports, because that is the message the user will otherwise meet later.
type Duplicate struct {
	ID    string
	Types []string
}

// DuplicateElementIDs reports element $IDs that appear on more than one element
// in raw, in a stable order.
//
// It counts only $ID on a document that also carries a $Type: that is what makes
// a sub-document an ELEMENT. Pointers to other elements are primitive properties
// holding the same 16-byte shape under a different key (ParentPointer and
// friends), and a containment walk sees plenty of them — counting those would
// report a duplicate for every reference, which is the normal case.
//
// A document that cannot be unmarshalled yields no duplicates rather than an
// error: this runs on the write path, where refusing a write because the guard
// could not read the bytes would be a worse failure than the one it prevents.
func DuplicateElementIDs(raw []byte) []Duplicate {
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil
	}

	seen := map[string][]string{}
	var order []string
	var walk func(any)
	walk = func(v any) {
		if doc, ok := asDoc(v); ok {
			if id, ok := elementID(doc); ok && hasType(doc) {
				if _, known := seen[id]; !known {
					order = append(order, id)
				}
				seen[id] = append(seen[id], typeOf(doc))
			}
			for _, k := range sortedKeys(doc) {
				walk(doc[k])
			}
			return
		}
		if s, ok := asSlice(v); ok {
			for _, e := range s {
				walk(e)
			}
		}
	}
	walk(d)

	var out []Duplicate
	for _, id := range order {
		if types := seen[id]; len(types) > 1 {
			sorted := append([]string(nil), types...)
			sort.Strings(sorted)
			out = append(out, Duplicate{ID: id, Types: sorted})
		}
	}
	return out
}

// DuplicateElementIDError returns the error a write should fail with, or nil.
// unitLabel is whatever the caller can name the unit by — an id is enough, a
// qualified name is better.
func DuplicateElementIDError(unitLabel string, raw []byte) error {
	dups := DuplicateElementIDs(raw)
	if len(dups) == 0 {
		return nil
	}
	// Report at most a few: a document that has gone wrong this way usually has
	// gone wrong repeatedly, and the first one is the one worth reading.
	const maxReported = 3
	msg := fmt.Sprintf("refusing to write unit %s: %d element id(s) are used more than once, "+
		"which produces a project that cannot be opened "+
		"(Studio Pro reports \"Duplicate Guid in unit ...\" and refuses the whole project)",
		unitLabel, len(dups))
	for i, d := range dups {
		if i == maxReported {
			msg += fmt.Sprintf("\n  ... and %d more", len(dups)-maxReported)
			break
		}
		msg += fmt.Sprintf("\n  %s held by %v", d.ID, d.Types)
	}
	return fmt.Errorf("%s", msg)
}

func hasType(d map[string]any) bool {
	_, ok := d["$Type"].(string)
	return ok
}

func typeOf(d map[string]any) string {
	if t, ok := d["$Type"].(string); ok {
		return t
	}
	return "(untyped)"
}
