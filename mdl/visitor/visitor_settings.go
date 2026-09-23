// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitAlterSettingsClause handles ALTER SETTINGS ... clauses.
func (b *Builder) ExitAlterSettingsClause(ctx *parser.AlterSettingsClauseContext) {
	stmt := &ast.AlterSettingsStmt{
		Properties: make(map[string]any),
	}

	if ctx.DROP() != nil && ctx.CONSTANT() != nil {
		// ALTER SETTINGS DROP CONSTANT 'name' [IN CONFIGURATION 'cfg']
		stmt.Section = "constant"
		stmt.DropConstant = true
		allStrings := ctx.AllSTRING_LITERAL()
		if len(allStrings) > 0 {
			stmt.ConstantId = unquoteString(allStrings[0].GetText())
		}
		if ctx.IN() != nil && ctx.CONFIGURATION() != nil && len(allStrings) > 1 {
			stmt.ConfigName = unquoteString(allStrings[1].GetText())
		}
	} else if ctx.CONSTANT() != nil {
		// ALTER SETTINGS CONSTANT 'name' (VALUE 'value' | DROP) [IN CONFIGURATION 'cfg']
		stmt.Section = "constant"
		allStrings := ctx.AllSTRING_LITERAL()
		if len(allStrings) > 0 {
			stmt.ConstantId = unquoteString(allStrings[0].GetText())
		}
		if ctx.DROP() != nil {
			stmt.DropConstant = true
		} else if ctx.SettingsValue() != nil {
			stmt.Value = settingsValueText(ctx.SettingsValue().(*parser.SettingsValueContext))
		}
		// Check for IN CONFIGURATION 'name'
		if ctx.IN() != nil && ctx.CONFIGURATION() != nil && len(allStrings) > 1 {
			stmt.ConfigName = unquoteString(allStrings[1].GetText())
		}
	} else if ctx.CONFIGURATION() != nil {
		// ALTER SETTINGS CONFIGURATION 'name' Key = Value, ...
		stmt.Section = "configuration"
		allStrings := ctx.AllSTRING_LITERAL()
		if len(allStrings) > 0 {
			stmt.ConfigName = unquoteString(allStrings[0].GetText())
		}
		for _, assignCtx := range ctx.AllSettingsAssignment() {
			assign, ok := assignCtx.(*parser.SettingsAssignmentContext)
			if !ok || assign == nil {
				continue
			}
			if assign.IDENTIFIER() == nil || assign.SettingsValue() == nil {
				continue
			}
			key := assign.IDENTIFIER().GetText()
			svCtx, ok := assign.SettingsValue().(*parser.SettingsValueContext)
			if !ok || svCtx == nil {
				continue
			}
			val := settingsValueText(svCtx)
			stmt.Properties[key] = val
		}
	} else if ctx.SettingsSection() != nil && ctx.GROUP() != nil {
		// ALTER SETTINGS WORKFLOWS ADD [OR MODIFY] GROUP 'Approvers' [( Description: '…' )]
		// ALTER SETTINGS WORKFLOWS MODIFY          GROUP 'Approvers'  ( Description: '…' )
		// ALTER SETTINGS WORKFLOWS REMOVE          GROUP 'Approvers'
		stmt.Section = ctx.SettingsSection().GetText()
		stmt.UpsertGroup = ctx.ADD() != nil && ctx.OR() != nil && ctx.MODIFY() != nil
		stmt.AddGroup = ctx.ADD() != nil && !stmt.UpsertGroup
		stmt.ModifyGroup = ctx.MODIFY() != nil && !stmt.UpsertGroup
		stmt.RemoveGroup = ctx.REMOVE() != nil
		if all := ctx.AllSTRING_LITERAL(); len(all) > 0 {
			stmt.GroupName = unquoteString(all[0].GetText())
		}
		collectSettingsItemOptions(ctx.SettingsItemOptions(), stmt.Properties)
	} else if ctx.SettingsSection() != nil && (ctx.ADD() != nil || ctx.MODIFY() != nil || ctx.REMOVE() != nil) {
		// ALTER SETTINGS LANGUAGE ADD    'ar_SD' [( key: value, … )]
		// ALTER SETTINGS LANGUAGE MODIFY 'ar_SD'  ( key: value, … )
		// ALTER SETTINGS LANGUAGE REMOVE 'ar_SD'
		stmt.Section = ctx.SettingsSection().GetText()
		stmt.UpsertLanguage = ctx.ADD() != nil && ctx.OR() != nil && ctx.MODIFY() != nil
		stmt.AddLanguage = ctx.ADD() != nil && !stmt.UpsertLanguage
		stmt.ModifyLanguage = ctx.MODIFY() != nil && !stmt.UpsertLanguage
		stmt.RemoveLanguage = ctx.REMOVE() != nil
		if all := ctx.AllSTRING_LITERAL(); len(all) > 0 {
			stmt.LanguageCode = unquoteString(all[0].GetText())
		}
		collectSettingsItemOptions(ctx.SettingsItemOptions(), stmt.Properties)
	} else if ctx.SettingsSection() != nil {
		// ALTER SETTINGS MODEL|LANGUAGE|WORKFLOWS Key = Value, ...
		stmt.Section = ctx.SettingsSection().GetText()
		for _, assignCtx := range ctx.AllSettingsAssignment() {
			assign, ok := assignCtx.(*parser.SettingsAssignmentContext)
			if !ok || assign == nil {
				continue
			}
			if assign.IDENTIFIER() == nil || assign.SettingsValue() == nil {
				continue
			}
			key := assign.IDENTIFIER().GetText()
			svCtx, ok := assign.SettingsValue().(*parser.SettingsValueContext)
			if !ok || svCtx == nil {
				continue
			}
			val := settingsValueToInterface(svCtx)
			stmt.Properties[key] = val
		}
	}

	b.statements = append(b.statements, stmt)
}

// ExitCreateConfigurationStatement handles CREATE CONFIGURATION 'name' [Key = Value, ...].
func (b *Builder) ExitCreateConfigurationStatement(ctx *parser.CreateConfigurationStatementContext) {
	stmt := &ast.CreateConfigurationStmt{
		Properties: make(map[string]any),
	}

	if sl := ctx.STRING_LITERAL(); sl != nil {
		stmt.Name = unquoteString(sl.GetText())
	}

	// CREATE OR MODIFY / OR REPLACE, read off the shared create wrapper.
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}

	for _, assignCtx := range ctx.AllSettingsAssignment() {
		assign, ok := assignCtx.(*parser.SettingsAssignmentContext)
		if !ok || assign == nil {
			continue
		}
		if assign.IDENTIFIER() == nil || assign.SettingsValue() == nil {
			continue
		}
		key := assign.IDENTIFIER().GetText()
		svCtx, ok := assign.SettingsValue().(*parser.SettingsValueContext)
		if !ok || svCtx == nil {
			continue
		}
		stmt.Properties[key] = settingsValueText(svCtx)
	}

	b.statements = append(b.statements, stmt)
}

// collectSettingsItemOptions reads a ( key: value, … ) option list — the shared
// form behind ALTER SETTINGS LANGUAGE's language options and WORKFLOWS' group
// options — into the statement's property map.
func collectSettingsItemOptions(opts parser.ISettingsItemOptionsContext, into map[string]any) {
	if opts == nil {
		return
	}
	oc, ok := opts.(*parser.SettingsItemOptionsContext)
	if !ok || oc == nil {
		return
	}
	for _, o := range oc.AllSettingsItemOption() {
		so, ok := o.(*parser.SettingsItemOptionContext)
		if !ok || so == nil || so.IdentifierOrKeyword() == nil || so.SettingsValue() == nil {
			continue
		}
		sv, ok := so.SettingsValue().(*parser.SettingsValueContext)
		if !ok || sv == nil {
			continue
		}
		into[unquoteIdentifier(so.IdentifierOrKeyword().GetText())] = settingsValueText(sv)
	}
}

// settingsValueText extracts the string value from a SettingsValue context.
func settingsValueText(ctx *parser.SettingsValueContext) string {
	if sl := ctx.STRING_LITERAL(); sl != nil {
		return unquoteString(sl.GetText())
	}
	if nl := ctx.NUMBER_LITERAL(); nl != nil {
		return nl.GetText()
	}
	if bl := ctx.BooleanLiteral(); bl != nil {
		return bl.GetText()
	}
	if qn := ctx.QualifiedName(); qn != nil {
		return getQualifiedNameText(qn)
	}
	return ctx.GetText()
}

// settingsValueToInterface extracts a typed value from a SettingsValue context.
func settingsValueToInterface(ctx *parser.SettingsValueContext) any {
	if sl := ctx.STRING_LITERAL(); sl != nil {
		return unquoteString(sl.GetText())
	}
	if nl := ctx.NUMBER_LITERAL(); nl != nil {
		if v, err := strconv.ParseInt(nl.GetText(), 10, 64); err == nil {
			return v
		}
		return nl.GetText()
	}
	if bl := ctx.BooleanLiteral(); bl != nil {
		text := bl.GetText()
		return text == "true" || text == "TRUE" || text == "True"
	}
	if qn := ctx.QualifiedName(); qn != nil {
		return getQualifiedNameText(qn)
	}
	return ctx.GetText()
}
