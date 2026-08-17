// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// renameAttrCall records one RenameReferences invocation.
type renameAttrCall struct {
	old, new string
	dryRun   bool
}

// renameAttrTestCtx builds an ExecContext over Sudoku.Game carrying attrs, and
// returns the call log so a test can assert what the rename asked the backend to
// do — and in which order.
func renameAttrTestCtx(t *testing.T, hits int, attrs ...string) (*ExecContext, *[]string, *[]renameAttrCall) {
	t.Helper()
	mod := mkModule("Sudoku")
	game := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		ContainerID: nextID("dm"),
		Name:        "Game",
		Persistable: true,
	}
	for _, a := range attrs {
		game.Attributes = append(game.Attributes, &domainmodel.Attribute{Name: a})
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{game},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	withContainer(h, game.ContainerID, dm.ID)

	var order []string
	var calls []renameAttrCall
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		UpdateEntityFunc: func(dmID model.ID, e *domainmodel.Entity) error {
			order = append(order, "UpdateEntity")
			return nil
		},
		RenameReferencesFunc: func(oldName, newName string, dryRun bool) ([]types.RenameHit, error) {
			order = append(order, "RenameReferences")
			calls = append(calls, renameAttrCall{oldName, newName, dryRun})
			if hits == 0 {
				return nil, nil
			}
			return []types.RenameHit{{
				UnitID: "u1", UnitType: "Microflows$Microflow", Name: "ACT_Play", Count: hits,
			}}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, &order, &calls
}

func renameAttrStmt(from, to string) *ast.AlterEntityStmt {
	return &ast.AlterEntityStmt{
		Name:          ast.QualifiedName{Module: "Sudoku", Name: "Game"},
		Operation:     ast.AlterEntityRenameAttribute,
		AttributeName: from,
		NewName:       to,
	}
}

// TestAlterEntityRenameAttributeUpdatesReferences is the regression test for
// issue #910 (problem 2). Renaming an attribute used to change only the domain
// model, leaving every microflow, page and validation rule pointing at the old
// qualified name — mxbuild reports each one as CE1613 "The selected attribute
// 'Mod.Entity.Old' no longer exists."
func TestAlterEntityRenameAttributeUpdatesReferences(t *testing.T) {
	ctx, _, calls := renameAttrTestCtx(t, 4, "PuzzleNo", "Score")

	assertNoError(t, execAlterEntity(ctx, renameAttrStmt("PuzzleNo", "PuzzleNumber")))

	if len(*calls) != 1 {
		t.Fatalf("expected exactly one reference scan, got %d: %+v", len(*calls), *calls)
	}
	got := (*calls)[0]
	if got.old != "Sudoku.Game.PuzzleNo" || got.new != "Sudoku.Game.PuzzleNumber" {
		t.Errorf("scanned for %q → %q, want the fully qualified attribute names "+
			"(Sudoku.Game.PuzzleNo → Sudoku.Game.PuzzleNumber)", got.old, got.new)
	}
	if got.dryRun {
		t.Error("the reference scan ran as a dry run, so nothing was written")
	}
}

// TestAlterEntityRenameAttributeReportsReferenceCount pins that the user is told
// how much the rename touched. A rename that silently rewrites four documents is
// as hard to review as one that rewrites none.
func TestAlterEntityRenameAttributeReportsReferenceCount(t *testing.T) {
	ctx, _, _ := renameAttrTestCtx(t, 4, "PuzzleNo")

	assertNoError(t, execAlterEntity(ctx, renameAttrStmt("PuzzleNo", "PuzzleNumber")))

	out := ctx.Output.(interface{ String() string }).String()
	if !strings.Contains(out, "4 reference") {
		t.Errorf("the rename did not report the references it updated, got: %q", out)
	}
}

// TestAlterEntityRenameAttributeWarnsAboutTextUses pins that the rename says
// what it did NOT do. Expressions and XPath constraints name an attribute in
// free text, where a bare name is only resolvable from the type of what precedes
// it, so the reference scan leaves them alone — and mxbuild then reports them as
// CE0117 / CE0161. A rename that reports only its successes reads as complete.
func TestAlterEntityRenameAttributeWarnsAboutTextUses(t *testing.T) {
	ctx, _, _ := renameAttrTestCtx(t, 0, "PuzzleNo")

	assertNoError(t, execAlterEntity(ctx, renameAttrStmt("PuzzleNo", "PuzzleNumber")))

	out := ctx.Output.(interface{ String() string }).String()
	for _, want := range []string{"expressions", "XPath", "PuzzleNo"} {
		if !strings.Contains(out, want) {
			t.Errorf("the rename does not mention %q in its note about text uses, got: %q", want, out)
		}
	}
}

// TestAlterEntityRenameAttributeReferencesAfterModel pins the ordering.
//
// The reference scan is a raw-BSON pass over every unit, the domain model
// included; UpdateEntity re-serializes the whole domain model from the parsed
// model. Scanning first and writing the model second would hand the model write
// the last word and undo the scan's edits inside the domain model — which is
// where a validation rule's attribute reference lives.
func TestAlterEntityRenameAttributeReferencesAfterModel(t *testing.T) {
	ctx, order, _ := renameAttrTestCtx(t, 1, "PuzzleNo")

	assertNoError(t, execAlterEntity(ctx, renameAttrStmt("PuzzleNo", "PuzzleNumber")))

	want := []string{"UpdateEntity", "RenameReferences"}
	if len(*order) != len(want) {
		t.Fatalf("got call order %v, want %v", *order, want)
	}
	for i := range want {
		if (*order)[i] != want[i] {
			t.Fatalf("got call order %v, want %v", *order, want)
		}
	}
}

// TestAlterEntityRenameAttributeCollision pins that renaming onto a name the
// entity already uses is refused. Without the check the two attributes become
// indistinguishable by name, and every reference to either one is rewritten to
// point at whichever the model resolves first.
func TestAlterEntityRenameAttributeCollision(t *testing.T) {
	ctx, _, calls := renameAttrTestCtx(t, 0, "PuzzleNo", "Score")

	err := execAlterEntity(ctx, renameAttrStmt("PuzzleNo", "Score"))
	if err == nil {
		t.Fatal("expected renaming onto an existing attribute name to be refused")
	}
	if !strings.Contains(err.Error(), "Score") {
		t.Errorf("the error does not name the colliding attribute: %v", err)
	}
	if len(*calls) != 0 {
		t.Errorf("references were rewritten despite the collision: %+v", *calls)
	}
}

// TestAlterEntityRenameAttributeMissingDoesNotScan pins that a rename that
// cannot find its attribute leaves the project alone.
func TestAlterEntityRenameAttributeMissingDoesNotScan(t *testing.T) {
	ctx, _, calls := renameAttrTestCtx(t, 0, "PuzzleNo")

	if err := execAlterEntity(ctx, renameAttrStmt("Nope", "Whatever")); err == nil {
		t.Fatal("expected a not-found error")
	}
	if len(*calls) != 0 {
		t.Errorf("references were rewritten for an attribute that does not exist: %+v", *calls)
	}
}
