// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateWidgetExpressionReturnTypes is MDL-WIDGET49: a pluggable widget's
// Expression property, declared to return a String, given an expression of
// another kind.
//
// A quoted MDL value on an Expression property IS the expression's text, so
// `dropdownfilter ddf (defaultValue: 'true')` stores the expression `true` — a
// Boolean where the drop-down filter's `defaultValue` returns a String. check
// and exec passed it and mxbuild failed CE0117 "Error(s) in expression."
// (mendixlabs/mxcli#1611 B). Measured on 11.14.0, one filter per value:
//
//	'true', 'false', '1', '1.5', 'abc'   → CE0117 (Boolean, Integer, Decimal, a bare name)
//	'''true''', '''abc''', 'empty'       → ok
//
// Only what is certain is judged: a String-returning property, and a value whose
// kind is a literal Boolean, Integer or Decimal, or that is one bare name (which
// is no Mendix expression at all). Anything that infers no kind — an attribute
// path, a variable, a function the checker does not type — is left alone.
func validateWidgetExpressionReturnTypes(w *ast.WidgetV3, def *WidgetDefinition,
	embedded *embeddedWidgetSchema, locationPrefix string) []linter.Violation {
	if w == nil || len(w.Properties) == 0 {
		return nil
	}
	schema := embedded
	if def != nil {
		schema = loadEmbeddedSchema(def.WidgetID, "")
	}
	if schema == nil || len(schema.returns) == 0 {
		return nil
	}
	keys := make([]string, 0, len(w.Properties))
	for k := range w.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []linter.Violation
	for _, key := range keys {
		lk := strings.ToLower(key)
		if schema.types[lk] != "Expression" || schema.returns[lk] != "String" {
			continue
		}
		var text string
		switch v := w.Properties[key].(type) {
		case string:
			text = v
		case *ast.WidgetExpressionV3:
			text = v.Text
		default:
			continue
		}
		text = strings.TrimSpace(text)
		got := ""
		switch {
		case bareExpressionNameRe.MatchString(text) && !isExpressionKeyword(text):
			got = "a bare name, which is no expression"
		default:
			switch exprcheck.InferSourceKind(text, nil) {
			case exprcheck.KindBoolean:
				got = "a Boolean"
			case exprcheck.KindInteger, exprcheck.KindLong:
				got = "an Integer"
			case exprcheck.KindDecimal:
				got = "a Decimal"
			}
		}
		if got == "" {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET49",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` property `%s` is an expression returning a String, and `%s` is %s — "+
					"mxbuild rejects it with CE0117 \"Error(s) in expression.\" A quoted value on an expression "+
					"property is the expression's text, not a string",
				locationPrefix, widgetLabelForError(w), key, text, got),
			Suggestion: fmt.Sprintf("Write the value as a String literal inside the quotes: `%s: %s`.",
				key, mdlStringOfExpressionLiteral(text)),
		})
	}
	return out
}

var bareExpressionNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func isExpressionKeyword(s string) bool {
	switch strings.ToLower(s) {
	case "true", "false", "empty":
		return true
	}
	return false
}

// mdlStringOfExpressionLiteral is the MDL spelling of the expression 'text': the
// Mendix string literal, quoted again for MDL with every apostrophe doubled.
func mdlStringOfExpressionLiteral(text string) string {
	literal := "'" + strings.ReplaceAll(text, "'", "''") + "'"
	return "'" + strings.ReplaceAll(literal, "'", "''") + "'"
}
