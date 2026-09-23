// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#600, the last open piece of #556. `CREATE OR REPLACE LAYOUT` was a
// DELETE followed by a CREATE with a freshly built layout, so the unit was
// replaced under a NEW GUID on every run.
//
// MEASURED on a blank 11.14.0 project, one idempotent statement, three runs:
//
//	run 1  Replaced layout …   D mprcontents/8a/a3/8aa37ee1-….mxunit  ?? mprcontents/fd/6e/
//	run 2  Replaced layout …   D mprcontents/fd/6e/fd6e9c96-….mxunit  ?? mprcontents/6d/2a/
//	run 3  Replaced layout …   3 changed files
//
// The whole .mxunit is renamed each run — a delete plus an untracked add, not
// churned bytes in a stable file — so `git status` never comes back clean.
//
// The storage-layer net from #556 cannot catch this: carryIdentityFromRemovedUnit
// keys on the unit ID and this path mints a fresh one, so there is nothing to
// reconcile the re-insert against. The fix has to be at the statement.
//
// Not a correctness bug, and the issue it came from says otherwise: #556 ties
// it to #553 ("the project stopped loading"). Measured with a real page bound
// to the churned layout, `mx check` reports 0 errors — pages resolve layouts by
// qualified NAME, not by unit GUID. The cost is reviewability.

func replaceLayoutCtx(t *testing.T, stored []*pages.Layout) (*ExecContext, *[]model.ID, *[]*pages.Layout, *[]*pages.Layout) {
	t.Helper()
	mod := &model.Module{
		BaseElement: model.BaseElement{ID: model.ID("mod-own")},
		Name:        "M",
	}
	h := mkHierarchy(mod)
	// A layout inside a folder of that module, so the foldered case resolves to
	// the module the way a real project's does.
	withContainer(h, model.ID("folder-7"), mod.ID)
	var deleted []model.ID
	var created, updated []*pages.Layout
	mb := &mock.MockBackend{
		IsConnectedFunc:  func() bool { return true },
		ListModulesFunc:  func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListLayoutsFunc:  func() ([]*pages.Layout, error) { return stored, nil },
		DeleteLayoutFunc: func(id model.ID) error { deleted = append(deleted, id); return nil },
		CreateLayoutFunc: func(l *pages.Layout) error { created = append(created, l); return nil },
		UpdateLayoutFunc: func(l *pages.Layout) error { updated = append(updated, l); return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	ctx.Output = &strings.Builder{}
	return ctx, &deleted, &created, &updated
}

func storedLayout(id, container string) *pages.Layout {
	l := &pages.Layout{Name: "App_Default"}
	l.ID = model.ID(id)
	l.ContainerID = model.ID(container)
	return l
}

func replaceStmt() *ast.CreateLayoutStmt {
	s := layoutStmt(map[string]any{"layouttype": "Responsive"}, scrollWithMain())
	s.IsReplace = true
	return s
}

func TestExecCreateLayout_RewritesTheStoredUnitInsteadOfReplacingIt(t *testing.T) {
	ctx, deleted, created, updated := replaceLayoutCtx(t, []*pages.Layout{storedLayout("lay-1", "mod-own")})

	if err := execCreateLayout(ctx, replaceStmt()); err != nil {
		t.Fatalf("execCreateLayout: %v", err)
	}

	if len(*deleted) != 0 {
		t.Errorf("the stored layout was deleted (%v) — a same-module rewrite must update the unit in place, "+
			"or it comes back under a new GUID and `git status` never clears (ako/mxcli#600)", *deleted)
	}
	if len(*created) != 0 {
		t.Errorf("CreateLayout was called for a layout that already exists (%d times)", len(*created))
	}
	if len(*updated) != 1 {
		t.Fatalf("UpdateLayout called %d times, want 1", len(*updated))
	}
	if got := (*updated)[0].ID; got != model.ID("lay-1") {
		t.Errorf("the rewrite wrote unit %q, want the stored lay-1 — a fresh id IS the bug", got)
	}
}

// The stored unit's container is preserved too. There is no FOLDER clause on
// CREATE LAYOUT, so the rebuild always sets ContainerID to the module root —
// which files a foldered layout back into the root on every rewrite, the same
// defect #932 fixed for REST clients. An in-place write does not touch the
// unit's row at all, so this comes for free; the assertion is here so it cannot
// regress if the write ever goes back through an insert.
func TestExecCreateLayout_KeepsAFolderedLayoutInItsFolder(t *testing.T) {
	ctx, _, _, updated := replaceLayoutCtx(t, []*pages.Layout{storedLayout("lay-1", "folder-7")})

	if err := execCreateLayout(ctx, replaceStmt()); err != nil {
		t.Fatalf("execCreateLayout: %v", err)
	}
	if len(*updated) != 1 {
		t.Fatalf("UpdateLayout called %d times, want 1", len(*updated))
	}
	if got := (*updated)[0].ContainerID; got != model.ID("folder-7") {
		t.Errorf("container = %q, want folder-7 — the rewrite moved the layout to the module root", got)
	}
}

// CONTROL: a layout that does NOT exist yet is still created, through the
// insert path. Without this, a fix broken into "always update" would pass the
// test above and write nothing at all for a new layout.
func TestExecCreateLayout_StillCreatesANewLayout(t *testing.T) {
	ctx, deleted, created, updated := replaceLayoutCtx(t, nil)

	if err := execCreateLayout(ctx, replaceStmt()); err != nil {
		t.Fatalf("execCreateLayout: %v", err)
	}
	if len(*created) != 1 {
		t.Fatalf("CreateLayout called %d times, want 1", len(*created))
	}
	if len(*updated) != 0 || len(*deleted) != 0 {
		t.Errorf("a brand-new layout took the rewrite path (updated=%d deleted=%d)", len(*updated), len(*deleted))
	}
	if out := ctx.Output.(*strings.Builder).String(); !strings.Contains(out, "Created layout M.App_Default") {
		t.Errorf("output %q, want it to report a create", out)
	}
}

// CONTROL: duplicates are still removed. A project that already holds two
// layouts of one name (the old delete+create could leave one behind on a failed
// run) must end up with one — the first is rewritten, the rest deleted.
func TestExecCreateLayout_RemovesDuplicatesAndKeepsTheFirst(t *testing.T) {
	ctx, deleted, created, updated := replaceLayoutCtx(t, []*pages.Layout{
		storedLayout("lay-1", "mod-own"),
		storedLayout("lay-2", "mod-own"),
	})

	if err := execCreateLayout(ctx, replaceStmt()); err != nil {
		t.Fatalf("execCreateLayout: %v", err)
	}
	if len(*updated) != 1 || (*updated)[0].ID != model.ID("lay-1") {
		t.Fatalf("updated = %v, want the first stored layout rewritten", *updated)
	}
	if len(*deleted) != 1 || (*deleted)[0] != model.ID("lay-2") {
		t.Errorf("deleted = %v, want only the duplicate lay-2", *deleted)
	}
	if len(*created) != 0 {
		t.Errorf("CreateLayout was called %d times", len(*created))
	}
}

// Without OR REPLACE / OR MODIFY an existing layout is still refused, and
// nothing is written. The rewrite path must not turn CREATE into an upsert.
func TestExecCreateLayout_StillRefusesAPlainCreateOverAnExistingLayout(t *testing.T) {
	ctx, deleted, created, updated := replaceLayoutCtx(t, []*pages.Layout{storedLayout("lay-1", "mod-own")})

	s := layoutStmt(map[string]any{"layouttype": "Responsive"}, scrollWithMain())
	if err := execCreateLayout(ctx, s); err == nil {
		t.Fatal("a plain CREATE over an existing layout must be refused")
	}
	if len(*deleted)+len(*created)+len(*updated) != 0 {
		t.Errorf("the refused statement still wrote something")
	}
}
