// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// indexTestCtx builds an ExecContext over a Sudoku.Game entity with Row, Col and
// Level attributes, plus whatever indexes the caller names as column lists.
func indexTestCtx(t *testing.T, indexes ...[]string) (*ExecContext, *domainmodel.Entity, *bool) {
	t.Helper()
	mod := mkModule("Sudoku")
	game := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		ContainerID: nextID("dm"),
		Name:        "Game",
		Persistable: true,
	}
	byName := map[string]model.ID{}
	for _, name := range []string{"Row", "Col", "Level"} {
		attr := &domainmodel.Attribute{Name: name}
		attr.ID = nextID("attr" + name)
		game.Attributes = append(game.Attributes, attr)
		byName[name] = attr.ID
	}
	for _, cols := range indexes {
		index := &domainmodel.Index{}
		index.ID = nextID("idx")
		for _, c := range cols {
			ia := &domainmodel.IndexAttribute{AttributeID: byName[c], Ascending: true}
			ia.ID = nextID("ia")
			index.Attributes = append(index.Attributes, ia)
		}
		game.Indexes = append(game.Indexes, index)
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{game},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	withContainer(h, game.ContainerID, dm.ID)

	updated := false
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		UpdateEntityFunc:     func(dmID model.ID, e *domainmodel.Entity) error { updated = true; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, game, &updated
}

func addIndex(cols ...ast.IndexColumn) *ast.AlterEntityStmt {
	return &ast.AlterEntityStmt{
		Name:      ast.QualifiedName{Module: "Sudoku", Name: "Game"},
		Operation: ast.AlterEntityAddIndex,
		Index:     &ast.Index{Columns: cols},
	}
}

func col(name string) ast.IndexColumn { return ast.IndexColumn{Name: name} }

// TestAddIndexRejectsADuplicate is the core of sudoku finding #10 and the
// sharpest part of it: re-running `add index` used to APPEND a second identical
// index and report success. mxbuild then fails the whole app with CE0072
// "Duplicate indexes", so a domain script run twice produced a model that no
// longer builds, with nothing in mxcli's output to say so.
func TestAddIndexRejectsADuplicate(t *testing.T) {
	ctx, game, updated := indexTestCtx(t, []string{"Row"})
	err := execAlterEntity(ctx, addIndex(col("Row")))
	if err == nil {
		t.Fatal("adding an index that already exists must not silently duplicate it")
	}
	if !strings.Contains(err.Error(), "CE0072") {
		t.Errorf("the error should name the build failure it prevents, got: %v", err)
	}
	if !strings.Contains(err.Error(), "if not exists") {
		t.Errorf("the error should point at the re-runnable spelling, got: %v", err)
	}
	if *updated {
		t.Error("no write should have been offered")
	}
	if len(game.Indexes) != 1 {
		t.Errorf("entity now carries %d indexes, want 1", len(game.Indexes))
	}
}

// TestAddIndexIfNotExistsSkips is the guarded half: same statement, no error,
// no write, and a notice naming the index in the spelling describe prints.
func TestAddIndexIfNotExistsSkips(t *testing.T) {
	ctx, game, updated := indexTestCtx(t, []string{"Row"})
	stmt := addIndex(col("Row"))
	stmt.IfNotExists = true
	assertNoError(t, execAlterEntity(ctx, stmt))
	if *updated {
		t.Error("expected no write when the index already exists")
	}
	if len(game.Indexes) != 1 {
		t.Errorf("entity now carries %d indexes, want 1", len(game.Indexes))
	}
	if out := ctxOutput(ctx); !strings.Contains(out, "(Row)") || !strings.Contains(out, "skipped") {
		t.Errorf("expected a skip notice naming the columns, got: %q", out)
	}
}

// TestIndexIdentityIsColumnsOrderAndDirection guards the matcher against being
// too eager. Mendix builds a composite database index, so (Row, Col) serves
// different queries from (Col, Row), and an ascending index is not a descending
// one — treating any of these as the same index would make ADD refuse a
// distinct index and DROP remove the wrong one.
func TestIndexIdentityIsColumnsOrderAndDirection(t *testing.T) {
	for _, tc := range []struct {
		name string
		cols []ast.IndexColumn
	}{
		{"different order", []ast.IndexColumn{col("Col"), col("Row")}},
		{"different direction", []ast.IndexColumn{{Name: "Row", Descending: true}}},
		{"extra column", []ast.IndexColumn{col("Row"), col("Col")}},
		{"different column", []ast.IndexColumn{col("Level")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, updated := indexTestCtx(t, []string{"Row"})
			assertNoError(t, execAlterEntity(ctx, addIndex(tc.cols...)))
			if !*updated {
				t.Error("a genuinely different index must still be added")
			}
		})
	}
	// The control: the same columns in the same order and direction DO match.
	ctx, _, _ := indexTestCtx(t, []string{"Row", "Col"})
	if err := execAlterEntity(ctx, addIndex(col("Row"), col("Col"))); err == nil {
		t.Error("an identical index must be recognised as a duplicate")
	}
}

// TestDropIndexByColumns covers the selector that can actually be written down.
// A Mendix index stores no name, so the ordinal "idx1" names a POSITION that
// shifts as soon as an earlier index is dropped; the column list is what
// `describe entity` prints and the only form that round-trips.
func TestDropIndexByColumns(t *testing.T) {
	ctx, game, updated := indexTestCtx(t, []string{"Row"}, []string{"Col"})
	assertNoError(t, execAlterEntity(ctx, &ast.AlterEntityStmt{
		Name:      ast.QualifiedName{Module: "Sudoku", Name: "Game"},
		Operation: ast.AlterEntityDropIndex,
		Index:     &ast.Index{Columns: []ast.IndexColumn{col("Row")}},
	}))
	if !*updated {
		t.Fatal("expected a write")
	}
	if len(game.Indexes) != 1 {
		t.Fatalf("entity carries %d indexes, want 1", len(game.Indexes))
	}
	if got := game.Indexes[0].Attributes[0].AttributeID; got != nextIDLookup(game, "Col") {
		t.Error("dropped the wrong index — Col should be the survivor")
	}
}

// TestDropIndexIfExistsSkips makes a drop re-runnable.
func TestDropIndexIfExistsSkips(t *testing.T) {
	ctx, _, updated := indexTestCtx(t)
	assertNoError(t, execAlterEntity(ctx, &ast.AlterEntityStmt{
		Name:      ast.QualifiedName{Module: "Sudoku", Name: "Game"},
		Operation: ast.AlterEntityDropIndex,
		Index:     &ast.Index{Columns: []ast.IndexColumn{col("Row")}},
		IfExists:  true,
	}))
	if *updated {
		t.Error("expected no write when there is nothing to drop")
	}
	if out := ctxOutput(ctx); !strings.Contains(out, "skipped") {
		t.Errorf("expected a skip notice, got: %q", out)
	}
}

// TestDropIndexByOrdinalNameExplainsItself keeps the legacy selector working and
// makes its failure legible: "IdxRowCol" looks like a name and never was one.
func TestDropIndexByOrdinalNameExplainsItself(t *testing.T) {
	ctx, game, _ := indexTestCtx(t, []string{"Row"})
	assertNoError(t, execAlterEntity(ctx, &ast.AlterEntityStmt{
		Name:      ast.QualifiedName{Module: "Sudoku", Name: "Game"},
		Operation: ast.AlterEntityDropIndex,
		IndexName: "idx1",
	}))
	if len(game.Indexes) != 0 {
		t.Error("the ordinal form must keep working")
	}

	ctx2, _, _ := indexTestCtx(t, []string{"Row"})
	err := execAlterEntity(ctx2, &ast.AlterEntityStmt{
		Name:      ast.QualifiedName{Module: "Sudoku", Name: "Game"},
		Operation: ast.AlterEntityDropIndex,
		IndexName: "IdxRowCol",
	})
	if err == nil {
		t.Fatal("expected an error for a name that cannot exist")
	}
	if !strings.Contains(err.Error(), "stores no name") {
		t.Errorf("the error should explain why a named index cannot be found, got: %v", err)
	}
}

func ctxOutput(ctx *ExecContext) string {
	return ctx.Output.(interface{ String() string }).String()
}

func nextIDLookup(e *domainmodel.Entity, attr string) model.ID {
	for _, a := range e.Attributes {
		if a.Name == attr {
			return a.ID
		}
	}
	return ""
}
