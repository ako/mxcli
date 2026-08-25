// SPDX-License-Identifier: Apache-2.0

// ALTER SETTINGS LANGUAGE ADD / REMOVE — the enabled-language list.
//
// The written shape is pinned against a Studio Pro-authored reference (11.13.0,
// added through App Settings ▸ Languages ▸ Add):
//
//	Languages: [3,
//	  {$Type:"Texts$Language", CheckCompleteness:false, Code:"en_US", CustomDateFormat:"", CustomDateTimeFormat:"", CustomTimeFormat:""},
//	  {$Type:"Texts$Language", CheckCompleteness:false, Code:"ar_SD", CustomDateFormat:"", CustomDateTimeFormat:"", CustomTimeFormat:""}]
//
// Studio Pro APPENDS rather than sorting. CheckCompleteness is a real setting —
// ticking it makes Mendix report errors for texts with no translation in that
// language — and it starts false on a newly added language. A stock project also
// stores false for the DEFAULT language, whose Languages-table row reads "Yes",
// because Mendix always checks the default whatever the flag says.
package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

func langCtx(t *testing.T, ps *model.ProjectSettings) (*ExecContext, *bytes.Buffer, **model.ProjectSettings) {
	t.Helper()
	var written *model.ProjectSettings
	out := &bytes.Buffer{}
	b := &mock.MockBackend{
		GetProjectSettingsFunc:    func() (*model.ProjectSettings, error) { return ps, nil },
		UpdateProjectSettingsFunc: func(p *model.ProjectSettings) error { written = p; return nil },
	}
	return &ExecContext{Backend: b, Output: out}, out, &written
}

func settingsWith(codes ...string) *model.ProjectSettings {
	ls := &model.LanguageSettings{DefaultLanguageCode: codes[0]}
	for _, c := range codes {
		ls.Languages = append(ls.Languages, model.Language{Code: c})
	}
	return &model.ProjectSettings{Language: ls}
}

func TestLanguageAdd_AppendsWithStudioProsDefaults(t *testing.T) {
	ps := settingsWith("en_US")
	ctx, out, written := langCtx(t, ps)

	err := alterSettingsLanguageAdd(ctx, ps, &ast.AlterSettingsStmt{
		Section: "language", AddLanguage: true, LanguageCode: "ar_SD",
		Properties: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := (*written).Language.Languages
	if len(got) != 2 || got[0].Code != "en_US" || got[1].Code != "ar_SD" {
		t.Fatalf("languages = %+v, want en_US then ar_SD — Studio Pro appends, it does not sort", got)
	}
	if got[1].CheckCompleteness {
		t.Error("CheckCompleteness = true; Studio Pro writes false for an added language")
	}
	if got[1].CustomDateFormat != "" || got[1].CustomTimeFormat != "" || got[1].CustomDateTimeFormat != "" {
		t.Errorf("custom formats = %+v, want empty", got[1])
	}
	if !strings.Contains(out.String(), "ar_SD") {
		t.Errorf("output %q does not name the language", out.String())
	}
}

func TestLanguageAdd_OptionsAreCarried(t *testing.T) {
	ps := settingsWith("en_US")
	ctx, _, written := langCtx(t, ps)

	err := alterSettingsLanguageAdd(ctx, ps, &ast.AlterSettingsStmt{
		Section: "language", AddLanguage: true, LanguageCode: "de_DE",
		Properties: map[string]any{"CheckCompleteness": "true", "CustomDateFormat": "yyyy-MM-dd"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := (*written).Language.Languages[1]
	if !got.CheckCompleteness || got.CustomDateFormat != "yyyy-MM-dd" {
		t.Errorf("options not carried: %+v", got)
	}
}

func TestLanguageAdd_RejectsDuplicateAndMalformedCode(t *testing.T) {
	ps := settingsWith("en_US")
	ctx, _, _ := langCtx(t, ps)

	if err := alterSettingsLanguageAdd(ctx, ps, &ast.AlterSettingsStmt{
		AddLanguage: true, LanguageCode: "en_US", Properties: map[string]any{},
	}); err == nil {
		t.Error("adding an already-enabled language was accepted")
	}
	for _, bad := range []string{"de", "de-DE", "DE_de", ""} {
		if err := alterSettingsLanguageAdd(ctx, ps, &ast.AlterSettingsStmt{
			AddLanguage: true, LanguageCode: bad, Properties: map[string]any{},
		}); err == nil {
			t.Errorf("malformed code %q was accepted", bad)
		}
	}
}

// The default language is the fallback for every missing translation, so removing
// it leaves the project with nothing to fall back on.
func TestLanguageRemove_RefusesTheDefault(t *testing.T) {
	ps := settingsWith("en_US", "de_DE")
	ctx, _, written := langCtx(t, ps)

	err := alterSettingsLanguageRemove(ctx, ps, &ast.AlterSettingsStmt{
		RemoveLanguage: true, LanguageCode: "en_US",
	})
	if err == nil {
		t.Fatal("removing the default language was accepted")
	}
	if !strings.Contains(err.Error(), "DefaultLanguageCode") {
		t.Errorf("message %q should say how to change the default first", err.Error())
	}
	if *written != nil {
		t.Error("a refused remove still wrote the settings")
	}
}

func TestLanguageRemove_DropsAnEnabledLanguage(t *testing.T) {
	ps := settingsWith("en_US", "de_DE")
	ctx, out, written := langCtx(t, ps)

	if err := alterSettingsLanguageRemove(ctx, ps, &ast.AlterSettingsStmt{
		RemoveLanguage: true, LanguageCode: "de_DE",
	}); err != nil {
		t.Fatal(err)
	}
	got := (*written).Language.Languages
	if len(got) != 1 || got[0].Code != "en_US" {
		t.Fatalf("languages = %+v, want only en_US", got)
	}
	if !strings.Contains(out.String(), "de_DE") {
		t.Errorf("output %q does not name the language", out.String())
	}
}

// Removing a language does NOT delete its translations, so it must not be
// refused because some exist. A stock blank app is the proof: it enables ONE
// language while its documents store translations in EIGHT — refusing here would
// make the operation impossible on any project with a marketplace module, and
// would assert something about Mendix that is false.
func TestLanguageRemove_TranslationsDoNotBlockIt(t *testing.T) {
	ps := settingsWith("en_US", "de_DE")
	ctx, _, written := langCtx(t, ps)

	if err := alterSettingsLanguageRemove(ctx, ps, &ast.AlterSettingsStmt{
		RemoveLanguage: true, LanguageCode: "de_DE",
	}); err != nil {
		t.Fatalf("remove refused: %v — translations are independent of the enabled list", err)
	}
	if got := (*written).Language.Languages; len(got) != 1 {
		t.Fatalf("languages = %+v, want only en_US", got)
	}
}

func TestLanguageRemove_RejectsAnUnenabledLanguage(t *testing.T) {
	ps := settingsWith("en_US")
	ctx, _, _ := langCtx(t, ps)

	err := alterSettingsLanguageRemove(ctx, ps, &ast.AlterSettingsStmt{
		RemoveLanguage: true, LanguageCode: "nl_NL",
	})
	if err == nil || !strings.Contains(err.Error(), "en_US") {
		t.Errorf("err = %v, want a rejection listing what IS enabled", err)
	}
}

// MODIFY changes an enabled language's settings. Only the options the statement
// names are touched — re-adding is not the same thing, and a script that turns on
// the completeness check must not clear a custom format set in Studio Pro.
func TestLanguageModify_TouchesOnlyWhatItNames(t *testing.T) {
	ps := settingsWith("en_US", "de_DE")
	ps.Language.Languages[1].CustomDateFormat = "dd.MM.yyyy"
	ctx, out, written := langCtx(t, ps)

	err := alterSettingsLanguageModify(ctx, ps, &ast.AlterSettingsStmt{
		Section: "language", ModifyLanguage: true, LanguageCode: "de_DE",
		Properties: map[string]any{"CheckCompleteness": "true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := (*written).Language.Languages[1]
	if !got.CheckCompleteness {
		t.Error("CheckCompleteness was not set — it is a real setting: it makes Mendix report errors for texts with no translation in that language")
	}
	if got.CustomDateFormat != "dd.MM.yyyy" {
		t.Errorf("CustomDateFormat = %q, want the stored value untouched — MODIFY changes only what it names", got.CustomDateFormat)
	}
	if !strings.Contains(out.String(), "CheckCompleteness") {
		t.Errorf("output %q does not say what changed", out.String())
	}
}

// Mendix always checks the DEFAULT language, so the flag has no effect there.
// The value is still stored (Studio Pro stores false for en_US), but a run that
// looked effective and was not would be worse than a note.
func TestLanguageModify_SaysTheDefaultIsAlwaysChecked(t *testing.T) {
	ps := settingsWith("en_US")
	ctx, out, _ := langCtx(t, ps)

	// The value has to MOVE: the note is suppressed on an unchanged run, because
	// a replayed DESCRIBE names every option of every language and printing it
	// each time buries the one run where it matters.
	if err := alterSettingsLanguageModify(ctx, ps, &ast.AlterSettingsStmt{
		ModifyLanguage: true, LanguageCode: "en_US",
		Properties: map[string]any{"CheckCompleteness": "true"},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "DEFAULT") {
		t.Errorf("output %q should note that the default is always checked", out.String())
	}
}

func TestLanguageModify_RejectsUnenabledAndEmpty(t *testing.T) {
	ps := settingsWith("en_US")
	ctx, _, _ := langCtx(t, ps)

	if err := alterSettingsLanguageModify(ctx, ps, &ast.AlterSettingsStmt{
		ModifyLanguage: true, LanguageCode: "nl_NL",
		Properties: map[string]any{"CheckCompleteness": "true"},
	}); err == nil {
		t.Error("modifying a language that is not enabled was accepted")
	}
	if err := alterSettingsLanguageModify(ctx, ps, &ast.AlterSettingsStmt{
		ModifyLanguage: true, LanguageCode: "en_US", Properties: map[string]any{},
	}); err == nil {
		t.Error("modify with no properties was accepted")
	}
}

// ADD OR MODIFY is the upsert DESCRIBE emits: it enables a language that is not
// there and changes one that is. Plain ADD refuses an already-enabled language —
// right for a hand-written script, wrong for a described project replayed onto
// itself, which is the whole reason this verb exists.
func TestLanguageUpsert_AddsWhenMissingModifiesWhenPresent(t *testing.T) {
	ps := settingsWith("en_US")
	ctx, _, written := langCtx(t, ps)

	// Missing → added.
	if err := alterSettingsLanguageUpsert(ctx, ps, &ast.AlterSettingsStmt{
		UpsertLanguage: true, LanguageCode: "de_DE",
		Properties: map[string]any{"CheckCompleteness": "true"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := (*written).Language.Languages; len(got) != 2 || !got[1].CheckCompleteness {
		t.Fatalf("languages = %+v, want de_DE added with the option applied", got)
	}

	// Present → modified, not duplicated.
	if err := alterSettingsLanguageUpsert(ctx, ps, &ast.AlterSettingsStmt{
		UpsertLanguage: true, LanguageCode: "de_DE",
		Properties: map[string]any{"CustomDateFormat": "dd.MM.yyyy"},
	}); err != nil {
		t.Fatal(err)
	}
	got := (*written).Language.Languages
	if len(got) != 2 {
		t.Fatalf("languages = %+v, want 2 — upsert must not duplicate", got)
	}
	if got[1].CustomDateFormat != "dd.MM.yyyy" || !got[1].CheckCompleteness {
		t.Errorf("de_DE = %+v, want the new format AND the earlier flag kept", got[1])
	}
}

// A replayed DESCRIBE names every option of every language, so the run must not
// report a rewrite for languages that did not change — the write is elided under
// ADR-0008 and the verb has to say so.
func TestLanguageModify_ReportsUnchangedWhenNothingMoves(t *testing.T) {
	ps := settingsWith("en_US", "de_DE")
	ps.Language.Languages[1].CheckCompleteness = true
	ctx, out, _ := langCtx(t, ps)

	if err := alterSettingsLanguageModify(ctx, ps, &ast.AlterSettingsStmt{
		ModifyLanguage: true, LanguageCode: "de_DE",
		Properties: map[string]any{"CheckCompleteness": "true"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "Unchanged language") {
		t.Errorf("output %q should report Unchanged — the value it names is the value already stored", got)
	}
}
