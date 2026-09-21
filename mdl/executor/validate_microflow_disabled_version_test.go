// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// `Microflows$ActionActivity.disabled` was introduced in 9.12.0 — mendixmodelsdk
// 4.115.0's own StructureVersionInfo, which is the arbiter for a metamodel
// property, and modelsdk/gen/microflows/version.go agrees. sdk/versions'
// Mendix 9 range starts at 9.0.0, so the window below the floor is real.
//
// This is the one gate with no downstream safety net: mxbuild tolerates a
// property it does not know, so a pre-9.12 project carrying `Disabled` builds
// green and Studio Pro throws `Sequence contains no matching element`. A build
// is not evidence here; the version floor is.
func TestDisabledActivityIsGatedAt912(t *testing.T) {
	atVersion := func(major, minor, patch int) *mock.MockBackend {
		return &mock.MockBackend{
			IsConnectedFunc: func() bool { return true },
			ProjectVersionFunc: func() *types.ProjectVersion {
				return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor, PatchVersion: patch}
			},
		}
	}
	disabledBody := []ast.MicroflowStatement{
		&ast.LogStmt{Annotations: &ast.ActivityAnnotations{Disabled: true}},
	}

	// Below the floor: refused, and the message has to name the version or the
	// reader cannot act on it.
	ctx, _ := newMockCtx(t, withBackend(atVersion(9, 11, 0)))
	err := checkDisabledActivityFeature(ctx, disabledBody)
	if err == nil {
		t.Fatal("@disabled accepted on Mendix 9.11 — the property does not exist there")
	}
	if !strings.Contains(err.Error(), "9.12") {
		t.Errorf("message does not name the version floor: %v", err)
	}

	// Controls. Without these the test passes against a gate that refuses
	// everything, or one that reads the wrong feature.
	for _, v := range [][3]int{{9, 12, 0}, {9, 24, 30}, {10, 0, 0}, {11, 12, 4}} {
		ctx, _ := newMockCtx(t, withBackend(atVersion(v[0], v[1], v[2])))
		if err := checkDisabledActivityFeature(ctx, disabledBody); err != nil {
			t.Errorf("@disabled refused on Mendix %d.%d.%d: %v", v[0], v[1], v[2], err)
		}
	}
	// A body that does not use the annotation is never gated, even below it.
	ctx, _ = newMockCtx(t, withBackend(atVersion(9, 11, 0)))
	if err := checkDisabledActivityFeature(ctx, []ast.MicroflowStatement{&ast.LogStmt{}}); err != nil {
		t.Errorf("a flow with no @disabled was gated on 9.11: %v", err)
	}
}

// The gate reads the STATEMENT, so it has to see an annotation nested inside a
// loop or a branch — those activities land in the same document.
func TestFlowBodyUsesDisabledWalksNestedBodies(t *testing.T) {
	nested := []ast.MicroflowStatement{
		&ast.LoopStmt{
			Body: []ast.MicroflowStatement{
				&ast.LogStmt{Annotations: &ast.ActivityAnnotations{Disabled: true}},
			},
		},
	}
	if !FlowBodyUsesDisabled(nested) {
		t.Error("@disabled inside a loop body was not seen; it is written into the same document")
	}
	if FlowBodyUsesDisabled([]ast.MicroflowStatement{&ast.LoopStmt{Body: []ast.MicroflowStatement{&ast.LogStmt{}}}}) {
		t.Error("a loop with no @disabled reported one")
	}
}
