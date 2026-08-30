// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ledger #148: MDL071 warned about `Year` on two NON-PERSISTENT entities. The
// rule's premise is that a view entity might one day reference the name
// unquoted — and a non-persistent entity has no table, so no view can reference
// it at all.
//
// Measured rather than reasoned about, on mxbuild 11.13.0, by pointing a view
// entity at a non-persistent one:
//
//	[error] [CE0174] "Error(s) in OQL query: Entity 'DS147.NpeYear' cannot be
//	        used in OQL, because it is a non-persistable entity"
//	        at Entity 'DS147.VOverNpe'
//
// Mendix refuses the reference outright, so the collision the warning predicts
// cannot occur. It cost only noise — but warnings that cannot come true are how
// people learn to skim warnings.
//
// Persistence is a THREE-way question here, not a boolean. ALTER ENTITY … ADD
// ATTRIBUTE does not carry the entity's kind, and treating "unknown" as
// "non-persistent" would silently drop MDL071 from that path — the case the rule
// most wants to catch, since a rename lands on a name that already has
// references.

func npeStmt(attrs ...string) *ast.CreateEntityStmt {
	stmt := &ast.CreateEntityStmt{
		Name: ast.QualifiedName{Module: "Ledger", Name: "BudgetContext"},
		Kind: ast.EntityNonPersistent,
	}
	for _, a := range attrs {
		stmt.Attributes = append(stmt.Attributes, ast.Attribute{Name: a, Type: ast.DataType{Kind: ast.TypeInteger}})
	}
	return stmt
}

// The reported case.
func TestMDL071_SkipsANonPersistentEntity(t *testing.T) {
	if got := oqlViolations(ValidateEntity(npeStmt("Year", "Month"))); len(got) != 0 {
		t.Errorf("a non-persistent entity cannot be referenced from OQL at all (CE0174 says so); got %+v", got)
	}
}

// …including when the ENTITY's own name is the reserved word.
func TestMDL071_SkipsANonPersistentEntityName(t *testing.T) {
	stmt := npeStmt("Label")
	stmt.Name = ast.QualifiedName{Module: "Ledger", Name: "Year"}
	if got := oqlViolations(ValidateEntity(stmt)); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

// CONTROL 1: the same attributes on a PERSISTENT entity are still warned about.
// Without this the fix is indistinguishable from deleting the rule.
func TestMDL071_StillFiresOnAPersistentEntity(t *testing.T) {
	stmt := npeStmt("Year", "Month")
	stmt.Kind = ast.EntityPersistent
	if got := oqlViolations(ValidateEntity(stmt)); len(got) != 2 {
		t.Fatalf("got %d violations, want 2 (Year, Month): %+v", len(got), got)
	}
}

// CONTROL 2: a VIEW entity is queryable, and its own attribute name is its
// select alias — the one case that genuinely needs a rename. It must not be
// swept up with the non-persistent skip.
func TestMDL071_StillFiresOnAViewEntity(t *testing.T) {
	stmt := npeStmt("Year")
	stmt.Kind = ast.EntityView
	if got := oqlViolations(ValidateEntity(stmt)); len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
}

// CONTROL 3: ALTER ENTITY … ADD ATTRIBUTE does not know the entity's kind. The
// tri-state is the whole reason this is not a boolean — "unknown" must keep the
// warning, or the path where a name arrives late loses its only check.
func TestMDL071_UnknownPersistenceKeepsTheWarning(t *testing.T) {
	stmt := &ast.AlterEntityStmt{
		Name:      ast.QualifiedName{Module: "Ledger", Name: "BudgetContext"},
		Operation: ast.AlterEntityAddAttribute,
		Attribute: &ast.Attribute{Name: "Year", Type: ast.DataType{Kind: ast.TypeInteger}},
	}
	if got := oqlViolations(ValidateAlterEntity(stmt)); len(got) != 1 {
		t.Fatalf("ALTER carries no persistence kind, so the warning must stand; got %+v", got)
	}
}

// CONTROL 4: MDL054, the other rule keyed on non-persistence, is untouched — the
// skip must be MDL071's alone.
func TestMDL071_SkipDoesNotDisturbMDL054(t *testing.T) {
	stmt := npeStmt()
	stmt.Attributes = []ast.Attribute{
		{Name: "Year", Type: ast.DataType{Kind: ast.TypeInteger}, NotNull: true},
	}
	var mdl054 int
	for _, v := range ValidateEntity(stmt) {
		if v.RuleID == "MDL054" {
			mdl054++
		}
	}
	if mdl054 != 1 {
		t.Fatalf("`not null` on a non-persistent entity is still CE0070 (MDL054); got %d", mdl054)
	}
}
