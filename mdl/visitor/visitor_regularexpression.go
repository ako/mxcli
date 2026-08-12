// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateRegularExpressionStatement builds a CreateRegularExpressionStmt from
// CREATE [OR REPLACE|MODIFY] REGULAR EXPRESSION Module.Name ( ... ).
func (b *Builder) ExitCreateRegularExpressionStatement(ctx *parser.CreateRegularExpressionStatementContext) {
	stmt := &ast.CreateRegularExpressionStmt{
		Name:          buildQualifiedName(ctx.QualifiedName()),
		Documentation: findDocCommentText(ctx),
	}
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}

	if body := ctx.RegularExpressionBody(); body != nil {
		bodyCtx := body.(*parser.RegularExpressionBodyContext)
		for _, prop := range bodyCtx.AllRegularExpressionProperty() {
			pc, ok := prop.(*parser.RegularExpressionPropertyContext)
			if !ok || pc == nil {
				continue
			}
			iok := pc.IdentifierOrKeyword(0)
			if iok == nil {
				continue
			}
			switch strings.ToLower(identifierOrKeywordText(iok)) {
			case "expression", "pattern":
				stmt.Expression = regularExpressionPropertyText(pc)
			case "exportlevel":
				stmt.ExportLevel = regularExpressionPropertyText(pc)
			case "documentation":
				stmt.Documentation = regularExpressionPropertyText(pc)
			}
		}
	}

	b.statements = append(b.statements, stmt)
}

// regularExpressionPropertyText returns the value side of a property, unquoted.
//
// The key is identifierOrKeyword(0), so an identifier value is index 1 —
// reading index 0 would echo the key back as the value.
func regularExpressionPropertyText(pc *parser.RegularExpressionPropertyContext) string {
	if s := pc.STRING_LITERAL(); s != nil {
		return unquoteString(s.GetText())
	}
	if bl := pc.BooleanLiteral(); bl != nil {
		return bl.GetText()
	}
	if v := pc.IdentifierOrKeyword(1); v != nil {
		return identifierOrKeywordText(v)
	}
	return ""
}
