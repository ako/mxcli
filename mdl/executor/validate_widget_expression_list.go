// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// listValuedExpressionProps are the widget properties whose value is ONE Mendix
// expression, read by a writer that accepts only a string: DynamicClasses
// (WidgetV3.GetDynamicClasses) and a datagrid column's DynamicCellClass (the
// `columnClass` builder in widgetobj/datagrid_column.go).
var listValuedExpressionProps = []string{"DynamicClasses", "DynamicCellClass"}

// validateExpressionPropertyLists (MDL-WIDGET32) rejects an expression property
// written in brackets:
//
//	container c1 (dynamicclasses: [ if $currentObject/Featured then 'x' else '' ])
//
// mendixlabs/mxcli#750 proposes exactly this spelling, and it already parses —
// as propertyValueV3's array alternative, into a []string. Both writers take
// only a string, so the value was discarded: `check` clean, `exec` reporting
// success, the widget stored with no dynamic class. The same silent drop as
// #999 (MDL-WIDGET27, which covers the empty `[]`), reached by the spelling an
// issue proposes as the fix.
//
// An error, not a warning, for MDL-WIDGET27's reason: `exec` refuses only on
// errors, and a warning would leave it free to write the page and drop the
// value. Keyed on the property, never on the brackets — `visible: [cond]` is how
// MDL spells a conditional, and a filter's `attributes: [Name]` is a real list.
// Needs no project: the value's shape is wrong whatever the widget declares.
func validateExpressionPropertyLists(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || len(w.Properties) == 0 {
		return nil
	}
	keys := make([]string, 0, len(w.Properties))
	for k := range w.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys) // stable output: map order would make two runs disagree

	var out []linter.Violation
	for _, key := range keys {
		items, ok := w.Properties[key].([]string)
		if !ok || len(items) == 0 || !isListValuedExpressionProp(key) {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET32",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` property `%s` holds one Mendix expression, but is written as a bracketed list — "+
					"the value is discarded on write",
				locationPrefix, w.Name, key),
			Suggestion: fmt.Sprintf(
				"write the expression itself, without brackets: "+
					"%s: if $currentObject/Featured then 'is-featured' else ''", key),
		})
	}
	return out
}

func isListValuedExpressionProp(key string) bool {
	for _, p := range listValuedExpressionProps {
		if strings.EqualFold(p, key) {
			return true
		}
	}
	return false
}

// legacyExpressionTextRe recognises the content of the OLD spelling of an
// expression property: a quoted string holding the expression's text. A class
// name or class list never contains a `$` (a variable) or a quote character, and
// does not start with `if`; the old expression text nearly always does one of
// the three.
var legacyExpressionTextRe = regexp.MustCompile(`\$|'|^\s*if\b`)

// validateLegacyExpressionText (MDL-WIDGET33) reports the old spelling of
// DynamicClasses / DynamicCellClass:
//
//	dynamicclasses: 'if $currentObject/F then ''a'' else '''''    -- old
//	dynamicclasses: if $currentObject/F then 'a' else ''            -- now
//
// The property holds a Mendix expression written as-is, so a quoted value is a
// Mendix string (PROPOSAL_first_class_expressions.md slice 2, the same rule as
// the OData client's credentials). The old spelling still parses and would now
// store the expression's TEXT as a class-name string — valid, silent, and never
// the class the author meant. An error, so exec refuses it too; the suggestion
// is the expression with the quoting removed.
func validateLegacyExpressionText(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || len(w.Properties) == 0 {
		return nil
	}
	keys := make([]string, 0, len(w.Properties))
	for k := range w.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []linter.Violation
	for _, key := range keys {
		if !isListValuedExpressionProp(key) {
			continue
		}
		s, ok := w.Properties[key].(string)
		if !ok {
			continue
		}
		if v, bad := legacyExpressionTextViolation(fmt.Sprintf("%s: widget `%s`", locationPrefix, w.Name), key, s); bad {
			out = append(out, v)
		}
	}
	return out
}

func legacyExpressionTextViolation(where, key, expr string) (linter.Violation, bool) {
	content, isLiteral := mendixStringLiteral(expr)
	if !isLiteral || !legacyExpressionTextRe.MatchString(content) {
		return linter.Violation{}, false
	}
	return linter.Violation{
		RuleID:   "MDL-WIDGET33",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"%s property `%s` is a quoted string holding an expression — %s now takes the expression itself, "+
				"so this would store the text as a class name", where, key, key),
		Suggestion: fmt.Sprintf("write the expression without the outer quotes and the doubled ones: %s: %s", key, content),
	}, true
}

// validateAlterSetLegacyExpressionText is MDL-WIDGET33 for ALTER PAGE … SET.
func validateAlterSetLegacyExpressionText(op *ast.SetPropertyOp, locationPrefix string) []linter.Violation {
	keys := make([]string, 0, len(op.Properties))
	for k := range op.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []linter.Violation
	for _, key := range keys {
		s, ok := op.Properties[key].(string)
		if !ok || !isListValuedExpressionProp(key) {
			continue
		}
		where := fmt.Sprintf("%s: set on `%s`", locationPrefix, op.Target.Widget)
		if v, bad := legacyExpressionTextViolation(where, key, s); bad {
			out = append(out, v)
		}
	}
	return out
}
