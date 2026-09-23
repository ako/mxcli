// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateListViewEditableInputs (MDL-WIDGET31) reports a list view that will be
// written with Editable false while it holds input widgets that are meant to be
// editable. (ako/mxcli#631)
//
// Pages$ListView.Editable is what makes the inputs INSIDE a list view editable,
// and its read-only context wins over `editable: Always` on the input itself —
// including inside a nested data view. Without it every input renders as
// <div class="form-control-static">, with a valid document, a clean `mx check`
// and a successful build: the failure only shows in the running app.
//
// The writer is right to default it to false: that is Mendix's own default
// (mendixmodelsdk 4.115.0, Pages$ListView `editable` defaults to false and
// _initializeDefaultProperties does not set it). Studio Pro agrees, measured on
// ako/TestApp (Mendix 11.14.0): 30 of its 32 list views are stored Editable false
// and none of those holds an input; the one list view with inputs
// (Pages.EditableLIstView, five textboxes at Editability Always) was set to
// Editable true by its author. So the fix is a diagnostic for the combination,
// not a different default.
//
// Editability is read with GetBoolProp, the same call buildListViewV3 uses, so
// the rule reports what will be written: a quoted `editable: 'true'` is a string
// and is written false.
//
// A warning, not an error: an input shown read-only in a list view is odd but
// legal. An input the author already marked `editable: Never` is not counted.
func validateListViewEditableInputs(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || !strings.EqualFold(w.Type, "listview") || w.GetBoolProp("Editable") {
		return nil
	}
	input := firstEditableInput(w.Children)
	if input == nil {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET31",
		Severity: linter.SeverityWarning,
		Message: fmt.Sprintf(
			"%s: list view `%s` is written with Editable false (the Mendix default), so input `%s` (%s) "+
				"inside it renders read-only — the list view's context wins over the input's own `editable:`",
			locationPrefix, w.Name, input.Name, input.Type,
		),
		Suggestion: fmt.Sprintf("Add `editable: true` to list view `%s`, or mark the inputs `editable: Never` "+
			"if they are meant to be read-only", w.Name),
	}}
}

// firstEditableInput finds an input widget below a list view that the author has
// not already made read-only. It does not descend into a nested list view, whose
// own Editable governs its inputs and which is reported on its own visit.
func firstEditableInput(widgets []*ast.WidgetV3) *ast.WidgetV3 {
	for _, c := range widgets {
		if c == nil {
			continue
		}
		typ := strings.ToLower(c.Type)
		if typ == "listview" {
			continue
		}
		if editableWidgetTypes[typ] && typ != "dataview" && !strings.EqualFold(c.GetStringProp("Editable"), "Never") {
			return c
		}
		if found := firstEditableInput(c.Children); found != nil {
			return found
		}
	}
	return nil
}
