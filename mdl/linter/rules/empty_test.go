// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"database/sql"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"

	_ "modernc.org/sqlite"
)

// setupMicroflowsDB creates an in-memory SQLite database with the microflows and modules tables.
// Each row is [Id, Name, QualifiedName, ModuleName, Folder, MicroflowType, Description, ReturnType, ParameterCount, ActivityCount, Complexity].
func setupMicroflowsDB(t *testing.T, rows [][]any) catalog.CatalogDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	// Id mirrors the real `modules` view. Platform filtering keys off the System
	// module's sentinel Id — Source is empty for System exactly as it is for a
	// user module — and these iterators swallow query errors, so a missing
	// column shows up as "no rows" rather than "no such column".
	_, err = db.Exec(`CREATE TABLE modules (Id TEXT, Name TEXT PRIMARY KEY, Source TEXT)`)
	if err != nil {
		t.Fatalf("failed to create modules table: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE microflows (
		Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
		MicroflowType TEXT, Description TEXT, ReturnType TEXT,
		ParameterCount INTEGER, ActivityCount INTEGER, Complexity INTEGER
	)`)
	if err != nil {
		t.Fatalf("failed to create microflows table: %v", err)
	}

	for _, row := range rows {
		_, err := db.Exec(`INSERT INTO microflows VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row...)
		if err != nil {
			t.Fatalf("failed to insert row: %v", err)
		}
		// Ensure module exists
		moduleName := row[3].(string)
		if _, err := db.Exec(`INSERT OR IGNORE INTO modules (Id, Name, Source) VALUES (?, ?, '')`, moduleName+"-id", moduleName); err != nil {
			t.Fatalf("failed to insert module: %v", err)
		}
	}

	return catalog.WrapSqlDB(db)
}

func TestEmptyMicroflowRule_NoViolations(t *testing.T) {
	db := setupMicroflowsDB(t, [][]any{
		{"id1", "ACT_Process", "MyModule.ACT_Process", "MyModule", "", "Microflow", "", "Void", 0, 3, 1},
	})
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewEmptyMicroflowRule()
	violations := rule.Check(ctx)

	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(violations))
	}
}

func TestEmptyMicroflowRule_DetectsEmpty(t *testing.T) {
	db := setupMicroflowsDB(t, [][]any{
		{"id1", "ACT_Process", "MyModule.ACT_Process", "MyModule", "", "Microflow", "", "Void", 0, 0, 0},
		{"id2", "ACT_Other", "MyModule.ACT_Other", "MyModule", "", "Microflow", "", "Void", 0, 5, 2},
	})
	defer db.Close()

	ctx := linter.NewLintContextFromDB(db)
	rule := NewEmptyMicroflowRule()
	violations := rule.Check(ctx)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].RuleID != "MPR002" {
		t.Errorf("expected rule ID MPR002, got %s", violations[0].RuleID)
	}
	if violations[0].Location.DocumentName != "ACT_Process" {
		t.Errorf("expected document ACT_Process, got %s", violations[0].Location.DocumentName)
	}
}

func TestEmptyMicroflowRule_Metadata(t *testing.T) {
	r := NewEmptyMicroflowRule()
	if r.ID() != "MPR002" {
		t.Errorf("ID = %q, want MPR002", r.ID())
	}
	if r.Category() != "quality" {
		t.Errorf("Category = %q, want quality", r.Category())
	}
}

// TestEmptyMicroflowRule_NamesTheDocumentType is the test this rule did not
// have: LintContext.Microflows() yields microflows, nanoflows and rules alike
// (one catalog table, three doctypes), and MPR002 called every one of them a
// microflow — "Microflow 'Rule1' has no activities" about a rule.
//
// The fixtures use the catalog's own spellings, uppercase, rather than the
// title-case ones the older tests in this file use. That difference is the
// point: the old spellings never had to be right, because nothing read the
// column. Now that the noun is derived from it, a fixture that does not match
// what the catalog writes would test the fallback instead of the mapping.
func TestEmptyMicroflowRule_NamesTheDocumentType(t *testing.T) {
	db := setupMicroflowsDB(t, [][]any{
		{"id1", "ACT_Process", "MyModule.ACT_Process", "MyModule", "", "MICROFLOW", "", "Void", 0, 0, 0},
		{"id2", "NF_Refresh", "MyModule.NF_Refresh", "MyModule", "", "NANOFLOW", "", "Void", 0, 0, 0},
		{"id3", "Rule1", "MyModule.Rule1", "MyModule", "", "RULE", "", "Boolean", 1, 0, 0},
		// An unknown type must still be reported, under the generic noun: a
		// finding with an imprecise label beats no finding at all.
		{"id4", "Mystery", "MyModule.Mystery", "MyModule", "", "SOMETHING_NEW", "", "Void", 0, 0, 0},
	})
	defer db.Close()

	violations := NewEmptyMicroflowRule().Check(linter.NewLintContextFromDB(db))
	if len(violations) != 4 {
		t.Fatalf("expected 4 violations, got %d", len(violations))
	}

	byName := map[string]linter.Violation{}
	for _, v := range violations {
		byName[v.Location.DocumentName] = v
	}

	for _, tc := range []struct{ doc, wantType, wantMessage string }{
		{"ACT_Process", "microflow", "Microflow 'ACT_Process' has no activities"},
		{"NF_Refresh", "nanoflow", "Nanoflow 'NF_Refresh' has no activities"},
		{"Rule1", "rule", "Rule 'Rule1' has no activities"},
		{"Mystery", "microflow", "Microflow 'Mystery' has no activities"},
	} {
		v, ok := byName[tc.doc]
		if !ok {
			t.Errorf("no violation for %s", tc.doc)
			continue
		}
		if v.Message != tc.wantMessage {
			t.Errorf("%s: message = %q, want %q", tc.doc, v.Message, tc.wantMessage)
		}
		// The doctype also reaches the JSON and SARIF output as documentType,
		// where a wrong value is not merely cosmetic.
		if v.Location.DocumentType != tc.wantType {
			t.Errorf("%s: documentType = %q, want %q", tc.doc, v.Location.DocumentType, tc.wantType)
		}
	}
}

// TestDocumentNounCoversEveryCatalogType pins the mapping against the values
// the catalog actually inserts (mdl/catalog/builder_microflows.go), so adding a
// fourth flow flavour there fails here rather than silently reporting it as a
// microflow.
func TestDocumentNounCoversEveryCatalogType(t *testing.T) {
	for stored, want := range map[string]string{
		"MICROFLOW": "microflow",
		"NANOFLOW":  "nanoflow",
		"RULE":      "rule",
	} {
		mf := linter.Microflow{MicroflowType: stored}
		if got := mf.DocumentNoun(); got != want {
			t.Errorf("DocumentNoun(%q) = %q, want %q", stored, got, want)
		}
	}
}
