// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func boundTo(names ...string) *mock.MockPageMutator {
	return &mock.MockPageMutator{BoundPlaceholdersFunc: func() []string { return names }}
}

// The rewrite happily produces `NewLayout.HeaderLeft` for a layout with no
// HeaderLeft. mxbuild does catch it (CE1613, measured on 11.13) but at the far
// end of a build, naming the page rather than the statement — and only if
// someone builds.
func TestCheckRepointPlaceholders_RefusesABindingTheTargetDoesNotDeclare(t *testing.T) {
	op := &ast.SetLayoutOp{NewLayout: ast.QualifiedName{Module: "M", Name: "Minimal"}}
	err := checkRepointPlaceholdersAgainst(boundTo("Main", "HeaderLeft"), op, "M.Minimal", []string{"Main"})
	if err == nil {
		t.Fatal("a binding the target layout does not declare must be refused")
	}
	if !strings.Contains(err.Error(), "HeaderLeft") {
		t.Errorf("error = %q, want it to name the offending placeholder", err.Error())
	}
	// The message has to carry the remedy in the syntax that actually parses:
	// the grammar is MAP (Old AS New), and an arrow reads plausibly and does not.
	if !strings.Contains(err.Error(), " as ") {
		t.Errorf("error = %q, want it to suggest `map (Old as New)`", err.Error())
	}

	// Control: the same page against a layout that does declare it is accepted,
	// so the refusal is about the missing placeholder, not about any binding.
	if err := checkRepointPlaceholdersAgainst(boundTo("Main", "HeaderLeft"), op, "M.Full", []string{"Main", "HeaderLeft"}); err != nil {
		t.Fatalf("control failed: %v", err)
	}
}

// MAP exists precisely to rename a binding onto a placeholder the new layout
// does have, so the check has to run after it is applied.
func TestCheckRepointPlaceholders_HonoursTheMapClause(t *testing.T) {
	op := &ast.SetLayoutOp{
		NewLayout: ast.QualifiedName{Module: "M", Name: "Minimal"},
		Mappings:  map[string]string{"HeaderLeft": "Main"},
	}
	if err := checkRepointPlaceholdersAgainst(boundTo("HeaderLeft"), op, "M.Minimal", []string{"Main"}); err != nil {
		t.Fatalf("a mapped binding must be accepted: %v", err)
	}

	// A mapping onto a placeholder that also does not exist is still refused,
	// and says so as the pair — otherwise the message names a placeholder the
	// script never wrote.
	bad := &ast.SetLayoutOp{
		NewLayout: ast.QualifiedName{Module: "M", Name: "Minimal"},
		Mappings:  map[string]string{"HeaderLeft": "Nope"},
	}
	err := checkRepointPlaceholdersAgainst(boundTo("HeaderLeft"), bad, "M.Minimal", []string{"Main"})
	if err == nil {
		t.Fatal("a mapping onto a non-existent placeholder must be refused")
	}
	if !strings.Contains(err.Error(), "HeaderLeft") || !strings.Contains(err.Error(), "Nope") {
		t.Errorf("error = %q, want both sides of the mapping", err.Error())
	}
}

// A layout that reports no placeholders is one the backend could not read (MCP
// exposes none), not one that has none — CREATE LAYOUT refuses a
// placeholder-less layout. Refusing every repoint on that basis would block
// valid work on the strength of a missing read.
func TestCheckRepointPlaceholders_SkipsWhenTheTargetCannotBeRead(t *testing.T) {
	op := &ast.SetLayoutOp{NewLayout: ast.QualifiedName{Module: "M", Name: "Unknown"}}
	if err := checkRepointPlaceholdersAgainst(boundTo("Main", "Whatever"), op, "M.Unknown", nil); err != nil {
		t.Errorf("an unreadable target must skip the check, got %v", err)
	}
}

// A page whose bindings cannot be read (the same MCP gap, from the other side)
// has nothing to check against, which is not the same as having nothing wrong.
func TestCheckRepointPlaceholders_AcceptsAPageWithNoReadableBindings(t *testing.T) {
	op := &ast.SetLayoutOp{NewLayout: ast.QualifiedName{Module: "M", Name: "Minimal"}}
	if err := checkRepointPlaceholdersAgainst(boundTo(), op, "M.Minimal", []string{"Main"}); err != nil {
		t.Errorf("a page with no readable bindings must pass, got %v", err)
	}
}

// The bulk form is the one that matters: an app has one layout and many pages.
// It must skip Marketplace modules rather than refusing outright — a
// project-wide repoint that stopped dead on Administration's pages would be
// unusable — and must say which ones it skipped rather than dropping them
// silently.
func TestExecAlterPagesLayout_SkipsMarketplacePagesAndFiltersOnWhere(t *testing.T) {
	own := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-own")}, Name: "Mine"}
	mp := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-mp")}, Name: "Administration", FromAppStore: true}

	target := &pages.Layout{BaseElement: model.BaseElement{ID: model.ID("lay-new")}, ContainerID: own.ID, Name: "App_Default"}
	old := &pages.Layout{BaseElement: model.BaseElement{ID: model.ID("lay-old")}, ContainerID: own.ID, Name: "Atlas_Default"}

	onOld := &pages.Page{BaseElement: model.BaseElement{ID: model.ID("p1")}, ContainerID: own.ID, Name: "OnOld"}
	onOther := &pages.Page{BaseElement: model.BaseElement{ID: model.ID("p2")}, ContainerID: own.ID, Name: "OnOther"}
	inMarket := &pages.Page{BaseElement: model.BaseElement{ID: model.ID("p3")}, ContainerID: mp.ID, Name: "Account"}

	currentLayout := map[model.ID]string{
		onOld.ID:    "Mine.Atlas_Default",
		onOther.ID:  "Mine.Something_Else",
		inMarket.ID: "Mine.Atlas_Default",
	}
	var opened []model.ID
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{own, mp}, nil },
		ListLayoutsFunc: func() ([]*pages.Layout, error) { return []*pages.Layout{target, old}, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{onOld, onOther, inMarket}, nil },
		LayoutPlaceholdersFunc: func(id model.ID) ([]string, error) {
			return []string{"Main"}, nil
		},
		PageLayoutNameFunc: func(id model.ID) (string, error) { return currentLayout[id], nil },
		OpenPageForMutationFunc: func(id model.ID) (backend.PageMutator, error) {
			opened = append(opened, id)
			return &mock.MockPageMutator{BoundPlaceholdersFunc: func() []string { return []string{"Main"} }}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(own, mp)))

	err := execAlterPagesLayout(ctx, &ast.AlterPagesLayoutStmt{
		NewLayout:   ast.QualifiedName{Module: "Mine", Name: "App_Default"},
		WhereLayout: &ast.QualifiedName{Module: "Mine", Name: "Atlas_Default"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != onOld.ID {
		t.Errorf("opened %v, want only the page on the WHERE layout (%s)", opened, onOld.ID)
	}
	out := ctx.Output.(interface{ String() string }).String()
	if !strings.Contains(out, "Repointed page Mine.OnOld") {
		t.Errorf("output does not report the repoint:\n%s", out)
	}
	if !strings.Contains(out, "Administration.Account") || !strings.Contains(out, "marketplace") {
		t.Errorf("a skipped marketplace page must be named, not dropped silently:\n%s", out)
	}
	if strings.Contains(out, "OnOther") {
		t.Errorf("a page on a different layout must not be touched:\n%s", out)
	}
}

// Naming a layout that does not exist would otherwise match nothing and report
// "0 pages" — success, for a typo.
func TestExecAlterPagesLayout_RefusesAnUnknownWhereLayout(t *testing.T) {
	own := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-own")}, Name: "Mine"}
	target := &pages.Layout{BaseElement: model.BaseElement{ID: model.ID("lay-new")}, ContainerID: own.ID, Name: "App_Default"}
	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{own}, nil },
		ListLayoutsFunc:        func() ([]*pages.Layout, error) { return []*pages.Layout{target}, nil },
		ListPagesFunc:          func() ([]*pages.Page, error) { return nil, nil },
		LayoutPlaceholdersFunc: func(model.ID) ([]string, error) { return []string{"Main"}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(own)))

	err := execAlterPagesLayout(ctx, &ast.AlterPagesLayoutStmt{
		NewLayout:   ast.QualifiedName{Module: "Mine", Name: "App_Default"},
		WhereLayout: &ast.QualifiedName{Module: "Mine", Name: "Atlas_Defualt"},
	})
	if err == nil {
		t.Fatal("a WHERE layout that does not exist must be an error, not a 0-page success")
	}
}
