// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1624: `describe module M with all` could not cover a large
// module. On a module of 229 microflows it hit the default 5-minute statement
// timeout, and with MXCLI_EXEC_TIMEOUT=40m it stopped at "output line limit
// exceeded ... statement aborted", truncated mid-flow. Both guards exist to
// stop a runaway statement; a module dump is bounded by the module, the way a
// catalog build (#651) is bounded by the project, so neither default applies
// to it. An explicit MXCLI_EXEC_TIMEOUT / MXCLI_OUTPUT_LINE_LIMIT still does.

var moduleDump = &ast.DescribeStmt{ObjectType: ast.DescribeModule, WithAll: true}

// executeLines runs stmt through a handler that writes n lines, as a describer
// does (ignoring Fprintf's error).
func executeLines(t *testing.T, stmt *ast.DescribeStmt, n int) (string, error) {
	t.Helper()
	var out bytes.Buffer
	e := New(&out)
	e.registry.handlers[reflect.TypeOf(&ast.DescribeStmt{})] = func(ctx *ExecContext, _ ast.Statement) error {
		for i := 0; i < n; i++ {
			fmt.Fprintf(ctx.Output, "line %d\n", i)
		}
		return nil
	}
	err := e.Execute(stmt)
	return out.String(), err
}

func TestDescribeModuleWithAll_NotCutAtTheDefaultLineLimit(t *testing.T) {
	t.Setenv("MXCLI_OUTPUT_LINE_LIMIT", "")
	n := maxOutputLines + 500

	// Control: any other statement is still cut at the default.
	if _, err := executeLines(t, &ast.DescribeStmt{ObjectType: ast.DescribeModule}, n); err == nil || !strings.Contains(err.Error(), "line limit") {
		t.Fatalf("control: a plain describe of %d lines returned %v, want the line-limit error", n, err)
	}

	out, err := executeLines(t, moduleDump, n)
	if err != nil {
		t.Fatalf("describe module … with all of %d lines failed: %v", n, err)
	}
	if got := strings.Count(out, "\n"); got != n {
		t.Errorf("describe module … with all wrote %d of %d lines", got, n)
	}
}

func TestOutputLineLimit_ExplicitSettingWins(t *testing.T) {
	t.Setenv("MXCLI_OUTPUT_LINE_LIMIT", "100")
	for _, stmt := range []*ast.DescribeStmt{moduleDump, {ObjectType: ast.DescribeMicroflow}} {
		if _, err := executeLines(t, stmt, 150); err == nil || !strings.Contains(err.Error(), "(100 lines)") {
			t.Errorf("with MXCLI_OUTPUT_LINE_LIMIT=100, 150 lines of %v returned %v, want the 100-line limit error", stmt.ObjectType, err)
		}
		if _, err := executeLines(t, stmt, 100); err != nil {
			t.Errorf("with MXCLI_OUTPUT_LINE_LIMIT=100, 100 lines of %v failed: %v", stmt.ObjectType, err)
		}
	}

	t.Setenv("MXCLI_OUTPUT_LINE_LIMIT", "0") // no limit
	if _, err := executeLines(t, &ast.DescribeStmt{ObjectType: ast.DescribeMicroflow}, maxOutputLines+500); err != nil {
		t.Errorf("with MXCLI_OUTPUT_LINE_LIMIT=0, a long describe failed: %v", err)
	}

	t.Setenv("MXCLI_OUTPUT_LINE_LIMIT", "lots") // unparseable: the default
	if _, err := executeLines(t, &ast.DescribeStmt{ObjectType: ast.DescribeMicroflow}, maxOutputLines+500); err == nil {
		t.Errorf("with an unparseable MXCLI_OUTPUT_LINE_LIMIT, a describe past the default limit succeeded")
	}
}

func TestDescribeModuleWithAll_NotUnderTheDefaultTimeout(t *testing.T) {
	t.Setenv("MXCLI_EXEC_TIMEOUT", "")
	if d := effectiveExecuteTimeout(moduleDump); d != 0 {
		t.Errorf("describe module … with all runs under a %v wall-clock limit, want none (it is bounded by the module)", d)
	}
	if d := effectiveExecuteTimeout(&ast.DescribeStmt{ObjectType: ast.DescribeModule}); d != defaultExecuteTimeout {
		t.Errorf("control: describe module without `with all` gets %v, want the default %v", d, defaultExecuteTimeout)
	}
	t.Setenv("MXCLI_EXEC_TIMEOUT", "40m")
	if d := effectiveExecuteTimeout(moduleDump); d != 40*time.Minute {
		t.Errorf("with MXCLI_EXEC_TIMEOUT=40m, describe module … with all gets %v, want 40m", d)
	}
}

// The time half of the report: the dump describes every document of the module
// one after another, and each describe listed what it needed from disk again —
// the folders for a path, every microflow to find one by name, the domain
// models. A describe writes nothing, so it now reads through the unit cache
// for its whole statement: each file at most once.
func TestDescribeModuleWithAll_ReadsEachUnitFileOnce(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	units := 0
	_ = filepath.Walk(filepath.Join(dir, "mprcontents"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".mxunit") {
			units++
		}
		return nil
	})
	before := unitFileReads(t, exec)
	if err := exec.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeModule, Name: ast.QualifiedName{Module: "Administration"}, WithAll: true}); err != nil {
		t.Fatalf("describe: %v\n%s", err, out.String())
	}
	reads := unitFileReads(t, exec) - before
	t.Logf("describe module Administration with all: %d unit-file reads; the project has %d", reads, units)
	if reads > int64(units) {
		t.Errorf("describe module … with all read %d unit files; the project has %d, so some were read more than once", reads, units)
	}
}

// listCountingBackend is the modelsdk backend, counting full microflow
// listings — each one decodes the whole flow of every microflow in the
// project. Embedding the concrete type keeps its optional lookups.
type listCountingBackend struct {
	*modelsdkbackend.Backend
	listMicroflows int
}

func (b *listCountingBackend) ListMicroflows() ([]*microflows.Microflow, error) {
	b.listMicroflows++
	return b.Backend.ListMicroflows()
}

// Where the dump's time actually went, by CPU profile: describing a microflow
// derives its layout by rebuilding it, and the rebuild runs through a read
// memo that hid the backend's by-name lookup, so every described flow decoded
// every microflow of the project — quadratic in the module, linear in the
// project. 229 generated flows took 20 s on pedapp, 63% of it in that decode.
func TestDescribeModuleWithAll_DoesNotDecodeEveryMicroflowPerFlow(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const flows = 8
	var sb strings.Builder
	sb.WriteString("mdl 1;\ncreate module Dump;\n")
	for i := 1; i <= flows; i++ {
		fmt.Fprintf(&sb, "create microflow Dump.MF_%d ($N: Integer) returns Boolean as $r\nbegin\n  if $N > %d then\n    return true;\n  end if;\n  return false;\nend;\n", i, i)
	}
	if err := agreeExec(t, exec, sb.String()); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	counting := &listCountingBackend{Backend: modelsdkbackend.New()}
	e := New(io.Discard)
	e.SetBackendFactory(func() backend.FullBackend { return counting })
	if err := e.Execute(&ast.ConnectStmt{Path: filepath.Join(dir, "PedApp.mpr")}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = e.Execute(&ast.DisconnectStmt{}) })
	if err := e.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeModule, Name: ast.QualifiedName{Module: "Dump"}, WithAll: true}); err != nil {
		t.Fatalf("describe: %v", err)
	}
	if counting.listMicroflows >= flows {
		t.Errorf("describing a module of %d microflows listed (and decoded) every microflow of the project %d times", flows, counting.listMicroflows)
	}
}
