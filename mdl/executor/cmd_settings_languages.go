// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/translations"
	"github.com/mendixlabs/mxcli/model"
)

// languageCodeRe matches Mendix's language code form: an ISO 639 language and an
// ISO 3166 region, e.g. `en_US`, `ar_SD`, `pt_PT`. Studio Pro derives the display
// name from this code — "Arabic, Sudan" is not stored anywhere in the model, so
// the code is the whole identity of an enabled language.
var languageCodeRe = regexp.MustCompile(`^[a-z]{2,3}_[A-Z]{2}$`)

// alterSettingsLanguageAdd enables a language: appends a Texts$Language to
// Settings$LanguageSettings.Languages.
//
// The shape is pinned against a Studio Pro-authored reference (11.13.0, added via
// App Settings ▸ Languages ▸ Add). Every field of the element:
//
//	{ $Type: "Texts$Language", CheckCompleteness: false, Code: "ar_SD",
//	  CustomDateFormat: "", CustomDateTimeFormat: "", CustomTimeFormat: "" }
//
// Two things that reference settles and a guess would not. Studio Pro APPENDS
// rather than sorting, so the stored order is the order languages were enabled.
// And `CheckCompleteness` is stored **false even for the default language**,
// whose row in the Languages table reads "Yes" — the dialog explains why ("the
// default language is always checked"), so that Yes is computed for display and
// writing true to match it would produce a document Studio Pro never writes.
func alterSettingsLanguageAdd(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	if ps.Language == nil {
		return mdlerrors.NewNotFound("settings section", "language")
	}
	code := stmt.LanguageCode
	if err := validateLanguageCodeForm(code); err != nil {
		return err
	}
	for _, l := range ps.Language.Languages {
		if strings.EqualFold(l.Code, code) {
			return mdlerrors.NewValidationf(
				"language %q is already enabled in this project — use `alter settings LANGUAGE remove '%s'` to disable it, "+
					"or `describe settings` to see the whole list", l.Code, l.Code)
		}
	}

	lang := model.Language{Code: code}
	for key, val := range stmt.Properties {
		valStr := settingsValueToString(val)
		switch key {
		case "CheckCompleteness":
			v, err := settingsBool(key, valStr)
			if err != nil {
				return err
			}
			lang.CheckCompleteness = v
		case "CustomDateFormat":
			lang.CustomDateFormat = valStr
		case "CustomTimeFormat":
			lang.CustomTimeFormat = valStr
		case "CustomDateTimeFormat":
			lang.CustomDateTimeFormat = valStr
		default:
			return mdlerrors.NewUnsupported(fmt.Sprintf(
				"unknown language option: %s\n  valid keys: CheckCompleteness, CustomDateFormat, CustomTimeFormat, CustomDateTimeFormat",
				key))
		}
	}

	ps.Language.Languages = append(ps.Language.Languages, lang)
	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update language settings", err)
	}
	fmt.Fprintf(ctx.Output, "Enabled language: %s (%d enabled)\n", code, len(ps.Language.Languages))
	return nil
}

// alterSettingsLanguageRemove disables a language.
//
// One refusal, and one thing that is deliberately NOT a refusal.
//
// The DEFAULT language cannot be removed: Mendix resolves every missing
// translation against it, so a project without one has no fallback.
//
// A language that still carries translations is reported, not refused. Removing
// it does NOT delete them — the enabled list and the translation data are
// independent, and a stock blank app is the proof: it enables exactly one
// language while its documents store translations in EIGHT. Disabling a language
// stops the build emitting it; the Texts$Translation elements stay in the model
// (which is also why `create translations` for an unenabled language warns rather
// than failing). An earlier draft of this refused, which would have made the
// operation impossible on any real project — every marketplace module ships
// translations — and told the user something about Mendix that is not true.
func alterSettingsLanguageRemove(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	if ps.Language == nil {
		return mdlerrors.NewNotFound("settings section", "language")
	}
	code := stmt.LanguageCode
	if err := validateLanguageCodeForm(code); err != nil {
		return err
	}

	idx := -1
	avail := make([]string, 0, len(ps.Language.Languages))
	for i, l := range ps.Language.Languages {
		avail = append(avail, l.Code)
		if strings.EqualFold(l.Code, code) {
			idx = i
		}
	}
	if idx < 0 {
		sort.Strings(avail)
		return mdlerrors.NewValidationf(
			"language %q is not enabled in this project (enabled: %s)", code, strings.Join(avail, ", "))
	}
	if strings.EqualFold(ps.Language.DefaultLanguageCode, code) {
		return mdlerrors.NewValidationf(
			"%s is the project's DEFAULT language and cannot be removed — every missing translation falls back on it. "+
				"Make another language the default first: `alter settings LANGUAGE DefaultLanguageCode = '<code>'`", code)
	}

	stored := ps.Language.Languages[idx].Code
	ps.Language.Languages = append(ps.Language.Languages[:idx], ps.Language.Languages[idx+1:]...)
	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update language settings", err)
	}
	fmt.Fprintf(ctx.Output, "Disabled language: %s (%d enabled)\n", code, len(ps.Language.Languages))

	// Say what became dead weight. The translations are still in the model and
	// still open in Studio Pro's language view; the build simply stops emitting
	// them. A read failure is not an answer, so say nothing rather than "0".
	if n, err := translationCount(ctx, stored); err == nil && n > 0 {
		fmt.Fprintf(ctx.Output,
			"\nNote: %d source string(s) still carry a %s translation. They stay in the model —\n"+
				"Mendix keeps translations for languages a project has not enabled — but the build\n"+
				"no longer emits them. Remove them with `create or replace translations for %s ( );`\n",
			n, stored, stored)
	}
	return nil
}

// translationCount reports how many translations the project carries for a
// language, project-wide. A read failure is not an answer — the caller treats an
// error as "cannot tell" and does NOT take that for zero.
func translationCount(ctx *ExecContext, code string) (int, error) {
	entries, _, err := translations.Collect(ctx.Backend, sourceLanguage(ctx), nil)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.Targets[code] != "" {
			n++
		}
	}
	return n, nil
}

func validateLanguageCodeForm(code string) error {
	if code == "" {
		return mdlerrors.NewValidation("no language code given — write `alter settings LANGUAGE add 'de_DE'`")
	}
	if !languageCodeRe.MatchString(code) {
		return mdlerrors.NewValidationf(
			"%q is not a Mendix language code — it is an ISO 639 language and an ISO 3166 region joined by an underscore, "+
				"e.g. 'en_US', 'de_DE', 'ar_SD'. Studio Pro shows the code beside the language name in "+
				"App Settings ▸ Languages ▸ Add", code)
	}
	return nil
}
