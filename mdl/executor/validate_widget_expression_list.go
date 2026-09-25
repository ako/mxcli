// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
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
				"write the expression as a quoted string, doubling the quotes inside it: "+
					"%s: 'if $currentObject/Featured then ''is-featured'' else '''''", key),
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
