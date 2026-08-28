// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// validatePageContextTree checks that page-internal references are consistent:
//   - PARAMETER DataSource references match declared page/snippet parameters
//   - SELECTION DataSource references match a widget name declared in the same page
//   - Attribute bindings have an enclosing data container providing entity context
//
// This runs at check time (no MPR needed) and catches issues that would otherwise
// only surface as CE errors in Studio Pro.
func validatePageContextTree(params []ast.PageParameter, widgets []*ast.WidgetV3) []string {
	// Build param name set
	paramNames := make(map[string]bool, len(params))
	for _, p := range params {
		paramNames[p.Name] = true
	}

	// Collect all widget names (first pass) for SELECTION validation
	widgetNames := make(map[string]bool)
	collectWidgetNames(widgets, widgetNames)

	// Walk the widget tree with context tracking
	var errors []string
	errors = append(errors, checkDuplicateWidgetNames(widgets)...)
	walkWidgetsWithContext(widgets, paramNames, widgetNames, false, &errors)
	return errors
}

// widgetKindsWithoutStoredNames are the widget kinds Mendix stores with no Name
// at all, so MDL's name for one is mxcli's own — derived at describe time to give
// ALTER PAGE something to address.
//
// Measured on a stored page: Forms$LayoutGridRow and Forms$LayoutGridColumn carry
// no `Name` key. A DataGrid2 column is the same, and mxcli says so itself in
// MDL-WIDGET16 ("DataGrid 2 stores no column names"), deriving one from the bound
// attribute — so two columns over the same attribute derive the same name and no
// renaming fixes that without breaking the documented addressing.
//
// A name the model does not hold cannot be a CE0495 duplicate, and reporting one
// made DESCRIBE output fail mxcli's own check (upstream #978).
var widgetKindsWithoutStoredNames = map[string]bool{
	"row":    true,
	"column": true,
}

// checkDuplicateWidgetNames flags any widget name that appears more than once on a
// page. Mendix requires widget names to be unique per page and rejects duplicates
// with CE0495 "Duplicate name" — which mxcli check otherwise passed (FINDINGS #15).
// Each duplicate name is reported once, in first-seen order.
//
// Widget kinds Mendix stores without a name are skipped: see
// widgetKindsWithoutStoredNames.
func checkDuplicateWidgetNames(widgets []*ast.WidgetV3) []string {
	counts := make(map[string]int)
	var order []string
	var walk func(ws []*ast.WidgetV3)
	walk = func(ws []*ast.WidgetV3) {
		for _, w := range ws {
			if w.Name != "" && !widgetKindsWithoutStoredNames[strings.ToLower(w.Type)] {
				if counts[w.Name] == 0 {
					order = append(order, w.Name)
				}
				counts[w.Name]++
			}
			walk(w.Children)
		}
	}
	walk(widgets)

	var errors []string
	for _, name := range order {
		if counts[name] > 1 {
			errors = append(errors,
				fmt.Sprintf("duplicate widget name '%s' (used %d times) — Mendix requires unique widget names per page (CE0495)", name, counts[name]))
		}
	}
	return errors
}

// collectWidgetNames recursively collects all widget names in the tree.
func collectWidgetNames(widgets []*ast.WidgetV3, names map[string]bool) {
	for _, w := range widgets {
		if w.Name != "" {
			names[w.Name] = true
		}
		collectWidgetNames(w.Children, names)
	}
}

// walkWidgetsWithContext validates each widget's DataSource and attribute bindings,
// tracking whether the current position is inside a data container (DataView,
// DataGrid, ListView, etc.) that provides entity context.
func walkWidgetsWithContext(widgets []*ast.WidgetV3, paramNames map[string]bool, widgetNames map[string]bool, hasEntityContext bool, errors *[]string) {
	for _, w := range widgets {
		ds := w.GetDataSource()
		childHasContext := hasEntityContext

		if ds != nil {
			switch ds.Type {
			case "parameter":
				// Strip leading $ if present
				paramRef := strings.TrimPrefix(ds.Reference, "$")
				if paramRef != "" && !paramNames[paramRef] {
					*errors = append(*errors,
						fmt.Sprintf("widget '%s': parameter DataSource references '$%s' but no such parameter is declared in Params", w.Name, paramRef))
				}
				childHasContext = true

			case "selection":
				if ds.Reference != "" && !widgetNames[ds.Reference] {
					*errors = append(*errors,
						fmt.Sprintf("widget '%s': selection DataSource references '%s' but no widget with that name exists on this page", w.Name, ds.Reference))
				}
				childHasContext = true

			case "database", "microflow", "nanoflow", "association":
				childHasContext = true
			}
		}

		// Check if this widget type is a data container that sets context
		widgetType := strings.ToLower(w.Type)
		switch widgetType {
		case "dataview", "datagrid", "listview", "gallery", "templateview":
			if ds == nil {
				// Data container without DataSource — context comes from enclosing container
				childHasContext = hasEntityContext
			}
		}

		// Validate attribute binding: needs entity context
		if attr := w.GetAttribute(); attr != "" {
			if !hasEntityContext {
				*errors = append(*errors,
					fmt.Sprintf("widget '%s': Attribute '%s' is bound but there is no enclosing data container providing entity context", w.Name, attr))
			}
		}

		// Recurse into children
		walkWidgetsWithContext(w.Children, paramNames, widgetNames, childHasContext, errors)
	}
}
