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

// SEC008 flagged Engineer.Email as readable by every role with entity-level
// READ, but the catalog emits that row when ANY member is readable. A role
// granted `read (FullName)` — Email = None in `show access` — was reported as
// reading PII. Only a role that can read a PII member counts.
func TestSEC008CountsOnlyRolesThatReadAPiiMember(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`CREATE TABLE modules (Id TEXT, Name TEXT, Source TEXT)`,
		`INSERT INTO modules VALUES ('m1', 'FS', '')`,
		`CREATE TABLE entities (
			Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
			EntityType TEXT, Description TEXT, Generalization TEXT,
			AttributeCount INTEGER, AccessRuleCount INTEGER, ValidationRuleCount INTEGER,
			HasEventHandlers INTEGER, IsExternal INTEGER,
			HasCreatedDate INTEGER, HasChangedDate INTEGER,
			HasOwner INTEGER, HasChangedBy INTEGER)`,
		`INSERT INTO entities VALUES
			('e1','Engineer','FS.Engineer','FS','','PERSISTENT','','',2,4,0,0,0, 0,0,0,0)`,
		`CREATE TABLE attributes (Id TEXT, Name TEXT, EntityId TEXT, EntityQualifiedName TEXT,
			ModuleName TEXT, DataType TEXT, Length INTEGER, IsUnique INTEGER, IsRequired INTEGER,
			DefaultValue TEXT, IsCalculated INTEGER, Description TEXT)`,
		`INSERT INTO attributes VALUES
			('a1','FullName','e1','FS.Engineer','FS','String',200,0,0,'',0,''),
			('a2','Email','e1','FS.Engineer','FS','String',200,0,0,'',0,'')`,
		`CREATE TABLE permissions (ModuleRoleName TEXT, ElementType TEXT, ElementName TEXT,
			MemberName TEXT, AccessType TEXT, XPathConstraint TEXT,
			DefaultMemberAccessRights TEXT, ModuleName TEXT,
			AccessRuleId TEXT, AccessRuleIndex INTEGER)`,
		// Coordinator reads Email (explicit, qualified member name), Admin reads
		// it through default rights (bare member name), FieldEngineer reads only
		// FullName, Requester reads Email but row-scoped.
		`INSERT INTO permissions (ModuleRoleName, ElementType, ElementName, MemberName, AccessType, XPathConstraint, DefaultMemberAccessRights, ModuleName) VALUES
			('FS.Coordinator','ENTITY','FS.Engineer',NULL,'READ','','None','FS'),
			('FS.Coordinator','ENTITY','FS.Engineer','FS.Engineer.FullName','MEMBER_READ','','None','FS'),
			('FS.Coordinator','ENTITY','FS.Engineer','FS.Engineer.Email','MEMBER_READ','','None','FS'),
			('FS.Admin','ENTITY','FS.Engineer',NULL,'READ','','ReadOnly','FS'),
			('FS.Admin','ENTITY','FS.Engineer','FullName','MEMBER_READ','','ReadOnly','FS'),
			('FS.Admin','ENTITY','FS.Engineer','Email','MEMBER_READ','','ReadOnly','FS'),
			('FS.FieldEngineer','ENTITY','FS.Engineer',NULL,'READ','','None','FS'),
			('FS.FieldEngineer','ENTITY','FS.Engineer','FS.Engineer.FullName','MEMBER_READ','','None','FS'),
			('FS.Requester','ENTITY','FS.Engineer',NULL,'READ','[id = ''[%CurrentUser%]'']','None','FS'),
			('FS.Requester','ENTITY','FS.Engineer','FS.Engineer.Email','MEMBER_READ','[id = ''[%CurrentUser%]'']','None','FS')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("fixture %q: %v", s, err)
		}
	}

	for _, dir := range []string{".claude/lint-rules", "cmd/mxcli/lint-rules"} {
		t.Run(dir, func(t *testing.T) {
			r, err := linter.LoadStarlarkRule(filepath.Join("..", "..", dir, "sec_unconstrained_pii_read.star"))
			if err != nil {
				t.Fatal(err)
			}
			got := r.Check(linter.NewLintContextFromDB(catalog.WrapSqlDB(db)))
			if len(got) != 1 {
				t.Fatalf("got %d violations, want 1: %v", len(got), got)
			}
			msg := got[0].Message
			for _, want := range []string{"FS.Coordinator", "FS.Admin"} {
				if !strings.Contains(msg, want) {
					t.Errorf("violation does not name %s, which reads Email unconstrained:\n%s", want, msg)
				}
			}
			for _, not := range []string{"FS.FieldEngineer", "FS.Requester"} {
				if strings.Contains(msg, not) {
					t.Errorf("violation names %s, which cannot read Email unconstrained:\n%s", not, msg)
				}
			}
		})
	}
}
