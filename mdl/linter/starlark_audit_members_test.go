// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	_ "modernc.org/sqlite"
)

// The four audit members are booleans on the entity's generalization node, not
// attributes, so a Starlark rule cannot find them by walking attributes() and
// attribute_count does not include them. Without these fields a rule asking
// "which persistent entities have no audit trail" has nothing to read.
func TestStarlarkEntityExposesAuditMembers(t *testing.T) {
	db := auditFixtureDB(t)

	dir := t.TempDir()
	src := `
RULE_ID = "TEST001"
RULE_NAME = "AuditTrail"
DESCRIPTION = "entities without an audit trail"
CATEGORY = "quality"
SEVERITY = "warning"

def check():
    violations = []
    for e in entities():
        if e.entity_type == "Persistent" and not e.has_created_date:
            violations.append(violation(
                message="{} has no CreatedDate (owner={} changed_by={} changed_date={})".format(
                    e.qualified_name, e.has_owner, e.has_changed_by, e.has_changed_date),
                location=location(module=e.module_name, document_type="Entity",
                                  document_name=e.qualified_name),
            ))
    return violations
`
	path := filepath.Join(dir, "audit.star")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := linter.LoadStarlarkRule(path)
	if err != nil {
		t.Fatalf("LoadStarlarkRule: %v -- a field the rule names must exist on the struct", err)
	}

	got := r.Check(linter.NewLintContextFromDB(db))
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1 (only the bare entity): %v", len(got), got)
	}
	msg := got[0].Message
	if !strings.Contains(msg, "Sales.Lookup") {
		t.Errorf("violation names %q, want the entity without CreatedDate", msg)
	}
	// The other three must be readable too, and false on this entity -- a
	// field that silently reads False everywhere would pass a weaker test.
	for _, want := range []string{"owner=False", "changed_by=False", "changed_date=False"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q missing %q", msg, want)
		}
	}
}

// The audited entity must read True, or the fields are wired to a constant.
func TestStarlarkEntityAuditMembersReadTrue(t *testing.T) {
	db := auditFixtureDB(t)

	dir := t.TempDir()
	src := `
RULE_ID = "TEST002"
RULE_NAME = "Audited"
DESCRIPTION = "audited entities"
CATEGORY = "quality"
SEVERITY = "info"

def check():
    violations = []
    for e in entities():
        if e.has_created_date and e.has_changed_date and e.has_owner and e.has_changed_by:
            violations.append(violation(
                message=e.qualified_name,
                location=location(module=e.module_name, document_type="Entity",
                                  document_name=e.qualified_name)))
    return violations
`
	path := filepath.Join(dir, "audited.star")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := linter.LoadStarlarkRule(path)
	if err != nil {
		t.Fatal(err)
	}
	got := r.Check(linter.NewLintContextFromDB(db))
	if len(got) != 1 || got[0].Message != "Sales.Order" {
		t.Fatalf("got %v, want exactly Sales.Order -- all four flags must read True there", got)
	}
}

func auditFixtureDB(t *testing.T) catalog.CatalogDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE modules (Id TEXT, Name TEXT, Source TEXT)`,
		`INSERT INTO modules VALUES ('m1', 'Sales', '')`,
		`CREATE TABLE entities (
			Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
			EntityType TEXT, Description TEXT, Generalization TEXT,
			AttributeCount INTEGER, AccessRuleCount INTEGER, ValidationRuleCount INTEGER,
			HasEventHandlers INTEGER, IsExternal INTEGER,
			HasCreatedDate INTEGER, HasChangedDate INTEGER,
			HasOwner INTEGER, HasChangedBy INTEGER)`,
		`INSERT INTO entities VALUES
			('e1','Order','Sales.Order','Sales','','PERSISTENT','','',3,1,0,0,0, 1,1,1,1),
			('e2','Lookup','Sales.Lookup','Sales','','PERSISTENT','','',2,1,0,0,0, 0,0,0,0)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("fixture %q: %v", s, err)
		}
	}
	return catalog.WrapSqlDB(db)
}
