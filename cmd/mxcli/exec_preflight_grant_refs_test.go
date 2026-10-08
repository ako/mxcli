// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1333: a valid grant followed by one on a missing entity and
// one to a missing module role. `check --references` printed "Check passed!",
// and exec wrote the first grant, then stopped with "entity not found:
// MyModule.NoSuchEntity" — leaving the first rule in the model. The pre-flight
// must refuse the whole script, naming both, before anything is written.
func TestExecPreflightRefusesGrantNamingMissingEntityOrRole(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	raw, err := os.ReadFile(filepath.Join(src, "PedApp.mpr"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PedApp.mpr"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(dir, "mprcontents"), os.DirFS(filepath.Join(src, "mprcontents"))); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dir, "PedApp.mpr")
	exec := executor.New(io.Discard)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: mpr}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Execute(&ast.DisconnectStmt{}) })

	run := func(script string) (string, string) {
		t.Helper()
		prog, errs := visitor.Build(script)
		if len(errs) > 0 {
			t.Fatal(errs[0])
		}
		var out bytes.Buffer
		refusal := execPreflight(exec, prog, mpr, "s.mdl", false, false, false, deprecation.Warn, &out, false)
		return refusal, out.String()
	}

	refusal, out := run(`mdl 1;
grant create, delete on entity Administration.Account to Administration.User;
grant create on entity Administration.NoSuchEntity to Administration.User;
grant create on entity Administration.Account to Administration.NoSuchRole;
`)
	if refusal == "" || !strings.Contains(refusal, "Nothing was written") {
		t.Fatalf("want the pre-flight to refuse the script, got %q\n%s", refusal, out)
	}
	for _, want := range []string{
		"entity Administration.NoSuchEntity does not exist",
		"module role Administration.NoSuchRole does not exist",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("pre-flight output must name %q:\n%s", want, out)
		}
	}

	// Control: the valid grant on its own runs.
	if refusal, out := run("mdl 1;\ngrant create, delete on entity Administration.Account to Administration.User;\n"); refusal != "" {
		t.Errorf("a grant naming a stored entity and role was refused: %s\n%s", refusal, out)
	}
}
