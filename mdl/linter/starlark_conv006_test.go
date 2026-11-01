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

// ako/mxcli#953 item 5: CONV006 emitted one finding per entity × role ×
// CREATE/DELETE — 111 on the JTSBootLogboek app, most of them the same advice
// about the same entity. One finding per entity names every role and right.
// Logboek is the grouped case; Note has READ/WRITE only and is the control
// (no finding); Draft is non-persistent and out of scope.
func TestCONV006GroupsByEntity(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`CREATE TABLE modules (Id TEXT, Name TEXT, Source TEXT)`,
		`INSERT INTO modules VALUES ('m1', 'App', '')`,
		`CREATE TABLE entities (
			Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
			EntityType TEXT, Description TEXT, Generalization TEXT,
			AttributeCount INTEGER, AccessRuleCount INTEGER, ValidationRuleCount INTEGER,
			HasEventHandlers INTEGER, IsExternal INTEGER,
			HasCreatedDate INTEGER, HasChangedDate INTEGER,
			HasOwner INTEGER, HasChangedBy INTEGER)`,
		`INSERT INTO entities VALUES
			('e1','Logboek','App.Logboek','App','','PERSISTENT','','',1,3,0,0,0, 0,0,0,0),
			('e2','Note','App.Note','App','','PERSISTENT','','',1,1,0,0,0, 0,0,0,0),
			('e3','Draft','App.Draft','App','','NON_PERSISTENT','','',1,1,0,0,0, 0,0,0,0)`,
		`CREATE TABLE permissions (ModuleRoleName TEXT, ElementType TEXT, ElementName TEXT,
			MemberName TEXT, AccessType TEXT, XPathConstraint TEXT,
			DefaultMemberAccessRights TEXT, ModuleName TEXT,
			AccessRuleId TEXT, AccessRuleIndex INTEGER)`,
		// Two access rules for App.User on Logboek: its CREATE must be listed once.
		`INSERT INTO permissions (ModuleRoleName, ElementType, ElementName, MemberName, AccessType, XPathConstraint, DefaultMemberAccessRights, ModuleName) VALUES
			('App.Admin','ENTITY','App.Logboek',NULL,'CREATE','','ReadWrite','App'),
			('App.Admin','ENTITY','App.Logboek',NULL,'DELETE','','ReadWrite','App'),
			('App.Admin','ENTITY','App.Logboek',NULL,'READ','','ReadWrite','App'),
			('App.User','ENTITY','App.Logboek',NULL,'CREATE','','ReadWrite','App'),
			('App.User','ENTITY','App.Logboek',NULL,'CREATE','[Owner = ''[%CurrentUser%]'']','ReadWrite','App'),
			('App.Viewer','ENTITY','App.Logboek',NULL,'READ','','ReadOnly','App'),
			('App.Admin','ENTITY','App.Note',NULL,'READ','','ReadWrite','App'),
			('App.Admin','ENTITY','App.Note',NULL,'WRITE','','ReadWrite','App'),
			('App.Admin','ENTITY','App.Draft',NULL,'CREATE','','ReadWrite','App')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("fixture %q: %v", s, err)
		}
	}

	for _, dir := range []string{".claude/lint-rules", "cmd/mxcli/lint-rules"} {
		t.Run(dir, func(t *testing.T) {
			r, err := linter.LoadStarlarkRule(filepath.Join("..", "..", dir, "conv006_no_create_delete_rights.star"))
			if err != nil {
				t.Fatal(err)
			}
			got := r.Check(linter.NewLintContextFromDB(catalog.WrapSqlDB(db)))
			if len(got) != 1 {
				t.Fatalf("got %d violations, want 1 (one per entity): %v", len(got), got)
			}
			msg := got[0].Message
			for _, want := range []string{"App.Logboek", "CREATE (App.Admin, App.User)", "DELETE (App.Admin)"} {
				if !strings.Contains(msg, want) {
					t.Errorf("message does not contain %q:\n%s", want, msg)
				}
			}
			if strings.Contains(msg, "App.Viewer") {
				t.Errorf("message names a role without CREATE/DELETE:\n%s", msg)
			}
		})
	}
}
