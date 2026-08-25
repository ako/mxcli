// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

func (b *Builder) ExitCreateMicroflowStatement(ctx *parser.CreateMicroflowStatementContext) {
	stmt := &ast.CreateMicroflowStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	// Parse parameters
	if paramList := ctx.MicroflowParameterList(); paramList != nil {
		stmt.Parameters = buildMicroflowParameters(paramList)
	}

	// Parse return type
	if retType := ctx.MicroflowReturnType(); retType != nil {
		stmt.ReturnType = buildMicroflowReturnType(retType)
	}

	// Parse options (FOLDER, COMMENT)
	if opts := ctx.MicroflowOptions(); opts != nil {
		optsCtx := opts.(*parser.MicroflowOptionsContext)
		for _, opt := range optsCtx.AllMicroflowOption() {
			optCtx := opt.(*parser.MicroflowOptionContext)
			if optCtx.COMMENT() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.Comment = unquoteString(optCtx.STRING_LITERAL().GetText())
			}
			if optCtx.FOLDER() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.Folder = unquoteString(optCtx.STRING_LITERAL().GetText())
			}
			if exposed := optCtx.MicroflowExposedClause(); exposed != nil {
				stmt.Expose = append(stmt.Expose, buildExposeActionClause(exposed))
			}
		}
	}

	// Parse body
	if body := ctx.MicroflowBody(); body != nil {
		stmt.Body = buildMicroflowBody(body)
	}

	// Check for CREATE OR MODIFY, extract doc comment, and parse @excluded
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
		for _, ann := range createStmt.AllAnnotation() {
			annCtx := ann.(*parser.AnnotationContext)
			if strings.EqualFold(annCtx.AnnotationName().GetText(), "excluded") {
				stmt.Excluded = true
			}
		}
	}
	stmt.Documentation = findDocCommentText(ctx)

	b.statements = append(b.statements, stmt)
}

func (b *Builder) ExitCreateNanoflowStatement(ctx *parser.CreateNanoflowStatementContext) {
	stmt := &ast.CreateNanoflowStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	// Parse parameters
	if paramList := ctx.MicroflowParameterList(); paramList != nil {
		stmt.Parameters = buildMicroflowParameters(paramList)
	}

	// Parse return type
	if retType := ctx.MicroflowReturnType(); retType != nil {
		stmt.ReturnType = buildMicroflowReturnType(retType)
	}

	// Parse options (FOLDER, COMMENT)
	if opts := ctx.MicroflowOptions(); opts != nil {
		optsCtx := opts.(*parser.MicroflowOptionsContext)
		for _, opt := range optsCtx.AllMicroflowOption() {
			optCtx := opt.(*parser.MicroflowOptionContext)
			if optCtx.COMMENT() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.Comment = unquoteString(optCtx.STRING_LITERAL().GetText())
			}
			if optCtx.FOLDER() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.Folder = unquoteString(optCtx.STRING_LITERAL().GetText())
			}
			if exposed := optCtx.MicroflowExposedClause(); exposed != nil {
				stmt.Expose = append(stmt.Expose, buildExposeActionClause(exposed))
			}
		}
	}

	// Parse body
	if body := ctx.MicroflowBody(); body != nil {
		stmt.Body = buildMicroflowBody(body)
	}

	// Check for CREATE OR MODIFY, extract doc comment, and parse @excluded
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
		for _, ann := range createStmt.AllAnnotation() {
			annCtx := ann.(*parser.AnnotationContext)
			if strings.EqualFold(annCtx.AnnotationName().GetText(), "excluded") {
				stmt.Excluded = true
			}
		}
	}
	stmt.Documentation = findDocCommentText(ctx)

	b.statements = append(b.statements, stmt)
}

// ExitCreateRuleStatement mirrors ExitCreateNanoflowStatement — a rule shares the
// microflow header and body grammar, so the only difference is the AST node.
func (b *Builder) ExitCreateRuleStatement(ctx *parser.CreateRuleStatementContext) {
	stmt := &ast.CreateRuleStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}

	if paramList := ctx.MicroflowParameterList(); paramList != nil {
		stmt.Parameters = buildMicroflowParameters(paramList)
	}
	if retType := ctx.MicroflowReturnType(); retType != nil {
		stmt.ReturnType = buildMicroflowReturnType(retType)
	}
	if opts := ctx.MicroflowOptions(); opts != nil {
		optsCtx := opts.(*parser.MicroflowOptionsContext)
		for _, opt := range optsCtx.AllMicroflowOption() {
			optCtx := opt.(*parser.MicroflowOptionContext)
			if optCtx.COMMENT() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.Comment = unquoteString(optCtx.STRING_LITERAL().GetText())
			}
			if optCtx.FOLDER() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.Folder = unquoteString(optCtx.STRING_LITERAL().GetText())
			}
			if exposed := optCtx.MicroflowExposedClause(); exposed != nil {
				stmt.Expose = append(stmt.Expose, buildExposeActionClause(exposed))
			}
		}
	}
	if body := ctx.MicroflowBody(); body != nil {
		stmt.Body = buildMicroflowBody(body)
	}

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
		for _, ann := range createStmt.AllAnnotation() {
			annCtx := ann.(*parser.AnnotationContext)
			if strings.EqualFold(annCtx.AnnotationName().GetText(), "excluded") {
				stmt.Excluded = true
			}
		}
	}
	stmt.Documentation = findDocCommentText(ctx)

	b.statements = append(b.statements, stmt)
}

// buildMicroflowDataType converts a data type context to ast.DataType for microflow context.
// In microflow parameters/return types, bare qualified names are entity references (not enumerations).
func buildMicroflowDataType(ctx parser.IDataTypeContext) ast.DataType {
	if ctx == nil {
		return ast.DataType{Kind: ast.TypeVoid}
	}
	dtCtx := ctx.(*parser.DataTypeContext)
	text := strings.ToUpper(dtCtx.GetText())

	// Check for List type (List OF Entity)
	if dtCtx.LIST_OF() != nil {
		dt := ast.DataType{Kind: ast.TypeListOf}
		if qn := dtCtx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			dt.EntityRef = &name
		}
		return dt
	}

	// Handle ENUMERATION(QualifiedName) or ENUM QualifiedName
	if dtCtx.ENUMERATION() != nil || dtCtx.ENUM_TYPE() != nil {
		if qn := dtCtx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			return ast.DataType{Kind: ast.TypeEnumeration, EnumRef: &name}
		}
	}

	// Handle bare qualified name - in microflow context, this is an ENTITY reference
	if qn := dtCtx.QualifiedName(); qn != nil {
		name := buildQualifiedName(qn)
		// "Nothing" / "Void" means void return type (no return value)
		if name.Module == "" && (strings.EqualFold(name.Name, "Nothing") || strings.EqualFold(name.Name, "Void")) {
			return ast.DataType{Kind: ast.TypeVoid}
		}
		return ast.DataType{Kind: ast.TypeEntity, EntityRef: &name}
	}

	// Parse primitive types
	if strings.HasPrefix(text, "STRING") {
		dt := ast.DataType{Kind: ast.TypeString, Length: 0}
		if dtCtx.NUMBER_LITERAL() != nil {
			dt.Length, _ = strconv.Atoi(dtCtx.NUMBER_LITERAL().GetText())
		}
		return dt
	}
	if strings.HasPrefix(text, "INTEGER") {
		return ast.DataType{Kind: ast.TypeInteger}
	}
	if strings.HasPrefix(text, "LONG") {
		return ast.DataType{Kind: ast.TypeLong}
	}
	if strings.HasPrefix(text, "DECIMAL") {
		return ast.DataType{Kind: ast.TypeDecimal}
	}
	if strings.HasPrefix(text, "BOOLEAN") {
		return ast.DataType{Kind: ast.TypeBoolean}
	}
	if strings.HasPrefix(text, "DATETIME") {
		return ast.DataType{Kind: ast.TypeDateTime}
	}
	if strings.HasPrefix(text, "DATE") {
		return ast.DataType{Kind: ast.TypeDate}
	}
	if strings.HasPrefix(text, "BINARY") {
		return ast.DataType{Kind: ast.TypeBinary}
	}

	return ast.DataType{Kind: ast.TypeVoid}
}

// buildNonListDataType converts a non-list data type context to ast.DataType.
// This is used for createObjectStatement to ensure it doesn't match "CREATE LIST OF" which is createListStatement.
func buildNonListDataType(ctx parser.INonListDataTypeContext) ast.DataType {
	if ctx == nil {
		return ast.DataType{Kind: ast.TypeVoid}
	}
	dtCtx := ctx.(*parser.NonListDataTypeContext)
	text := strings.ToUpper(dtCtx.GetText())

	// Handle ENUMERATION(QualifiedName) or ENUM QualifiedName
	if dtCtx.ENUMERATION() != nil || dtCtx.ENUM_TYPE() != nil {
		if qn := dtCtx.QualifiedName(); qn != nil {
			name := buildQualifiedName(qn)
			return ast.DataType{Kind: ast.TypeEnumeration, EnumRef: &name}
		}
	}

	// Handle bare qualified name - in microflow context, this is an ENTITY reference
	if qn := dtCtx.QualifiedName(); qn != nil {
		name := buildQualifiedName(qn)
		return ast.DataType{Kind: ast.TypeEntity, EntityRef: &name}
	}

	// Parse primitive types
	if strings.HasPrefix(text, "STRING") {
		dt := ast.DataType{Kind: ast.TypeString, Length: 0}
		if dtCtx.NUMBER_LITERAL() != nil {
			dt.Length, _ = strconv.Atoi(dtCtx.NUMBER_LITERAL().GetText())
		}
		return dt
	}
	if strings.HasPrefix(text, "INTEGER") {
		return ast.DataType{Kind: ast.TypeInteger}
	}
	if strings.HasPrefix(text, "LONG") {
		return ast.DataType{Kind: ast.TypeLong}
	}
	if strings.HasPrefix(text, "DECIMAL") {
		return ast.DataType{Kind: ast.TypeDecimal}
	}
	if strings.HasPrefix(text, "BOOLEAN") {
		return ast.DataType{Kind: ast.TypeBoolean}
	}
	if strings.HasPrefix(text, "DATETIME") {
		return ast.DataType{Kind: ast.TypeDateTime}
	}
	if strings.HasPrefix(text, "DATE") {
		return ast.DataType{Kind: ast.TypeDate}
	}
	if strings.HasPrefix(text, "BINARY") {
		return ast.DataType{Kind: ast.TypeBinary}
	}

	return ast.DataType{Kind: ast.TypeVoid}
}

// buildMicroflowParameters converts parameter list context to MicroflowParam slice.
func buildMicroflowParameters(ctx parser.IMicroflowParameterListContext) []ast.MicroflowParam {
	if ctx == nil {
		return nil
	}
	listCtx := ctx.(*parser.MicroflowParameterListContext)
	var params []ast.MicroflowParam

	for _, paramCtx := range listCtx.AllMicroflowParameter() {
		p := paramCtx.(*parser.MicroflowParameterContext)
		param := ast.MicroflowParam{}

		// Get parameter name (can be parameterName or VARIABLE)
		if pn := p.ParameterName(); pn != nil {
			param.Name = parameterNameText(pn)
		} else if v := p.VARIABLE(); v != nil {
			// Remove $ prefix
			param.Name = strings.TrimPrefix(v.GetText(), "$")
		}

		// Get parameter type (use microflow-specific data type builder)
		if dt := p.DataType(); dt != nil {
			param.Type = buildMicroflowDataType(dt)
		}

		params = append(params, param)
	}

	return params
}

// buildMicroflowReturnType converts return type context to MicroflowReturnType.
func buildMicroflowReturnType(ctx parser.IMicroflowReturnTypeContext) *ast.MicroflowReturnType {
	if ctx == nil {
		return nil
	}
	rtCtx := ctx.(*parser.MicroflowReturnTypeContext)

	ret := &ast.MicroflowReturnType{}

	// Get return type (use microflow-specific data type builder)
	if dt := rtCtx.DataType(); dt != nil {
		ret.Type = buildMicroflowDataType(dt)
	}

	// Get optional AS $Variable clause
	if v := rtCtx.VARIABLE(); v != nil {
		ret.Variable = strings.TrimPrefix(v.GetText(), "$")
	}

	return ret
}

// buildExposeActionClause reads one EXPOSED AS <kind> ACTION clause.
//
// A microflow has two toolbox entries, stored under different keys, so the kind
// is part of the clause rather than implied. NOT EXPOSED is the removal form:
// an absent clause preserves what is stored, because the entry also holds icon
// and image bitmaps MDL cannot name.
func buildExposeActionClause(ctx parser.IMicroflowExposedClauseContext) ast.ExposeActionClause {
	out := ast.ExposeActionClause{Workflow: ctx.WORKFLOW() != nil}
	if ctx.NOT() != nil {
		out.Remove = true
		return out
	}
	if strs := ctx.AllSTRING_LITERAL(); len(strs) >= 2 {
		out.Caption = unquoteString(strs[0].GetText())
		out.Category = unquoteString(strs[1].GetText())
	}
	out.Bitmaps = buildExposeBitmaps(ctx.AllExposeBitmapClause())
	return out
}

// buildExposeBitmaps reads the ICON/IMAGE clauses hanging off an exposed clause.
//
// DROP ICON clears one; an omitted clause leaves what is stored alone, which is
// why clearing has to be written out.
func buildExposeBitmaps(clauses []parser.IExposeBitmapClauseContext) []ast.ExposeBitmap {
	var out []ast.ExposeBitmap
	for _, c := range clauses {
		b := ast.ExposeBitmap{
			Image: c.IMAGE() != nil,
			Dark:  c.DARK() != nil,
			Clear: c.DROP() != nil,
		}
		if lit := c.STRING_LITERAL(); lit != nil {
			b.Path = unquoteString(lit.GetText())
		}
		out = append(out, b)
	}
	return out
}
