// SPDX-License-Identifier: Apache-2.0

// Bug 12: MODIFY ATTRIBUTE constraint capture (12a) and CREATE ENUMERATION
// FOLDER capture (12b) — both were dropped by the visitor before the fix.
package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestModifyAttributeConstraintsCaptured(t *testing.T) {
	cases := []struct {
		name       string
		mdl        string
		wantNN     *bool // ModifyNotNull
		wantUnique *bool // ModifyUnique
		wantDflt   bool  // ModifyHasDefault
	}{
		{"nullable clears not-null", `alter entity M.E modify attribute "Foo": String(50) NULLABLE;`, ptrBool(false), nil, false},
		{"not null sets it", `alter entity M.E modify attribute "Foo": String(50) NOT NULL;`, ptrBool(true), nil, false},
		{"required sets not-null", `alter entity M.E modify attribute "Foo": String(50) REQUIRED;`, ptrBool(true), nil, false},
		{"unique", `alter entity M.E modify attribute "Foo": String(50) UNIQUE;`, nil, ptrBool(true), false},
		{"nullable + unique", `alter entity M.E modify attribute "Foo": String(50) NULLABLE UNIQUE;`, ptrBool(false), ptrBool(true), false},
		{"default", `alter entity M.E modify attribute "Foo": String(50) DEFAULT 'x';`, nil, nil, true},
		{"type only preserves (all nil)", `alter entity M.E modify attribute "Foo": String(50);`, nil, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, errs := Build(c.mdl)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			stmt, ok := prog.Statements[0].(*ast.AlterEntityStmt)
			if !ok {
				t.Fatalf("expected AlterEntityStmt, got %T", prog.Statements[0])
			}
			if !eqBoolPtr(stmt.ModifyNotNull, c.wantNN) {
				t.Errorf("ModifyNotNull = %v, want %v", derefBool(stmt.ModifyNotNull), derefBool(c.wantNN))
			}
			if !eqBoolPtr(stmt.ModifyUnique, c.wantUnique) {
				t.Errorf("ModifyUnique = %v, want %v", derefBool(stmt.ModifyUnique), derefBool(c.wantUnique))
			}
			if stmt.ModifyHasDefault != c.wantDflt {
				t.Errorf("ModifyHasDefault = %v, want %v", stmt.ModifyHasDefault, c.wantDflt)
			}
		})
	}
}

func TestCreateEnumerationFolderCaptured(t *testing.T) {
	prog, errs := Build(`create enumeration M.Currency ( USD 'US Dollar', EUR 'Euro' ) FOLDER 'Shared';`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateEnumerationStmt)
	if !ok {
		t.Fatalf("expected CreateEnumerationStmt, got %T", prog.Statements[0])
	}
	if stmt.Folder != "Shared" {
		t.Errorf("Folder = %q, want Shared", stmt.Folder)
	}
}

func ptrBool(b bool) *bool { return &b }
func derefBool(p *bool) any {
	if p == nil {
		return nil
	}
	return *p
}
func eqBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
