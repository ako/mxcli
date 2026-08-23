// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// The GRANT path end to end: the executor statement, not just the backend
// method. #704 added AddEntityAccessRule and unit-tested it in isolation; the
// executor reaches it only after validateModuleRole reads the module's security
// document, which the MCP backend did not serve — so every GRANT --mcp aborted
// before a single PED call (#900). A test at the backend method could not see
// that, which is why this one drives the statement.
func TestGrantEntityAccess_ReachesPED(t *testing.T) {
	f := newFakePED(t, pedGrantResponder(t))
	e, out := connectExecutorToFakePED(t, f)
	defer e.Execute(&ast.DisconnectStmt{})

	if err := runMDL(t, e, "GRANT Administration.Administrator ON Administration.AccountPasswordData (READ *);"); err != nil {
		t.Fatalf("GRANT failed: %v\noutput:\n%s", err, out.String())
	}

	call, ok := f.callByName("ped_update_document")
	if !ok {
		t.Fatalf("no ped_update_document call — the statement never reached the PED write path.\ncalls: %v\noutput:\n%s",
			callNames(f), out.String())
	}
	ops, _ := json.Marshal(call.Args["operations"])
	if !strings.Contains(string(ops), "DomainModels$AccessRule") {
		t.Fatalf("ped_update_document did not add an access rule: %s", ops)
	}
	if !strings.Contains(string(ops), "Administration.Administrator") {
		t.Fatalf("access rule does not name the granted role: %s", ops)
	}
}

// pedGrantResponder scripts the fake PED for the reads and writes a GRANT makes
// against the Administration module of testdata/expr-checker/minimal.mpr.
func pedGrantResponder(t *testing.T) func(string, map[string]any) (string, bool) {
	t.Helper()
	return func(name string, args map[string]any) (string, bool) {
		switch name {
		case "ped_read_document":
			paths, _ := args["paths"].([]any)
			if len(paths) == 0 {
				return `{"results":[]}`, false
			}
			switch p, _ := paths[0].(string); p {
			case "/entities":
				return `{"results":[{"path":"/entities","result":[{"name":"Account"},{"name":"AccountPasswordData"}]}]}`, false
			case "/entities/1/accessRules":
				// AccountPasswordData ships one rule, for BOTH roles — so the
				// {Administrator} set the test grants is genuinely new.
				return `{"results":[{"path":"/entities/1/accessRules","result":[` +
					`{"moduleRoles":["Administration.Administrator","Administration.User"]}]}]}`, false
			default:
				return `{"results":[{"path":"` + p + `","result":null}]}`, false
			}
		case "ped_check_errors":
			return "No errors found.", false
		}
		return "SUCCESS: Applying operations (1)", false
	}
}

// connectExecutorToFakePED wires a real executor to an MCP backend whose PED is
// the fake server and whose local reader is a copy of the expr-checker fixture.
func connectExecutorToFakePED(t *testing.T, f *fakePED) (*executor.Executor, *bytes.Buffer) {
	t.Helper()
	path := copyFixtureProject(t)

	addr := strings.TrimPrefix(f.srv.URL, "http://")
	out := &bytes.Buffer{}
	e := executor.New(out)
	e.SetBackendFactory(func() backend.FullBackend {
		b := New(f.srv.URL+"/mcp", addr)
		b.settleDelay, b.settleWindow = 0, 0 // no error-list settle against a fake
		return b
	})
	if err := e.Execute(&ast.ConnectStmt{Path: path}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return e, out
}

// copyFixtureProject copies the shared expr-checker fixture (an MPR v2 project
// with a real Administration module) into a temp dir. It is only ever read here,
// but PED writes must not be able to reach the checked-in copy.
func copyFixtureProject(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "..", "testdata", "expr-checker")
	dst := t.TempDir()
	for _, name := range []string{"minimal.mpr"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dst, name), data, 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	if err := copyTree(filepath.Join(src, "mprcontents"), filepath.Join(dst, "mprcontents")); err != nil {
		t.Fatalf("copy mprcontents: %v", err)
	}
	return filepath.Join(dst, "minimal.mpr")
}

func copyTree(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, ent := range entries {
		s, d := filepath.Join(src, ent.Name()), filepath.Join(dst, ent.Name())
		if ent.IsDir() {
			if err := copyTree(s, d); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(s)
		if err != nil {
			return err
		}
		if err := os.WriteFile(d, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func runMDL(t *testing.T, e *executor.Executor, mdl string) error {
	t.Helper()
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", mdl, errs[0])
	}
	for _, stmt := range prog.Statements {
		if err := e.Execute(stmt); err != nil {
			return err
		}
	}
	return nil
}

func callNames(f *fakePED) []string {
	names := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		names = append(names, c.Name)
	}
	return names
}

// The security reads are a class, not one method: SHOW MODULE ROLES, SHOW USER
// ROLES and SHOW PROJECT SECURITY were unreachable over --mcp for exactly the
// same reason GRANT was, and none of them writes anything at all.
func TestSecurityReads_ServedFromLocalMpr(t *testing.T) {
	f := newFakePED(t, pedGrantResponder(t))
	e, out := connectExecutorToFakePED(t, f)
	defer e.Execute(&ast.DisconnectStmt{})

	for _, tc := range []struct{ mdl, want string }{
		{"SHOW MODULE ROLES IN Administration;", "Administrator"},
		{"SHOW USER ROLES;", "Administrator"},
		{"SHOW PROJECT SECURITY;", "Security Level"},
	} {
		out.Reset()
		if err := runMDL(t, e, tc.mdl); err != nil {
			t.Errorf("%s: %v", tc.mdl, err)
			continue
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Errorf("%s: output missing %q:\n%s", tc.mdl, tc.want, out.String())
		}
	}

	// A read must not have reached PED.
	if names := callNames(f); len(names) > 0 {
		t.Errorf("security reads issued PED calls %v; they are served from the local .mpr", names)
	}
}

// A module created over PED this session has no security document on disk. The
// bare reader answer ("not found for module mcp~module~X") reads like a bug, so
// the backend says what is actually true.
func TestGetModuleSecurity_SessionCreatedModule(t *testing.T) {
	f := newFakePED(t, pedGrantResponder(t))
	b := New(f.srv.URL+"/mcp", strings.TrimPrefix(f.srv.URL, "http://"))
	b.sessionModules = append(b.sessionModules, &model.Module{BaseElement: model.BaseElement{ID: "mcp~module~Fresh"}, Name: "Fresh"})

	_, err := b.GetModuleSecurity("mcp~module~Fresh")
	if err == nil {
		t.Fatal("expected an error for a session-created module")
	}
	for _, want := range []string{"Fresh", "created over MCP this session", "Studio Pro"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
