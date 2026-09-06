// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// `UPDATE SECURITY` returned on the first module it could not reconcile, and one
// module always can: System, whose domain model is SYNTHESIZED rather than
// stored. So the command died there having reconciled nothing, on every MPR v2
// project (mendixlabs/mxcli#1047):
//
//	Error: failed to reconcile security for module System: load domain model
//	00000000-…-002: …/mprcontents/00/00/…002.mxunit: no such file or directory

// updateSecurityFixture builds four modules and records which ones actually got
// reconciled. Reconciling `Broken` fails, the way System's does in a real
// project.
func updateSecurityFixture(t *testing.T) (*ExecContext, *[]string, *bytes.Buffer) {
	t.Helper()
	var mods []*model.Module
	dms := map[model.ID]*domainmodel.DomainModel{}
	// Order matters: System and Broken sit BEFORE Zulu, so a run that stops at
	// the first failure never reaches it. That ordering is the regression.
	for _, name := range []string{"Alpha", "System", "Broken", "Zulu"} {
		m := &model.Module{Name: name}
		m.ID = nextID("mod" + name)
		mods = append(mods, m)
		dm := &domainmodel.DomainModel{ContainerID: m.ID}
		dm.ID = nextID("dm" + name)
		dms[m.ID] = dm
	}

	var reconciled []string
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return mods, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if dm, ok := dms[id]; ok {
				return dm, nil
			}
			return nil, fmt.Errorf("no domain model %s", id)
		},
		ReconcileMemberAccessesFunc: func(_ model.ID, moduleName string) (int, error) {
			if moduleName == "Broken" {
				return 0, fmt.Errorf("load domain model: no such file or directory")
			}
			reconciled = append(reconciled, moduleName)
			return 1, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	return ctx, &reconciled, buf
}

// THE REGRESSION. One module that cannot be read must not end the run, or the
// command is inert on every project that has such a module — which is every
// project, because of System.
func TestUpdateSecurity_OneUnreadableModuleDoesNotEndTheRun(t *testing.T) {
	ctx, reconciled, buf := updateSecurityFixture(t)

	if err := execUpdateSecurity(ctx, &ast.UpdateSecurityStmt{}); err != nil {
		t.Fatalf("a single unreadable module aborted the whole run: %v", err)
	}
	// Zulu sits after both the skipped and the failing module.
	if got := strings.Join(*reconciled, ","); got != "Alpha,Zulu" {
		t.Errorf("reconciled %q, want Alpha,Zulu — System is skipped, Broken fails, "+
			"and neither may stop the modules after them", got)
	}
	// A silent skip is how "up to date" comes to mean "not looked at".
	if !strings.Contains(buf.String(), "Skipped Broken") {
		t.Errorf("the skipped module was not reported: %q", buf.String())
	}
}

// System is skipped by name rather than left to fail: reconciling it is not
// merely impossible (its domain model is not stored) but wrong — its entities
// are the platform's and its access rules are not the project's to rewrite.
func TestUpdateSecurity_SkipsSystem(t *testing.T) {
	ctx, reconciled, _ := updateSecurityFixture(t)
	if err := execUpdateSecurity(ctx, &ast.UpdateSecurityStmt{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, name := range *reconciled {
		if name == "System" {
			t.Error("System was reconciled — its access rules are Mendix's, not the project's")
		}
	}
}

// Naming System explicitly is a mistake worth reporting. A silent skip would
// report success having done nothing, which is the failure mode this whole fix
// is about.
func TestUpdateSecurity_NamingSystemIsRefused(t *testing.T) {
	ctx, _, _ := updateSecurityFixture(t)
	err := execUpdateSecurity(ctx, &ast.UpdateSecurityStmt{Module: "System"})
	if err == nil {
		t.Fatal("naming System was accepted")
	}
	if !strings.Contains(err.Error(), "platform") {
		t.Errorf("the error should say why, not just refuse: %v", err)
	}
}

// A typo used to match no module, reconcile nothing, and print "All entity
// access rules are up to date" — a success message for a run that did nothing.
func TestUpdateSecurity_UnknownModuleIsRefused(t *testing.T) {
	ctx, _, buf := updateSecurityFixture(t)
	err := execUpdateSecurity(ctx, &ast.UpdateSecurityStmt{Module: "Nope"})
	if err == nil {
		t.Fatal("an unknown module was accepted")
	}
	if !strings.Contains(err.Error(), "Nope") {
		t.Errorf("the error should quote the name as typed: %v", err)
	}
	if strings.Contains(buf.String(), "up to date") {
		t.Errorf("a run that matched nothing reported success: %q", buf.String())
	}
}

// Scoping must actually scope, and match the way Mendix resolves module names.
func TestUpdateSecurity_ScopeIsHonouredAndCaseInsensitive(t *testing.T) {
	ctx, reconciled, _ := updateSecurityFixture(t)
	if err := execUpdateSecurity(ctx, &ast.UpdateSecurityStmt{Module: "alpha"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(*reconciled, ","); got != "Alpha" {
		t.Errorf("reconciled %q, want just Alpha", got)
	}
}

// CONTROL: with nothing skipped and nothing changed, the run still says so.
// Without this a fix that always printed a skip line would pass the tests above.
func TestUpdateSecurity_ReportsAnUpToDateProject(t *testing.T) {
	mod := &model.Module{Name: "Alpha"}
	mod.ID = nextID("modAlpha")
	dm := &domainmodel.DomainModel{ContainerID: mod.ID}
	dm.ID = nextID("dmAlpha")

	mb := &mock.MockBackend{
		IsConnectedFunc:             func() bool { return true },
		ListModulesFunc:             func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc:          func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ReconcileMemberAccessesFunc: func(model.ID, string) (int, error) { return 0, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb))

	if err := execUpdateSecurity(ctx, &ast.UpdateSecurityStmt{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "up to date") {
		t.Errorf("an up-to-date project was not reported as such: %q", buf.String())
	}
	if strings.Contains(buf.String(), "Skipped") {
		t.Errorf("nothing was skipped, but a skip was reported: %q", buf.String())
	}
}

// `update security RestLab` — without IN — used to reach the parser's error
// recovery, which consumed the module name silently: the statement parsed as one
// statement with no error and the run went PROJECT-WIDE. A scope the author
// asked for and did not get is worse than a parse error, so IN is now optional
// before the name rather than before the whole clause (mendixlabs/mxcli#1047).
func TestUpdateSecurity_BareModuleNameScopes(t *testing.T) {
	for _, src := range []string{
		"update security RestLab;",
		"update security in RestLab;",
	} {
		t.Run(src, func(t *testing.T) {
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			if len(prog.Statements) != 1 {
				t.Fatalf("got %d statements, want 1", len(prog.Statements))
			}
			s, ok := prog.Statements[0].(*ast.UpdateSecurityStmt)
			if !ok {
				t.Fatalf("statement is %T, want *ast.UpdateSecurityStmt", prog.Statements[0])
			}
			if s.Module != "RestLab" {
				t.Errorf("Module = %q, want RestLab — the scope was dropped, so the "+
					"run would touch every module in the project", s.Module)
			}
		})
	}

	// CONTROL: no name still means the whole project.
	prog, errs := visitor.Build("update security;")
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if s := prog.Statements[0].(*ast.UpdateSecurityStmt); s.Module != "" {
		t.Errorf("Module = %q, want empty for an unscoped run", s.Module)
	}
}
