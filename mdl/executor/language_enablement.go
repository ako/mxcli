// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"
)

// languageIsEnabled reports whether the project has enabled a language — i.e.
// whether the build will emit anything for it.
//
// An ENABLED language is not the same thing as one that has translations. A stock
// 11.13.0 app enables exactly one (en_US) while its marketplace modules ship
// translations in nine (nl_NL, de_DE, es_ES, pt_PT, tr_TR, fr_FR, hi_IN, ar_DZ),
// so Mendix clearly tolerates translations for languages a project has not
// enabled — it just never builds them. Measured: a page caption translated into
// de_DE on a project that enables only en_US builds at 0 errors and the German
// string appears NOWHERE in deployment/; no translations_de_DE.properties is
// emitted and "de_DE" does not occur in the built model at all.
//
// Unknown is deliberately NOT disabled: with no settings, or a settings part that
// carries no Languages, the honest answer is "cannot tell", and a false warning
// about a language that is in fact enabled teaches people to ignore the real one.
func languageIsEnabled(ls *model.LanguageSettings, code string) bool {
	if ls == nil || len(ls.Languages) == 0 {
		return true // not known — never guess
	}
	for _, l := range ls.Languages {
		if strings.EqualFold(l.Code, code) {
			return true
		}
	}
	return false
}

// unenabledLanguageWarning returns the text to print when a script writes
// translations for a language the project has not enabled, or "" when there is
// nothing to warn about. It names what was written, what the project actually
// enables, and where to change that — mxcli cannot enable a language itself,
// because `alter settings LANGUAGE` only carries DefaultLanguageCode today.
func unenabledLanguageWarning(ls *model.LanguageSettings, code string) string {
	if languageIsEnabled(ls, code) {
		return ""
	}
	enabled := make([]string, 0, len(ls.Languages))
	for _, l := range ls.Languages {
		enabled = append(enabled, l.Code)
	}
	return fmt.Sprintf(
		"\nWarning: %s is not one of this project's enabled languages (%s).\n"+
			"The translations are stored in the model and pass every check, but the build\n"+
			"emits nothing for a language the project has not enabled — measured on 11.13.0:\n"+
			"no translations_%s.properties is produced and the strings reach no page.\n"+
			"Enable it in Studio Pro under Project > Settings > Languages; mxcli cannot yet\n"+
			"(`alter settings LANGUAGE` carries DefaultLanguageCode only).\n",
		code, strings.Join(enabled, ", "), code)
}

// projectLanguageSettings reads the project's language settings, or nil when they
// cannot be read — which callers must treat as "unknown", not as "none enabled".
func projectLanguageSettings(ctx *ExecContext) *model.LanguageSettings {
	if ctx == nil || ctx.Backend == nil {
		return nil
	}
	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil || ps == nil {
		return nil
	}
	return ps.Language
}

// enabledLanguageCodes lists the codes a project has enabled, in stored order.
func enabledLanguageCodes(ls *model.LanguageSettings) []string {
	if ls == nil {
		return nil
	}
	codes := make([]string, 0, len(ls.Languages))
	for _, l := range ls.Languages {
		if l.Code != "" {
			codes = append(codes, l.Code)
		}
	}
	return codes
}
