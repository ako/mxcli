// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// jsActionActivity runs the CALL JAVASCRIPT ACTION builder and returns the
// action it produced.
func jsActionActivity(t *testing.T, fb *flowBuilder, stmt *ast.CallJavaScriptActionStmt) *microflows.JavaScriptActionCallAction {
	t.Helper()
	id := fb.addCallJavaScriptActionAction(stmt)
	for _, obj := range fb.objects {
		if obj.GetID() != id {
			continue
		}
		activity, ok := obj.(*microflows.ActionActivity)
		if !ok {
			t.Fatalf("object = %T, want *ActionActivity", obj)
		}
		action, ok := activity.Action.(*microflows.JavaScriptActionCallAction)
		if !ok {
			t.Fatalf("action = %T, want *JavaScriptActionCallAction", activity.Action)
		}
		return action
	}
	t.Fatal("expected JavaScript action activity")
	return nil
}

// TestBuildJavaScriptAction_EntityTypeParameterEmitsEntityValue reproduces
// mendixlabs/mxcli#1137: a nanoflow calling a JavaScript action with an
// entity-type parameter (`NanoflowCommons.RefreshEntity`, whose EntityToRefresh
// is declared `entity <> not null`, i.e. a CodeActions$EntityTypeParameterType)
// was written as
//
//	"$Type": "Microflows$BasicCodeActionParameterValue",
//	"Argument": "CustomModule.BufferDefinition"
//
// where Studio Pro stores
//
//	"$Type": "Microflows$EntityTypeCodeActionParameterValue",
//	"Entity": "CustomModule.BufferDefinition"
//
// The Basic shape leaves the entity picker empty in Studio Pro. Measured on
// mxbuild 11.6.6, it is also CE0115 "The arguments that are passed to
// JavaScript action 'NanoflowCommons.RefreshEntity' do not match the expected
// parameters and need to be refreshed" — the same code finding #36 recorded for
// the microflow-typed Java action parameter. The Java-action builder has made
// this distinction since ako/mxcli#656; the JavaScript-action builder hardcoded
// Basic for every parameter.
func TestBuildJavaScriptAction_EntityTypeParameterEmitsEntityValue(t *testing.T) {
	fb := &flowBuilder{
		posX:    100,
		posY:    100,
		spacing: HorizontalSpacing,
		backend: &mock.MockBackend{
			ReadJavaScriptActionByNameFunc: func(qualifiedName string) (*types.JavaScriptAction, error) {
				if qualifiedName != "NanoflowCommons.RefreshEntity" {
					t.Fatalf("javascript action lookup = %q", qualifiedName)
				}
				return &types.JavaScriptAction{
					Name: "RefreshEntity",
					Parameters: []*types.JavaActionParameter{
						{Name: "EntityToRefresh", ParameterType: &types.EntityTypeParameterType{}},
					},
				}, nil
			},
		},
	}
	stmt := &ast.CallJavaScriptActionStmt{
		ActionName: ast.QualifiedName{Module: "NanoflowCommons", Name: "RefreshEntity"},
		Arguments: []ast.CallArgument{
			{Name: "EntityToRefresh", Value: &ast.IdentifierExpr{Name: "CustomModule.BufferDefinition"}},
		},
	}

	action := jsActionActivity(t, fb, stmt)
	if len(action.ParameterMappings) != 1 {
		t.Fatalf("parameter mappings = %d, want 1", len(action.ParameterMappings))
	}
	value, ok := action.ParameterMappings[0].Value.(*microflows.EntityTypeCodeActionParameterValue)
	if !ok {
		t.Fatalf("value = %T, want *EntityTypeCodeActionParameterValue (issue #1137: Basic leaves the Studio Pro entity picker empty)", action.ParameterMappings[0].Value)
	}
	if value.Entity != "CustomModule.BufferDefinition" {
		t.Errorf("Entity = %q, want CustomModule.BufferDefinition", value.Entity)
	}
}

// TestBuildJavaScriptAction_ConcreteEntityParameterStaysBasic is the control for
// the test above: a parameter declared with a CONCRETE entity type
// (CodeActions$EntityType, e.g. NanoflowCommons.TakePicture's `Picture:
// System.Image`) is bound to an object-valued expression, not to an entity name,
// so it keeps the Basic shape. Without this case a fix that promoted every
// entity-ish parameter would pass.
func TestBuildJavaScriptAction_ConcreteEntityParameterStaysBasic(t *testing.T) {
	fb := &flowBuilder{
		posX:    100,
		posY:    100,
		spacing: HorizontalSpacing,
		backend: &mock.MockBackend{
			ReadJavaScriptActionByNameFunc: func(qualifiedName string) (*types.JavaScriptAction, error) {
				return &types.JavaScriptAction{
					Name: "TakePicture",
					Parameters: []*types.JavaActionParameter{
						{Name: "Picture", ParameterType: &types.EntityType{Entity: "System.Image"}},
					},
				}, nil
			},
		},
	}
	stmt := &ast.CallJavaScriptActionStmt{
		ActionName: ast.QualifiedName{Module: "NanoflowCommons", Name: "TakePicture"},
		Arguments: []ast.CallArgument{
			{Name: "Picture", Value: &ast.VariableExpr{Name: "Picture"}},
		},
	}

	action := jsActionActivity(t, fb, stmt)
	value, ok := action.ParameterMappings[0].Value.(*microflows.BasicCodeActionParameterValue)
	if !ok {
		t.Fatalf("value = %T, want *BasicCodeActionParameterValue", action.ParameterMappings[0].Value)
	}
	if value.Argument != "$Picture" {
		t.Errorf("Argument = %q, want $Picture", value.Argument)
	}
}

// TestBuildJavaScriptAction_NoBackendStaysBasic pins the offline path: with no
// backend to resolve the signature (mxcli check without -p), the builder cannot
// know the parameter is entity-typed and keeps the prior Basic shape rather than
// guessing from the argument's spelling.
func TestBuildJavaScriptAction_NoBackendStaysBasic(t *testing.T) {
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing}
	stmt := &ast.CallJavaScriptActionStmt{
		ActionName: ast.QualifiedName{Module: "NanoflowCommons", Name: "RefreshEntity"},
		Arguments: []ast.CallArgument{
			{Name: "EntityToRefresh", Value: &ast.IdentifierExpr{Name: "CustomModule.BufferDefinition"}},
		},
	}

	action := jsActionActivity(t, fb, stmt)
	if _, ok := action.ParameterMappings[0].Value.(*microflows.BasicCodeActionParameterValue); !ok {
		t.Fatalf("value = %T, want *BasicCodeActionParameterValue without a backend", action.ParameterMappings[0].Value)
	}
}
