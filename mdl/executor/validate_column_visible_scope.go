// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

var currentObjectRe = regexp.MustCompile(`(?i)\$currentObject\b`)

// validateColumnVisibleScope (MDL-WIDGET43) refuses `$currentObject` in a
// DataGrid 2 column's Visible:
//
//	column (attribute: Name, Visible: $currentObject/Featured)   -- CE0117
//
// A column's `visible` expression is evaluated once for the grid, not per row:
// the widget declares it with no dataSource, unlike `columnClass`, so there is
// no row object and mxbuild reports CE0117 "Error(s) in expression". The
// bracketed form `Visible: [Attr > 1]` roots a bare attribute in $currentObject
// and lands here too. Until the column builder read the expression it was
// dropped, so these always checked and executed clean; an error now, so exec
// refuses rather than writing a page that fails the build.
func validateColumnVisibleScope(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || !strings.EqualFold(w.Type, "column") {
		return nil
	}
	return columnVisibleScopeViolation(fmt.Sprintf("%s: datagrid column `%s`", locationPrefix, columnLabel(w)), w.Properties)
}

// validateAlterSetColumnVisibleScope is MDL-WIDGET43 for
// `alter page … set (Visible: …) on grid column(…)`.
func validateAlterSetColumnVisibleScope(op *ast.SetPropertyOp, locationPrefix string) []linter.Violation {
	if !op.Target.IsColumnAddress() && op.Target.Column == "" {
		return nil
	}
	return columnVisibleScopeViolation(fmt.Sprintf("%s: set on `%s`", locationPrefix, op.Target.Name()), op.Properties)
}

func columnVisibleScopeViolation(where string, props map[string]any) []linter.Violation {
	var expr string
	for k, v := range props {
		if s, ok := v.(string); ok && (strings.EqualFold(k, "VisibleIf") || strings.EqualFold(k, "Visible")) && s != "" {
			expr = s
		}
	}
	if !currentObjectRe.MatchString(expr) {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET43",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"%s Visible uses $currentObject — a column's visibility is evaluated once for the grid, "+
				"with no row object, so Mendix rejects it (CE0117): %s",
			where, expr),
		Suggestion: "use a page parameter or variable (`Visible: $ShowPrices`), or hide per row with " +
			"DynamicCellClass or the cell content instead",
	}}
}

func columnLabel(w *ast.WidgetV3) string {
	if c := w.GetCaption(); c != "" {
		return c
	}
	if a := w.GetAttribute(); a != "" {
		return a
	}
	return w.Name
}
