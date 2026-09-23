// SPDX-License-Identifier: Apache-2.0

package catalog

import "sort"

// defaultLanguage returns the project's DefaultLanguageCode, resolved once per
// build. Mendix has no language-neutral text: every Texts$Translation carries a
// LanguageCode, so a caption stored by a Dutch project exists only under
// "nl_NL". A catalog column filled by asking for "en_US" is therefore empty —
// or, worse, filled from whichever other language sorts first — on every
// project whose default is not en_US (mendixlabs/mxcli#1113).
func (b *Builder) defaultLanguage() string {
	if b.defaultLangLoaded {
		return b.defaultLang
	}
	b.defaultLang = catalogFallbackLanguage
	if ps, err := b.reader.GetProjectSettings(); err == nil &&
		ps != nil && ps.Language != nil && ps.Language.DefaultLanguageCode != "" {
		b.defaultLang = ps.Language.DefaultLanguageCode
	}
	b.defaultLangLoaded = true
	return b.defaultLang
}

const catalogFallbackLanguage = "en_US"

// pickTranslation mirrors the executor's pickTextTranslation: the preferred
// language, else en_US, else the non-empty translation with the lowest language
// code. The last step is sorted rather than a bare map range because a catalog
// row that varies between refreshes is indistinguishable from a model change.
func pickTranslation(translations map[string]string, preferredLang string) string {
	if len(translations) == 0 {
		return ""
	}
	if preferredLang != "" {
		if v := translations[preferredLang]; v != "" {
			return v
		}
	}
	if v := translations[catalogFallbackLanguage]; v != "" {
		return v
	}
	langs := make([]string, 0, len(translations))
	for lang := range translations {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	for _, lang := range langs {
		if v := translations[lang]; v != "" {
			return v
		}
	}
	return ""
}
