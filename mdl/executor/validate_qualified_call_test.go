// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func qualifiedCall(name string) ast.Expression {
	return &ast.FunctionCallExpr{
		Name: name,
		Arguments: []ast.Expression{&ast.BinaryExpr{
			Left:     &ast.IdentifierExpr{Name: "IsActive"},
			Operator: "=",
			Right:    &ast.VariableExpr{Name: "IsActive"},
		}},
	}
}

func mdl066(vs []linter.Violation) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == "MDL066" {
			out = append(out, v)
		}
	}
	return out
}

// A rule (or microflow, or Java action) call in a VALUE position is CE0117
// "Error(s) in expression" on mxbuild — measured on 11.13.0 for both a rule and
// a microflow, and on both engines, because a Mendix expression has no
// user-callable functions at all. mxcli used to write the literal text and say
// nothing (upstream #939).
func TestQualifiedCallInValuePositionIsAnError(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "MF"},
		Body: []ast.MicroflowStatement{
			&ast.DeclareStmt{
				Variable:     "Active",
				Type:         ast.DataType{Kind: ast.TypeBoolean},
				InitialValue: qualifiedCall("Sample.Rule_IsActive"),
			},
		},
	}
	vs := mdl066(ValidateMicroflow(stmt))
	if len(vs) != 1 {
		t.Fatalf("got %d MDL066 violations, want 1: %+v", len(vs), vs)
	}
	if vs[0].Severity != linter.SeverityError {
		t.Errorf("severity = %v, want error — the build fails", vs[0].Severity)
	}
	if !strings.Contains(vs[0].Message, "CE0117") || !strings.Contains(vs[0].Message, "Sample.Rule_IsActive") {
		t.Errorf("message should name the call and the build error, got %q", vs[0].Message)
	}
	if !strings.Contains(vs[0].Suggestion, "CALL MICROFLOW") {
		t.Errorf("suggestion should name the working spelling, got %q", vs[0].Suggestion)
	}
}

// The one position where a bare qualified call IS valid MDL: a decision, which
// mxcli stores as a Microflows$RuleSplitCondition. Flagging it here would reject
// the form the fix for #939 exists to write.
func TestQualifiedCallInIfConditionIsNotFlagged(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "MF"},
		Body: []ast.MicroflowStatement{
			&ast.IfStmt{
				Condition: qualifiedCall("Sample.Rule_IsActive"),
				ThenBody:  []ast.MicroflowStatement{&ast.ReturnStmt{}},
				ElseBody:  []ast.MicroflowStatement{&ast.ReturnStmt{}},
			},
		},
	}
	if vs := mdl066(ValidateMicroflow(stmt)); len(vs) != 0 {
		t.Errorf("an if condition is the legal position for a rule call, got %+v", vs)
	}
}

// A while condition has no rule-split form — Mendix loops take a boolean
// expression or a list — so the same call there is not legal. The while body was
// not walked at all before, so this also covers value positions inside one.
func TestQualifiedCallInWhileIsAnError(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "MF"},
		Body: []ast.MicroflowStatement{
			&ast.WhileStmt{
				Condition: qualifiedCall("Sample.Rule_IsActive"),
				Body: []ast.MicroflowStatement{
					&ast.DeclareStmt{
						Variable:     "N",
						Type:         ast.DataType{Kind: ast.TypeInteger},
						InitialValue: qualifiedCall("Sample.MF_Callee"),
					},
				},
			},
		},
	}
	vs := mdl066(ValidateMicroflow(stmt))
	if len(vs) != 2 {
		t.Fatalf("got %d MDL066 violations, want 2 (the condition and the body), %+v", len(vs), vs)
	}
}

// A built-in call is unqualified and must stay untouched — no Mendix expression
// function has a dot in its name, which is what makes the rule project-less.
func TestBuiltinCallIsNotFlagged(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "MF"},
		Body: []ast.MicroflowStatement{
			&ast.DeclareStmt{
				Variable: "N",
				Type:     ast.DataType{Kind: ast.TypeInteger},
				InitialValue: &ast.FunctionCallExpr{
					Name:      "length",
					Arguments: []ast.Expression{&ast.VariableExpr{Name: "Name"}},
				},
			},
		},
	}
	if vs := mdl066(ValidateMicroflow(stmt)); len(vs) != 0 {
		t.Errorf("length() is a built-in, got %+v", vs)
	}
}

// The decision position needs the project to judge: only a RULE can be called
// there. When the name resolves to something else the builder used to fall back
// to an ExpressionSplitCondition holding the call text — valid-looking MDL that
// is CE0117 on mxbuild — so it refuses instead.
func TestIfConditionCallingANonRuleIsRefused(t *testing.T) {
	mb := &mock.MockBackend{
		IsRuleFunc: func(string) (bool, error) { return false, nil },
	}
	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing, backend: mb,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
	}
	fb.buildFlowGraph([]ast.MicroflowStatement{&ast.IfStmt{
		Condition: qualifiedCall("Sample.MF_Callee"),
		ThenBody:  []ast.MicroflowStatement{&ast.ReturnStmt{}},
		ElseBody:  []ast.MicroflowStatement{&ast.ReturnStmt{}},
	}}, nil)

	errs := fb.GetErrors()
	if len(errs) == 0 {
		t.Fatal("a non-rule call in a decision was accepted; it is written as an " +
			"expression and fails the build with CE0117")
	}
	if !strings.Contains(errs[0], "Sample.MF_Callee") || !strings.Contains(errs[0], "CE0117") {
		t.Errorf("error should name the call and the build failure, got %q", errs[0])
	}

	// Control: the same shape with a real rule builds a RuleSplitCondition and
	// reports nothing.
	ok := &mock.MockBackend{IsRuleFunc: func(string) (bool, error) { return true, nil }}
	fb2 := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing, backend: ok,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
	}
	fb2.buildFlowGraph([]ast.MicroflowStatement{&ast.IfStmt{
		Condition: qualifiedCall("Sample.Rule_IsActive"),
		ThenBody:  []ast.MicroflowStatement{&ast.ReturnStmt{}},
		ElseBody:  []ast.MicroflowStatement{&ast.ReturnStmt{}},
	}}, nil)
	if errs := fb2.GetErrors(); len(errs) != 0 {
		t.Fatalf("a real rule call must be accepted, got %v", errs)
	}
	var found bool
	for _, obj := range fb2.objects {
		if sp, ok := obj.(*microflows.ExclusiveSplit); ok {
			if _, isRule := sp.SplitCondition.(*microflows.RuleSplitCondition); isRule {
				found = true
			}
		}
	}
	if !found {
		t.Error("control: the rule call did not produce a RuleSplitCondition")
	}
}
