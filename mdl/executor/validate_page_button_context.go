// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for button action context on pages and
// snippets. A DataGrid/Gallery control bar is not row-scoped, so $currentObject
// has no value there; passing it to a button action builds to CE1571 "No
// argument has been selected for parameter …". This heuristic catches it before
// MxBuild does. See docs/11-proposals/PROPOSAL_check_mxbuild_gap_heuristics.md.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidatePageButtonContext warns (MDL-BUTTON01) when a button inside a control
// bar passes $currentObject to its action. A control bar sits above the grid and
// is not bound to a row, so $currentObject is unbound there — MxBuild reports
// CE1571. Row-scoped buttons (inside a grid column / list item) are fine.
func ValidatePageButtonContext(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreatePageStmtV3:
			out = append(out, checkButtonContextTree(s.Widgets, "", "page "+s.Name.String())...)
		case *ast.CreateSnippetStmtV3:
			out = append(out, checkButtonContextTree(s.Widgets, "", "snippet "+s.Name.String())...)
		}
	}
	return out
}

// checkButtonContextTree walks the widget tree, carrying the name of the data
// widget whose control bar it is inside ("" when it is not inside one).
//
// The name is carried rather than just a flag because it IS the remedy: a data
// widget's selection is addressed by the widget's own name, so `$dgMaterials` is
// only spellable from here. Advice that stops at "move it into a column" sends
// an author looking for syntax that does not need to exist — which is how
// mendixlabs/mxcli#1082 was filed.
func checkButtonContextTree(widgets []*ast.WidgetV3, controlBarOf, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, w := range widgets {
		if w == nil {
			continue
		}
		if controlBarOf != "" {
			if a := w.GetAction(); a != nil {
				out = append(out, checkControlBarAction(a, w.Name, controlBarOf, locationPrefix)...)
			}
		}
		for _, c := range w.Children {
			childOf := controlBarOf
			if c != nil && strings.EqualFold(c.Type, "controlbar") {
				childOf = w.Name
			}
			out = append(out, checkButtonContextTree([]*ast.WidgetV3{c}, childOf, locationPrefix)...)
		}
	}
	return out
}

// checkControlBarAction flags any $currentObject argument on an action (and its
// chained THEN action) that sits inside a control bar.
func checkControlBarAction(a *ast.ActionV3, widgetName, controlBarOf, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for a != nil {
		for _, arg := range a.Args {
			if s, ok := arg.Value.(string); ok && strings.EqualFold(s, "$currentObject") {
				out = append(out, linter.Violation{
					RuleID:   "MDL-BUTTON01",
					Severity: linter.SeverityError,
					Message: fmt.Sprintf(
						"%s: control-bar button `%s` passes $currentObject to its %s action, but a control bar is not row-scoped — $currentObject is unbound there (CE1571)",
						locationPrefix, widgetName, a.Type),
					Suggestion: controlBarSuggestion(controlBarOf),
				})
			}
		}
		a = a.ThenAction
	}
	return out
}

// controlBarSuggestion names the remedy that actually applies to a control bar.
//
// The selection comes first because it is the one that keeps the button where
// the author put it: a data widget with `Selection:` set exposes the selected
// object as `$<widgetName>`, which an action takes as an ordinary argument.
func controlBarSuggestion(controlBarOf string) string {
	if controlBarOf == "" {
		return "Move the button into a grid column (row-scoped) so it has a current row, or pass a page parameter instead of $currentObject."
	}
	return fmt.Sprintf(
		"Pass the selection of `%s` instead — `Action: microflow M.F($Param = $%s)`, with `Selection:` set on the widget. "+
			"Or move the button into a grid column (row-scoped) so it has a current row, or pass a page parameter.",
		controlBarOf, controlBarOf)
}
