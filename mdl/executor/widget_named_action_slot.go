// SPDX-License-Identifier: Apache-2.0

// Package executor - named pluggable-widget action slots.
package executor

import (
	"sort"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// namedActionSlotValue reads the action written on a widget's action slot that
// MDL addresses by the widget's own property key — `createFileAction: microflow
// M.ACT_CreateFile` — rather than through one of the two fixed AST slots
// (`Action`/`OnChange`). Returns nil when the script did not name the slot.
//
// It accepts two AST shapes, and the second is the whole difficulty.
// `dataSourceExprV3` and `actionExprV3` overlap on MICROFLOW / NANOFLOW /
// VARIABLE, and `(IDENTIFIER | keyword) COLON dataSourceExprV3` comes first in
// widgetPropertyV3 — so `createFileAction: microflow M.X` parses as a
// *DataSourceV3, and no reordering can fix it without turning a chart series'
// `staticDataSource: microflow M.X` into an action. The grammar genuinely cannot
// tell them apart; only the widget definition can, and it does: this is called
// for a mapping whose operation is `action`.
//
// MDL already resolves this exact ambiguity the same way one rule up —
// fragmentArgValue is `dataSourceExprV3 | actionExprV3`, with "the executor
// disambiguates using the parameter's declared kind".
//
// A datasource shape with no action form (database/association/selection) is
// REFUSED rather than coerced: an action slot holding an empty action is not the
// same as an unset one, and writing a NoAction over the slot would clear what
// the widget had.
func namedActionSlotValue(w *ast.WidgetV3, propertyKey string) (*ast.ActionV3, error) {
	raw, ok := lookupProperty(w.Properties, propertyKey)
	if !ok || raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case *ast.ActionV3:
		return v, nil
	case *ast.DataSourceV3:
		return actionFromFlowDataSource(v, propertyKey)
	default:
		return nil, mdlerrors.NewValidationf(
			"widget %q property %q is an action slot — write it as an action "+
				"(e.g. `%s: microflow Module.Flow`), not as %T",
			w.Name, propertyKey, propertyKey, raw)
	}
}

// actionFromFlowDataSource converts the parse of a flow call written in an
// action position. Only the shapes that exist as both a data source and an
// action convert; the rest name something an action slot cannot hold.
func actionFromFlowDataSource(ds *ast.DataSourceV3, propertyKey string) (*ast.ActionV3, error) {
	switch ds.Type {
	case "microflow", "nanoflow":
		return &ast.ActionV3{Type: ds.Type, Target: ds.Reference, Args: ds.Args}, nil
	case "parameter":
		// `$handler` — a fragment action parameter, the same shape actionExprV3's
		// own VARIABLE alternative accepts.
		return &ast.ActionV3{Type: "parameter", Target: ds.Reference}, nil
	default:
		return nil, mdlerrors.NewValidationf(
			"property %q is an action slot and cannot hold a %s data source — "+
				"write a microflow, nanoflow or one of the client actions "+
				"(show_page, save_changes, close_page, …)",
			propertyKey, ds.Type)
	}
}

// namedActionSlotsOf reads back every action slot a stored widget holds under a
// key that MDL has no name for, so DESCRIBE can emit it as `<key>: <action>` and
// the flow round-trips. The click and change slots are excluded: they are
// addressed as `onClick:` / `OnChange:` and are carried separately, and emitting
// them twice would write two actions into one slot on re-execution.
//
// Slots holding a NoAction (the default) are skipped, so an untouched widget
// describes as cleanly as before.
func namedActionSlotsOf(ctx *ExecContext, w map[string]any) []rawNamedAction {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return nil
	}
	propTypeKeyMap := buildPropertyTypeKeyMap(w, true)
	var out []rawNamedAction
	for _, prop := range getBsonArrayElements(obj["Properties"]) {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		key := propTypeKeyMap[extractBinaryID(propMap["TypePointer"])]
		if key == "" || actionSourceForKey(key) != "" {
			continue // no key, or a slot MDL names — carried as OnClick/OnChange
		}
		action := customWidgetPropertyActionMap(ctx, w, key)
		if action == nil {
			continue
		}
		if mdl := renderClientActionMDL(ctx, action); mdl != "" {
			out = append(out, rawNamedAction{Key: key, MDL: mdl})
		}
	}
	// Stable order: the stored Properties order is the widget's PropertyTypes
	// order, which is stable, but sort anyway so a describe never depends on it.
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
