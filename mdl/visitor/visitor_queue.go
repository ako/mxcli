// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateQueueStatement builds a CreateQueueStmt from
// CREATE [OR REPLACE|MODIFY] QUEUE Module.Name ( ... ).
func (b *Builder) ExitCreateQueueStatement(ctx *parser.CreateQueueStatementContext) {
	stmt := &ast.CreateQueueStmt{
		Name:          buildQualifiedName(ctx.QualifiedName()),
		Documentation: findDocCommentText(ctx),
	}
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}

	if body := ctx.QueueBody(); body != nil {
		bodyCtx := body.(*parser.QueueBodyContext)
		for _, prop := range bodyCtx.AllQueueProperty() {
			pc, ok := prop.(*parser.QueuePropertyContext)
			if !ok || pc == nil {
				continue
			}
			iok := pc.IdentifierOrKeyword(0)
			if iok == nil {
				continue
			}
			key := strings.ToLower(identifierOrKeywordText(iok))
			switch key {
			case "parallelism":
				stmt.Parallelism = queuePropertyText(pc)
			case "clusterwide":
				stmt.ClusterWide = strings.EqualFold(queuePropertyText(pc), "true")
			case "exportlevel":
				stmt.ExportLevel = queuePropertyText(pc)
			case "documentation":
				stmt.Documentation = queuePropertyText(pc)
			}
		}
	}

	b.statements = append(b.statements, stmt)
}

// queuePropertyText returns the value side of a queue property, unquoted.
//
// The value alternatives are NUMBER_LITERAL | STRING_LITERAL | booleanLiteral |
// identifierOrKeyword. The key is identifierOrKeyword(0), so an identifier value
// is index 1 — reading index 0 would echo the key back as the value.
func queuePropertyText(pc *parser.QueuePropertyContext) string {
	if n := pc.NUMBER_LITERAL(); n != nil {
		return n.GetText()
	}
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
