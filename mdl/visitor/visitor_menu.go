// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateMenuStatement handles CREATE [OR MODIFY] MENU Module.Name ( items ).
//
// The items reuse navMenuItemDef, the same rule CREATE NAVIGATION's MENU block
// uses, so buildNavMenuItemDef is reused verbatim — a menu item is written the
// same way wherever it appears.
func (b *Builder) ExitCreateMenuStatement(ctx *parser.CreateMenuStatementContext) {
	qn := ctx.QualifiedName()
	if qn == nil {
		return
	}

	stmt := &ast.CreateMenuStmt{Name: buildQualifiedName(qn)}
	// A menu's doc comment was parsed and then read by nobody, so the
	// documentation never reached the model at all — a write gap rather than
	// the rewrite gap of #1018, found while testing the latter.
	stmt.Documentation, stmt.DocumentationSet = findDocComment(ctx)
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteString(lit.GetText())
	}
	for _, itemCtx := range ctx.AllNavMenuItemDef() {
		stmt.Items = append(stmt.Items, buildNavMenuItemDef(itemCtx))
	}

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}

	b.statements = append(b.statements, stmt)
}
