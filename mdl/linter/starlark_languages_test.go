// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
)

type settingsReader struct {
	minimalReader
	ps  *model.ProjectSettings
	err error
}

func (r *settingsReader) GetProjectSettings() (*model.ProjectSettings, error) { return r.ps, r.err }

func emptyCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	return cat
}

const languagesDumpRule = `
def check():
    return [violation(message = "%s|%s|%s" % (l.code, l.is_default, l.check_completeness)) for l in languages()]
`

// mendixlabs/mxcli#1306: a rule could not tell which languages are enabled.
// strings() has a row for every stored translation, and a fresh en_US-only app
// already carries nl_NL texts, so the enabled set has to come from the
// project's language settings, not from the texts.
func TestLanguagesReturnsTheEnabledProjectLanguages(t *testing.T) {
	reader := &settingsReader{ps: &model.ProjectSettings{Language: &model.LanguageSettings{
		DefaultLanguageCode: "en_US",
		Languages: []model.Language{
			{Code: "en_US"},
			{Code: "nl_NL", CheckCompleteness: true},
		},
	}}}
	vs, r := runSrc(t, linter.NewLintContext(emptyCatalog(t), reader), languagesDumpRule)
	var got []string
	for _, v := range vs {
		got = append(got, v.Message)
	}
	want := []string{"en_US|True|False", "nl_NL|False|True"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("languages() = %v, want %v", got, want)
	}
	// Settings come from the MPR reader, not a catalog table, so the rule must
	// not force a FULL catalog build.
	if m := r.RequiredCatalogMode(); m != linter.CatalogFast {
		t.Errorf("a rule calling languages() requires %v, want CatalogFast", m)
	}
}

// Control for the default flag: a project whose default is not en_US.
func TestLanguagesMarksTheProjectDefault(t *testing.T) {
	reader := &settingsReader{ps: &model.ProjectSettings{Language: &model.LanguageSettings{
		DefaultLanguageCode: "nl_NL",
		Languages:           []model.Language{{Code: "en_US"}, {Code: "nl_NL"}},
	}}}
	vs, _ := runSrc(t, linter.NewLintContext(emptyCatalog(t), reader), languagesDumpRule)
	var got []string
	for _, v := range vs {
		got = append(got, v.Message)
	}
	want := []string{"en_US|False|False", "nl_NL|True|False"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("languages() = %v, want %v", got, want)
	}
}

func TestLanguagesWithoutSettingsIsEmpty(t *testing.T) {
	for name, ctx := range map[string]*linter.LintContext{
		"no reader":   linter.NewLintContext(emptyCatalog(t), nil),
		"no settings": linter.NewLintContext(emptyCatalog(t), &settingsReader{}),
		"no language settings": linter.NewLintContext(emptyCatalog(t),
			&settingsReader{ps: &model.ProjectSettings{}}),
	} {
		if vs, _ := runSrc(t, ctx, languagesDumpRule); len(vs) != 0 {
			t.Errorf("%s: languages() returned %d rows, want 0", name, len(vs))
		}
	}
}

// A settings read that fails must fail the rule, not read as "no languages":
// a per-language rule over [] checks nothing and reports clean.
func TestLanguagesSurfacesAReaderError(t *testing.T) {
	ctx := linter.NewLintContext(emptyCatalog(t), &settingsReader{err: errors.New("settings unreadable")})
	path := filepath.Join(t.TempDir(), "rule.star")
	src := "RULE_ID = \"T1306\"\nRULE_NAME = \"T1306\"\nDESCRIPTION = \"test\"\nCATEGORY = \"quality\"\nSEVERITY = \"info\"\n" + languagesDumpRule
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := linter.LoadStarlarkRule(path)
	if err != nil {
		t.Fatalf("LoadStarlarkRule: %v", err)
	}
	vs := r.Check(ctx)
	if len(vs) != 1 || !strings.Contains(vs[0].Message, "settings unreadable") {
		t.Errorf("want one rule-error violation naming the read failure, got %+v", vs)
	}
}
