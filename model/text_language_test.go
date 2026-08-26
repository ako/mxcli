// SPDX-License-Identifier: Apache-2.0

package model

import "testing"

func TestAuthoringLanguageDefaultsToEnUS(t *testing.T) {
	// The default is load-bearing: every writer leaf reads it, so an unset
	// process must behave exactly as mxcli did before mendixlabs/mxcli#970.
	if got := AuthoringLanguage(); got != DefaultTextLanguage {
		t.Fatalf("AuthoringLanguage() = %q on a fresh process, want %q", got, DefaultTextLanguage)
	}
}

func TestSetAuthoringLanguage(t *testing.T) {
	orig := AuthoringLanguage()
	t.Cleanup(func() { SetAuthoringLanguage(orig) })

	SetAuthoringLanguage("nl_NL")
	if got := AuthoringLanguage(); got != "nl_NL" {
		t.Errorf("after set = %q, want nl_NL", got)
	}

	// An empty code must NOT store an empty LanguageCode — no Mendix document
	// has one and Studio Pro would not resolve it. It resets to the default.
	SetAuthoringLanguage("")
	if got := AuthoringLanguage(); got != DefaultTextLanguage {
		t.Errorf("after set(\"\") = %q, want %q", got, DefaultTextLanguage)
	}
}
