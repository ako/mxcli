// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// ---------------------------------------------------------------------------
// execShowFeatures
// ---------------------------------------------------------------------------

func TestShowFeatures_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not connected")
}

func TestShowFeatures_ForVersion(t *testing.T) {
	// ForVersion doesn't require connection — uses embedded registry directly.
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{ForVersion: "10.0"})
	assertNoError(t, err)
	out := buf.String()
	if len(out) == 0 {
		t.Fatal("expected output, got empty")
	}
	assertContainsStr(t, out, "Features for Mendix")
}

func TestShowFeatures_ForVersion_InvalidVersion(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{ForVersion: "not-a-version"})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "invalid version")
}

func TestShowFeatures_AddedSince(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{AddedSince: "10.0"})
	assertNoError(t, err)
	out := buf.String()
	if len(out) == 0 {
		t.Fatal("expected output, got empty")
	}
	assertContainsStr(t, out, "Features added since Mendix")
}

func TestShowFeatures_AddedSince_InvalidVersion(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{AddedSince: "xyz"})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "invalid version")
}

func TestShowFeatures_Connected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{
				ProductVersion: "10.6.0",
				MajorVersion:   10,
				MinorVersion:   6,
				PatchVersion:   0,
			}
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{})
	assertNoError(t, err)
	out := buf.String()
	if len(out) == 0 {
		t.Fatal("expected output, got empty")
	}
	assertContainsStr(t, out, "Features for Mendix")
}

func TestShowFeatures_InArea_ForVersion(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	err := execShowFeatures(ctx, &ast.ShowFeaturesStmt{ForVersion: "10.6", InArea: "domain_model"})
	assertNoError(t, err)
	// Area filter narrows output; assert header contains area name.
	assertContainsStr(t, buf.String(), "domain_model")
}

// TestCheckFeature_AgentDocumentsRefusedBelow119 exercises the gate that
// mxcli-formula1 FINDINGS §53 reported missing, at the layer a user meets it.
//
// The agent doctypes are the one version gate with no downstream safety net:
// their documents are custom blobs, so mxbuild validates nothing and a project
// below 11.9 builds green while Studio Pro cannot open the result. If this gate
// is absent the only signal the user gets is silence.
func TestCheckFeature_AgentDocumentsRefusedBelow119(t *testing.T) {
	atVersion := func(major, minor int) *mock.MockBackend {
		return &mock.MockBackend{
			IsConnectedFunc: func() bool { return true },
			ProjectVersionFunc: func() *types.ProjectVersion {
				return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor}
			},
		}
	}

	ctx, _ := newMockCtx(t, withBackend(atVersion(11, 8)))
	err := checkFeature(ctx, "agent_documents", "agent", "create agent", "upgrade")
	assertError(t, err)
	// The message must name the version and the missing module, or the user
	// cannot act: installing AgentEditorCommons is half the requirement.
	assertContainsStr(t, err.Error(), "11.9")

	ctx, _ = newMockCtx(t, withBackend(atVersion(11, 12)))
	if err := checkFeature(ctx, "agent_documents", "agent", "create agent", "upgrade"); err != nil {
		t.Errorf("11.12 satisfies the 11.9 minimum; got: %v", err)
	}
}

// TestCheckFeature_SkipsWhenTheBackendHasNoVersion guards the fail-open path.
// A backend that cannot report a version cannot be version-checked, and a gate
// exists to give an actionable error rather than to block work it cannot
// evaluate. Without the guard every gated handler panics on a nil version.
func TestCheckFeature_SkipsWhenTheBackendHasNoVersion(t *testing.T) {
	mb := &mock.MockBackend{IsConnectedFunc: func() bool { return true }} // ProjectVersion() → nil
	ctx, _ := newMockCtx(t, withBackend(mb))

	if err := checkFeature(ctx, "agent_documents", "agent", "create agent", "upgrade"); err != nil {
		t.Errorf("an unknown project version must not block execution; got: %v", err)
	}
}
