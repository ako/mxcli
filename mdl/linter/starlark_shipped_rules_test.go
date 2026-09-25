// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	_ "modernc.org/sqlite"
)

// The catalog stores an entity's kind as PERSISTENT / NON_PERSISTENT / VIEW and
// LintContext.Entities normalizes it to Persistent / NonPersistent / View before a
// Starlark rule ever sees it. A rule that compares entity_type to the stored
// spelling therefore skips every entity, reports nothing, and passes every project
// in silence -- there is no error and no output to notice.
//
// These two rules ship with mxcli and are copied into every project by `mxcli init`,
// so the check runs against the files themselves rather than against a copy of their
// text: a rule reintroducing the stored spelling has to fail here.
func TestShippedEntityRulesSeeAPersistentEntity(t *testing.T) {
	db := shippedRuleFixtureDB(t)

	for _, tc := range []struct {
		rule string
		want string
	}{
		{"entity_business_key.star", "Order"},
		{"data_change_microflows.star", "Order"},
	} {
		t.Run(tc.rule, func(t *testing.T) {
			path := filepath.Join("..", "..", ".claude", "lint-rules", tc.rule)
			r, err := linter.LoadStarlarkRule(path)
			if err != nil {
				t.Fatalf("LoadStarlarkRule(%s): %v", tc.rule, err)
			}
			got := r.Check(linter.NewLintContextFromDB(db))
			if len(got) != 1 {
				t.Fatalf("got %d violations, want 1 -- the fixture is one persistent entity with "+
					"no unique attribute and no microflow touching it: %v", len(got), got)
			}
			if msg := got[0].Message; !strings.Contains(msg, tc.want) {
				t.Errorf("violation says %q, want it to name %q", msg, tc.want)
			}
		})
	}
}

// One module, one persistent entity, no attributes marked unique and nothing
// referring to it: the shape both rules exist to report.
func shippedRuleFixtureDB(t *testing.T) catalog.CatalogDB {
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
			('e1','Order','Sales.Order','Sales','','PERSISTENT','','',1,1,0,0,0, 0,0,0,0)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("fixture %q: %v", s, err)
		}
	}
	return catalog.WrapSqlDB(db)
}
