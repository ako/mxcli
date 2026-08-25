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
// Two things that reference settles and a guess would not.
//
// Studio Pro APPENDS rather than sorting, so the stored order is the order
// languages were enabled.
//
// And a newly added language starts with `CheckCompleteness: false`. That flag
// is a real setting, not display sugar: ticking "Check completeness" makes
// Mendix report errors and warnings for texts with no translation in that
// language, which is how a team stops a translation silently falling back to the
// default. It is authorable here and changeable later with MODIFY. The one
// subtlety is the DEFAULT language — a stock project stores false for en_US
// while the Languages table shows "Yes", because Mendix always checks the
// default whatever the flag says (the Edit dialog states it). So false on the
// default is what Studio Pro writes, and it does not mean the default is
// unchecked.
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

// alterSettingsLanguageUpsert is ADD OR MODIFY: enable the language when it is
// not enabled, change the named options when it is.
//
// It exists because DESCRIBE has to emit something that re-executes. Plain ADD
// refuses a language that is already enabled — the right answer for a script
// someone writes by hand, and the wrong one for a described project replayed
// onto itself. Same reasoning as CREATE OR MODIFY elsewhere in MDL.
func alterSettingsLanguageUpsert(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	if ps.Language == nil {
		return mdlerrors.NewNotFound("settings section", "language")
	}
	if err := validateLanguageCodeForm(stmt.LanguageCode); err != nil {
		return err
	}
	for _, l := range ps.Language.Languages {
		if strings.EqualFold(l.Code, stmt.LanguageCode) {
			if len(stmt.Properties) == 0 {
				// Nothing to change and nothing to add: report the state rather
				// than an error, so a replay of a described project is quiet.
				fmt.Fprintf(ctx.Output, "Unchanged language: %s\n", l.Code)
				return nil
			}
			return alterSettingsLanguageModify(ctx, ps, stmt)
		}
	}
	return alterSettingsLanguageAdd(ctx, ps, stmt)
}

// alterSettingsLanguageModify changes the settings of a language that is already
// enabled: the completeness check and the custom date/time formats.
//
// Only the options the statement NAMES are touched — that is what distinguishes
// MODIFY from re-adding, and it means a script turning on the completeness check
// cannot silently clear a custom date format somebody set in Studio Pro. The
// language's stored document is preserved either way, so its $ID and any
// property this mxcli does not model survive.
func alterSettingsLanguageModify(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
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
			"language %q is not enabled in this project (enabled: %s) — add it first with "+
				"`alter settings LANGUAGE add '%s';`", code, strings.Join(avail, ", "), code)
	}
	if len(stmt.Properties) == 0 {
		return mdlerrors.NewValidationf(
			"no properties given — write e.g. `alter settings LANGUAGE modify '%s' (CheckCompleteness: true);`", code)
	}

	lang := ps.Language.Languages[idx]
	changed := make([]string, 0, len(stmt.Properties))
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
		changed = append(changed, key)
	}
	sort.Strings(changed)
	unchanged := lang == ps.Language.Languages[idx]
	ps.Language.Languages[idx] = lang

	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update language settings", err)
	}
	// Say which of the two happened. A replayed DESCRIBE names every option of
	// every language, so reporting "Modified" for all of them would describe a
	// rewrite that ADR-0008 elides — the same verb honesty the flow writers use
	// ("Unchanged nanoflow: …" where nothing landed).
	if unchanged {
		fmt.Fprintf(ctx.Output, "Unchanged language: %s\n", lang.Code)
	} else {
		fmt.Fprintf(ctx.Output, "Modified language: %s (%s)\n", lang.Code, strings.Join(changed, ", "))
	}

	// Mendix always checks the default language, so the flag changes nothing
	// there. Say so rather than let the script look effective.
	if _, named := stmt.Properties["CheckCompleteness"]; named &&
		strings.EqualFold(ps.Language.DefaultLanguageCode, lang.Code) {
		fmt.Fprintf(ctx.Output,
			"\nNote: %s is the DEFAULT language, which Mendix checks for completeness whatever\n"+
				"this flag says. The value is stored, and takes effect if another language\n"+
				"becomes the default.\n", lang.Code)
	}
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
