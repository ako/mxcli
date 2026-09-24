// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Issue #572: `use fragment` inside ALTER PAGE … INSERT / REPLACE failed in exec
// with "failed to build widget SaveCancelFooter: unsupported widget type:
// USE_FRAGMENT" — fragments were expanded only on the CREATE PAGE path.

func withFragments(frags ...*ast.DefineFragmentStmt) mockCtxOption {
	return func(ctx *ExecContext) {
		ctx.Fragments = map[string]*ast.DefineFragmentStmt{}
		for _, f := range frags {
			ctx.Fragments[f.Name] = f
		}
	}
}

func saveCancelFooterFragment() *ast.DefineFragmentStmt {
	return &ast.DefineFragmentStmt{
		Name: "SaveCancelFooter",
		Widgets: []*ast.WidgetV3{{
			Type: "container", Name: "ctnFooter", Properties: map[string]any{},
			Children: []*ast.WidgetV3{{
				Type: "dynamictext", Name: "lblFooter", Properties: map[string]any{"Content": "x"},
			}},
		}},
	}
}

func useFragment(name string) *ast.WidgetV3 {
	return &ast.WidgetV3{Type: "USE_FRAGMENT", Name: name, Properties: map[string]any{}}
}

func widgetNames(ws []pages.Widget) []string {
	var out []string
	for _, w := range ws {
		out = append(out, w.GetName())
	}
	return out
}

func TestAlterPageInsertExpandsFragment(t *testing.T) {
	var got []pages.Widget
	mutator := &mock.MockPageMutator{
		EnclosingEntityForChildrenFunc: func(string) string { return "" },
		InsertWidgetFunc: func(_ string, _ string, _ backend.InsertPosition, ws []pages.Widget) error {
			got = ws
			return nil
		},
	}
	err := alterPageWith(t, mutator, &ast.InsertWidgetOp{
		Position: "INTO",
		Target:   ast.WidgetRef{Widget: "frm_dataView7"},
		Widgets:  []*ast.WidgetV3{useFragment("SaveCancelFooter")},
	}, withFragments(saveCancelFooterFragment()))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if names := widgetNames(got); len(names) != 1 || names[0] != "ctnFooter" {
		t.Fatalf("inserted widgets = %v, want [ctnFooter]", names)
	}
}

// A fragment nested inside an inserted container is expanded too.
func TestAlterPageInsertExpandsNestedFragment(t *testing.T) {
	var got []pages.Widget
	mutator := &mock.MockPageMutator{
		EnclosingEntityFunc: func(string) string { return "" },
		InsertWidgetFunc: func(_ string, _ string, _ backend.InsertPosition, ws []pages.Widget) error {
			got = ws
			return nil
		},
	}
	err := alterPageWith(t, mutator, &ast.InsertWidgetOp{
		Position: "AFTER",
		Target:   ast.WidgetRef{Widget: "txtName"},
		Widgets: []*ast.WidgetV3{{
			Type: "container", Name: "ctnOuter", Properties: map[string]any{},
			Children: []*ast.WidgetV3{useFragment("SaveCancelFooter")},
		}},
	}, withFragments(saveCancelFooterFragment()))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("inserted %d widgets, want 1", len(got))
	}
	outer, ok := got[0].(*pages.Container)
	if !ok {
		t.Fatalf("inserted %T, want *pages.Container", got[0])
	}
	if names := widgetNames(outer.Widgets); len(names) != 1 || names[0] != "ctnFooter" {
		t.Fatalf("outer children = %v, want [ctnFooter]", names)
	}
}

func TestAlterPageReplaceExpandsFragment(t *testing.T) {
	var got []pages.Widget
	mutator := &mock.MockPageMutator{
		EnclosingEntityFunc: func(string) string { return "" },
		ReplaceWidgetFunc: func(_ string, _ string, ws []pages.Widget) error {
			got = ws
			return nil
		},
	}
	err := alterPageWith(t, mutator, &ast.ReplaceWidgetOp{
		Target:     ast.WidgetRef{Widget: "ctnOldFooter"},
		NewWidgets: []*ast.WidgetV3{useFragment("SaveCancelFooter")},
	}, withFragments(saveCancelFooterFragment()))
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if names := widgetNames(got); len(names) != 1 || names[0] != "ctnFooter" {
		t.Fatalf("replacement widgets = %v, want [ctnFooter]", names)
	}
}

// The duplicate-name check must see the fragment's widgets, not the sentinel —
// otherwise a fragment that re-inserts an existing name slips past it.
func TestAlterPageInsertFragmentDuplicateName(t *testing.T) {
	mutator := &mock.MockPageMutator{
		FindWidgetFunc:                 func(name string) bool { return name == "ctnFooter" },
		EnclosingEntityForChildrenFunc: func(string) string { return "" },
		InsertWidgetFunc: func(string, string, backend.InsertPosition, []pages.Widget) error {
			return nil
		},
	}
	err := alterPageWith(t, mutator, &ast.InsertWidgetOp{
		Position: "INTO",
		Target:   ast.WidgetRef{Widget: "frm_dataView7"},
		Widgets:  []*ast.WidgetV3{useFragment("SaveCancelFooter")},
	}, withFragments(saveCancelFooterFragment()))
	if err == nil || !strings.Contains(err.Error(), "ctnFooter") {
		t.Fatalf("err = %v, want a duplicate-name error naming ctnFooter", err)
	}
}

func TestAlterPageInsertUndefinedFragment(t *testing.T) {
	mutator := &mock.MockPageMutator{
		EnclosingEntityForChildrenFunc: func(string) string { return "" },
	}
	err := alterPageWith(t, mutator, &ast.InsertWidgetOp{
		Position: "INTO",
		Target:   ast.WidgetRef{Widget: "frm_dataView7"},
		Widgets:  []*ast.WidgetV3{useFragment("Nope")},
	})
	if err == nil || strings.Contains(err.Error(), "unsupported widget type") || !strings.Contains(err.Error(), "Nope") {
		t.Fatalf("err = %v, want a not-found error naming the fragment", err)
	}
}
