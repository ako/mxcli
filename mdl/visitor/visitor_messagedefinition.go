// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateMessageDefinitionCollectionStatement builds
//
//	CREATE [OR MODIFY] MESSAGE DEFINITION COLLECTION Module.Name
//	  [FOLDER 'path']
//	( definition Name for Module.Entity [as 'Exposed'] ( members ), ... );
func (b *Builder) ExitCreateMessageDefinitionCollectionStatement(ctx *parser.CreateMessageDefinitionCollectionStatementContext) {
	stmt := &ast.CreateMessageDefinitionCollectionStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	if ctx.FOLDER() != nil {
		if lit := ctx.STRING_LITERAL(); lit != nil {
			stmt.Folder = unquoteString(lit.GetText())
		}
	}
	for _, d := range ctx.AllMessageDefinitionDef() {
		if def := b.buildMessageDefinitionDef(d); def != nil {
			stmt.Definitions = append(stmt.Definitions, def)
		}
	}
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.REPLACE() != nil || createStmt.MODIFY() != nil) {
			stmt.CreateOrModify = true
		}
	}
	b.statements = append(b.statements, stmt)
}

// buildMessageDefinitionDef builds one `definition <Name> for <Entity>` block.
func (b *Builder) buildMessageDefinitionDef(c parser.IMessageDefinitionDefContext) *ast.MessageDefinitionDef {
	ctx, ok := c.(*parser.MessageDefinitionDefContext)
	if ctx == nil || !ok {
		return nil
	}
	def := &ast.MessageDefinitionDef{
		Name:        identifierOrKeywordText(ctx.IdentifierOrKeyword()),
		Entity:      buildQualifiedName(ctx.QualifiedName()),
		ExposedName: exposedNameOf(ctx.MessageExposedName()),
	}
	for _, m := range ctx.AllMessageMember() {
		if mem := b.buildMessageMember(m); mem != nil {
			def.Members = append(def.Members, mem)
		}
	}
	return def
}

// buildMessageMember builds an exposed attribute or an exposed association.
//
// The discriminator is the association form's two qualified names separated by a
// slash — the same one import and export mappings use, so a reader who knows one
// knows the other.
func (b *Builder) buildMessageMember(c parser.IMessageMemberContext) *ast.MessageMemberDef {
	ctx, ok := c.(*parser.MessageMemberContext)
	if ctx == nil || !ok {
		return nil
	}
	mem := &ast.MessageMemberDef{ExposedName: exposedNameOf(ctx.MessageExposedName())}

	if qns := ctx.AllQualifiedName(); len(qns) == 2 {
		// Association: Assoc/Module.Entity ( members ). The target entity is
		// spelled out because MaxOccurs tracks the DIRECTION of traversal, not
		// the association's type — see the AST doc comment.
		mem.Association = buildQualifiedName(qns[0])
		mem.Entity = buildQualifiedName(qns[1])
		for _, sub := range ctx.AllMessageMember() {
			if child := b.buildMessageMember(sub); child != nil {
				mem.Members = append(mem.Members, child)
			}
		}
		return mem
	}

	mem.Attribute = identifierOrKeywordText(ctx.IdentifierOrKeyword())
	if ex, ok := ctx.MessageExample().(*parser.MessageExampleContext); ok && ex != nil {
		if lit := ex.STRING_LITERAL(); lit != nil {
			mem.Example = unquoteString(lit.GetText())
		}
	}
	return mem
}

// exposedNameOf reads the optional `as 'Name'` clause.
func exposedNameOf(c parser.IMessageExposedNameContext) string {
	ctx, ok := c.(*parser.MessageExposedNameContext)
	if ctx == nil || !ok {
		return ""
	}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		return unquoteString(lit.GetText())
	}
	return ""
}

// ExitAlterMessageDefinitionCollectionStatement builds the collection-level
// ALTER: add, drop or rename a DEFINITION.
func (b *Builder) ExitAlterMessageDefinitionCollectionStatement(ctx *parser.AlterMessageDefinitionCollectionStatementContext) {
	stmt := &ast.AlterMessageDefinitionCollectionStmt{
		Name: buildQualifiedName(ctx.QualifiedName()),
	}
	op, ok := ctx.AlterMessageCollectionOperation().(*parser.AlterMessageCollectionOperationContext)
	if !ok || op == nil {
		return
	}
	ids := op.AllIdentifierOrKeyword()

	switch {
	case op.ADD() != nil:
		stmt.Op = "ADD"
		stmt.IfNotExist = op.NOT() != nil
		def := &ast.MessageDefinitionDef{
			Entity:      buildQualifiedName(op.QualifiedName()),
			ExposedName: exposedNameOf(op.MessageExposedName()),
		}
		if len(ids) > 0 {
			def.Name = identifierOrKeywordText(ids[0])
		}
		for _, m := range op.AllMessageMember() {
			if mem := b.buildMessageMember(m); mem != nil {
				def.Members = append(def.Members, mem)
			}
		}
		stmt.Definition = def
	case op.RENAME() != nil:
		stmt.Op = "RENAME"
		if len(ids) > 0 {
			stmt.Target = identifierOrKeywordText(ids[0])
		}
		if len(ids) > 1 {
			stmt.NewName = identifierOrKeywordText(ids[1])
		}
	case op.DROP() != nil:
		stmt.Op = "DROP"
		stmt.IfExists = op.IF() != nil
		if len(ids) > 0 {
			stmt.Target = identifierOrKeywordText(ids[0])
		}
	default:
		return
	}
	b.statements = append(b.statements, stmt)
}

// ExitAlterMessageDefinitionStatement builds the member-level ALTER.
//
// The definition is addressed as Module.Collection.Definition — the same
// three-part reference `WITH MESSAGE DEFINITION` takes — so the last segment is
// split off here rather than being a separate clause.
func (b *Builder) ExitAlterMessageDefinitionStatement(ctx *parser.AlterMessageDefinitionStatementContext) {
	full := buildQualifiedName(ctx.QualifiedName())
	collection, definition, ok := splitDefinitionRef(full)
	if !ok {
		b.addErrorWithExample(
			"ALTER MESSAGE DEFINITION takes a three-part name — Module.Collection.Definition, "+
				"the same reference WITH MESSAGE DEFINITION uses",
			"alter message definition Sales.MD_Order.Order add member Total;")
		return
	}
	stmt := &ast.AlterMessageDefinitionStmt{Collection: collection, Definition: definition}

	op, opOK := ctx.AlterMessageDefinitionOperation().(*parser.AlterMessageDefinitionOperationContext)
	if !opOK || op == nil {
		return
	}
	if p, pathOK := op.MessageMemberPath().(*parser.MessageMemberPathContext); pathOK && p != nil {
		for _, seg := range p.AllIdentifierOrKeyword() {
			stmt.Path = append(stmt.Path, identifierOrKeywordText(seg))
		}
	}
	// The path's segments live inside the MessageMemberPath sub-rule, so the
	// operation's own identifier is the only direct IdentifierOrKeyword child —
	// no slicing needed to tell them apart.
	target := identifierOrKeywordText(op.IdentifierOrKeyword())

	switch {
	case op.ADD() != nil:
		stmt.Op = "ADD"
		stmt.IfNotExist = op.NOT() != nil
		stmt.Member = b.buildMessageMember(op.MessageMember())
	case op.SET() != nil:
		stmt.Op = "SET"
		stmt.Target = target
		stmt.ExposedName = exposedNameOf(op.MessageExposedName())
	case op.DROP() != nil:
		stmt.Op = "DROP"
		stmt.IfExists = op.IF() != nil
		stmt.Target = target
	default:
		return
	}
	b.statements = append(b.statements, stmt)
}

// splitDefinitionRef splits Module.Collection.Definition into the collection's
// qualified name and the definition's name.
func splitDefinitionRef(qn ast.QualifiedName) (ast.QualifiedName, string, bool) {
	// buildQualifiedName puts everything after the module into Name, so a
	// three-part reference arrives as Module="Sales", Name="MD_Order.Order".
	if qn.Module == "" || !strings.Contains(qn.Name, ".") {
		return ast.QualifiedName{}, "", false
	}
	idx := strings.LastIndex(qn.Name, ".")
	return ast.QualifiedName{Module: qn.Module, Name: qn.Name[:idx]}, qn.Name[idx+1:], true
}

// DROP MESSAGE DEFINITION COLLECTION is built alongside the other DROP forms, in
// visitor_entity.go's dropStatement switch.
