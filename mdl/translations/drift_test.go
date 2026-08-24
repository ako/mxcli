// SPDX-License-Identifier: Apache-2.0

package translations

import (
	"strings"
	"testing"
)

// The scenario the whole design has to answer for: a source string edited in
// Studio Pro after the file was written. The key matches nothing, and skipping
// in silence would leave the translation attached to a string that no longer
// exists while the file quietly stopped describing the project.
func TestSuggestDrift_NamesTheSourceTheKeyBecame(t *testing.T) {
	dict := Dictionary{"Save": "Opslaan"}
	entries := []Entry{
		{Source: "Store", Targets: map[string]string{"nl_NL": "Opslaan"}},
		{Source: "Cancel", Targets: map[string]string{"nl_NL": "Annuleren"}},
	}

	got := SuggestDrift([]string{"Save"}, dict, entries, "nl_NL")
	if len(got) != 1 {
		t.Fatalf("got %d drifts, want 1", len(got))
	}
	if got[0].Now != "Store" {
		t.Errorf("Now = %q, want Store — the text carrying 'Opslaan' is where the "+
			"source went, and that is the only evidence available", got[0].Now)
	}
	if msg := got[0].Explain("nl_NL"); !strings.Contains(msg, "Store") {
		t.Errorf("the advice does not name the new source:\n%s", msg)
	}
}

// Two sources sharing a translation cannot be told apart, so no suggestion is
// made. Measured on a real project this is ~3% of targets and they are short
// generic words ("Knop" ← 6 sources), which is exactly where a confident wrong
// answer would be most misleading.
func TestSuggestDrift_SaysNothingWhenTheTranslationIsAmbiguous(t *testing.T) {
	dict := Dictionary{"Button": "Knop"}
	entries := []Entry{
		{Source: "Btn", Targets: map[string]string{"nl_NL": "Knop"}},
		{Source: "Push", Targets: map[string]string{"nl_NL": "Knop"}},
	}

	got := SuggestDrift([]string{"Button"}, dict, entries, "nl_NL")
	if got[0].Now != "" {
		t.Errorf("Now = %q, want no suggestion — two sources carry this translation "+
			"and naming one would be a confident guess", got[0].Now)
	}
	if msg := got[0].Explain("nl_NL"); !strings.Contains(msg, "no telling") {
		t.Errorf("the advice should say it does not know:\n%s", msg)
	}
}

// A key with no translation in the file (an untranslated entry that no longer
// matches) has nothing to correlate by.
func TestSuggestDrift_UntranslatedKeyHasNothingToCorrelate(t *testing.T) {
	got := SuggestDrift([]string{"Gone"}, Dictionary{"Gone": ""}, nil, "nl_NL")
	if got[0].Now != "" {
		t.Errorf("Now = %q, want empty", got[0].Now)
	}
}
