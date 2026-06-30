// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for the layout-grid wrapping of edit/new
// forms. Mirrors the MPR010 lint rule (mdl/linter/rules/dataview_layout_grid.go)
// but works on the MDL AST so `mxcli check` warns while authoring, before the
// page is written. A parameter-bound DataView's label/input widths are expressed
// in Bootstrap grid columns and only render correctly inside a layoutgrid.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidatePageLayoutGrid warns (MPR010) when a parameter-bound DataView — the
// edit/new-form signature — is not nested inside a layout grid. Any layoutgrid
// ancestor satisfies the rule (grid → column → container → dataview is fine);
// only a DataView with no layoutgrid ancestor at all is flagged.
func ValidatePageLayoutGrid(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreatePageStmtV3:
			out = append(out, checkLayoutGridTree(s.Widgets, false, "page "+s.Name.String())...)
		case *ast.CreateSnippetStmtV3:
			out = append(out, checkLayoutGridTree(s.Widgets, false, "snippet "+s.Name.String())...)
		}
	}
	return out
}

func checkLayoutGridTree(widgets []*ast.WidgetV3, underGrid bool, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, w := range widgets {
		if w == nil {
			continue
		}
		if !underGrid && isContextEditDataViewAST(w) {
			out = append(out, linter.Violation{
				RuleID:   "MPR010",
				Severity: linter.SeverityWarning,
				Message: fmt.Sprintf(
					"%s: DataView `%s` is bound to a parameter (edit/new form) but is not inside a layout grid — label and input widths only render correctly inside a layoutgrid",
					locationPrefix, w.Name),
				Suggestion: fmt.Sprintf("Wrap dataview `%s` in `layoutgrid { row { column (desktopwidth: autofill) { … } } }`", w.Name),
			})
		}
		childUnder := underGrid || strings.EqualFold(w.Type, "layoutgrid")
		out = append(out, checkLayoutGridTree(w.Children, childUnder, locationPrefix)...)
	}
	return out
}

// isContextEditDataViewAST reports whether w is a DataView bound to a page/snippet
// parameter (DataSourceV3.Type == "parameter").
func isContextEditDataViewAST(w *ast.WidgetV3) bool {
	if !strings.EqualFold(w.Type, "dataview") {
		return false
	}
	ds := w.GetDataSource()
	return ds != nil && ds.Type == "parameter"
}
