// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestValidateMicroflow_EnumSplitAllBranchesReturn(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "Route"},
		ReturnType: &ast.MicroflowReturnType{
			Type: ast.DataType{Kind: ast.TypeBoolean},
		},
		Body: []ast.MicroflowStatement{
			&ast.EnumSplitStmt{
				Variable: "Status",
				Cases: []ast.EnumSplitCase{
					{Values: []string{"Open"}, Body: []ast.MicroflowStatement{
						&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true}},
					}},
					{Values: []string{"Closed"}, Body: []ast.MicroflowStatement{
						&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: false}},
					}},
				},
			},
		},
	}

	violations := ValidateMicroflow(stmt)
	for _, v := range violations {
		if v.RuleID == "MDL003" {
			t.Fatalf("enum split with all cases returning must not trigger MDL003: %#v", v)
		}
	}
}

func TestValidateMicroflow_EnumSplitElseForbidden(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "Route"},
		Body: []ast.MicroflowStatement{
			&ast.EnumSplitStmt{
				Variable: "Status",
				Cases: []ast.EnumSplitCase{
					{Values: []string{"Open"}, Body: []ast.MicroflowStatement{
						&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true}},
					}},
				},
				ElseBody: []ast.MicroflowStatement{
					&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: false}},
				},
			},
		},
	}

	violations := ValidateMicroflow(stmt)
	for _, v := range violations {
		if v.RuleID == "MDL008" {
			return
		}
	}
	t.Fatalf("expected MDL008 for enum split with else branch, got %#v", violations)
}

// TestValidateMicroflow_EnumSplitMultipleValuesAllowed inverts what MDL009 used
// to assert. The old rule claimed "Mendix enumeration splits require exactly one
// value per branch" and errored on `when Open, Pending then`. That is wrong:
// verified on mxbuild 11.6.6, a multi-value branch covering every enum value
// (plus `(empty)`) builds with 0 errors, and the shipped write-microflows skill
// documents exactly that form. The rule rejected valid MDL.
func TestValidateMicroflow_EnumSplitMultipleValuesAllowed(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "Route"},
		Body: []ast.MicroflowStatement{
			&ast.EnumSplitStmt{
				Variable: "Status",
				Cases: []ast.EnumSplitCase{
					{Values: []string{"Open", "Pending"}, Body: []ast.MicroflowStatement{
						&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true}},
					}},
					{Values: []string{"(empty)"}, Body: []ast.MicroflowStatement{
						&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: false}},
					}},
				},
			},
		},
	}

	for _, v := range ValidateMicroflow(stmt) {
		if v.RuleID == "MDL009" {
			t.Fatalf("MDL009 rejected a multi-value branch, which Mendix accepts: %s", v.Message)
		}
	}
}

// TestValidateMicroflow_EnumSplitRequiresEmptyBranch pins what MDL009 SHOULD
// have been checking. An enumeration split needs an outgoing flow for `(empty)`
// as well as for each value; without one the build fails
//
//	CE0079 "The '(empty)' condition value should be configured in properties
//	        for an outgoing flow."
//
// Verified on mxbuild 11.6.6, and it is universal: it holds even when the split
// is on a `not null` enum attribute, so no nullability analysis is needed.
func TestValidateMicroflow_EnumSplitRequiresEmptyBranch(t *testing.T) {
	mk := func(values ...[]string) *ast.CreateMicroflowStmt {
		var cases []ast.EnumSplitCase
		for _, vals := range values {
			cases = append(cases, ast.EnumSplitCase{Values: vals, Body: []ast.MicroflowStatement{
				&ast.ReturnStmt{Value: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true}},
			}})
		}
		return &ast.CreateMicroflowStmt{
			Name: ast.QualifiedName{Module: "Sample", Name: "Route"},
			Body: []ast.MicroflowStatement{&ast.EnumSplitStmt{Variable: "Status", Cases: cases}},
		}
	}
	fires := func(stmt *ast.CreateMicroflowStmt) bool {
		for _, v := range ValidateMicroflow(stmt) {
			if v.RuleID == "MDL056" {
				return true
			}
		}
		return false
	}

	if !fires(mk([]string{"Open"}, []string{"Closed"})) {
		t.Error("expected MDL056 when no (empty) branch is present (CE0079)")
	}
	if fires(mk([]string{"Open"}, []string{"Closed"}, []string{"(empty)"})) {
		t.Error("MDL056 must not fire when an (empty) branch is present")
	}
	// The (empty) marker may share a branch with real values.
	if fires(mk([]string{"Open", "(empty)"}, []string{"Closed"})) {
		t.Error("MDL056 must not fire when (empty) shares a multi-value branch")
	}
	// Case is not significant in the marker.
	if fires(mk([]string{"Open"}, []string{"(EMPTY)"})) {
		t.Error("MDL056 must accept the (empty) marker regardless of case")
	}
}

func TestValidateMicroflow_EnumSplitBranchScopedVariable(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "Route"},
		Body: []ast.MicroflowStatement{
			&ast.EnumSplitStmt{
				Variable: "Status",
				Cases: []ast.EnumSplitCase{
					{Values: []string{"Open"}, Body: []ast.MicroflowStatement{
						&ast.DeclareStmt{Variable: "OnlyInsideCase", Type: ast.DataType{Kind: ast.TypeString}},
					}},
				},
			},
			&ast.MfSetStmt{
				Target: "OnlyInsideCase",
				Value:  &ast.LiteralExpr{Kind: ast.LiteralString, Value: "outside"},
			},
		},
	}

	violations := ValidateMicroflow(stmt)
	for _, v := range violations {
		if v.RuleID == "MDL005" {
			return
		}
	}
	t.Fatalf("expected MDL005 for variable declared inside ENUM split branch, got %#v", violations)
}

func TestValidateMicroflowBody_EnumSplitRejectsMoreThanSupportedBranches(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "Route"},
		Body: []ast.MicroflowStatement{
			enumSplitWithBranchCount(maxEnumSplitBranches + 1),
		},
	}

	errors := strings.Join(ValidateMicroflowBody(stmt), "\n")
	if !strings.Contains(errors, "enum split has 17 branches; at most 16 branches are supported") {
		t.Fatalf("expected unsupported branch count error, got %q", errors)
	}
}
