// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateImportMappingStatement is called when exiting the createImportMappingStatement production.
func (b *Builder) ExitCreateImportMappingStatement(ctx *parser.CreateImportMappingStatementContext) {
	stmt := &ast.CreateImportMappingStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteString(lit.GetText())
	}

	// Parse WITH clause
	if wc := ctx.ImportMappingWithClause(); wc != nil {
		sc := wc.(*parser.ImportMappingWithClauseContext)
		if sc.JSON() != nil {
			stmt.SchemaKind = "JSON_STRUCTURE"
		} else {
			stmt.SchemaKind = "XML_SCHEMA"
		}
		if sc.QualifiedName() != nil {
			stmt.SchemaRef = buildQualifiedName(sc.QualifiedName())
		}
	}

	// Parse root element
	if root := ctx.ImportMappingRootElement(); root != nil {
		stmt.RootElement = buildImportRootElement(root.(*parser.ImportMappingRootElementContext))
	}

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	b.statements = append(b.statements, stmt)
}

// buildImportRootElement builds the root element from the grammar context.
// Grammar: importMappingObjectHandling qualifiedName LBRACE importMappingChild (COMMA importMappingChild)* RBRACE
func buildImportRootElement(ctx *parser.ImportMappingRootElementContext) *ast.ImportMappingElementDef {
	elem := &ast.ImportMappingElementDef{}

	// Object handling: CREATE | FIND | FIND OR CREATE
	if hCtx := ctx.ImportMappingObjectHandling(); hCtx != nil {
		elem.ObjectHandling = extractObjectHandling(hCtx.(*parser.ImportMappingObjectHandlingContext))
	}

	// Entity name
	if ctx.QualifiedName() != nil {
		elem.Entity = buildQualifiedName(ctx.QualifiedName()).String()
	}

	// Children
	for _, childCtx := range ctx.AllImportMappingChild() {
		child := buildImportChild(childCtx.(*parser.ImportMappingChildContext))
		elem.Children = append(elem.Children, child)
	}

	return elem
}

// buildImportChild builds a child element from the grammar context.
// Four alternatives:
// 1. handling assocPath = jsonKey { children }   (nested object with children)
// 2. handling assocPath = jsonKey                 (leaf object)
// 3. attr = Module.MF(jsonField)                 (value transform)
// 4. attr = jsonField KEY?                        (value assignment)
func buildImportChild(ctx *parser.ImportMappingChildContext) *ast.ImportMappingElementDef {
	elem := &ast.ImportMappingElementDef{}

	// Check if this is an object mapping (has handling keyword)
	if hCtx := ctx.ImportMappingObjectHandling(); hCtx != nil {
		// Object mapping: CREATE/FIND/FIND OR CREATE Assoc/Entity = jsonKey
		elem.ObjectHandling = extractObjectHandling(hCtx.(*parser.ImportMappingObjectHandlingContext))

		// Association path: qualifiedName SLASH qualifiedName
		allQN := ctx.AllQualifiedName()
		if len(allQN) >= 2 {
			elem.Association = buildQualifiedName(allQN[0]).String()
			elem.Entity = buildQualifiedName(allQN[1]).String()
		}

		// JSON key after EQUALS
		if id := ctx.IdentifierOrKeyword(); id != nil {
			elem.JsonName = identifierOrKeywordText(id)
		}

		// Nested children
		for _, childCtx := range ctx.AllImportMappingChild() {
			child := buildImportChild(childCtx.(*parser.ImportMappingChildContext))
			elem.Children = append(elem.Children, child)
		}
	} else if ctx.LPAREN() != nil {
		// Value transform: attr = Module.MF(a/b/c)
		if id := ctx.IdentifierOrKeyword(); id != nil {
			elem.Attribute = identifierOrKeywordText(id)
		}
		allQN := ctx.AllQualifiedName()
		if len(allQN) >= 1 {
			elem.Converter = buildQualifiedName(allQN[0]).String()
		}
		elem.ConverterParam = jsonMemberPathText(ctx.JsonMemberPath())
	} else {
		// Value assignment: attr = a/b/c KEY?
		if id := ctx.IdentifierOrKeyword(); id != nil {
			elem.Attribute = identifierOrKeywordText(id)
		}
		elem.JsonName = jsonMemberPathText(ctx.JsonMemberPath())
		if ctx.KEY() != nil {
			elem.IsKey = true
		}
	}

	return elem
}

// ExitCreateExportMappingStatement is called when exiting the createExportMappingStatement production.
func (b *Builder) ExitCreateExportMappingStatement(ctx *parser.CreateExportMappingStatementContext) {
	stmt := &ast.CreateExportMappingStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteString(lit.GetText())
	}

	// Parse WITH clause
	if wc := ctx.ExportMappingWithClause(); wc != nil {
		sc := wc.(*parser.ExportMappingWithClauseContext)
		if sc.JSON() != nil {
			stmt.SchemaKind = "JSON_STRUCTURE"
		} else {
			stmt.SchemaKind = "XML_SCHEMA"
		}
		if sc.QualifiedName() != nil {
			stmt.SchemaRef = buildQualifiedName(sc.QualifiedName())
		}
	}

	// Parse null values clause
	if nc := ctx.ExportMappingNullValuesClause(); nc != nil {
		ncc := nc.(*parser.ExportMappingNullValuesClauseContext)
		if ncc.IdentifierOrKeyword() != nil {
			stmt.NullValueOption = identifierOrKeywordText(ncc.IdentifierOrKeyword().(*parser.IdentifierOrKeywordContext))
		}
	}

	// Parse root element
	if root := ctx.ExportMappingRootElement(); root != nil {
		stmt.RootElement = buildExportRootElement(root.(*parser.ExportMappingRootElementContext))
	}

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}
	b.statements = append(b.statements, stmt)
}

// buildExportRootElement builds the root element from the grammar context.
// Grammar: qualifiedName LBRACE exportMappingChild (COMMA exportMappingChild)* RBRACE
func buildExportRootElement(ctx *parser.ExportMappingRootElementContext) *ast.ExportMappingElementDef {
	elem := &ast.ExportMappingElementDef{}

	if ctx.QualifiedName() != nil {
		elem.Entity = buildQualifiedName(ctx.QualifiedName()).String()
	}

	for _, childCtx := range ctx.AllExportMappingChild() {
		child := buildExportChild(childCtx.(*parser.ExportMappingChildContext))
		elem.Children = append(elem.Children, child)
	}

	return elem
}

// buildExportChild builds a child element from the grammar context.
// Three alternatives:
// 1. Assoc/Entity AS jsonKey { children }   (nested object with children)
// 2. Assoc/Entity AS jsonKey                 (leaf object)
// 3. jsonField = Attr                        (value assignment)
func buildExportChild(ctx *parser.ExportMappingChildContext) *ast.ExportMappingElementDef {
	elem := &ast.ExportMappingElementDef{}

	allQN := ctx.AllQualifiedName()

	if len(allQN) >= 2 {
		// Object mapping: Assoc/Entity AS jsonKey
		elem.Association = buildQualifiedName(allQN[0]).String()
		elem.Entity = buildQualifiedName(allQN[1]).String()

		// JSON key after AS
		if id := ctx.IdentifierOrKeyword(); id != nil {
			elem.JsonName = identifierOrKeywordText(id.(*parser.IdentifierOrKeywordContext))
		}

		// Nested children
		for _, childCtx := range ctx.AllExportMappingChild() {
			child := buildExportChild(childCtx.(*parser.ExportMappingChildContext))
			elem.Children = append(elem.Children, child)
		}
	} else {
		// Value mapping: a/b/c = Attr
		elem.JsonName = jsonMemberPathText(ctx.JsonMemberPath())
		if id := ctx.IdentifierOrKeyword(); id != nil {
			elem.Attribute = identifierOrKeywordText(id.(*parser.IdentifierOrKeywordContext))
		}
	}

	return elem
}

// buildImportFromMappingStatement builds an ImportFromMappingStmt from the grammar context.
// Grammar: (VARIABLE EQUALS)? IMPORT FROM MAPPING qualifiedName LPAREN VARIABLE RPAREN
//
//	importMappingRange? onErrorClause?
func buildImportFromMappingStatement(ctx antlr.ParserRuleContext) ast.MicroflowStatement {
	c := ctx.(*parser.ImportFromMappingStatementContext)
	stmt := &ast.ImportFromMappingStmt{
		Mapping: buildQualifiedName(c.QualifiedName()),
	}

	vars := c.AllVARIABLE()
	if c.EQUALS() != nil && len(vars) >= 2 {
		stmt.OutputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
		stmt.SourceVariable = strings.TrimPrefix(vars[1].GetText(), "$")
	} else if len(vars) >= 1 {
		stmt.SourceVariable = strings.TrimPrefix(vars[0].GetText(), "$")
	}

	// Range: FIRST, or LIMIT/OFFSET. Absent means "All", which leaves the builder
	// inferring cardinality from the mapping's root shape as it always has. (#881)
	if r := c.ImportMappingRange(); r != nil {
		rc := r.(*parser.ImportMappingRangeContext)
		if rc.ALL() != nil {
			stmt.All = true
		}
		if rc.FIRST() != nil {
			stmt.First = true
		}
		if e := rc.GetLimitExpr(); e != nil {
			stmt.LimitExpr = buildExpression(e)
		}
		if e := rc.GetOffsetExpr(); e != nil {
			stmt.OffsetExpr = buildExpression(e)
		}
	}

	if ec := c.OnErrorClause(); ec != nil {
		stmt.ErrorHandling = buildOnErrorClause(ec)
	}

	return stmt
}

// buildExportToMappingStatement builds an ExportToMappingStmt from the grammar context.
// Grammar: (VARIABLE EQUALS)? EXPORT TO MAPPING qualifiedName LPAREN VARIABLE RPAREN onErrorClause?
func buildExportToMappingStatement(ctx antlr.ParserRuleContext) ast.MicroflowStatement {
	c := ctx.(*parser.ExportToMappingStatementContext)
	stmt := &ast.ExportToMappingStmt{
		Mapping: buildQualifiedName(c.QualifiedName()),
	}

	vars := c.AllVARIABLE()
	if c.EQUALS() != nil && len(vars) >= 2 {
		stmt.OutputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
		stmt.SourceVariable = strings.TrimPrefix(vars[1].GetText(), "$")
	} else if len(vars) >= 1 {
		stmt.SourceVariable = strings.TrimPrefix(vars[0].GetText(), "$")
	}

	if ec := c.OnErrorClause(); ec != nil {
		stmt.ErrorHandling = buildOnErrorClause(ec)
	}

	return stmt
}

// buildTransformJsonStatement builds a TransformJsonStmt from the grammar context.
// Grammar: (VARIABLE EQUALS)? TRANSFORM VARIABLE WITH qualifiedName onErrorClause?
func buildTransformJsonStatement(ctx antlr.ParserRuleContext) ast.MicroflowStatement {
	c := ctx.(*parser.TransformJsonStatementContext)
	stmt := &ast.TransformJsonStmt{
		Transformation: buildQualifiedName(c.QualifiedName()),
	}

	vars := c.AllVARIABLE()
	if c.EQUALS() != nil && len(vars) >= 2 {
		stmt.OutputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
		stmt.InputVariable = strings.TrimPrefix(vars[1].GetText(), "$")
	} else if len(vars) >= 1 {
		stmt.InputVariable = strings.TrimPrefix(vars[0].GetText(), "$")
	}

	if ec := c.OnErrorClause(); ec != nil {
		stmt.ErrorHandling = buildOnErrorClause(ec)
	}

	return stmt
}

// extractObjectHandling extracts the handling mode from the grammar context.
func extractObjectHandling(ctx *parser.ImportMappingObjectHandlingContext) string {
	if ctx.FIND() != nil && ctx.OR() != nil {
		return "FindOrCreate"
	}
	if ctx.FIND() != nil {
		return "Find"
	}
	return "Create"
}

// jsonMemberPathText renders a jsonMemberPath as the `/`-separated string the
// AST carries. A single segment is the ordinary direct-child case; several
// segments reach a leaf below the enclosing object element without an entity
// for the levels in between (issue #927). The executor translates `/` to
// Mendix's `|` when resolving against the JSON structure.
func jsonMemberPathText(ctx parser.IJsonMemberPathContext) string {
	if ctx == nil {
		return ""
	}
	pathCtx, ok := ctx.(*parser.JsonMemberPathContext)
	if !ok {
		return ""
	}
	var segments []string
	for _, seg := range pathCtx.AllIdentifierOrKeyword() {
		segments = append(segments, identifierOrKeywordText(seg))
	}
	return strings.Join(segments, "/")
}
