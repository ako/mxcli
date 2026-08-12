// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// alterPageActionCtx builds the mock plumbing shared by the tests below and
// returns the statement runner plus a pointer to the action the mutator saw.
func alterPageActionCtx(t *testing.T, mutErr error) (func(*ast.ActionV3) error, *pages.ClientAction) {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	// The CREATE PAGE builder resolves a microflow action against the backend,
	// so the mock has to hold one — that resolution is the delegation working.
	mf := mkMicroflow(mod.ID, "ACT_Other")
	var got pages.ClientAction

	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:    func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:      func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				SetWidgetActionFunc: func(widgetRef string, action pages.ClientAction) error {
					if widgetRef != "btnSave" {
						t.Errorf("widgetRef = %q, want btnSave", widgetRef)
					}
					got = action
					return mutErr
				},
				SaveFunc: func() error { return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	withContainer(h, mf.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	return func(action *ast.ActionV3) error {
		return execAlterPage(ctx, &ast.AlterPageStmt{
			PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
			Operations: []ast.AlterPageOperation{
				&ast.SetPropertyOp{
					Target:     ast.WidgetRef{Widget: "btnSave"},
					Properties: map[string]any{"Action": action},
				},
			},
		})
	}, &got
}

// TestAlterPage_SetAction_RoutesToMutator pins that `SET Action` reaches
// SetWidgetAction rather than being written through SetWidgetProperty as a
// scalar. An action is a polymorphic node; storing it as a value would produce a
// document Studio Pro cannot open.
func TestAlterPage_SetAction_RoutesToMutator(t *testing.T) {
	run, got := alterPageActionCtx(t, nil)
	assertNoError(t, run(&ast.ActionV3{Type: "microflow", Target: "MyModule.ACT_Other"}))

	if *got == nil {
		t.Fatal("SetWidgetAction was not called")
	}
	mf, ok := (*got).(*pages.MicroflowClientAction)
	if !ok {
		t.Fatalf("action type = %T, want *pages.MicroflowClientAction", *got)
	}
	if mf.MicroflowName != "MyModule.ACT_Other" {
		t.Errorf("Microflow = %q, want MyModule.ACT_Other", mf.MicroflowName)
	}
}

// TestAlterPage_SetAction_DelegatesToCreatePageBuilder covers the point of the
// change: SET builds through the CREATE PAGE builder, so every action form works
// here without a second switch to keep in sync. `SAVE_CHANGES CLOSE_PAGE` is the
// interesting one — the close is a flag on the action, not a separate action.
func TestAlterPage_SetAction_DelegatesToCreatePageBuilder(t *testing.T) {
	tests := []struct {
		name   string
		action *ast.ActionV3
		verify func(t *testing.T, a pages.ClientAction)
	}{
		{
			name:   "save and close",
			action: &ast.ActionV3{Type: "save", ClosePage: true},
			verify: func(t *testing.T, a pages.ClientAction) {
				s, ok := a.(*pages.SaveChangesClientAction)
				if !ok {
					t.Fatalf("type = %T", a)
				}
				if !s.ClosePage {
					t.Error("ClosePage = false, want true")
				}
			},
		},
		{
			name:   "save without close",
			action: &ast.ActionV3{Type: "save"},
			verify: func(t *testing.T, a pages.ClientAction) {
				s, ok := a.(*pages.SaveChangesClientAction)
				if !ok {
					t.Fatalf("type = %T", a)
				}
				if s.ClosePage {
					t.Error("ClosePage = true, want false")
				}
			},
		},
		{
			name:   "close page",
			action: &ast.ActionV3{Type: "close"},
			verify: func(t *testing.T, a pages.ClientAction) {
				if _, ok := a.(*pages.ClosePageClientAction); !ok {
					t.Fatalf("type = %T, want *pages.ClosePageClientAction", a)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, got := alterPageActionCtx(t, nil)
			assertNoError(t, run(tt.action))
			if *got == nil {
				t.Fatal("SetWidgetAction was not called")
			}
			tt.verify(t, *got)
		})
	}
}

// TestAlterPage_SetAction_RejectsNonAction guards the type assertion: a scalar
// where an action expression belongs must be a clear error, not a panic.
func TestAlterPage_SetAction_RejectsNonAction(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{SaveFunc: func() error { return nil }}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	err := execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.SetPropertyOp{
				Target:     ast.WidgetRef{Widget: "btnSave"},
				Properties: map[string]any{"Action": "not-an-action"},
			},
		},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "must be an action expression")
}
