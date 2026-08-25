// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// An EMPTY entry list is legal, and under OR REPLACE it is the statement's own
// semantics taken to the limit: the file names nothing, so every translation in
// scope is removed. It is also the only way to take a language's translations
// out of the model — which `alter settings LANGUAGE remove` points at — and it
// parsed as an error before, so the documented way to do it did not exist.
func TestCreateTranslations_EmptyEntryListParses(t *testing.T) {
	cases := []struct {
		name, src string
		wantMode  ast.TranslationMode
		wantScope string
	}{
		{"replace, project-wide", "create or replace translations for sv_SE ( );", ast.TranslationsReplace, ""},
		{"replace, scoped", "create or replace translations in Sales for sv_SE ( );", ast.TranslationsReplace, "Sales"},
		{"merge with nothing is a legal no-op", "create or modify translations for sv_SE ( );", ast.TranslationsModify, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, errs := Build(c.src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			if len(prog.Statements) != 1 {
				t.Fatalf("statements = %d, want 1", len(prog.Statements))
			}
			stmt, ok := prog.Statements[0].(*ast.CreateTranslationsStmt)
			if !ok {
				t.Fatalf("statement is %T, want *ast.CreateTranslationsStmt", prog.Statements[0])
			}
			if len(stmt.Entries) != 0 {
				t.Errorf("entries = %d, want 0", len(stmt.Entries))
			}
			if stmt.Language != "sv_SE" {
				t.Errorf("language = %q, want sv_SE", stmt.Language)
			}
			if stmt.Module != c.wantScope {
				t.Errorf("module = %q, want %q", stmt.Module, c.wantScope)
			}
			if stmt.Mode != c.wantMode {
				t.Errorf("mode = %v, want %v", stmt.Mode, c.wantMode)
			}
		})
	}
}

// The non-empty forms must be untouched by the grammar change.
func TestCreateTranslations_EntriesStillParse(t *testing.T) {
	prog, errs := Build("create translations for de_DE ( 'Save' as 'Speichern', 'Cancel' as 'Abbrechen', );")
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateTranslationsStmt)
	if len(stmt.Entries) != 2 {
		t.Fatalf("entries = %d, want 2 (trailing comma is legal)", len(stmt.Entries))
	}
	if stmt.Entries[0].Source != "Save" || stmt.Entries[0].Target != "Speichern" {
		t.Errorf("first entry = %+v", stmt.Entries[0])
	}
}
