// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// mendixlabs/mxcli#1469: with two documents of one qualified name — one
// excluded, one live, which Mendix accepts (CE0122 counts active documents
// only) — every by-name command must resolve the same copy DESCRIBE does: the
// live one. The excluded twin is deliberately FIRST in list order, because a
// first-match loop then picks it; that is the reported "show access / grant
// view act on the excluded one" and, for DROP, deletion of the wrong page.

type twinFixture struct {
	mod    *model.Module
	mb     *mock.MockBackend
	ctx    *ExecContext
	out    *bytes.Buffer
	wrote  model.ID // UpdateAllowedRoles target
	delete model.ID // DeletePage target
}

func newTwinFixture(t *testing.T) *twinFixture {
	t.Helper()
	f := &twinFixture{mod: mkModule("MyModule")}
	f.mb = &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{f.mod}, nil },
		GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{ModuleRoles: []*security.ModuleRole{{Name: "User"}}}, nil
		},
		UpdateAllowedRolesFunc: func(id model.ID, _ []string) error { f.wrote = id; return nil },
		DeletePageFunc:         func(id model.ID) error { f.delete = id; return nil },
	}
	return f
}

func (f *twinFixture) withPages(t *testing.T) {
	excl := mkPage(f.mod.ID, "Portal")
	excl.ID = "pg-excluded"
	excl.Excluded = true
	excl.AllowedRoles = []model.ID{"MyModule.Excluded"}
	live := mkPage(f.mod.ID, "Portal")
	live.ID = "pg-live"
	live.AllowedRoles = []model.ID{"MyModule.Live"}
	f.mb.ListPagesFunc = func() ([]*pages.Page, error) { return []*pages.Page{excl, live}, nil }
	f.ctx, f.out = newMockCtx(t, withBackend(f.mb), withHierarchy(mkHierarchy(f.mod)))
}

func (f *twinFixture) withFlows(t *testing.T) {
	mfx := mkMicroflow(f.mod.ID, "Calc")
	mfx.ID, mfx.Excluded = "mf-excluded", true
	mfx.AllowedModuleRoles = []model.ID{"MyModule.Excluded"}
	mfl := mkMicroflow(f.mod.ID, "Calc")
	mfl.ID = "mf-live"
	mfl.AllowedModuleRoles = []model.ID{"MyModule.Live"}
	nfx := mkNanoflow(f.mod.ID, "Calc")
	nfx.ID, nfx.Excluded = "nf-excluded", true
	nfx.AllowedModuleRoles = []model.ID{"MyModule.Excluded"}
	nfl := mkNanoflow(f.mod.ID, "Calc")
	nfl.ID = "nf-live"
	nfl.AllowedModuleRoles = []model.ID{"MyModule.Live"}
	f.mb.ListMicroflowsFunc = func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mfx, mfl}, nil }
	f.mb.ListNanoflowsFunc = func() ([]*microflows.Nanoflow, error) { return []*microflows.Nanoflow{nfx, nfl}, nil }
	f.ctx, f.out = newMockCtx(t, withBackend(f.mb), withHierarchy(mkHierarchy(f.mod)))
}

var (
	twinPage  = ast.QualifiedName{Module: "MyModule", Name: "Portal"}
	twinFlow  = ast.QualifiedName{Module: "MyModule", Name: "Calc"}
	twinRoles = []ast.QualifiedName{{Module: "MyModule", Name: "User"}}
)

func TestIssue1469_DropPage_DeletesLiveTwin(t *testing.T) {
	f := newTwinFixture(t)
	f.withPages(t)
	if err := execDropPage(f.ctx, &ast.DropPageStmt{Name: twinPage}); err != nil {
		t.Fatal(err)
	}
	if f.delete != "pg-live" {
		t.Errorf("drop page deleted %q, want the live page %q", f.delete, "pg-live")
	}
}

func TestIssue1469_PageAccess_TargetsLiveTwin(t *testing.T) {
	t.Run("grant", func(t *testing.T) {
		f := newTwinFixture(t)
		f.withPages(t)
		if err := execGrantPageAccess(f.ctx, &ast.GrantPageAccessStmt{Page: twinPage, Roles: twinRoles}); err != nil {
			t.Fatal(err)
		}
		if f.wrote != "pg-live" {
			t.Errorf("grant view wrote %q, want the live page %q", f.wrote, "pg-live")
		}
	})
	t.Run("revoke", func(t *testing.T) {
		f := newTwinFixture(t)
		f.withPages(t)
		if err := execRevokePageAccess(f.ctx, &ast.RevokePageAccessStmt{Page: twinPage, Roles: twinRoles}); err != nil {
			t.Fatal(err)
		}
		if f.wrote != "pg-live" {
			t.Errorf("revoke view wrote %q, want the live page %q", f.wrote, "pg-live")
		}
	})
	t.Run("show access", func(t *testing.T) {
		f := newTwinFixture(t)
		f.withPages(t)
		if err := listAccessOnPage(f.ctx, &twinPage); err != nil {
			t.Fatal(err)
		}
		if got := f.out.String(); !strings.Contains(got, "MyModule.Live") || strings.Contains(got, "MyModule.Excluded") {
			t.Errorf("show access listed the excluded twin's roles:\n%s", got)
		}
	})
	t.Run("alter page lookup", func(t *testing.T) {
		f := newTwinFixture(t)
		f.withPages(t)
		pg, err := findPageByName(f.ctx, twinPage, mkHierarchy(f.mod))
		if err != nil {
			t.Fatal(err)
		}
		if pg.ID != "pg-live" {
			t.Errorf("alter page resolved %q, want the live page %q", pg.ID, "pg-live")
		}
	})
}

func TestIssue1469_FlowAccess_TargetsLiveTwin(t *testing.T) {
	cases := []struct {
		name string
		run  func(ctx *ExecContext) error
		want model.ID
	}{
		{"grant microflow", func(ctx *ExecContext) error {
			return execGrantMicroflowAccess(ctx, &ast.GrantMicroflowAccessStmt{Microflow: twinFlow, Roles: twinRoles})
		}, "mf-live"},
		{"revoke microflow", func(ctx *ExecContext) error {
			return execRevokeMicroflowAccess(ctx, &ast.RevokeMicroflowAccessStmt{Microflow: twinFlow, Roles: twinRoles})
		}, "mf-live"},
		{"grant nanoflow", func(ctx *ExecContext) error {
			return execGrantNanoflowAccess(ctx, &ast.GrantNanoflowAccessStmt{Nanoflow: twinFlow, Roles: twinRoles})
		}, "nf-live"},
		{"revoke nanoflow", func(ctx *ExecContext) error {
			return execRevokeNanoflowAccess(ctx, &ast.RevokeNanoflowAccessStmt{Nanoflow: twinFlow, Roles: twinRoles})
		}, "nf-live"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newTwinFixture(t)
			f.withFlows(t)
			if err := c.run(f.ctx); err != nil {
				t.Fatal(err)
			}
			if f.wrote != c.want {
				t.Errorf("wrote %q, want the live document %q", f.wrote, c.want)
			}
		})
	}

	for _, show := range []struct {
		name string
		fn   func(*ExecContext, *ast.QualifiedName) error
	}{{"show access microflow", listAccessOnMicroflow}, {"show access nanoflow", listAccessOnNanoflow}} {
		t.Run(show.name, func(t *testing.T) {
			f := newTwinFixture(t)
			f.withFlows(t)
			if err := show.fn(f.ctx, &twinFlow); err != nil {
				t.Fatal(err)
			}
			if got := f.out.String(); !strings.Contains(got, "MyModule.Live") || strings.Contains(got, "MyModule.Excluded") {
				t.Errorf("show access listed the excluded twin's roles:\n%s", got)
			}
		})
	}
}

// All-excluded still resolves (pickLive's fallback): dropping the only page of
// a name works even when it is excluded.
func TestIssue1469_DropPage_AllExcludedStillResolves(t *testing.T) {
	f := newTwinFixture(t)
	only := mkPage(f.mod.ID, "Portal")
	only.ID, only.Excluded = "pg-only", true
	f.mb.ListPagesFunc = func() ([]*pages.Page, error) { return []*pages.Page{only}, nil }
	f.ctx, f.out = newMockCtx(t, withBackend(f.mb), withHierarchy(mkHierarchy(f.mod)))
	if err := execDropPage(f.ctx, &ast.DropPageStmt{Name: twinPage}); err != nil {
		t.Fatal(err)
	}
	if f.delete != "pg-only" {
		t.Errorf("drop page deleted %q, want %q", f.delete, "pg-only")
	}
}
