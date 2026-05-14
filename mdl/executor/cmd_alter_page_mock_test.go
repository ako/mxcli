// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ---------------------------------------------------------------------------
// Not connected
// ---------------------------------------------------------------------------

func TestAlterPage_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return false }}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "M", Name: "P"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not connected")
}

// ---------------------------------------------------------------------------
// Page not found
// ---------------------------------------------------------------------------

func TestAlterPage_PageNotFound(t *testing.T) {
	mod := mkModule("MyModule")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return nil, nil },
	}
	h := mkHierarchy(mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "Missing"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}

// ---------------------------------------------------------------------------
// Page happy path — SET property + Save
// ---------------------------------------------------------------------------

func TestAlterPage_SetProperty_Success(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	saved := false
	setPropCalled := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				SetWidgetPropertyFunc: func(widgetRef string, prop string, value any) error {
					setPropCalled = true
					if widgetRef != "myWidget" {
						t.Errorf("expected widgetRef myWidget, got %s", widgetRef)
					}
					if prop != "Caption" {
						t.Errorf("expected prop Caption, got %s", prop)
					}
					return nil
				},
				SaveFunc: func() error { saved = true; return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.SetPropertyOp{
				Target:     ast.WidgetRef{Widget: "myWidget"},
				Properties: map[string]any{"Caption": "Hello"},
			},
		},
	}))
	if !setPropCalled {
		t.Error("expected SetWidgetProperty to be called")
	}
	if !saved {
		t.Error("expected Save to be called")
	}
	assertContainsStr(t, buf.String(), "Altered page")
	assertContainsStr(t, buf.String(), "MyModule.TestPage")
}

// ---------------------------------------------------------------------------
// Snippet happy path
// ---------------------------------------------------------------------------

func TestAlterPage_Snippet_Success(t *testing.T) {
	mod := mkModule("MyModule")
	snp := mkSnippet(mod.ID, "TestSnippet")
	saved := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) {
			return []*pages.Snippet{snp}, nil
		},
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				SaveFunc: func() error { saved = true; return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		ContainerType: "snippet",
		PageName:      ast.QualifiedName{Module: "MyModule", Name: "TestSnippet"},
	}))
	if !saved {
		t.Error("expected Save to be called")
	}
	assertContainsStr(t, buf.String(), "Altered snippet")
}

// Issue #402 — visitor sets ContainerType to uppercase "SNIPPET"; executor
// must normalise before comparing so the snippet branch is taken.
func TestAlterPage_Snippet_UppercaseContainerType_Issue402(t *testing.T) {
	mod := mkModule("MyModule")
	snp := mkSnippet(mod.ID, "TestSnippet")
	saved := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) {
			return []*pages.Snippet{snp}, nil
		},
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				SaveFunc: func() error { saved = true; return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		ContainerType: "SNIPPET", // uppercase as produced by the AST visitor
		PageName:      ast.QualifiedName{Module: "MyModule", Name: "TestSnippet"},
	}))
	if !saved {
		t.Error("expected Save to be called")
	}
	assertContainsStr(t, buf.String(), "Altered snippet")
}

// ---------------------------------------------------------------------------
// Open mutator error
// ---------------------------------------------------------------------------

func TestAlterPage_OpenMutatorError(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return nil, fmt.Errorf("lock error")
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "open page")
}

// ---------------------------------------------------------------------------
// Save error
// ---------------------------------------------------------------------------

func TestAlterPage_SaveError(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				SaveFunc: func() error { return fmt.Errorf("disk full") },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "save")
}

// ---------------------------------------------------------------------------
// DROP widget via mutator
// ---------------------------------------------------------------------------

func TestAlterPage_DropWidget_Success(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	dropCalled := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				DropWidgetFunc: func(refs []backend.WidgetRef) error {
					dropCalled = true
					if len(refs) != 1 || refs[0].Widget != "oldWidget" {
						t.Errorf("unexpected refs: %v", refs)
					}
					return nil
				},
				SaveFunc: func() error { return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.DropWidgetOp{
				Targets: []ast.WidgetRef{{Widget: "oldWidget"}},
			},
		},
	}))
	if !dropCalled {
		t.Error("expected DropWidget to be called")
	}
	assertContainsStr(t, buf.String(), "Altered page")
}

// ---------------------------------------------------------------------------
// ADD VARIABLE
// ---------------------------------------------------------------------------

func TestAlterPage_AddVariable_Success(t *testing.T) {
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "TestPage")
	addVarCalled := false
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{
				AddVariableFunc: func(name, dataType, defaultValue string) error {
					addVarCalled = true
					if name != "MyVar" || dataType != "String" || defaultValue != "hello" {
						t.Errorf("unexpected variable: %s %s %s", name, dataType, defaultValue)
					}
					return nil
				},
				SaveFunc: func() error { return nil },
			}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execAlterPage(ctx, &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.AddVariableOp{
				Variable: ast.PageVariable{Name: "MyVar", DataType: "String", DefaultValue: "hello"},
			},
		},
	}))
	if !addVarCalled {
		t.Error("expected AddVariable to be called")
	}
	assertContainsStr(t, buf.String(), "Altered page")
}

// ---------------------------------------------------------------------------
// SET Layout on snippet — unsupported
// ---------------------------------------------------------------------------

func TestAlterPage_SetLayout_Snippet_Unsupported(t *testing.T) {
	mod := mkModule("MyModule")
	snp := mkSnippet(mod.ID, "TestSnippet")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListSnippetsFunc: func() ([]*pages.Snippet, error) {
			return []*pages.Snippet{snp}, nil
		},
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return &mock.MockPageMutator{}, nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, snp.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execAlterPage(ctx, &ast.AlterPageStmt{
		ContainerType: "snippet",
		PageName:      ast.QualifiedName{Module: "MyModule", Name: "TestSnippet"},
		Operations: []ast.AlterPageOperation{
			&ast.SetLayoutOp{
				NewLayout: ast.QualifiedName{Module: "M", Name: "L"},
			},
		},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not supported")
}

// ---------------------------------------------------------------------------
// REPLACE widget — same-name replacement must not false-positive on "duplicate"
// ---------------------------------------------------------------------------

// TestApplyReplaceWidgetMutator_SameNameAllowed verifies that replacing a widget
// with a new widget of the same name does not fail with "duplicate widget name".
// Before the fix, buildWidgetsFromAST received the full page WidgetScope (including
// the target widget), so registerWidgetName rejected the same-name replacement.
func TestApplyReplaceWidgetMutator_SameNameAllowed(t *testing.T) {
	existingID := model.ID("existing-id-123")
	replaceCalled := false

	mutator := &mock.MockPageMutator{
		FindWidgetFunc: func(name string) bool { return name == "myTitle" },
		WidgetScopeFunc: func() map[string]model.ID {
			return map[string]model.ID{"myTitle": existingID}
		},
		ParamScopeFunc: func() (map[string]model.ID, map[string]string) {
			return nil, nil
		},
		EnclosingEntityFunc: func(widgetRef string) string { return "" },
		ReplaceWidgetFunc: func(widgetRef, columnRef string, ws []pages.Widget) error {
			replaceCalled = true
			if widgetRef != "myTitle" || columnRef != "" {
				t.Errorf("unexpected refs: widgetRef=%q columnRef=%q", widgetRef, columnRef)
			}
			return nil
		},
		SaveFunc: func() error { return nil },
	}

	op := &ast.ReplaceWidgetOp{
		Target: ast.WidgetRef{Widget: "myTitle"},
		NewWidgets: []*ast.WidgetV3{
			{Name: "myTitle", Type: "title", Properties: map[string]any{"content": "New Title"}},
		},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	err := applyReplaceWidgetMutator(ctx, mutator, op, "MyModule", model.ID("mod-id"))
	if err != nil {
		t.Errorf("same-name replacement should be allowed, got: %v", err)
	}
	if !replaceCalled {
		t.Error("expected ReplaceWidget to be called")
	}
}

// ---------------------------------------------------------------------------
// ValidateAlterPage static checks
// ---------------------------------------------------------------------------

// TestValidateAlterPage_CustomContentColumnInsert ensures that inserting a
// COLUMN with nested widgets (custom content column) is flagged before
// execution — this pattern crashes MxBuild with InvalidCastException.
func TestValidateAlterPage_CustomContentColumnInsert(t *testing.T) {
	stmt := &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.InsertWidgetOp{
				Position: "AFTER",
				Target:   ast.WidgetRef{Widget: "colName"},
				Widgets: []*ast.WidgetV3{
					{
						Type: "column",
						Name: "colActions",
						Children: []*ast.WidgetV3{
							{Type: "actionbutton", Name: "btnEdit"},
						},
					},
				},
			},
		},
	}
	violations := ValidateAlterPage(stmt)
	if len(violations) == 0 {
		t.Fatal("expected a violation for custom content column INSERT, got none")
	}
	v := violations[0]
	if v.RuleID != "MDL-ALTPAGE001" {
		t.Errorf("expected rule MDL-ALTPAGE001, got %s", v.RuleID)
	}
	if !strings.Contains(v.Message, "colActions") {
		t.Errorf("violation message should mention column name 'colActions': %s", v.Message)
	}
}

// TestValidateAlterPage_AttributeColumnInsert_OK ensures that inserting a
// plain attribute column (no nested widgets) is not flagged.
func TestValidateAlterPage_AttributeColumnInsert_OK(t *testing.T) {
	stmt := &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "MyModule", Name: "TestPage"},
		Operations: []ast.AlterPageOperation{
			&ast.InsertWidgetOp{
				Position: "AFTER",
				Target:   ast.WidgetRef{Widget: "colName"},
				Widgets: []*ast.WidgetV3{
					{
						Type: "column",
						Name: "colPrice",
						Properties: map[string]any{"attribute": "Price", "caption": "Price"},
						// No Children — this is a safe attribute column
					},
				},
			},
		},
	}
	violations := ValidateAlterPage(stmt)
	if len(violations) != 0 {
		t.Errorf("expected no violations for attribute column INSERT, got: %v", violations)
	}
}
