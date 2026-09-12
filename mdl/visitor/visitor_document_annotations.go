// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"

	"github.com/antlr4-go/antlr/v4"
)

// ExitCreateStatement records every annotation written before a CREATE, paired
// with the kind of document it was written on.
//
// The grammar attaches `annotation*` to createStatement itself, so ANY of the
// forty-odd create kinds accepts one while only seven read one. An annotation on
// a document that does not read it — or with a typo in it — therefore parsed and
// did nothing, which is exactly the failure MDL059 refuses one node family over.
//
// Recording happens here rather than in each document's own builder for two
// reasons: those builders only look for the names they implement, so a name none
// of them implements is invisible to all of them; and deriving the kind from the
// parse tree means a create statement added later is covered without anyone
// remembering to add it.
func (b *Builder) ExitCreateStatement(ctx *parser.CreateStatementContext) {
	if ctx == nil {
		return
	}
	anns := ctx.AllAnnotation()
	if len(anns) == 0 {
		return
	}
	kind := createStatementKind(ctx)
	for _, a := range anns {
		annCtx, ok := a.(*parser.AnnotationContext)
		if !ok || annCtx.AnnotationName() == nil {
			continue
		}
		b.documentAnnotations = append(b.documentAnnotations, ast.DocumentAnnotation{
			Kind:   kind,
			Name:   strings.ToLower(annCtx.AnnotationName().GetText()),
			Target: createStatementTarget(ctx),
		})
	}
}

// createStatementKind names the document a create statement builds, in MDL's own
// words: "microflow", "entity", "scheduledevent".
//
// It reads the child rule's own name out of the parser's rule table rather than
// testing forty accessors, so a create statement added to the grammar is named
// correctly here the day it is added. "" when no child rule is present, which
// only happens on a parse error.
func createStatementKind(ctx *parser.CreateStatementContext) string {
	for _, child := range ctx.GetChildren() {
		rc, ok := child.(antlr.RuleContext)
		if !ok {
			continue
		}
		names := parser.MDLParserParserStaticData.RuleNames
		idx := rc.GetRuleIndex()
		if idx < 0 || idx >= len(names) {
			continue
		}
		name := names[idx]
		if !strings.HasPrefix(name, "create") || !strings.HasSuffix(name, "Statement") {
			continue // docComment / annotation, not the document itself
		}
		return strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(name, "create"), "Statement"))
	}
	return ""
}

// createStatementTarget is the first qualified name inside the create statement,
// used only to point the message at the right statement in a long script.
func createStatementTarget(ctx *parser.CreateStatementContext) string {
	var walk func(antlr.Tree) string
	walk = func(n antlr.Tree) string {
		if qn, ok := n.(*parser.QualifiedNameContext); ok {
			return qn.GetText()
		}
		for _, c := range n.GetChildren() {
			if got := walk(c); got != "" {
				return got
			}
		}
		return ""
	}
	return walk(ctx)
}
