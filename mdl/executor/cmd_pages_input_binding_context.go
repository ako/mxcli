// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// An `Attribute:` binding is only storable when there is an object to bind to.
//
// The writer stores a binding as a DomainModels$AttributeRef holding
// Module.Entity.Attribute and writes a null reference for anything shorter
// (attributeRefToGen), so a bare name nothing could qualify — `textbox t
// (Attribute: FullName)` at the top of a page, where there is no entity in
// scope — went out as `AttributeRef: null`. `exec` said "Created page", and
// mxbuild 11.13.0 reported, per widget kind:
//
//	textbox/textarea/datepicker/checkbox/radiobuttons/dropdown
//	    CE0544 "This widget can only function inside a data context" + CE7005
//	dynamictext  CE0402 "No value specified."
//	combobox     CE0642 "Property 'Attribute' is required."
//
// Qualifying the name does not rescue it: the reference is stored, but outside a
// data container there is no object of that entity to edit, and mxbuild rejects
// that too — CE0544/CE2421 on a text box, CE1365 on a dynamic text, CE7247 on a
// combo box ("Move this widget into a data container"), each with CE7006.
//
// `Attribute: $P/Name` is a third shape of the same drop: it does not parse as
// an attribute path but as a data-source expression, which no input builder
// reads, so it was dropped INSIDE a data view as well as outside one.
//
// `mxcli check -p … --references` already refused the first two on CREATE
// PAGE/SNIPPET (validatePageContextTree), but plain `mxcli check`, `exec
// --no-check` and ALTER PAGE did not. The check-time rule below needs no
// project; the builder guard catches what reaches the writer by any route.

// inputBindingProblem says why w's `Attribute:` binding cannot be stored, or ""
// when it can (or the widget has none).
//
// c is the context the widget sits in. noEntity says that nothing could qualify
// a bare name here — the builder knows that; the check-time walk has no project
// to ask, passes false, and lets the context alone decide.
func inputBindingProblem(w *ast.WidgetV3, c pageArgContext, noEntity bool) string {
	raw, present := lookupPropCI(w, "Attribute")
	if !present || raw == nil {
		return ""
	}
	kind := strings.ToLower(w.Type)
	attr, isString := raw.(string)
	if !isString {
		return fmt.Sprintf("%s `%s`: `Attribute: %s` is not an attribute binding MDL can store — the widget "+
			"would be written with no binding at all, inside a data view or outside one. Bind the attribute by "+
			"name inside a data container over that object: `dataview dv (DataSource: $Param) { %s %s "+
			"(Attribute: Name) }` ($currentObject/Name is written `Name`)",
			kind, w.Name, nonStringAttributeText(raw), kind, w.Name)
	}
	if attr == "" {
		return ""
	}
	qualified := !strings.Contains(attr, "/") && strings.Count(attr, ".") >= 2
	if c.known && !c.present {
		if qualified {
			return fmt.Sprintf("%s `%s`: attribute `%s` is qualified, but the widget is not inside a data view, "+
				"list view, gallery or data grid, so there is no object of that entity for it to show or edit — "+
				"mxbuild rejects it (CE0544 \"This widget can only function inside a data context\", or \"Move "+
				"this widget into a data container\"). Place it inside a data container over that entity",
				kind, w.Name, attr)
		}
		return fmt.Sprintf("%s `%s`: attribute `%s` has no entity to bind against — the widget is not inside "+
			"a data view, list view, gallery or data grid, so it would be written with no binding "+
			"(AttributeRef: null; mxbuild: %s). "+
			"Place it inside a data container, e.g. `dataview dv (DataSource: $Param) { %s %s (Attribute: %s) }`",
			kind, w.Name, attr, unboundBuildError(kind), kind, w.Name, attr)
	}
	if noEntity && !qualified {
		return fmt.Sprintf("%s `%s`: attribute `%s` has no entity to bind against — no enclosing data source "+
			"with a resolvable entity was found, so it would be written with no binding (AttributeRef: null). "+
			"Place it inside a data container or qualify it (Module.Entity.Attribute)",
			kind, w.Name, attr)
	}
	return ""
}

// unboundBuildError is what mxbuild 11.13.0 reports for a widget of this kind
// written with a null binding at the top of a page — measured per kind.
func unboundBuildError(kind string) string {
	switch kind {
	case "dynamictext":
		return `CE0402 "No value specified."`
	case "combobox":
		return `CE0642 "Property 'Attribute' is required."`
	}
	return `CE0544 "This widget can only function inside a data context"`
}

// nonStringAttributeText renders an `Attribute:` value that did not parse as an
// attribute path back into the spelling the author wrote, for the message.
func nonStringAttributeText(v any) string {
	ds, ok := v.(*ast.DataSourceV3)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	switch {
	case ds.ContextVariable != "":
		return "$" + ds.ContextVariable + "/" + ds.Reference
	case strings.HasPrefix(ds.Reference, "$"):
		return ds.Reference
	}
	return ds.Type + " " + ds.Reference
}

// checkInputBinding is the builder's refusal: the widget being built must not
// reach the writer with a binding the writer will turn into nothing.
func (pb *pageBuilder) checkInputBinding(w *ast.WidgetV3, entity string) error {
	if msg := inputBindingProblem(w, pb.argCtx, entity == ""); msg != "" {
		return mdlerrors.NewValidation(msg)
	}
	return nil
}

// validateInputBindingContext is the check-time mirror (MDL-WIDGET34). It runs
// in the widget-tree walk, which has no project and so no entity: only the
// document-root context — known, and empty — refuses a string binding. ALTER PAGE's subtree walk has an unknown
// context and is left to the builder.
//
// A widget with a data source of its own binds its attributes against that
// source, so it is judged in the context it creates, not the one it sits in.
func validateInputBindingContext(w *ast.WidgetV3, c pageArgContext, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	if own := argContextForOwnAction(w, c); own != c {
		c = own
	}
	msg := inputBindingProblem(w, c, false)
	if msg == "" {
		return nil
	}
	return []linter.Violation{{
		RuleID:     "MDL-WIDGET34",
		Severity:   linter.SeverityError,
		Message:    locationPrefix + ": " + msg,
		Suggestion: "An input or dynamic text shows an attribute of the object a data view, list view, gallery or data grid supplies — wrap it in one.",
	}}
}
