// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// Forms that parsed, passed `check`, and were then dropped or stored as
// something else (ako/mxcli#706). Each is refused here (`date` excepted, see
// recordDateType), where the parse tree
// says exactly what was written, rather than removed from the grammar: the
// words involved are also keywords-as-identifiers (`Currency`, `Date`, `throw`),
// so deleting the alternative would let several of them re-parse as something
// else — an enumeration called `float`, say — which is the same silence again.
//
// None of them ever worked, so refusing them changes no script's meaning and
// is not gated on the `mdl` header (ADR-0011).

// ctxPos formats the start of a rule as the `line L:C` prefix syntax errors use.
func ctxPos(ctx antlr.ParserRuleContext) string {
	tok := ctx.GetStart()
	return fmt.Sprintf("line %d:%d", tok.GetLine(), tok.GetColumn())
}

// ExitThrowStatement refuses `throw <expr>`. The rule had no listener, so the
// statement disappeared from the microflow it was written in. Mendix has no
// action that raises a new error carrying a value: inside an error handler an
// error end event re-raises the error being handled (`raise error`); on the
// main flow the only way to fail is a Java action that throws.
func (b *Builder) ExitThrowStatement(ctx *parser.ThrowStatementContext) {
	b.addError(fmt.Errorf("%s: `throw` is not a Mendix action and was never written to the microflow — "+
		"it was parsed and dropped.\n"+
		"  Inside an `on error begin … end error` handler, re-raise the error being handled:\n"+
		"    raise error;\n"+
		"  On the main flow Mendix has no throw: call a Java action that throws, or\n"+
		"  report the problem with `validation feedback` / `log error` and return.", ctxPos(ctx)))
}

// ExitClosePageStatement refuses a page count that is not a positive whole
// number (`close page 0`, `close page 1.5`): Mendix stores the count as
// NumberOfPagesToClose and closes at least one page, so anything else would be
// written as something the statement did not say.
func (b *Builder) ExitClosePageStatement(ctx *parser.ClosePageStatementContext) {
	num := ctx.NUMBER_LITERAL()
	if num == nil {
		return
	}
	if n, err := strconv.Atoi(num.GetText()); err != nil || n < 1 {
		b.addErrorWithExample(
			fmt.Sprintf("%s: `close page %s`: the number of pages to close must be a whole number of at least 1",
				ctxPos(ctx), num.GetText()),
			"  close page;      -- the current page\n  close page 2;    -- the current page and the one that opened it")
	}
}

// removedPrimitiveType reports the replacement for a type word Mendix does not
// have, or "" when the token is a real type.
//
// float and currency were Mendix 6 attribute types, removed in Mendix 7 in
// favour of Decimal. mxcli mapped them to String(unlimited) on an attribute
// (Void in a microflow) without a word, so they are refused.
func removedPrimitiveType(floatTok, currencyTok antlr.TerminalNode) (word, replacement string) {
	switch {
	case floatTok != nil:
		return floatTok.GetText(), "Decimal"
	case currencyTok != nil:
		return currencyTok.GetText(), "Decimal"
	}
	return "", ""
}

func (b *Builder) rejectRemovedPrimitiveType(ctx antlr.ParserRuleContext, word, replacement string) {
	if word == "" {
		return
	}
	b.addError(fmt.Errorf("%s: type `%s` is not supported — Mendix has no %s type; it was removed in Mendix 7 "+
		"and mxcli stored it as the wrong type.\n  Write `%s` instead.",
		ctxPos(ctx), word, word, replacement))
}

// recordDateType records `date` as a type (MDL-DEPR160). There is no date-only
// type in Mendix: `date` was always stored as a DateTime, so it is a
// respelling of DateTime and builds exactly what DateTime builds. #706 refused
// it in every version, which stopped scripts that ran; it now warns without
// the header, fmt --upgrade writes DateTime, and mdl 1 refuses it
// (recordDeprecation, RemovedIn 1).
func (b *Builder) recordDateType(dateTok antlr.TerminalNode, subject string) {
	if dateTok == nil {
		return
	}
	tok := dateTok.GetSymbol()
	b.recordDeprecation(deprecation.DateType, tok, subject)
	repl := "DateTime"
	if w := tok.GetText(); w == strings.ToUpper(w) {
		repl = "DATETIME"
	}
	b.fixLastDeprecation(deprecation.DateType, &ast.Fix{Edits: []ast.TextEdit{
		{Start: tok.GetStart(), Stop: tok.GetStop() + 1, Text: repl},
	}}, "")
}

// EnterDataType covers every place a type is written: attributes, microflow
// parameters and return types, declare, constants, and the service rules.
func (b *Builder) EnterDataType(ctx *parser.DataTypeContext) {
	word, repl := removedPrimitiveType(ctx.FLOAT_TYPE(), ctx.CURRENCY_TYPE())
	b.rejectRemovedPrimitiveType(ctx, word, repl)
	b.recordDateType(ctx.DATE_TYPE(), "")
	if ctx.HASHEDSTRING_TYPE() != nil && !isAttributeTypeSlot(ctx) {
		b.rejectHashedStringOutsideAttribute(ctx)
	}
}

// isAttributeTypeSlot reports whether a dataType is an entity attribute's type:
// an attribute definition (create entity, add attribute, view entity) or the
// type of `alter entity … modify attribute`.
func isAttributeTypeSlot(ctx antlr.ParserRuleContext) bool {
	switch ctx.GetParent().(type) {
	case *parser.AttributeDefinitionContext, *parser.AlterEntityActionContext:
		return true
	}
	return false
}

// rejectHashedStringOutsideAttribute refuses `HashedString` where it is not an
// entity attribute's type. Mendix has HashedString only as a DomainModels
// attribute type — microflows, constants, Java actions and parameters have no
// such type — and mxcli stored it as a String there (a microflow parameter as
// Void), silently.
func (b *Builder) rejectHashedStringOutsideAttribute(ctx antlr.ParserRuleContext) {
	b.addError(fmt.Errorf("%s: type `HashedString` is only an entity attribute type — Mendix has no hashed "+
		"string type for parameters, variables, constants or return values.\n"+
		"  Write `String` instead (a hashed attribute's value is a String).", ctxPos(ctx)))
}

// EnterNonListDataType is the same check for the create-object type slot.
func (b *Builder) EnterNonListDataType(ctx *parser.NonListDataTypeContext) {
	word, repl := removedPrimitiveType(ctx.FLOAT_TYPE(), ctx.CURRENCY_TYPE())
	b.rejectRemovedPrimitiveType(ctx, word, repl)
	b.recordDateType(ctx.DATE_TYPE(), "")
	if ctx.HASHEDSTRING_TYPE() != nil {
		b.rejectHashedStringOutsideAttribute(ctx)
	}
}

// rejectParenthesisedAssociation refuses `association X (from … to …, opt, …)`.
// The visitor read only the unparenthesised options, so every option in this
// form was dropped: a ReferenceSet with table storage was stored as a Reference
// in a column. The message rewrites the statement in the form that works.
func (b *Builder) rejectParenthesisedAssociation(ctx *parser.CreateAssociationStatementContext) {
	names := ctx.AllQualifiedName()
	if len(names) < 3 {
		return
	}
	var opts []string
	for _, o := range ctx.AllAssociationOption() {
		var toks []string
		collectLeafTokens(o, &toks)
		kept := toks[:0]
		for _, t := range toks {
			if t != ":" {
				kept = append(kept, t)
			}
		}
		opts = append(opts, strings.Join(kept, " "))
	}
	canonical := fmt.Sprintf("create association %s from %s to %s",
		names[0].GetText(), names[1].GetText(), names[2].GetText())
	if len(opts) > 0 {
		canonical += " " + strings.Join(opts, " ")
	}
	b.addError(fmt.Errorf("%s: the parenthesised association form is not supported — its options "+
		"were parsed and dropped, so a ReferenceSet was stored as a Reference.\n"+
		"  Write the options after the entities, without parentheses or colons:\n"+
		"    %s;", ctxPos(ctx), canonical))
}

// ExitAttributeConstraint refuses `localized` / `not localized` on an attribute
// whose type is not a DateTime. LocalizeDate is a property of
// DomainModels$DateTimeAttributeType only; on any other type there is nowhere
// to store it, and accepting it would be a silent drop (#1373).
func (b *Builder) ExitAttributeConstraint(ctx *parser.AttributeConstraintContext) {
	if ctx.LOCALIZED() == nil {
		return
	}
	var dtCtx parser.IDataTypeContext
	switch p := ctx.GetParent().(type) {
	case *parser.AttributeDefinitionContext:
		dtCtx = p.DataType()
	case *parser.AlterEntityActionContext:
		dtCtx = p.DataType()
	}
	if dtCtx == nil {
		return
	}
	if dt := buildDataType(dtCtx); dt.Kind == ast.TypeDateTime {
		return
	}
	clause := "localized"
	if ctx.NOT() != nil {
		clause = "not localized"
	}
	b.addError(fmt.Errorf("%s: `%s` applies only to a DateTime attribute, not %s — "+
		"LocalizeDate is a property of the DateTime type and there is nowhere to store it on another type.\n"+
		"  Remove the clause, or declare the attribute as `DateTime %s`.",
		ctxPos(ctx), clause, dtCtx.GetText(), clause))
}
