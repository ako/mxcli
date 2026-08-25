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
// Studio Pro APPENDS rather than sorting, and stores CheckCompleteness FALSE even
// for the default language — whose Languages-table row reads "Yes", because the
// default is always checked and the column is computed. Writing true to match the
// UI would produce a document Studio Pro never writes.
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
