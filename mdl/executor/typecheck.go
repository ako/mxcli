// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcatalog"
	"github.com/mendixlabs/mxcli/mdl/exprcheck/adapters"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// TypeCheckProgram type-checks the expressions in a script's microflows and
// nanoflows against the connected project, returning one violation per hint.
//
// This is the catalog-backed tier of PROPOSAL_expression_type_checking: the
// rules that need to know an attribute's type, an enumeration's cases, or a
// microflow's return type. The scope-local tier already runs unconditionally in
// ValidateProgram, so this only adds what a project can answer.
//
// Hints keep exprcheck's own E0xx codes rather than being remapped. They are a
// coherent, documented set with a hints registry behind them, and renaming them
// at the boundary would mean two vocabularies for one diagnostic — the code in
// the message would no longer match the code you can look up.
//
// A project that cannot be read, or a catalog that cannot be built, returns no
// violations rather than an error. Type checking is advisory: a caller that
// could not consult the project should report what it could check, not fail.
func (e *Executor) TypeCheckProgram(prog *ast.Program) []linter.Violation {
	if prog == nil || e == nil {
		return nil
	}
	ctx := e.newExecContext(context.Background())
	if !ctx.Connected() {
		return nil
	}

	// Fast mode is enough: attributes, enumeration values, microflows and their
	// parameters are all built in it. Only permissions, references, strings and
	// XPath need a full build, and none of them feed a type lookup — so a check
	// never pays for a full catalog.
	if err := ensureCatalog(ctx, false); err != nil {
		return nil
	}
	cat := ctx.Catalog
	if cat == nil {
		return nil
	}

	reader, err := exprcatalog.Load(cat.CatalogDB())
	if err != nil {
		return nil
	}

	// microflowExprSource falls back to rendering the AST when the visitor did
	// not attach source text, which it does for some slots and not others. The
	// adapter's own default reads SourceExpr only, and would silently skip
	// whichever half of a flow happened not to carry one.
	adapter := adapters.NewCheckAdapter(reader, adapters.WithSourceFunc(microflowExprSource))
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			out = append(out, adapter.CheckMicroflow(s).AsViolations()...)
		case *ast.CreateNanoflowStmt:
			out = append(out, adapter.CheckNanoflow(s).AsViolations()...)
		}
	}
	return out
}
