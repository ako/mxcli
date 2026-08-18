// SPDX-License-Identifier: Apache-2.0

package adapters

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

func TestCheckAdapter_ConvertsHintsToViolations(t *testing.T) {
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "M", Name: "F"},
		Body: []ast.MicroflowStatement{},
	}
	v := NewCheckAdapter(nil)
	out := v.CheckMicroflow(stmt)
	var got []linter.Violation = out.AsViolations()
	if len(got) != 0 {
		t.Errorf("empty microflow should produce 0 violations, got %d", len(got))
	}
}

// catalogStub records every AttributeKind call so the test can assert
// the adapter passed (entity, attr) precisely, not the placeholder ("","").
type catalogStub struct {
	calls []string
}

func (c *catalogStub) AttributeKind(entity, attr string) (exprcheck.TypeKind, bool) {
	c.calls = append(c.calls, entity+"|"+attr)
	return exprcheck.KindUnknown, false
}
func (c *catalogStub) AttributeEnumQN(string, string) (string, bool) { return "", false }
func (c *catalogStub) EnumCases(string) ([]string, bool)             { return nil, false }
func (c *catalogStub) MicroflowReturn(string) (exprcheck.TypeKind, bool) {
	return exprcheck.KindUnknown, false
}
func (c *catalogStub) MicroflowParam(string, string) (exprcheck.TypeKind, bool) {
	return exprcheck.KindUnknown, false
}

func TestCheckAdapter_CreateItemEmbedsEntityAttrInSlotPath(t *testing.T) {
	stub := &catalogStub{}
	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "M", Name: "F"},
		Body: []ast.MicroflowStatement{
			&ast.CreateObjectStmt{
				Variable:   "C",
				EntityType: ast.QualifiedName{Module: "Sales", Name: "Customer"},
				Changes: []ast.ChangeItem{
					{Attribute: "Status", Value: &ast.SourceExpr{Source: "'Active'"}},
				},
			},
		},
	}
	v := NewCheckAdapter(stub)
	v.CheckMicroflow(stmt)
	if len(stub.calls) == 0 {
		t.Fatalf("catalog never queried; adapter did not embed entity.attr in SlotPath")
	}
	want := "Sales.Customer|Status"
	var saw bool
	for _, c := range stub.calls {
		if c == want {
			saw = true
			break
		}
	}
	if !saw {
		t.Errorf("expected catalog query %q, got %+v", want, stub.calls)
	}
}

// TestWithSourceFuncIsUsed pins that a caller can supply the source recovery.
//
// The default reads only ast.SourceExpr, and the visitor attaches one to some
// slots and not others — measured on a fixture project, neither a CREATE's nor a
// CHANGE's enum value carried one, so the adapter saw nothing to check. A caller
// that can render the AST back to text (mdl/executor's expressionToString) must
// be able to say so.
func TestWithSourceFuncIsUsed(t *testing.T) {
	called := 0
	c := NewCheckAdapter(nil, WithSourceFunc(func(ast.Expression) string {
		called++
		return "'x'"
	}))
	r := &Result{}
	c.checkExpr(&ast.LiteralExpr{}, "DeclareStmt.InitialValue", "M.A", r)
	if called != 1 {
		t.Errorf("the supplied source function was called %d times, want 1", called)
	}
}

// TestSourceIsTrimmedBeforeParsing pins that captured layout does not reach the
// lexer. SourceExpr.Source arrives carrying the statement's trailing newline and
// indentation ("'Open'\n  "), which is the parser's problem only if we hand it
// over.
func TestSourceIsTrimmedBeforeParsing(t *testing.T) {
	var seen string
	c := NewCheckAdapter(nil, WithSourceFunc(func(ast.Expression) string {
		return "  'Open'\n  "
	}))
	c.parser = parserFunc(func(src string, ctx exprcheck.Context) (exprcheck.RobustExpr, []exprcheck.Hint) {
		seen = src
		return nil, nil
	})
	c.checkExpr(&ast.LiteralExpr{}, "CreateItem.Value", "M.A", &Result{})
	if seen != "'Open'" {
		t.Errorf("the parser received %q, want the trimmed source", seen)
	}
}

// TestEmptySourceIsSkipped pins that an expression with no recoverable text
// costs nothing — the parser is never called for it.
func TestEmptySourceIsSkipped(t *testing.T) {
	calls := 0
	c := NewCheckAdapter(nil, WithSourceFunc(func(ast.Expression) string { return "   " }))
	c.parser = parserFunc(func(src string, ctx exprcheck.Context) (exprcheck.RobustExpr, []exprcheck.Hint) {
		calls++
		return nil, nil
	})
	c.checkExpr(&ast.LiteralExpr{}, "CreateItem.Value", "M.A", &Result{})
	if calls != 0 {
		t.Errorf("the parser was called %d times for blank source, want 0", calls)
	}
}

type parserFunc func(string, exprcheck.Context) (exprcheck.RobustExpr, []exprcheck.Hint)

func (f parserFunc) Parse(src string, ctx exprcheck.Context) (exprcheck.RobustExpr, []exprcheck.Hint) {
	return f(src, ctx)
}
