// SPDX-License-Identifier: Apache-2.0

// A project's ENABLED languages decide what the build emits. A translation for a
// language the project has not enabled is stored in the model, survives every
// check, and is then silently discarded — so the feature must say so.
//
// Measured on mxbuild 11.13.0, blank app (enables en_US only): a page captioned
// "Zebra Widgets" with a de_DE translation "Zebrastreifen" builds at 0 errors, and
// the German string appears NOWHERE in deployment/ — no translations_de_DE.
// properties is emitted and "de_DE" does not occur in the built model at all. The
// control is the English caption, which appears in three places.
//
// A stock app makes this easy to hit by accident: it enables ONE language while
// its marketplace modules ship translations in NINE.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

func langSettings(codes ...string) *model.LanguageSettings {
	ls := &model.LanguageSettings{DefaultLanguageCode: codes[0]}
	for _, c := range codes {
		ls.Languages = append(ls.Languages, model.Language{Code: c})
	}
	return ls
}

func TestLanguageIsEnabled(t *testing.T) {
	ls := langSettings("en_US", "nl_NL")

	for _, c := range []string{"en_US", "nl_NL", "NL_nl"} {
		if !languageIsEnabled(ls, c) {
			t.Errorf("%s reported as not enabled — codes compare case-insensitively", c)
		}
	}
	if languageIsEnabled(ls, "de_DE") {
		t.Error("de_DE reported as enabled; the build would discard every translation written for it")
	}
}

// Unknown is not the same as disabled: with no settings to consult, warning would
// be a guess, and a false warning about a language that IS enabled trains people
// to ignore the real one.
func TestLanguageIsEnabled_NoSettingsNeverWarns(t *testing.T) {
	if !languageIsEnabled(nil, "de_DE") {
		t.Error("with no language settings the code must not claim a language is disabled")
	}
	if !languageIsEnabled(&model.LanguageSettings{DefaultLanguageCode: "en_US"}, "de_DE") {
		t.Error("an empty Languages list is 'not known', not 'nothing is enabled'")
	}
}

func TestUnenabledLanguageWarning_NamesTheConsequenceAndTheFix(t *testing.T) {
	w := unenabledLanguageWarning(langSettings("en_US"), "de_DE")
	if w == "" {
		t.Fatal("no warning for a language the project has not enabled")
	}
	for _, want := range []string{"de_DE", "en_US", "Studio Pro"} {
		if !strings.Contains(w, want) {
			t.Errorf("warning does not mention %q — it must name the language, what IS\n"+
				"enabled, and where to enable it:\n%s", want, w)
		}
	}
	if unenabledLanguageWarning(langSettings("en_US", "de_DE"), "de_DE") != "" {
		t.Error("warned about an enabled language")
	}
}
