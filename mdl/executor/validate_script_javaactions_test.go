// SPDX-License-Identifier: Apache-2.0

// mxcli-chat FINDINGS §37: `mxcli check --references` reported "java action not
// found" for an action the same script had just created. Entities, microflows,
// pages, snippets and nanoflows were all exempted from the project lookup when
// defined in the script; java and JavaScript actions were not, so a script that
// created an action and then called it failed reference checking against its own
// output — a false negative with no way to silence it short of splitting the
// script in two and running it twice.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// javaActionScript is a two-statement program: create an action, then call it.
func javaActionScript(argName string) *ast.Program {
	return &ast.Program{Statements: []ast.Statement{
		&ast.CreateJavaActionStmt{
			Name:       ast.QualifiedName{Module: "MyFirstModule", Name: "ZzHelper"},
			Parameters: []ast.JavaActionParam{{Name: "Input"}},
		},
		&ast.CreateMicroflowStmt{
			Name: ast.QualifiedName{Module: "MyFirstModule", Name: "MF_UseHelper"},
			Body: []ast.MicroflowStatement{
				&ast.CallJavaActionStmt{
					ActionName: ast.QualifiedName{Module: "MyFirstModule", Name: "ZzHelper"},
					Arguments: []ast.CallArgument{
						{Name: argName, Value: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"}},
					},
				},
			},
		},
	}}
}

func TestValidate_JavaActionCreatedInScriptIsNotReportedMissing(t *testing.T) {
	ctx, _ := newMockCtx(t) // an empty project: no java actions stored

	prog := javaActionScript("Input")
	sc := newScriptContext()
	sc.collectDefinitions(prog)

	mf := prog.Statements[1].(*ast.CreateMicroflowStmt)
	if errs := validateMicroflowReferences(ctx, mf, sc); len(errs) != 0 {
		t.Fatalf("reference errors for an action created in the same script: %v", errs)
	}
}

// Exempting the action from the "not found" check must not also exempt it from
// the parameter check — the script declares the parameters, so a typo is still
// catchable and still worth catching (Mendix reports it as CE1613).
func TestValidate_JavaActionCreatedInScriptStillChecksParameterNames(t *testing.T) {
	ctx, _ := newMockCtx(t)

	prog := javaActionScript("Inputt")
	sc := newScriptContext()
	sc.collectDefinitions(prog)

	mf := prog.Statements[1].(*ast.CreateMicroflowStmt)
	errs := validateMicroflowReferences(ctx, mf, sc)
	if len(errs) != 1 || !strings.Contains(errs[0], `has no parameter "Inputt"`) {
		t.Fatalf("errors = %v, want one complaint about the misspelled parameter", errs)
	}
}

// The forward-reference hint reads allNames()/has(); both have to know about the
// new categories or a created action looks "defined later" forever.
func TestScriptContext_KnowsCodeActionsByName(t *testing.T) {
	sc := newScriptContext()
	sc.collectDefinitions(&ast.Program{Statements: []ast.Statement{
		&ast.CreateJavaActionStmt{Name: ast.QualifiedName{Module: "M", Name: "Ja"}},
		&ast.CreateJavaScriptActionStmt{Name: ast.QualifiedName{Module: "M", Name: "Jsa"}},
	}})

	for _, name := range []string{"M.Ja", "M.Jsa"} {
		if !sc.has(name) {
			t.Errorf("has(%q) = false", name)
		}
		found := false
		for _, n := range sc.allNames() {
			if n == name {
				found = true
			}
		}
		if !found {
			t.Errorf("allNames() omits %q", name)
		}
	}
}
