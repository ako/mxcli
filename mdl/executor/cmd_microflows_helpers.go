// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow helper functions
package executor

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// convertASTToMicroflowDataType converts an AST DataType to a microflows.DataType.
// entityResolver is optional - if provided, it resolves entity qualified names to IDs.
func convertASTToMicroflowDataType(dt ast.DataType, entityResolver func(ast.QualifiedName) model.ID) microflows.DataType {
	switch dt.Kind {
	case ast.TypeBoolean:
		return &microflows.BooleanType{}
	case ast.TypeInteger:
		return &microflows.IntegerType{}
	case ast.TypeLong:
		return &microflows.LongType{}
	case ast.TypeDecimal:
		return &microflows.DecimalType{}
	case ast.TypeString:
		return &microflows.StringType{}
	case ast.TypeDateTime:
		return &microflows.DateTimeType{}
	case ast.TypeDate:
		return &microflows.DateType{}
	case ast.TypeBinary:
		return &microflows.BinaryType{}
	case ast.TypeVoid:
		return &microflows.VoidType{}
	case ast.TypeEntity:
		lt := &microflows.ObjectType{}
		if dt.EntityRef != nil {
			// Set qualified name for BY_NAME_REFERENCE serialization
			lt.EntityQualifiedName = dt.EntityRef.Module + "." + dt.EntityRef.Name
			if entityResolver != nil {
				lt.EntityID = entityResolver(*dt.EntityRef)
			}
		}
		return lt
	case ast.TypeListOf:
		lt := &microflows.ListType{}
		if dt.EntityRef != nil {
			// Set qualified name for BY_NAME_REFERENCE serialization
			lt.EntityQualifiedName = dt.EntityRef.Module + "." + dt.EntityRef.Name
			if entityResolver != nil {
				lt.EntityID = entityResolver(*dt.EntityRef)
			}
		}
		return lt
	case ast.TypeEnumeration:
		et := &microflows.EnumerationType{}
		if dt.EnumRef != nil {
			// Set qualified name for BY_NAME_REFERENCE serialization
			et.EnumerationQualifiedName = dt.EnumRef.Module + "." + dt.EnumRef.Name
		}
		return et
	default:
		return &microflows.VoidType{}
	}
}

// mendixBuiltinFunctions is the canonical spelling of every built-in Mendix
// expression function. The expression runtime is case-sensitive: it only
// recognises these names as spelt here (lower-case with camelCase for
// compound words). Emitting an alternative spelling causes CE0117
// ("Error(s) in expression.") on Studio Pro validation.
//
// Source: https://docs.mendix.com/refguide/expressions/ and the linked
// function-specific pages (string, math, date arithmetic, parse/format,
// trim-to-date, list operations, aggregates, type conversions).
//
// The map key is the upper-case spelling for case-insensitive lookup; the
// value is the runtime-accepted canonical spelling. Custom user-defined
// java actions, sub-microflows, and unknown function names pass through
// unchanged so user case is preserved.
var mendixBuiltinFunctions = func() map[string]string {
	canonical := []string{
		// List operations
		"head", "tail", "find", "filter", "sort", "union",
		"intersect", "subtract", "contains", "equals", "range",
		// List aggregates
		"count", "sum", "average", "minimum", "maximum",
		"allTrue", "anyTrue",
		// String functions (docs.mendix.com/refguide/string-function-calls)
		"toUpperCase", "toLowerCase", "trim", "length", "substring",
		"findLast", "replaceAll", "replaceFirst", "startsWith", "endsWith",
		"isMatch", "isInvariantMatch", "stringFromRegex", "stringListFromRegex",
		"urlEncode", "urlDecode", "reverse", "indexOf",
		// Math functions (docs.mendix.com/refguide/mathematical-function-calls)
		"abs", "ceil", "floor", "round", "max", "min", "pow",
		"sqrt", "ln", "log10", "random", "rand",
		// Date creation (docs.mendix.com/refguide/date-creation)
		"dateTime", "dateTimeUTC",
		// Begin-of-date / end-of-date / trim-to-date
		"trimToDays", "trimToHours", "trimToMinutes", "trimToSeconds",
		"trimToDaysUTC", "trimToHoursUTC", "trimToMinutesUTC", "trimToSecondsUTC",
		"beginOfDay", "beginOfWeek", "beginOfMonth", "beginOfYear",
		"beginOfDayUTC", "beginOfWeekUTC", "beginOfMonthUTC", "beginOfYearUTC",
		"endOfDay", "endOfWeek", "endOfMonth", "endOfYear",
		"endOfDayUTC", "endOfWeekUTC", "endOfMonthUTC", "endOfYearUTC",
		// Between-date functions
		"millisecondsBetween", "secondsBetween", "minutesBetween",
		"hoursBetween", "daysBetween", "weeksBetween", "monthsBetween",
		"yearsBetween", "calendarDaysBetween", "calendarMonthsBetween",
		"calendarYearsBetween",
		// Add-date functions
		"addMilliseconds", "addSeconds", "addMinutes", "addHours",
		"addDays", "addWeeks", "addMonths", "addYears",
		"addDaysUTC", "addWeeksUTC", "addMonthsUTC", "addYearsUTC",
		// Subtract-date functions
		"subtractMilliseconds", "subtractSeconds", "subtractMinutes",
		"subtractHours", "subtractDays", "subtractWeeks", "subtractMonths",
		"subtractYears", "subtractDaysUTC", "subtractWeeksUTC",
		"subtractMonthsUTC", "subtractYearsUTC",
		// Day-of / timestamp conversion helpers
		"dayOfWeek", "dayOfWeekFromDateTime", "weekOfYearFromDateTime",
		"dayOfYearFromDateTime", "daysInMonth", "daysInYear",
		"dateTimeToEpoch", "epochToDateTime",
		// Parse / format (parse-and-format-date, parse-and-format-decimal)
		"formatDateTime", "formatDateTimeUTC", "parseDateTime", "parseDateTimeUTC",
		"parseInteger", "parseLong", "parseDecimal", "formatDecimal",
		// To-string / length  (to-string, length refguide pages)
		"toString", "toBoolean", "toFloat",
		// Enumeration helpers
		"getCaption", "getKey",
		// Miscellaneous
		"if", "empty", "isNew", "isAnonymous",
		// Boolean operators expressed as functions (true(), false())
		"true", "false",
		// Not / and / or appear as operators, not function calls — omitted.
	}
	m := make(map[string]string, len(canonical))
	for _, c := range canonical {
		m[strings.ToUpper(c)] = c
	}
	return m
}()

// mendixFunctionName normalises the case of built-in Mendix expression
// functions. The visitor canonicalises list / aggregate operations in
// UPPERCASE for AST dispatch; the expression runtime only recognises the
// documented camelCase spelling. For every built-in Mendix function we
// always emit the canonical spelling so that:
//
//   - round-tripping a pristine microflow never mutates `find(...)` into
//     `FIND(...)` (which Studio Pro rejects with CE0117).
//   - LLM-generated MDL with accidental capitalisation (`LENGTH(...)`,
//     `ToString(...)`) still validates when executed.
//
// Custom (user-defined) java actions, sub-microflows and entity member
// references pass through unchanged so user case is preserved.
func mendixFunctionName(name string) string {
	if canonical, ok := mendixBuiltinFunctions[strings.ToUpper(name)]; ok {
		return canonical
	}
	return name
}

// quoteExpressionLiteral renders a Go string as a MENDIX expression literal.
//
// Its output goes into the stored document, so it must be what Mendix's
// expression engine reads — and that engine has exactly ONE escape: an
// apostrophe is doubled, SQL-style. There are no backslash escapes.
// Measured against a running 11.13 runtime: a microflow storing 'a\tb'
// (backslash, t) put FOUR bytes in the database, 61 5c 74 62 — a literal
// backslash and a 't', not a tab. That is the bug this function used to have.
//
// It escaped \n, \r and \t on the premise that "STRING_LITERAL does not accept
// them raw and the describe output has to survive check". The premise is false
// on both halves. The lexer rule is
//
//	STRING_LITERAL : '\'' ( ~['\\] | '\\' . | '\'\'' )* '\''
//
// and `~['\\]` admits every byte except an apostrophe and a backslash —
// newline, tab and carriage return included. A raw newline already round-tripped
// through describe → exec for exactly that reason, which is why only the tab
// looked broken: the newline path never reached the escape.
//
// So the escaping bought nothing and cost the value. A raw control character is
// now emitted as itself: correct in the document, and parseable on the way back.
//
// What IS still escaped, and must be:
//
//   - an apostrophe, doubled — the engine's only escape, and the MDL lexer's too;
//   - a backslash whose NEXT byte is one of n/r/t/\/', doubled — otherwise
//     unquoteString would decode the pair into a control character on reparse,
//     turning a literal two-character `\t` into a tab. A backslash before any
//     other byte passes through verbatim, so a regex literal like `^\d+$`
//     survives (the engine reads `\d` literally and hands it to the regex
//     compiler).
func quoteExpressionLiteral(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\'':
			b.WriteString(`''`)
		case '\\':
			// Double the backslash only when the next byte would otherwise be
			// interpreted as an escape by unquoteString — that is, n/r/t/\/'.
			// For any other follower (letters like d/w, punctuation) the
			// backslash can pass through verbatim so regex escape characters
			// roundtrip without mutation.
			if i+1 < len(s) {
				switch s[i+1] {
				case 'n', 'r', 't':
					b.WriteString(`\\`)
					b.WriteByte(s[i+1])
					i++
					continue
				case '\\':
					// Literal backslash-backslash in AST. To survive roundtrip
					// it must be written as four backslashes: unquoteString
					// decodes `\\` twice, producing two backslashes again.
					b.WriteString(`\\\\`)
					i++
					continue
				case '\'':
					// Literal backslash-apostrophe: double the backslash and
					// double the apostrophe, so the reparsed value stays
					// [\, '].
					b.WriteString(`\\`)
					b.WriteString(`''`)
					i++
					continue
				}
				b.WriteByte('\\')
				continue
			}
			// Trailing backslash at end-of-string: the lexer's `'\\' .` escape
			// rule requires a following character, so emitting a bare `\'`
			// terminator would be reinterpreted as an escape pair and never
			// close the literal. Double the backslash — unquoteString decodes
			// `\\` back to a single backslash.
			b.WriteString(`\\`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// expressionToString converts an AST Expression to a Mendix expression string.
//
// The string it returns is STORED — `action.Expression = expressionToString(...)`
// — so it is Mendix's grammar throughout, not MDL's. Literals go through
// quoteExpressionLiteral, which doubles an apostrophe and otherwise emits the
// value as it is.
//
// This used to say the opposite, and reasoned from it: that escaping a control
// character was "the correct trade-off for describe→re-execute flows" because
// "emitting raw control chars in MDL would break the parser". Both halves were
// wrong. STRING_LITERAL's `~['\\]` accepts every byte but an apostrophe and a
// backslash, so raw control characters parse — and the trade-off was not one,
// because the escaped form is what the runtime then stores: 'a\tb' became the
// four bytes `a`, `\`, `t`, `b` in the database, not a tab.
func expressionToString(expr ast.Expression) string {
	// Check for nil interface
	if expr == nil {
		return ""
	}

	// Use reflection to check for nil pointer inside interface
	// This handles the Go interface gotcha where the type is set but pointer is nil
	if reflect.ValueOf(expr).IsNil() {
		return ""
	}

	switch e := expr.(type) {
	case *ast.LiteralExpr:
		switch e.Kind {
		case ast.LiteralString:
			return quoteExpressionLiteral(fmt.Sprintf("%v", e.Value))
		case ast.LiteralBoolean:
			if e.Value.(bool) {
				return "true"
			}
			return "false"
		case ast.LiteralNull:
			return "empty"
		default:
			return fmt.Sprintf("%v", e.Value)
		}
	case *ast.VariableExpr:
		return "$" + e.Name
	case *ast.AttributePathExpr:
		return "$" + e.Variable + "/" + strings.Join(e.Path, "/")
	case *ast.BinaryExpr:
		left := expressionToString(e.Left)
		right := expressionToString(e.Right)
		// Mendix expressions use lowercase operators (and, or, div, mod)
		op := strings.ToLower(e.Operator)
		return left + " " + op + " " + right
	case *ast.UnaryExpr:
		// Mendix expressions use lowercase operators (not)
		op := strings.ToLower(e.Operator)
		// not(expr) — always emit parens; Mendix CE0117 rejects bare "not expr"
		if op == "not" {
			inner := e.Operand
			if paren, ok := inner.(*ast.ParenExpr); ok {
				// Unwrap double-parens: not((expr)) → not(expr)
				return "not(" + expressionToString(paren.Inner) + ")"
			}
			return "not(" + expressionToString(inner) + ")"
		}
		operand := expressionToString(e.Operand)
		return op + " " + operand
	case *ast.FunctionCallExpr:
		var args []string
		for _, arg := range e.Arguments {
			args = append(args, expressionToString(arg))
		}
		return mendixFunctionName(e.Name) + "(" + strings.Join(args, ", ") + ")"
	case *ast.TokenExpr:
		return "[%" + e.Token + "%]"
	case *ast.ParenExpr:
		return "(" + expressionToString(e.Inner) + ")"
	case *ast.IdentifierExpr:
		// Unquoted identifier (attribute name in XPath)
		return e.Name
	case *ast.QualifiedNameExpr:
		// Qualified name (association name, entity reference) - unquoted
		return e.QualifiedName.String()
	case *ast.ConstantRefExpr:
		return "@" + e.QualifiedName.String()
	case *ast.IfThenElseExpr:
		cond := expressionToString(e.Condition)
		thenStr := expressionToString(e.ThenExpr)
		elseStr := expressionToString(e.ElseExpr)
		return "if " + cond + " then " + thenStr + " else " + elseStr
	case *ast.SourceExpr:
		if e.Source != "" {
			return normalizeMendixOperatorCase(e.Source)
		}
		return expressionToString(e.Expression)
	default:
		return ""
	}
}

// mendixLowercaseOperators are the word operators Mendix requires in lowercase.
// A rebuilt BinaryExpr/UnaryExpr already gets this via strings.ToLower on the
// operator; preserved source text does not, which is the whole bug below.
var mendixLowercaseOperators = map[string]bool{
	"and": true, "or": true, "not": true, "div": true, "mod": true,
}

// normalizeMendixOperatorCase lowercases word operators in preserved expression
// source, leaving everything else — including string literals and member names —
// byte-identical.
//
// Some conditions are kept as a SourceExpr (original text plus the parsed tree)
// rather than rebuilt from the AST, and the raw branch skipped the lowercasing
// that a rebuilt expression gets. So `IF A != x AND B != empty` stored `AND`
// verbatim and the build failed with
//
//	[CE0117] "Error(s) in expression."
//
// while the same condition written with `=` was rebuilt as a BinaryExpr and
// normalised — which is why it looked like `!=` inside a conjunction was
// unsupported. It is the casing, not the operator. (mxcli-todo findings #14b)
//
// A word preceded by `.`, `/` or `$` is a member or variable name, never an
// operator, so `Module.Enum.And` and `$Task/Mod` are left alone.
func normalizeMendixOperatorCase(src string) string {
	var b strings.Builder
	b.Grow(len(src))

	inString := false
	for i := 0; i < len(src); {
		c := src[i]
		if inString {
			b.WriteByte(c)
			if c == '\'' {
				// '' is an escaped quote inside a Mendix string literal.
				if i+1 < len(src) && src[i+1] == '\'' {
					b.WriteByte(src[i+1])
					i += 2
					continue
				}
				inString = false
			}
			i++
			continue
		}
		if c == '\'' {
			inString = true
			b.WriteByte(c)
			i++
			continue
		}
		if !isWordByte(c) {
			b.WriteByte(c)
			i++
			continue
		}
		j := i
		for j < len(src) && isWordByte(src[j]) {
			j++
		}
		word := src[i:j]
		prev := byte(0)
		if i > 0 {
			prev = src[i-1]
		}
		if prev != '.' && prev != '/' && prev != '$' && mendixLowercaseOperators[strings.ToLower(word)] {
			b.WriteString(strings.ToLower(word))
		} else {
			b.WriteString(word)
		}
		i = j
	}
	return b.String()
}

func isWordByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// expressionToXPath converts an AST Expression to an XPath constraint string.
// Unlike expressionToString (for Mendix expressions), XPath requires Mendix
// tokens like [%CurrentDateTime%] to be quoted: '[%CurrentDateTime%]'.
func expressionToXPath(expr ast.Expression) string {
	if expr == nil {
		return ""
	}
	if reflect.ValueOf(expr).IsNil() {
		return ""
	}

	switch e := expr.(type) {
	case *ast.TokenExpr:
		return "'[%" + e.Token + "%]'"
	case *ast.BinaryExpr:
		left := expressionToXPath(e.Left)
		right := expressionToXPath(e.Right)
		op := strings.ToLower(e.Operator)
		return left + " " + op + " " + right
	case *ast.UnaryExpr:
		operand := expressionToXPath(e.Operand)
		op := strings.ToLower(e.Operator)
		// For 'not' with parenthesized operand, output as not(expr)
		if op == "not" {
			if p, ok := e.Operand.(*ast.ParenExpr); ok {
				return "not(" + expressionToXPath(p.Inner) + ")"
			}
			return "not(" + operand + ")"
		}
		return op + " " + operand
	case *ast.ParenExpr:
		return "(" + expressionToXPath(e.Inner) + ")"
	case *ast.XPathPathExpr:
		return xpathPathExprToString(e)
	case *ast.FunctionCallExpr:
		var args []string
		for _, arg := range e.Arguments {
			args = append(args, expressionToXPath(arg))
		}
		return mendixFunctionName(e.Name) + "(" + strings.Join(args, ", ") + ")"
	case *ast.LiteralExpr:
		if e.Kind == ast.LiteralEmpty {
			return "empty"
		}
		return expressionToString(expr)
	case *ast.QualifiedNameExpr:
		return qualifiedNameToXPath(e)
	case *ast.SourceExpr:
		if e.Source != "" {
			return e.Source
		}
		return expressionToXPath(e.Expression)
	default:
		// For all other expression types, the standard serialization is correct
		return expressionToString(expr)
	}
}

// qualifiedNameToXPath converts a QualifiedNameExpr to XPath format.
// XPath constraints are evaluated at the database level where enum values are stored as plain strings.
// 3-part names (Module.EnumName.Value) must be converted to string literals ('Value').
// 2-part names (Module.AssocName) are association references and are passed through as-is.
func qualifiedNameToXPath(e *ast.QualifiedNameExpr) string {
	if dotIdx := strings.LastIndex(e.QualifiedName.Name, "."); dotIdx >= 0 {
		return "'" + e.QualifiedName.Name[dotIdx+1:] + "'"
	}
	return e.QualifiedName.String()
}

// xpathEnumRefRe matches 3-part qualified enum value references like Module.EnumName.Value
// in a raw XPath string. These must be replaced with string literals for database queries.
var xpathEnumRefRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]*\.[A-Za-z][A-Za-z0-9_]*\.[A-Za-z][A-Za-z0-9_]*`)

// normalizeXPathEnumRefs converts 3-part qualified enum value references in a raw XPath
// string to the string literal format that Mendix database queries require.
// Example: "[Status = XpathTest.OrderStatus.Open]" → "[Status = 'Open']".
// This handles the SourceExpr (bracketed) path where qualifiedNameToXPath is bypassed.
//
// INSIDE A STRING LITERAL the letters are data and are left alone, the same rule
// NormalizeXPathOperators follows. Running the regex over the whole constraint
// rewrote a ref the author had already quoted into a DOUBLED quote —
// `'Mod.Enum.Value'` became `''Value''` — which every downstream reader lexes as
// an empty string literal followed by a stray bare word. The stored constraint
// then read `Status = Mod.Enum.` with the value, the closing quote and the ENTIRE
// following clause gone, `mxcli check` passing, and mxbuild reporting only a
// generic CE0161 that does not say which clause (ako/CapTrackV3 FINDINGS §29).
//
// A quoted ref is left exactly as written rather than repaired. Consuming the
// author's quotes would make `Name = 'a.b.c'` — an ordinary literal that happens
// to have two dots — silently match something else, and telling those two apart
// needs the project's enumerations, which this string-level pass does not have.
// MDL-XPATH01 reports the quoted spelling at check time, where the enum can
// actually be resolved.
func normalizeXPathEnumRefs(xpath string) string {
	var b strings.Builder
	b.Grow(len(xpath))
	for i := 0; i < len(xpath); {
		if xpath[i] != '\'' {
			j := strings.IndexByte(xpath[i:], '\'')
			if j < 0 {
				b.WriteString(xpathEnumRefRe.ReplaceAllStringFunc(xpath[i:], enumRefToLiteral))
				break
			}
			b.WriteString(xpathEnumRefRe.ReplaceAllStringFunc(xpath[i:i+j], enumRefToLiteral))
			i += j
			continue
		}
		b.WriteString(xpath[i:xpathLiteralEnd(xpath, i)])
		i = xpathLiteralEnd(xpath, i)
	}
	return b.String()
}

// enumRefToLiteral turns `Module.Enum.Value` into the `'Value'` a database query
// compares against.
func enumRefToLiteral(match string) string {
	return "'" + match[strings.LastIndex(match, ".")+1:] + "'"
}

// xpathLiteralEnd returns the index just past the string literal starting at
// start. A doubled quote inside one is an escaped quote, not the end — the same
// rule the MDL expression scanner and NormalizeXPathOperators use. An unclosed
// literal runs to the end of the constraint, which keeps the caller's loop
// terminating on malformed input instead of stalling.
func xpathLiteralEnd(s string, start int) int {
	for j := start + 1; j < len(s); j++ {
		if s[j] != '\'' {
			continue
		}
		if j+1 < len(s) && s[j+1] == '\'' {
			j++
			continue
		}
		return j + 1
	}
	return len(s)
}

// memberExpressionToString converts an AST Expression to a Mendix expression string,
// resolving enum string literals to qualified enum names when the attribute type is known.
// For example, 'Processing' becomes MyModule.ENUM_Status.Processing when the attribute
// is of type Enumeration(MyModule.ENUM_Status).
func (fb *flowBuilder) memberExpressionToString(expr ast.Expression, entityQN, attrName string) string {
	// Only transform string literals for enum attributes
	if lit, ok := expr.(*ast.LiteralExpr); ok && lit.Kind == ast.LiteralString {
		if enumRef := fb.lookupEnumRef(entityQN, attrName); enumRef != "" {
			// Convert 'Value' to Module.EnumName.Value
			return enumRef + "." + fmt.Sprintf("%v", lit.Value)
		}
	}
	return fb.exprToString(expr)
}

// lookupEnumRef returns the enumeration qualified name (e.g., "MyModule.ENUM_Status")
// for an attribute if it is an enumeration type. Returns "" if the attribute is not
// an enumeration or if the domain model is not available.
func (fb *flowBuilder) lookupEnumRef(entityQN, attrName string) string {
	if fb.backend == nil || entityQN == "" || attrName == "" {
		return ""
	}
	parts := strings.SplitN(entityQN, ".", 2)
	if len(parts) != 2 {
		return ""
	}
	mod, err := fb.backend.GetModuleByName(parts[0])
	if err != nil || mod == nil {
		return ""
	}
	dm, err := fb.backend.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return ""
	}
	for _, entity := range dm.Entities {
		if entity.Name == parts[1] {
			for _, attr := range entity.Attributes {
				if attr.Name == attrName {
					if enumType, ok := attr.Type.(*domainmodel.EnumerationAttributeType); ok {
						return enumType.EnumerationRef
					}
					return ""
				}
			}
			return ""
		}
	}
	return ""
}

// ============================================================================
// XPath Enum Enrichment (DESCRIBE output)
// ============================================================================

// enrichXPathExprWithEnums walks an XPath AST and replaces string-literal comparisons
// against known enum attributes with QualifiedNameExpr references, so DESCRIBE output
// reads as Module.EnumName.Value rather than 'Value'.
//
// enumAttrs maps bare attribute name → enumeration qualified name, e.g.
// "Status" → "XpathTest.OrderStatus".
func enrichXPathExprWithEnums(expr ast.Expression, enumAttrs map[string]string) ast.Expression {
	if expr == nil || len(enumAttrs) == 0 {
		return expr
	}
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		// attr = 'Value' or 'Value' = attr
		if attr := xpathBareAttrName(e.Left); attr != "" {
			if enumRef, ok := enumAttrs[attr]; ok {
				if lit, ok2 := e.Right.(*ast.LiteralExpr); ok2 && lit.Kind == ast.LiteralString {
					return &ast.BinaryExpr{
						Left:     e.Left,
						Operator: e.Operator,
						Right:    enumStringToQN(enumRef, fmt.Sprintf("%v", lit.Value)),
					}
				}
			}
		} else if attr := xpathBareAttrName(e.Right); attr != "" {
			if enumRef, ok := enumAttrs[attr]; ok {
				if lit, ok2 := e.Left.(*ast.LiteralExpr); ok2 && lit.Kind == ast.LiteralString {
					return &ast.BinaryExpr{
						Left:     enumStringToQN(enumRef, fmt.Sprintf("%v", lit.Value)),
						Operator: e.Operator,
						Right:    e.Right,
					}
				}
			}
		}
		return &ast.BinaryExpr{
			Left:     enrichXPathExprWithEnums(e.Left, enumAttrs),
			Operator: e.Operator,
			Right:    enrichXPathExprWithEnums(e.Right, enumAttrs),
		}
	case *ast.UnaryExpr:
		return &ast.UnaryExpr{Operator: e.Operator, Operand: enrichXPathExprWithEnums(e.Operand, enumAttrs)}
	case *ast.ParenExpr:
		return &ast.ParenExpr{Inner: enrichXPathExprWithEnums(e.Inner, enumAttrs)}
	case *ast.FunctionCallExpr:
		enriched := make([]ast.Expression, len(e.Arguments))
		for i, arg := range e.Arguments {
			enriched[i] = enrichXPathExprWithEnums(arg, enumAttrs)
		}
		return &ast.FunctionCallExpr{Name: e.Name, Arguments: enriched}
	}
	return expr
}

// xpathBareAttrName returns the bare attribute name if expr is a simple
// IdentifierExpr (e.g. "Status"), otherwise "".
func xpathBareAttrName(expr ast.Expression) string {
	if id, ok := expr.(*ast.IdentifierExpr); ok {
		return id.Name
	}
	return ""
}

// enumStringToQN builds a QualifiedNameExpr for an enum value reference.
// enumRef = "Module.EnumName", valueKey = "Open" → Module: "Module", Name: "EnumName.Open"
//
// A value that is ALREADY the qualified name is returned as it stands. Mendix
// stores an enum comparison as the bare value, so a stored `'Module.Enum.Value'`
// is a constraint the author quoted by mistake (ako/CapTrackV3 FINDINGS §29) —
// prefixing it again rendered `Xp.Status.Xp.Status.Confirmed`, which a
// describe → exec round-trip would then write back into the model. Describing an
// invalid constraint should show what is stored, not compound it.
func enumStringToQN(enumRef, valueKey string) *ast.QualifiedNameExpr {
	if strings.HasPrefix(valueKey, enumRef+".") {
		valueKey = strings.TrimPrefix(valueKey, enumRef+".")
	}
	parts := strings.SplitN(enumRef, ".", 2)
	if len(parts) != 2 {
		return &ast.QualifiedNameExpr{QualifiedName: ast.QualifiedName{Module: enumRef, Name: valueKey}}
	}
	return &ast.QualifiedNameExpr{
		QualifiedName: ast.QualifiedName{Module: parts[0], Name: parts[1] + "." + valueKey},
	}
}

// xpathExprToMDLString serializes an XPath expression for MDL DESCRIBE output.
// Unlike expressionToXPath (which converts enum QualifiedNameExpr → 'Value' for BSON),
// this preserves enum QualifiedNameExpr as Module.EnumName.Value for readability.
func xpathExprToMDLString(expr ast.Expression) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.QualifiedNameExpr:
		// Output qualified names as-is (including 3-part enum references)
		return e.QualifiedName.String()
	case *ast.BinaryExpr:
		left := xpathExprToMDLString(e.Left)
		right := xpathExprToMDLString(e.Right)
		op := strings.ToLower(e.Operator)
		return left + " " + op + " " + right
	case *ast.UnaryExpr:
		operand := xpathExprToMDLString(e.Operand)
		op := strings.ToLower(e.Operator)
		if op == "not" {
			if p, ok := e.Operand.(*ast.ParenExpr); ok {
				return "not(" + xpathExprToMDLString(p.Inner) + ")"
			}
			return "not(" + operand + ")"
		}
		return op + " " + operand
	case *ast.ParenExpr:
		return "(" + xpathExprToMDLString(e.Inner) + ")"
	case *ast.FunctionCallExpr:
		var args []string
		for _, arg := range e.Arguments {
			args = append(args, xpathExprToMDLString(arg))
		}
		return mendixFunctionName(e.Name) + "(" + strings.Join(args, ", ") + ")"
	case *ast.XPathPathExpr:
		var parts []string
		for _, step := range e.Steps {
			s := xpathExprToMDLString(step.Expr)
			if step.Predicate != nil {
				s += "[" + xpathExprToMDLString(step.Predicate) + "]"
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, "/")
	default:
		// For all other types (literals, variables, tokens, etc.) use the BSON serializer —
		// they don't need MDL-specific output.
		return expressionToXPath(expr)
	}
}

// xpathPathExprToString serializes an XPathPathExpr to an XPath path string.
func xpathPathExprToString(path *ast.XPathPathExpr) string {
	var parts []string
	for _, step := range path.Steps {
		s := expressionToXPath(step.Expr)
		if step.Predicate != nil {
			s += "[" + expressionToXPath(step.Predicate) + "]"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "/")
}

// countMicroflowActivities counts the number of meaningful activities in a microflow.
// Excludes structural elements like StartEvent, EndEvent, and merge nodes.
func countMicroflowActivities(mf *microflows.Microflow) int {
	if mf.ObjectCollection == nil {
		return 0
	}

	count := 0
	for _, obj := range mf.ObjectCollection.Objects {
		switch obj.(type) {
		case *microflows.StartEvent, *microflows.EndEvent:
			// Don't count start/end events
		case *microflows.ExclusiveMerge:
			// Don't count merge nodes (they're structural)
		default:
			// Count all other activities (ActionActivity, ExclusiveSplit, LoopedActivity, etc.)
			count++
		}
	}
	return count
}

// calculateMicroflowComplexity calculates the McCabe cyclomatic complexity of a microflow.
// McCabe complexity = 1 + number of decision points (IF, LOOP, error handlers)
// A higher complexity indicates more paths through the code and higher testing burden.
// Typical thresholds: 1-10 (simple), 11-20 (moderate), 21-50 (complex), 50+ (untestable)
func calculateMicroflowComplexity(mf *microflows.Microflow) int {
	// Base complexity is 1 (the main path through the microflow)
	complexity := 1

	if mf.ObjectCollection == nil {
		return complexity
	}

	// Count decision points in the main flow
	complexity += countMicroflowDecisionPoints(mf.ObjectCollection.Objects)

	return complexity
}

// countMicroflowDecisionPoints counts decision points in a list of microflow objects.
// This recursively processes nested structures like LoopedActivity.
func countMicroflowDecisionPoints(objects []microflows.MicroflowObject) int {
	count := 0

	for _, obj := range objects {
		switch activity := obj.(type) {
		case *microflows.ExclusiveSplit:
			// Each IF/decision adds 1 to complexity
			count++

		case *microflows.InheritanceSplit:
			// Type check split adds 1 to complexity
			count++

		case *microflows.LoopedActivity:
			// Each loop adds 1 to complexity
			count++
			// Also count decision points inside the loop body
			if activity.ObjectCollection != nil {
				count += countMicroflowDecisionPoints(activity.ObjectCollection.Objects)
			}

		case *microflows.ErrorEvent:
			// Error handling path adds complexity
			count++
		}
	}

	return count
}
