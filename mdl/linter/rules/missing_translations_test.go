// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"

	_ "modernc.org/sqlite"
)

// setupTranslationsDB creates an in-memory SQLite database with the strings FTS5 table
// and inserts test data. Rows are [QualifiedName, ObjectType, StringValue, StringContext, Language, ModuleName].
// ElementId is auto-generated from QualifiedName+StringContext for simplicity.
func setupTranslationsDB(t *testing.T, rows [][]string) catalog.CatalogDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	_, err = db.Exec(`CREATE VIRTUAL TABLE strings USING fts5(
		QualifiedName, ObjectType, StringValue, StringContext, Language, ElementId, ModuleName
	)`)
	if err != nil {
		t.Fatalf("failed to create strings table: %v", err)
	}

	stmt, err := db.Prepare(`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, Language, ElementId, ModuleName)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatalf("failed to prepare insert: %v", err)
	}
	defer stmt.Close()

	for _, row := range rows {
		if len(row) != 6 {
			t.Fatalf("expected 6 columns, got %d", len(row))
		}
		elementID := row[0] + ":" + row[3] // synthetic ID for tests
		_, err := stmt.Exec(row[0], row[1], row[2], row[3], row[4], elementID, row[5])
		if err != nil {
			t.Fatalf("failed to insert row: %v", err)
		}
	}

	return catalog.WrapSqlDB(db)
}

func TestMissingTranslations_NoViolationsWhenComplete(t *testing.T) {
	db := setupTranslationsDB(t, [][]string{
		{"MyModule.HomePage", "PAGE", "Welcome", "page_title", "en_US", "MyModule"},
		{"MyModule.HomePage", "PAGE", "Welkom", "page_title", "nl_NL", "MyModule"},
	})
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewMissingTranslationsRule()
	violations := rule.Check(ctx)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestMissingTranslations_DetectsMissingLanguage(t *testing.T) {
	db := setupTranslationsDB(t, [][]string{
		{"MyModule.HomePage", "PAGE", "Welcome", "page_title", "en_US", "MyModule"},
		{"MyModule.HomePage", "PAGE", "Welkom", "page_title", "nl_NL", "MyModule"},
		{"MyModule.EditCustomer", "PAGE", "Edit Customer", "page_title", "en_US", "MyModule"},
		// nl_NL translation missing for EditCustomer
	})
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewMissingTranslationsRule()
	violations := rule.Check(ctx)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}

	v := violations[0]
	if v.RuleID != "QUAL005" {
		t.Errorf("expected rule ID QUAL005, got %s", v.RuleID)
	}
	if v.Location.DocumentName != "MyModule.EditCustomer" {
		t.Errorf("expected document MyModule.EditCustomer, got %s", v.Location.DocumentName)
	}
}

func TestMissingTranslations_SingleLanguageNoViolations(t *testing.T) {
	// Only one language in the project — nothing to compare
	db := setupTranslationsDB(t, [][]string{
		{"MyModule.HomePage", "PAGE", "Welcome", "page_title", "en_US", "MyModule"},
		{"MyModule.EditCustomer", "PAGE", "Edit Customer", "page_title", "en_US", "MyModule"},
	})
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewMissingTranslationsRule()
	violations := rule.Check(ctx)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations for single-language project, got %d", len(violations))
	}
}

func TestMissingTranslations_NonTranslatableStringsIgnored(t *testing.T) {
	// Non-translatable strings (empty language) should not trigger violations
	db := setupTranslationsDB(t, [][]string{
		{"MyModule.HomePage", "PAGE", "Welcome", "page_title", "en_US", "MyModule"},
		{"MyModule.HomePage", "PAGE", "Welkom", "page_title", "nl_NL", "MyModule"},
		{"MyModule.HomePage", "PAGE", "/home", "page_url", "", "MyModule"},
		{"MyModule.ACT_Process", "MICROFLOW", "Processing items", "documentation", "", "MyModule"},
	})
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewMissingTranslationsRule()
	violations := rule.Check(ctx)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestMissingTranslations_ThreeLanguages(t *testing.T) {
	db := setupTranslationsDB(t, [][]string{
		{"MyModule.HomePage", "PAGE", "Welcome", "page_title", "en_US", "MyModule"},
		{"MyModule.HomePage", "PAGE", "Welkom", "page_title", "nl_NL", "MyModule"},
		{"MyModule.HomePage", "PAGE", "Bienvenue", "page_title", "fr_FR", "MyModule"},
		// EditCustomer only has en_US — missing nl_NL and fr_FR
		{"MyModule.EditCustomer", "PAGE", "Edit Customer", "page_title", "en_US", "MyModule"},
	})
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewMissingTranslationsRule()
	violations := rule.Check(ctx)

	if len(violations) != 2 {
		t.Errorf("expected 2 violations (nl_NL + fr_FR missing), got %d: %v", len(violations), violations)
	}
}

func TestMissingTranslationsRuleMetadata(t *testing.T) {
	rule := NewMissingTranslationsRule()

	if rule.ID() != "QUAL005" {
		t.Errorf("expected ID QUAL005, got %s", rule.ID())
	}
	if rule.Category() != "quality" {
		t.Errorf("expected category 'quality', got %s", rule.Category())
	}
	if rule.DefaultSeverity() != linter.SeverityWarning {
		t.Errorf("expected severity Warning, got %v", rule.DefaultSeverity())
	}
}

// setupTranslationsDBWithIDs is setupTranslationsDB with the ElementId given
// rather than synthesized, which is what lets a test put two sibling elements in
// one document.
// Rows are [QualifiedName, ObjectType, StringValue, StringContext, Language, ElementId, ModuleName].
func setupTranslationsDBWithIDs(t *testing.T, rows [][]string) catalog.CatalogDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	if _, err := db.Exec(`CREATE VIRTUAL TABLE strings USING fts5(
		QualifiedName, ObjectType, StringValue, StringContext, Language, ElementId, ModuleName
	)`); err != nil {
		t.Fatalf("failed to create strings table: %v", err)
	}
	stmt, err := db.Prepare(`INSERT INTO strings (QualifiedName, ObjectType, StringValue, StringContext, Language, ElementId, ModuleName)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		t.Fatalf("failed to prepare insert: %v", err)
	}
	defer stmt.Close()
	for _, row := range rows {
		if len(row) != 7 {
			t.Fatalf("expected 7 columns, got %d", len(row))
		}
		if _, err := stmt.Exec(row[0], row[1], row[2], row[3], row[4], row[5], row[6]); err != nil {
			t.Fatalf("failed to insert row: %v", err)
		}
	}
	return catalog.WrapSqlDB(db)
}

// Sibling elements of one type share a QualifiedName and a StringContext — an
// enumeration's twelve values, a page's action buttons. Grouping without the
// ElementId folds them into one, so translating a single value makes the whole
// set look complete and eleven missing translations go unreported.
func TestMissingTranslations_SiblingElementsAreNotOneGroup(t *testing.T) {
	db := setupTranslationsDBWithIDs(t, [][]string{
		// "Low" is translated.
		{"MyModule.Priority", "ENUMERATION", "Low", "Enumerations$EnumerationValue.Caption", "en_US", "id-low", "MyModule"},
		{"MyModule.Priority", "ENUMERATION", "Laag", "Enumerations$EnumerationValue.Caption", "nl_NL", "id-low", "MyModule"},
		// "High" is not.
		{"MyModule.Priority", "ENUMERATION", "High", "Enumerations$EnumerationValue.Caption", "en_US", "id-high", "MyModule"},
	})
	defer db.Close()

	violations := NewMissingTranslationsRule().Check(linter.NewLintContextFromDB(db))

	if len(violations) != 1 {
		t.Fatalf("want 1 violation for the untranslated value, got %d: %v", len(violations), violations)
	}
	if !strings.Contains(violations[0].Message, "High") {
		t.Errorf("the violation names the wrong value: %q", violations[0].Message)
	}
}

// The control for the test above: with every sibling translated there is nothing
// to report, so the violation there is the missing translation and not an
// artifact of splitting the group.
func TestMissingTranslations_SiblingElementsAllTranslatedIsClean(t *testing.T) {
	db := setupTranslationsDBWithIDs(t, [][]string{
		{"MyModule.Priority", "ENUMERATION", "Low", "Enumerations$EnumerationValue.Caption", "en_US", "id-low", "MyModule"},
		{"MyModule.Priority", "ENUMERATION", "Laag", "Enumerations$EnumerationValue.Caption", "nl_NL", "id-low", "MyModule"},
		{"MyModule.Priority", "ENUMERATION", "High", "Enumerations$EnumerationValue.Caption", "en_US", "id-high", "MyModule"},
		{"MyModule.Priority", "ENUMERATION", "Hoog", "Enumerations$EnumerationValue.Caption", "nl_NL", "id-high", "MyModule"},
	})
	defer db.Close()

	if v := NewMissingTranslationsRule().Check(linter.NewLintContextFromDB(db)); len(v) != 0 {
		t.Fatalf("want 0 violations, got %d: %v", len(v), v)
	}
}
