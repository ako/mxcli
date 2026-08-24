// SPDX-License-Identifier: Apache-2.0

package translations

import (
	"fmt"
	"sort"
)

// Drift is a dictionary key that matched nothing, with the source string it has
// probably become.
type Drift struct {
	// Key is the source string as the file spells it.
	Key string
	// Now is the source string that carries the translation the file assigns to
	// Key — empty when nothing correlates.
	Now string
	// Translation is the value the file gives Key, which is what was matched.
	Translation string
}

// SuggestDrift explains dictionary keys that matched nothing.
//
// A dictionary keyed on the source string cannot see the source move. Edit
// "Save" to "Store" in Studio Pro and the file's key matches nothing — so
// skipping in silence would leave the Dutch translating a string that no longer
// exists, while DESCRIBE went on emitting the new source paired with the old
// translation as though that were a fact.
//
// The evidence is available: the key matches nothing AND some text carries the
// very translation the file assigns to it. Correlating backwards by the
// translation identifies the source it moved to. Measured on a real project this
// is nearly always unambiguous — 209 distinct (source, target) pairs across 191
// distinct targets, with only 6 targets (3%) shared by more than one source, and
// those are short generic words. Where it IS ambiguous, Now is left empty rather
// than guessed at.
//
// The suggestion is never applied. `Save` → `Store` keeps a translation valid;
// `Save` → `Delete` does not, and only a person or a model reading both can tell
// which happened.
func SuggestDrift(unmatched []string, dict Dictionary, entries []Entry, lang string) []Drift {
	// translation → the source strings that currently carry it
	carriers := map[string][]string{}
	for _, e := range entries {
		if t := e.Targets[lang]; t != "" {
			carriers[t] = append(carriers[t], e.Source)
		}
	}
	var out []Drift
	for _, key := range unmatched {
		d := Drift{Key: key, Translation: dict[key]}
		if d.Translation != "" {
			if srcs := carriers[d.Translation]; len(srcs) == 1 {
				d.Now = srcs[0]
			}
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Explain renders one drift as the advice to print. It names the correlated
// source when there is exactly one, and says plainly that it does not know when
// there is not.
func (d Drift) Explain(lang string) string {
	if d.Now != "" {
		return fmt.Sprintf(
			"  %q as %q\n"+
				"      No text has %q as its source. A text now reads %q and carries the\n"+
				"      %s %q — the source was probably edited. Change the file to:\n"+
				"        %q as %q\n"+
				"      and check the translation still fits.",
			d.Key, d.Translation, d.Key, d.Now, lang, d.Translation, d.Now, d.Translation)
	}
	return fmt.Sprintf(
		"  %q as %q\n"+
			"      No text has %q as its source, and nothing carries this %s translation,\n"+
			"      so there is no telling where it went. The text may have been deleted, or\n"+
			"      the key may be a typo.",
		d.Key, d.Translation, d.Key, lang)
}
