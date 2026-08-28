// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

func oqlViolations(vs []linter.Violation) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == "MDL071" {
			out = append(out, v)
		}
	}
	return out
}

func strAttr(name string) ast.Attribute {
	return ast.Attribute{Name: name, Type: ast.DataType{Kind: ast.TypeString, Length: 50}}
}

func TestMDL071_FlagsReservedAttributeNamesAtCreate(t *testing.T) {
	// The reported case: `Month`, `Year` and `Quarter` pass every check until a
	// view entity needs them, by which time the name is everywhere.
	stmt := &ast.CreateEntityStmt{
		Name: ast.QualifiedName{Module: "FteCapTrack", Name: "ReservedProbe"},
		Kind: ast.EntityPersistent,
		Attributes: []ast.Attribute{
			{Name: "Month", Type: ast.DataType{Kind: ast.TypeInteger}},
			{Name: "Year", Type: ast.DataType{Kind: ast.TypeInteger}},
			strAttr("Quarter"),
			strAttr("Label"), // control: not reserved
		},
	}
	got := oqlViolations(ValidateEntity(stmt))
	if len(got) != 3 {
		t.Fatalf("got %d MDL071 violations, want 3 (Month, Year, Quarter): %+v", len(got), got)
	}
	for _, v := range got {
		if v.Severity != linter.SeverityWarning {
			t.Errorf("severity = %v, want warning — the name is legal Mendix and most "+
				"entities never reach a view; an error would refuse working models", v.Severity)
		}
		if !strings.Contains(v.Message, "CE0174") {
			t.Errorf("message should name the build error: %s", v.Message)
		}
	}
}

func TestMDL071_FlagsTheEntityNameToo(t *testing.T) {
	// Measured on 11.13.0: an entity called Year makes `from MyFirstModule.Year
	// as y` fail with the same CE0174 as an attribute does. The message has to
	// say "entity", because that is what the author has to rename.
	stmt := &ast.CreateEntityStmt{
		Name:       ast.QualifiedName{Module: "MyFirstModule", Name: "Year"},
		Kind:       ast.EntityPersistent,
		Attributes: []ast.Attribute{strAttr("Label")},
	}
	got := oqlViolations(ValidateEntity(stmt))
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1 for the entity name: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "entity 'Year'") {
		t.Errorf("message must name it as an entity, not an attribute: %s", got[0].Message)
	}
}

func TestMDL071_AppliesToAlterAddAndRename(t *testing.T) {
	// An attribute added later, or renamed INTO a reserved word, is the same
	// trap — and the rename case is worse, because it lands on a name that
	// already has references.
	add := &ast.AlterEntityStmt{
		Name:      ast.QualifiedName{Module: "M", Name: "Sales"},
		Operation: ast.AlterEntityAddAttribute,
		Attribute: &ast.Attribute{Name: "Quarter", Type: ast.DataType{Kind: ast.TypeInteger}},
	}
	if got := oqlViolations(ValidateAlterEntity(add)); len(got) != 1 {
		t.Errorf("ADD ATTRIBUTE Quarter: got %d violations, want 1", len(got))
	}

	rename := &ast.AlterEntityStmt{
		Name:          ast.QualifiedName{Module: "M", Name: "Sales"},
		Operation:     ast.AlterEntityRenameAttribute,
		AttributeName: "Period",
		NewName:       "Month",
	}
	got := oqlViolations(ValidateAlterEntity(rename))
	if len(got) != 1 {
		t.Fatalf("RENAME TO Month: got %d violations, want 1", len(got))
	}
	if !strings.Contains(got[0].Message, "Month") {
		t.Errorf("message should name the NEW name: %s", got[0].Message)
	}

	// Control: renaming AWAY from a reserved word is the fix, not the problem.
	away := &ast.AlterEntityStmt{
		Name:          ast.QualifiedName{Module: "M", Name: "Sales"},
		Operation:     ast.AlterEntityRenameAttribute,
		AttributeName: "Month",
		NewName:       "MonthNumber",
	}
	if got := oqlViolations(ValidateAlterEntity(away)); len(got) != 0 {
		t.Errorf("renaming away from a reserved word must not warn: %+v", got)
	}
}

func TestMDL071_SkipsAutoXPseudoTypes(t *testing.T) {
	// An AutoX attribute's declared name is discarded on write — it materialises
	// under its fixed system member name — so warning about a name that never
	// reaches the model would be noise. MDL022 already reports the rename.
	stmt := &ast.CreateEntityStmt{
		Name: ast.QualifiedName{Module: "M", Name: "E"},
		Kind: ast.EntityPersistent,
		Attributes: []ast.Attribute{
			{Name: "Day", Type: ast.DataType{Kind: ast.TypeAutoCreatedDate}},
		},
	}
	if got := oqlViolations(ValidateEntity(stmt)); len(got) != 0 {
		t.Errorf("AutoX attribute should not raise MDL071: %+v", got)
	}
}

func TestMDL071_WordListIsSharedWithMDL032(t *testing.T) {
	// Two lists would drift, and the drift is invisible: a word added for the
	// view check but missing here reverts this rule to "too late" for that word.
	for _, w := range oqlReservedWords {
		if !isOQLReservedName(w) {
			t.Errorf("%q is in oqlReservedWords but isOQLReservedName says no", w)
		}
		if !isOQLReservedName(strings.ToLower(w)) {
			t.Errorf("%q not matched case-insensitively — OQL's grammar does not care how it was typed", w)
		}
	}
	for _, ok := range []string{"Label", "Amount", "Name", "MonthNumber", "YearValue"} {
		if isOQLReservedName(ok) {
			t.Errorf("%q must not be flagged", ok)
		}
	}
}

func TestMDL071_DoesNotBlockACreateThatMendixAccepts(t *testing.T) {
	// End-to-end intent of the severity choice: an entity with Month/Year/Quarter
	// is 0 errors in mxbuild (measured on 11.13.0), so mxcli must write it.
	stmt := &ast.CreateEntityStmt{
		Name: ast.QualifiedName{Module: "M", Name: "ReservedProbe"},
		Kind: ast.EntityPersistent,
		Attributes: []ast.Attribute{
			{Name: "Month", Type: ast.DataType{Kind: ast.TypeInteger}},
			{Name: "Year", Type: ast.DataType{Kind: ast.TypeInteger}},
			strAttr("Quarter"),
		},
	}
	if v := firstBlockingViolation(ValidateEntity(stmt)); v != nil {
		t.Errorf("MDL071 must not block a legal Mendix model, got %s: %s", v.RuleID, v.Message)
	}
}
