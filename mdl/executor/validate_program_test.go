// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mdlast "github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func build(t *testing.T, src string) *mdlast.Program {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return prog
}

func ruleSeverities(vs []linter.Violation) map[string]linter.Severity {
	out := map[string]linter.Severity{}
	for _, v := range vs {
		out[v.RuleID] = v.Severity
	}
	return out
}

// TestValidateProgram_ReportsWidgetPropertyError is the case from the
// mxcli-banking report: a widget property the widget does not have. `mxcli check`
// reported it as an error and `mxcli exec` applied the script anyway, writing a
// page whose property was silently dropped — a picker that does nothing.
//
// Both commands now run this one function, so the error `check` prints is exactly
// the error `exec` refuses on.
func TestValidateProgram_ReportsWidgetPropertyError(t *testing.T) {
	prog := build(t, `
create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default)
{
  listview lv (datasource: database M.E) {
    combobox cmbX (label: 'X', attribute: Name, onChangeEvent: 'nope')
  }
}`)

	got := ruleSeverities(ValidateProgram(prog, ""))
	sev, found := got["MDL-WIDGET01"]
	if !found {
		t.Fatalf("no MDL-WIDGET01 for an unknown widget property; got %v", got)
	}
	if sev != linter.SeverityError {
		t.Errorf("MDL-WIDGET01 severity = %v, want error — exec only refuses on errors", sev)
	}
}

// A clean script must produce no errors, or the gate would block correct work.
func TestValidateProgram_CleanScriptHasNoErrors(t *testing.T) {
	prog := build(t, `
create module M;
create persistent entity M.Thing ( Name: String(100) );
`)
	for _, v := range ValidateProgram(prog, "") {
		if v.Severity == linter.SeverityError {
			t.Errorf("clean script produced an error: %s %s", v.RuleID, v.Message)
		}
	}
}

// TestValidateProgram_WiresEveryWholeProgramValidator is the drift guard.
//
// ValidateProgram was extracted from an inline block in cmd/mxcli/cmd_check.go so
// that `exec` could run the same checks. The failure mode it replaces is a check
// that exists but is wired into nothing — which is invisible, because a check that
// never runs reports no violations and looks like a clean project.
//
// The rule is structural rather than a hand-kept list: any exported
// Validate<Something>(prog *ast.Program, …) in this package is a whole-program
// check and must be called from ValidateProgram. Per-statement helpers
// (ValidateMicroflowBody, ValidateWidgetPropertiesForStatement, …) take a
// statement instead and are deliberately excluded — they are called from inside
// the whole-program validators and from the LSP.
func TestValidateProgram_WiresEveryWholeProgramValidator(t *testing.T) {
	body, err := os.ReadFile("validate_program.go")
	if err != nil {
		t.Fatalf("read validate_program.go: %v", err)
	}
	wired := string(body)

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0

	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") || path == "validate_program.go" {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Validate") || !fn.Name.IsExported() {
				continue
			}
			if len(fn.Type.Params.List) == 0 || !isProgramParam(fn.Type.Params.List[0].Type) {
				continue // per-statement helper, not a whole-program check
			}
			checked++
			if !strings.Contains(wired, fn.Name.Name+"(") {
				t.Errorf("%s (%s) takes a *ast.Program but ValidateProgram never calls it — "+
					"the check would never run for `mxcli check` or `mxcli exec`", fn.Name.Name, path)
			}
		}
	}

	// Guard the guard: if the signature heuristic ever stops matching anything,
	// this test would pass while checking nothing.
	if checked < 10 {
		t.Fatalf("only matched %d whole-program validators; the detection is broken", checked)
	}
}

// isProgramParam reports whether a parameter type is *ast.Program (as written in
// the executor sources, i.e. a starred selector ending in ".Program").
func isProgramParam(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Program"
}
