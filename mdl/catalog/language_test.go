// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// languageOnlyReader answers GetProjectSettings and nothing else. The embedded
// interface is nil on purpose: any other call panics, which keeps the test
// honest about what defaultLanguage() is allowed to touch.
type languageOnlyReader struct {
	CatalogReader
	settings *model.ProjectSettings
	err      error
}

func (r languageOnlyReader) GetProjectSettings() (*model.ProjectSettings, error) {
	return r.settings, r.err
}

func TestBuilderDefaultLanguage_Issue1113(t *testing.T) {
	nl := &model.ProjectSettings{Language: &model.LanguageSettings{DefaultLanguageCode: "nl_NL"}}
	tests := []struct {
		name   string
		reader languageOnlyReader
		want   string
	}{
		{"project default is used", languageOnlyReader{settings: nl}, "nl_NL"},
		{"no settings falls back", languageOnlyReader{}, "en_US"},
		{"no language block falls back", languageOnlyReader{settings: &model.ProjectSettings{}}, "en_US"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &Builder{reader: tc.reader}
			if got := b.defaultLanguage(); got != tc.want {
				t.Errorf("defaultLanguage = %q, want %q", got, tc.want)
			}
		})
	}
}

// The catalog's caption column is filled from the same translations DESCRIBE
// reads, so the two must agree on which language wins. Before #1113 the column
// preferred en_US unconditionally, which disagreed with DESCRIBE on any
// multi-language project.
func TestPickTranslation_Issue1113(t *testing.T) {
	tests := []struct {
		name         string
		translations map[string]string
		preferred    string
		want         string
	}{
		{"only the project language is stored", map[string]string{"nl_NL": "Nieuw"}, "nl_NL", "Nieuw"},
		{"project language wins over en_US", map[string]string{"en_US": "New", "nl_NL": "Nieuw"}, "nl_NL", "Nieuw"},
		{"en_US is the first fallback", map[string]string{"en_US": "New", "de_DE": "Neu"}, "nl_NL", "New"},
		{"lowest language code is the last resort", map[string]string{"nl_NL": "Nieuw", "de_DE": "Neu"}, "pt_BR", "Neu"},
		{"empty translation is skipped", map[string]string{"de_DE": "", "nl_NL": "Nieuw"}, "pt_BR", "Nieuw"},
		{"no translations", nil, "nl_NL", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 20; i++ {
				if got := pickTranslation(tc.translations, tc.preferred); got != tc.want {
					t.Fatalf("pickTranslation = %q, want %q", got, tc.want)
				}
			}
		})
	}
}
