// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// TestApplySetProperty_RoutesNamedActionSlot is the executor half of
// mendixlabs/mxcli#995: an action written against any key other than `Action`
// is a pluggable widget's named slot, and has to reach SetWidgetNamedAction
// with the author's key — not SetWidgetProperty, which would stringify the
// action into a PrimitiveValue, and not SetWidgetAction, which writes the
// built-in click action.
func TestApplySetProperty_RoutesNamedActionSlot(t *testing.T) {
	var gotWidget, gotKey string
	var gotAction pages.ClientAction
	var wrongPath string
	m := &mock.MockPageMutator{
		SetWidgetNamedActionFunc: func(w, key string, a pages.ClientAction) error {
			gotWidget, gotKey, gotAction = w, key, a
			return nil
		},
		SetWidgetPropertyFunc: func(string, string, any) error { wrongPath = "SetWidgetProperty"; return nil },
		SetWidgetActionFunc:   func(string, pages.ClientAction) error { wrongPath = "SetWidgetAction"; return nil },
	}
	ctx, _ := newMockCtx(t)
	op := &ast.SetPropertyOp{
		Target:     ast.WidgetRef{Widget: "fileUploader1"},
		Properties: map[string]any{"createFileAction": &ast.ActionV3{Type: "save", ClosePage: true}},
	}
	if err := applySetPropertyMutator(ctx, m, op, "MyModule", model.ID("mod")); err != nil {
		t.Fatalf("set: %v", err)
	}
	if wrongPath != "" {
		t.Fatalf("the named slot went through %s", wrongPath)
	}
	if gotWidget != "fileUploader1" || gotKey != "createFileAction" {
		t.Errorf("SetWidgetNamedAction(%q, %q), want (fileUploader1, createFileAction)", gotWidget, gotKey)
	}
	if _, ok := gotAction.(*pages.SaveChangesClientAction); !ok {
		t.Errorf("action = %T, want *pages.SaveChangesClientAction — built by the CREATE PAGE action builder", gotAction)
	}
}

// Control: `set Action = …` keeps its own route to the built-in click action.
func TestApplySetProperty_ActionKeepsItsRoute(t *testing.T) {
	var named, click bool
	m := &mock.MockPageMutator{
		SetWidgetNamedActionFunc: func(string, string, pages.ClientAction) error { named = true; return nil },
		SetWidgetActionFunc:      func(string, pages.ClientAction) error { click = true; return nil },
	}
	ctx, _ := newMockCtx(t)
	op := &ast.SetPropertyOp{
		Target:     ast.WidgetRef{Widget: "btnGo"},
		Properties: map[string]any{"Action": &ast.ActionV3{Type: "save"}},
	}
	if err := applySetPropertyMutator(ctx, m, op, "MyModule", model.ID("mod")); err != nil {
		t.Fatalf("set: %v", err)
	}
	if !click || named {
		t.Errorf("SetWidgetAction called = %v, SetWidgetNamedAction called = %v; want true, false", click, named)
	}
}

// A named action slot on a column or at page level has no setter that can hold
// it — both would write the action as a scalar — so it is refused.
func TestApplySetProperty_NamedActionNeedsWidgetTarget(t *testing.T) {
	for _, target := range []ast.WidgetRef{{}, {Widget: "dg", Column: "Name"}} {
		m := &mock.MockPageMutator{}
		ctx, _ := newMockCtx(t)
		op := &ast.SetPropertyOp{
			Target:     target,
			Properties: map[string]any{"createFileAction": &ast.ActionV3{Type: "save"}},
		}
		if err := applySetPropertyMutator(ctx, m, op, "MyModule", model.ID("mod")); err == nil {
			t.Errorf("target %q: expected a refusal", target.Name())
		}
	}
}
