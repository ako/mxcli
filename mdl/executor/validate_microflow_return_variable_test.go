// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestValidateMicroflow_ReturnsAsVariableNeedsNoExplicitReturn covers the
// MDL003 false positive.
//
// `RETURNS T AS $Var` names the return variable, and buildFlowGraph then sets
// the final EndEvent's ReturnValue to "$"+Var unconditionally — so the return
// is synthesized whether or not the body spells one out. Requiring an explicit
// RETURN on top of that flags the documented idiom as broken.
//
// It stopped being cosmetic when `mxcli exec` began refusing any script whose
// checks report an error: MDL003 is an error, so seven shipped example scripts
// could no longer run without --no-check. Measured on mxbuild 11.13.0, the very
// microflow below builds with 0 errors and `describe microflow` renders the
// synthesized `return $Found;`.
func TestValidateMicroflow_ReturnsAsVariableNeedsNoExplicitReturn(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "MF_FindByAttribute"},
		ReturnType: &ast.MicroflowReturnType{
			Type:     ast.DataType{Kind: ast.TypeBoolean},
			Variable: "Found", // RETURNS Boolean AS $Found
		},
		Body: []ast.MicroflowStatement{
			&ast.MfSetStmt{Target: "Found", Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true}},
		},
	}

	for _, v := range ValidateMicroflow(stmt) {
		if v.RuleID == "MDL003" {
			t.Fatalf("`returns … as $Found` synthesizes the return; MDL003 must not fire: %#v", v)
		}
	}
}

// The check must still fire without the AS clause — that is the case it exists
// for, and a fix that simply disabled MDL003 would pass the test above.
func TestValidateMicroflow_ReturnsWithoutVariableStillNeedsReturn(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name:       ast.QualifiedName{Module: "Sample", Name: "MF_NoReturn"},
		ReturnType: &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeBoolean}},
		Body: []ast.MicroflowStatement{
			&ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"}},
		},
	}

	found := false
	for _, v := range ValidateMicroflow(stmt) {
		if v.RuleID == "MDL003" {
			found = true
		}
	}
	if !found {
		t.Fatal("a non-void microflow with no AS clause and no return must still be flagged")
	}
}
