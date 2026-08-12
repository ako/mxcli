// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// TestDescribeModule_EmitsModuleRoles covers the one part of module security that
// no other describe reaches.
//
// Entity access rules already surface in DESCRIBE ENTITY as `grant ... on ...`,
// page access in DESCRIBE PAGE as `grant view on page ...`, and microflow access
// in DESCRIBE MICROFLOW as `grant execute on microflow ...`. Module *roles* live
// in the module's own Security$ModuleSecurity unit and belong to no document, so
// before this they were invisible to any describe-based comparison — a
// marketplace update that added or renamed a role would read as no change.
func TestDescribeModule_EmitsModuleRoles(t *testing.T) {
	mod := mkModule("Administration")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleSecurityFunc: func(id model.ID) (*security.ModuleSecurity, error) {
			// Deliberately out of alphabetical order: the emitter must sort, or a
			// differ sees a phantom change when the reader's order shifts.
			return &security.ModuleSecurity{
				ModuleRoles: []*security.ModuleRole{
					{Name: "User"},
					{Name: "Administrator", Description: "Full access"},
				},
			}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeModule(ctx, "Administration", false))
	out := buf.String()

	assertContainsStr(t, out, "create module role Administration.Administrator description 'Full access';")
	assertContainsStr(t, out, "create module role Administration.User;")

	if strings.Index(out, "Administration.Administrator") > strings.Index(out, "Administration.User") {
		t.Errorf("module roles must be emitted in a stable sorted order, got:\n%s", out)
	}
}

// TestDescribeModule_RoleDescriptionQuoting guards the literal: an apostrophe in a
// description would otherwise close the string and make the output unparseable.
func TestDescribeModule_RoleDescriptionQuoting(t *testing.T) {
	mod := mkModule("Sales")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleSecurityFunc: func(id model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{
				ModuleRoles: []*security.ModuleRole{{Name: "Rep", Description: "it's theirs"}},
			}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeModule(ctx, "Sales", false))
	assertContainsStr(t, buf.String(), "description 'it''s theirs'")
}

// TestDescribeModule_SurvivesUnreadableSecurity keeps DESCRIBE MODULE working on a
// backend that cannot read module security (the MCP backend, for one) rather than
// turning a useful describe into an error.
func TestDescribeModule_SurvivesUnreadableSecurity(t *testing.T) {
	mod := mkModule("Administration")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		// GetModuleSecurityFunc left nil: the mock's default returns an error.
	}

	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeModule(ctx, "Administration", false))
	assertContainsStr(t, buf.String(), "create module Administration;")
}
