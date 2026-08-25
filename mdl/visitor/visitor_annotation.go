// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateAnnotationStatement handles CREATE [OR MODIFY] ANNOTATION IN Module (…).
func (b *Builder) ExitCreateAnnotationStatement(ctx *parser.CreateAnnotationStatementContext) {
	stmt := &ast.CreateAnnotationStmt{}
	if id := ctx.IdentifierOrKeyword(); id != nil {
		stmt.Module = unquoteIdentifier(id.GetText())
	}

	for _, p := range ctx.AllAnnotationProperty() {
		propCtx, ok := p.(*parser.AnnotationPropertyContext)
		if !ok {
			continue
		}
		switch {
		case propCtx.CAPTION() != nil:
			// A note is usually several lines, and an MDL string literal is
			// single-quoted and single-line, so a dollar-quoted block is accepted
			// for the caption as it is for Java and JavaScript source.
			if lit := propCtx.STRING_LITERAL(); lit != nil {
				stmt.Caption = unquoteString(lit.GetText())
			} else if dollar := propCtx.DOLLAR_STRING(); dollar != nil {
				stmt.Caption = unquoteDollarString(dollar.GetText())
			}
		case propCtx.POSITION() != nil:
			nums := propCtx.AllNUMBER_LITERAL()
			if len(nums) >= 2 {
				x, errX := strconv.Atoi(nums[0].GetText())
				y, errY := strconv.Atoi(nums[1].GetText())
				if errX == nil && errY == nil {
					stmt.Position = &ast.Position{X: x, Y: y}
				}
			}
		case propCtx.WIDTH() != nil:
			if nums := propCtx.AllNUMBER_LITERAL(); len(nums) >= 1 {
				if w, err := strconv.Atoi(nums[0].GetText()); err == nil {
					stmt.Width = w
				}
			}
		}
	}

	if parent, ok := ctx.GetParent().(*parser.CreateStatementContext); ok {
		if parent.OR() != nil && (parent.MODIFY() != nil || parent.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}

	b.statements = append(b.statements, stmt)
}
