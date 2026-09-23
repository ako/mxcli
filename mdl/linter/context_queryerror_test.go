// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// brokenCatalogDB is a catalog whose `modules` table is missing the Id column
// that platform filtering reads, so every joined query fails with
// "no such column". This is not hypothetical: three test fixtures in this repo
// drifted exactly this way, and the failure reached the tests as
// "expected 1 violation, got 0".
func brokenCatalogDB(t *testing.T) catalog.CatalogDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, q := range []string{
		`CREATE TABLE modules (Name TEXT PRIMARY KEY, Source TEXT)`, // no Id
		`CREATE TABLE entities (Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT,
			Folder TEXT, EntityType TEXT, Description TEXT, Generalization TEXT,
			AttributeCount INTEGER, AccessRuleCount INTEGER, ValidationRuleCount INTEGER,
			HasEventHandlers INTEGER, IsExternal INTEGER,
			HasCreatedDate INTEGER, HasChangedDate INTEGER,
			HasOwner INTEGER, HasChangedBy INTEGER)`,
		`INSERT INTO modules VALUES ('ModA', '')`,
		`INSERT INTO entities (Id, Name, QualifiedName, ModuleName, Folder, EntityType,
		    Description, Generalization, AttributeCount, AccessRuleCount,
		    ValidationRuleCount, HasEventHandlers, IsExternal)
		 VALUES ('e1','E','ModA.E','ModA','','PERSISTENT','','',0,0,0,0,0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("exec %s: %v", q, err)
		}
	}
	return catalog.WrapSqlDB(db)
}

// The defect: an iterator that cannot run its query returned no rows and said
// nothing, so "the catalog is broken" and "your project is clean" produced
// identical output. For a linter that is the worst possible failure — the tool
// reports success precisely when it has checked nothing.
func TestQueryError_BrokenQueryIsReportedNotSilent(t *testing.T) {
	ctx := NewLintContextFromDB(brokenCatalogDB(t))

	var n int
	for range ctx.Entities() {
		n++
	}
	if n != 0 {
		t.Fatalf("fixture is not broken: Entities() yielded %d rows", n)
	}

	errs := ctx.QueryErrors()
	if len(errs) == 0 {
		t.Fatal("Entities() failed and reported nothing — a broken query is " +
			"indistinguishable from a clean project")
	}
	if errs[0].Iterator != "Entities" {
		t.Errorf("Iterator = %q, want %q — the report must name what failed", errs[0].Iterator, "Entities")
	}
	if !strings.Contains(errs[0].Err.Error(), "no such column") {
		t.Errorf("underlying cause was lost: %v", errs[0].Err)
	}
	// The message a user sees has to carry both halves.
	if msg := errs[0].Error(); !strings.Contains(msg, "Entities") || !strings.Contains(msg, "no such column") {
		t.Errorf("QueryError.Error() = %q, want it to name the iterator and the cause", msg)
	}
}

// A healthy catalog must not report anything, or the signal is worthless.
func TestQueryError_HealthyCatalogReportsNothing(t *testing.T) {
	ctx := NewLintContextFromDB(setupModuleFilterDB(t))

	var n int
	for range ctx.Entities() {
		n++
	}
	if n == 0 {
		t.Fatal("healthy fixture yielded no entities")
	}
	if errs := ctx.QueryErrors(); len(errs) != 0 {
		t.Errorf("healthy catalog reported query errors: %v", errs)
	}
}

// Every iterator must report, not just the one that happened to be fixed first.
// A partial rollout leaves the same silent hole behind a different accessor.
func TestQueryError_AllIteratorsReport(t *testing.T) {
	ctx := NewLintContextFromDB(brokenCatalogDB(t))

	// Drain the iterators that join modules. Each should fail and say so.
	for range ctx.Entities() {
	}
	for range ctx.Microflows() {
	}
	for range ctx.Pages() {
	}
	for range ctx.Enumerations() {
	}
	for range ctx.Constants() {
	}
	for range ctx.Snippets() {
	}
	for range ctx.JavaActions() {
	}
	for range ctx.DocumentableElements() {
	}
	ctx.FindUnused("entity")

	seen := map[string]bool{}
	for _, e := range ctx.QueryErrors() {
		// DocumentableElements labels per table; collapse to the iterator name.
		name := e.Iterator
		if i := strings.IndexByte(name, '('); i > 0 {
			name = name[:i]
		}
		seen[name] = true
	}

	for _, want := range []string{
		"Entities", "Microflows", "Pages", "Enumerations", "Constants",
		"Snippets", "JavaActions", "DocumentableElements", "FindUnused",
	} {
		if !seen[want] {
			t.Errorf("%s failed silently — no QueryError recorded (have: %v)", want, seen)
		}
	}
}

// Several rules iterate the same accessor. Without dedup, one broken view is
// reported once per rule that touched it, so the user counts failures instead
// of reading them.
func TestQueryError_DeduplicatesRepeatedFailures(t *testing.T) {
	ctx := NewLintContextFromDB(brokenCatalogDB(t))

	for i := 0; i < 3; i++ {
		for range ctx.Entities() {
		}
	}

	errs := ctx.QueryErrors()
	if len(errs) != 1 {
		t.Errorf("draining Entities() 3 times recorded %d errors, want 1: %v", len(errs), errs)
	}
}

// Dedup must key on the cause, not just the iterator, or a second distinct
// failure in the same accessor is dropped.
func TestQueryError_DistinctCausesBothRecorded(t *testing.T) {
	ctx := NewLintContextFromDB(brokenCatalogDB(t))
	ctx.recordQueryError("X", errStub("first"))
	ctx.recordQueryError("X", errStub("second"))
	ctx.recordQueryError("X", errStub("first"))

	if got := len(ctx.QueryErrors()); got != 2 {
		t.Errorf("recorded %d errors, want 2 (two distinct causes): %v", got, ctx.QueryErrors())
	}
}

type errStub string

func (e errStub) Error() string { return string(e) }
