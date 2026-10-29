// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// ako/mxcli#1218: "A bare `create or modify module role M.R;` silently erases
// the role's description". The statement reported "Modified module role" and
// the role's Description was gone: the executor handed the statement's empty
// description to AddModuleRole, which overwrites. An omitted clause keeps what
// is stored, as an omitted user role property already did (ako/mxcli#707); a
// description clause with an empty string is how a script clears it.
//
// The statements are parsed with the real visitor, so the test covers the
// whole chain from "the statement does not say" to the value written.
func TestIssue1218_CreateOrModifyModuleRole_KeepsUnstatedDescription(t *testing.T) {
	const stored = "Read the published OData services. Nothing else."
	cases := []struct {
		name   string
		src    string
		stored *security.ModuleRole // nil: the role does not exist yet
		want   string               // the description AddModuleRole writes
	}{
		{"bare statement keeps the stored description",
			"create or modify module role Shop.ApiUser;",
			&security.ModuleRole{Name: "ApiUser", Description: stored}, stored},
		{"a stated description replaces it",
			"create or modify module role Shop.ApiUser description 'Calls the API';",
			&security.ModuleRole{Name: "ApiUser", Description: stored}, "Calls the API"},
		{"an empty description clears it",
			"create or modify module role Shop.ApiUser description '';",
			&security.ModuleRole{Name: "ApiUser", Description: stored}, ""},
		{"a casing change keeps the stored description",
			"create or modify module role Shop.apiUser;",
			&security.ModuleRole{Name: "ApiUser", Description: stored}, stored},
		{"or replace reads as or modify and keeps it too",
			"mdl 1;\ncreate or replace module role Shop.ApiUser;",
			&security.ModuleRole{Name: "ApiUser", Description: stored}, stored},
		// Adopting a role mxcli auto-provisioned takes it over: its marker
		// description is not the author's documentation, so it is not kept.
		{"adopting an auto-provisioned role drops its marker",
			"create module role Shop.ApiUser;",
			&security.ModuleRole{Name: "ApiUser", Description: autoDocumentRoleDescription}, ""},
		{"a new role without a description has none",
			"create or modify module role Shop.ApiUser;",
			nil, ""},
		{"a new role takes the stated description",
			"create module role Shop.ApiUser description 'Calls the API';",
			nil, "Calls the API"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-shop"}, Name: "Shop"}
			ms := &security.ModuleSecurity{BaseElement: model.BaseElement{ID: "ms-shop"}, ContainerID: mod.ID}
			if tc.stored != nil {
				ms.ModuleRoles = []*security.ModuleRole{tc.stored}
			}
			var written []string
			mb := &mock.MockBackend{
				IsConnectedFunc:       func() bool { return true },
				ListModulesFunc:       func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
				GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) { return ms, nil },
				AddModuleRoleFunc: func(_ model.ID, _ string, description string) error {
					written = append(written, description)
					return nil
				},
			}
			ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

			prog, errs := visitor.Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			stmt := findStmt[*ast.CreateModuleRoleStmt](t, prog, tc.src)
			if err := execCreateModuleRole(ctx, stmt); err != nil {
				t.Fatalf("exec: %v\n%s", err, buf.String())
			}
			if len(written) != 1 {
				t.Fatalf("AddModuleRole called %d time(s), want 1\n%s", len(written), buf.String())
			}
			if written[0] != tc.want {
				t.Errorf("description written = %q, want %q\n%s", written[0], tc.want, buf.String())
			}
		})
	}
}
