// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#970: every MDL write path that sets a caption/title/label
// stored the string under a hardcoded "en_US". On a project whose enabled
// language is something else, Studio Pro renders the widget as the empty-caption
// placeholder and warns there is no translation — while mxcli, mx check and
// mxbuild all report success. Measured on an nl_NL-only 11.13 project: the three
// texts of a created page all landed under en_US, and `mx check` reported
// "The app contains: 0 errors."
//
// The control that localised it came from the same project: Administration's own
// Studio Pro-authored pages store the same Dutch word under nl_NL, so the model
// was not the problem — the writer's choice of language was.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// ctxWithDefaultLanguage builds an ExecContext whose project reports code as its
// DefaultLanguageCode. An empty code stands for "settings unavailable", which is
// the fallback path.
func ctxWithDefaultLanguage(code string) *ExecContext {
	mb := &mock.MockBackend{
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			if code == "" {
				return nil, nil
			}
			return &model.ProjectSettings{
				Language: &model.LanguageSettings{DefaultLanguageCode: code},
			}, nil
		},
	}
	return &ExecContext{Backend: mb, Cache: &executorCache{}}
}

func TestAuthoringLanguage(t *testing.T) {
	tests := []struct {
		name string
		ctx  *ExecContext
		want string
	}{
		{"project default is used", ctxWithDefaultLanguage("nl_NL"), "nl_NL"},
		{"en_US project is unaffected", ctxWithDefaultLanguage("en_US"), "en_US"},
		{"settings unavailable falls back", ctxWithDefaultLanguage(""), "en_US"},
		{"nil context falls back", nil, "en_US"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := authoringLanguage(tc.ctx); got != tc.want {
				t.Errorf("authoringLanguage = %q, want %q", got, tc.want)
			}
		})
	}
}

// pageTextLanguages is the assertion that matters: it drives the real builders
// and reports which language each produced text is keyed under.
func TestPageTextsUseProjectDefaultLanguage(t *testing.T) {
	for _, lang := range []string{"nl_NL", "en_US"} {
		t.Run(lang, func(t *testing.T) {
			pb := &pageBuilder{
				ctx:              ctxWithDefaultLanguage(lang),
				paramEntityNames: map[string]string{},
				widgetScope:      map[string]model.ID{},
			}

			// Page title — cmd_pages_builder_v3.go
			page, err := pb.buildPageV3(&ast.CreatePageStmtV3{
				Name:  ast.QualifiedName{Module: "M", Name: "Pg"},
				Title: "Opslaanpagina",
			})
			if err != nil {
				t.Fatalf("buildPageV3: %v", err)
			}
			assertTextLang(t, "page Title", page.Title, lang, "Opslaanpagina")

			// Action button caption — cmd_pages_builder_v3_widgets.go
			btn, err := pb.buildButtonV3(&ast.WidgetV3{
				Type: "actionbutton", Name: "b",
				Properties: map[string]any{"Caption": "Opslaan", "Action": "SAVE_CHANGES"},
			})
			if err != nil {
				t.Fatalf("buildActionButtonV3: %v", err)
			}
			if btn.CaptionTemplate == nil {
				t.Fatal("button has no CaptionTemplate")
			}
			assertTextLang(t, "button Caption", btn.CaptionTemplate.Template, lang, "Opslaan")

			// Static text content — the same builder family, a different property.
			txt, err := pb.buildTextWidgetV3(&ast.WidgetV3{
				Type: "text", Name: "t",
				Properties: map[string]any{"Content": "Welkom"},
			})
			if err != nil {
				t.Fatalf("buildTextWidgetV3: %v", err)
			}
			assertTextLang(t, "text Content", txt.Caption, lang, "Welkom")
		})
	}
}

func assertTextLang(t *testing.T, what string, txt *model.Text, wantLang, wantText string) {
	t.Helper()
	if txt == nil {
		t.Fatalf("%s: no text produced", what)
	}
	if got, ok := txt.Translations[wantLang]; !ok || got != wantText {
		t.Errorf("%s: translations = %v, want %q under %q (mendixlabs/mxcli#970)",
			what, txt.Translations, wantText, wantLang)
	}
	// A single MDL string must produce exactly one translation — writing it under
	// several languages would claim a translation nobody authored.
	if len(txt.Translations) != 1 {
		t.Errorf("%s: %d translations %v, want exactly 1", what, len(txt.Translations), txt.Translations)
	}
}
