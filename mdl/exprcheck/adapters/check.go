// SPDX-License-Identifier: Apache-2.0

package adapters

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	exprhints "github.com/mendixlabs/mxcli/mdl/exprcheck/hints"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

type CheckAdapter struct {
	parser  exprcheck.Parser
	slots   exprcheck.SlotResolver
	catalog exprcheck.CatalogReader
	source  func(ast.Expression) string
}

// Option configures a CheckAdapter.
type Option func(*CheckAdapter)

// WithSourceFunc supplies the function that recovers an expression's source
// text.
//
// The default reads only ast.SourceExpr, which the visitor produces for some
// slots and not others: measured on a create-and-change microflow, the CREATE's
// value arrived as a SourceExpr and the CHANGE's as a bare LiteralExpr, so half
// the enum-literal mistakes in one flow were invisible. Callers that can render
// an expression back to text — mdl/executor has expressionToString — should pass
// it in so coverage does not depend on which slot the visitor happened to wrap.
func WithSourceFunc(f func(ast.Expression) string) Option {
	return func(c *CheckAdapter) {
		if f != nil {
			c.source = f
		}
	}
}

func NewCheckAdapter(cat exprcheck.CatalogReader, opts ...Option) *CheckAdapter {
	c := &CheckAdapter{
		parser:  exprcheck.NewParser(),
		slots:   exprcheck.DefaultSlotResolver(),
		catalog: cat,
		source:  exprSource,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type Result struct {
	Hints []exprcheck.Hint
}

func (c *CheckAdapter) CheckMicroflow(stmt *ast.CreateMicroflowStmt) *Result {
	r := &Result{}
	if stmt == nil {
		return r
	}
	c.walkBody(stmt.Body, stmt.Name.String(), r)
	return r
}

// CheckNanoflow checks a nanoflow's expressions.
//
// A nanoflow's body is the same []ast.MicroflowStatement, and every rule here
// is about expressions rather than about which activities are legal, so the two
// share one walk. Giving nanoflows their own entry point rather than leaving
// callers to reach for CheckMicroflow keeps the asymmetry from looking
// deliberate.
func (c *CheckAdapter) CheckNanoflow(stmt *ast.CreateNanoflowStmt) *Result {
	r := &Result{}
	if stmt == nil {
		return r
	}
	c.walkBody(stmt.Body, stmt.Name.String(), r)
	return r
}

func (c *CheckAdapter) walkBody(body []ast.MicroflowStatement, mf string, r *Result) {
	scope := buildVarEntityScope(body)
	c.walkBodyWithScope(body, mf, scope, r)
}

func (c *CheckAdapter) walkBodyWithScope(body []ast.MicroflowStatement, mf string, scope map[string]string, r *Result) {
	for _, s := range body {
		switch n := s.(type) {
		case *ast.IfStmt:
			c.checkExpr(n.Condition, "IfStmt.Condition", mf, r)
			c.walkBodyWithScope(n.ThenBody, mf, scope, r)
			c.walkBodyWithScope(n.ElseBody, mf, scope, r)
		case *ast.WhileStmt:
			c.checkExpr(n.Condition, "WhileStmt.Condition", mf, r)
			c.walkBodyWithScope(n.Body, mf, scope, r)
		case *ast.LoopStmt:
			c.walkBodyWithScope(n.Body, mf, scope, r)
		case *ast.ReturnStmt:
			c.checkExpr(n.Value, "ReturnStmt.Value", mf, r)
		case *ast.DeclareStmt:
			c.checkExpr(n.InitialValue, "DeclareStmt.InitialValue", mf, r)
		case *ast.MfSetStmt:
			c.checkExpr(n.Value, "MfSetStmt.Value", mf, r)
		case *ast.LogStmt:
			c.checkExpr(n.Message, "LogStmt.Message", mf, r)
		case *ast.CreateObjectStmt:
			entityQN := n.EntityType.String()
			for _, ci := range n.Changes {
				slot := "CreateItem.Value:" + entityQN + "." + ci.Attribute
				c.checkExpr(ci.Value, slot, mf, r)
			}
		case *ast.ChangeObjectStmt:
			entityQN := scope[n.Variable]
			for _, ci := range n.Changes {
				slot := "ChangeItem.Value"
				if entityQN != "" {
					slot = "ChangeItem.Value:" + entityQN + "." + ci.Attribute
				}
				c.checkExpr(ci.Value, slot, mf, r)
			}
		case *ast.CallMicroflowStmt:
			for _, a := range n.Arguments {
				c.checkExpr(a.Value, "CallArgument.Value", mf, r)
			}
		case *ast.CallNanoflowStmt:
			for _, a := range n.Arguments {
				c.checkExpr(a.Value, "CallArgument.Value", mf, r)
			}
		}
	}
}

func (c *CheckAdapter) checkExpr(expr ast.Expression, slot, mf string, r *Result) {
	// The captured source can carry the trailing layout of the statement it was
	// lifted from ("'Open'\n  "), which the lexer would then have to recover
	// from. Trim before parsing rather than teaching every rule about it.
	src := strings.TrimSpace(c.source(expr))
	if src == "" {
		return
	}
	_, hints := c.parser.Parse(src, exprcheck.Context{
		SlotPath:  slot,
		Microflow: mf,
		Slots:     c.slots,
		Catalog:   c.catalog,
	})
	r.Hints = append(r.Hints, hints...)
}

func exprSource(expr ast.Expression) string {
	if se, ok := expr.(*ast.SourceExpr); ok {
		return se.Source
	}
	return ""
}

func (r *Result) AsViolations() []linter.Violation {
	out := make([]linter.Violation, 0, len(r.Hints))
	for _, h := range r.Hints {
		out = append(out, linter.Violation{
			RuleID:     h.Code,
			Severity:   mapSeverity(h.Severity),
			Message:    h.Problem,
			Suggestion: h.Fix,
			Location: linter.Location{
				DocumentType: "microflow",
				DocumentName: h.Where.Microflow,
			},
		})
	}
	return out
}

func mapSeverity(s exprhints.Severity) linter.Severity {
	switch s {
	case exprhints.SeverityError:
		return linter.SeverityError
	case exprhints.SeverityWarning:
		return linter.SeverityWarning
	case exprhints.SeverityInfo:
		return linter.SeverityInfo
	}
	return linter.SeverityHint
}
