// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// ExitCreateNavigationStatement handles CREATE [OR REPLACE] NAVIGATION <profile> <clauses>.
func (b *Builder) ExitCreateNavigationStatement(ctx *parser.CreateNavigationStatementContext) {
	// Extract profile name from qualifiedName or IDENTIFIER
	profileName := ""
	if qn := ctx.QualifiedName(); qn != nil {
		profileName = getQualifiedNameText(qn)
	} else if id := ctx.IDENTIFIER(); id != nil {
		profileName = id.GetText()
	}
	if profileName == "" {
		return
	}

	stmt := &ast.AlterNavigationStmt{
		ProfileName: profileName,
	}

	// Process each navigation clause
	for _, clauseCtx := range ctx.AllNavigationClause() {
		clause := clauseCtx.(*parser.NavigationClauseContext)
		b.processNavigationClause(stmt, clause)
	}

	// Check for CREATE OR REPLACE/MODIFY
	createStmt := findParentCreateStatement(ctx)
	if createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}

	b.statements = append(b.statements, stmt)
}

// processNavigationClause processes a single navigation clause.
func (b *Builder) processNavigationClause(stmt *ast.AlterNavigationStmt, ctx *parser.NavigationClauseContext) {
	if ctx.HOME() != nil {
		// HOME PAGE/MICROFLOW qualifiedName [FOR qualifiedName]
		names := ctx.AllQualifiedName()
		if len(names) == 0 {
			return
		}
		hp := ast.NavHomePageDef{
			IsPage: ctx.PAGE() != nil,
			Target: buildQualifiedName(names[0]),
		}
		if ctx.FOR() != nil && len(names) >= 2 {
			forRole := buildQualifiedName(names[1])
			hp.ForRole = &forRole
		}
		stmt.HomePages = append(stmt.HomePages, hp)
	} else if ctx.LOGIN() != nil {
		// LOGIN PAGE qualifiedName
		names := ctx.AllQualifiedName()
		if len(names) > 0 {
			qn := buildQualifiedName(names[0])
			stmt.LoginPage = &qn
		}
	} else if ctx.NOT() != nil && ctx.FOUND() != nil {
		// NOT FOUND PAGE qualifiedName
		names := ctx.AllQualifiedName()
		if len(names) > 0 {
			qn := buildQualifiedName(names[0])
			stmt.NotFoundPage = &qn
		}
	} else if ctx.MENU_KW() != nil {
		// MENU (navMenuItemDef*)
		stmt.HasMenuBlock = true
		for _, itemCtx := range ctx.AllNavMenuItemDef() {
			item := buildNavMenuItemDef(itemCtx)
			stmt.MenuItems = append(stmt.MenuItems, item)
		}
	}
}

// buildNavMenuItemDef recursively builds a NavMenuItemDef from the parse context.
func buildNavMenuItemDef(ctx parser.INavMenuItemDefContext) ast.NavMenuItemDef {
	c := ctx.(*parser.NavMenuItemDefContext)

	caption := ""
	if sl := c.STRING_LITERAL(); sl != nil {
		caption = unquoteString(sl.GetText())
	}

	item := ast.NavMenuItemDef{Caption: caption}

	// The PAGE/MICROFLOW target is the item's only qualifiedName now that the
	// icon is its own sub-rule — which is what removed the old positional
	// bookkeeping, where the target and the icon shared one indexed list and the
	// icon was "whatever remains".
	if qn := c.QualifiedName(); qn != nil {
		switch {
		case c.PAGE() != nil:
			built := buildQualifiedName(qn)
			item.Page = &built
		case c.MICROFLOW() != nil:
			built := buildQualifiedName(qn)
			item.Microflow = &built
		}
	}
	// SIGN_OUT names no target, which is why it is read separately rather than
	// as a third switch arm.
	if c.SIGN_OUT() != nil {
		item.SignOut = true
	}
	applyNavMenuIcon(&item, c.NavMenuIcon())

	// Recurse into sub-items (for MENU 'caption' (...))
	for _, subCtx := range c.AllNavMenuItemDef() {
		subItem := buildNavMenuItemDef(subCtx)
		item.Items = append(item.Items, subItem)
	}

	return item
}

// applyNavMenuIcon reads the ICON clause onto the item.
//
// Mendix stores three different icon ELEMENTS, not three spellings of one
// value: a collection icon and an image icon each hold a qualified name (into an
// icon collection and an image collection — different documents), while a glyph
// icon holds a numeric character code and no name at all. The kind is recorded
// so the writer emits the right $Type; collapsing them onto one string is what
// made a rewrite turn a glyph into nothing.
//
// The bare form is the collection icon, which keeps every existing script
// meaning exactly what it did.
func applyNavMenuIcon(item *ast.NavMenuItemDef, ctx parser.INavMenuIconContext) {
	if ctx == nil {
		return
	}
	c, ok := ctx.(*parser.NavMenuIconContext)
	if !ok {
		return
	}
	switch {
	case c.GLYPH() != nil:
		item.IconKind = types.MenuIconGlyph
		if n := c.NUMBER_LITERAL(); n != nil {
			// A glyph code is a character code: whole, and small. A fractional or
			// unparseable literal leaves the code at zero rather than guessing,
			// and the writer refuses to emit a glyph without one.
			if v, err := strconv.Atoi(n.GetText()); err == nil {
				item.IconCode = v
			}
		}
	case c.IMAGE() != nil:
		item.IconKind = types.MenuIconImage
		if qn := c.QualifiedName(); qn != nil {
			item.Icon = buildQualifiedName(qn).String()
		}
	default:
		item.IconKind = types.MenuIconCollection
		if qn := c.QualifiedName(); qn != nil {
			item.Icon = buildQualifiedName(qn).String()
		}
	}
}
