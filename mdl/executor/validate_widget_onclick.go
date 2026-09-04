// SPDX-License-Identifier: Apache-2.0

// Check-time validation for the `onclick:` / `action:` widget property.
//
// `OnClick:` is an ALIAS for `Action:` — the visitor stores both under
// Properties["Action"] (issue #603, the clickable container) — so by the time a
// validator sees it the two are indistinguishable, and this rule covers both
// spellings on purpose.
//
// mxcli writes that property for exactly three static widget kinds. Everywhere
// else it parses, passes check, passes exec, reaches no stored property, and
// the rendered element has no handler: measured on Mendix 11.13, a data view,
// a listview and a dynamictext each keep their OnClick through `exec` and lose
// it by `describe`, while the container beside them keeps it. The same
// type-agnostic property allow-list that hid #928's `editable:` hides this one —
// isBuiltinPropName answers "is this a real MDL property anywhere", not "is it
// valid on THIS widget". (CapTrackV2 FINDINGS §21)
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// The rule names the widget types it reports rather than reporting everything
// outside an allow-list of the three that DO write the action (container,
// button, navigation-list item). An allow-list looked tighter and was wrong:
// running it over the shipped examples turned up two false positives, both
// because `mxcli check` without `-p` has no widget registry, so `lookupWidgetDef`
// returns nil for a PLUGGABLE widget too and the caller's "static only" branch
// does not hold. `datagrid` is DataGrid 2 — a pluggable widget whose `onClick`
// the widget engine writes — and a bare `pluggablewidget` is the same story.
//
// So the default is silence, and a type earns a report only by being named
// below. A missed report costs a warning; a false one tells an author their
// working page is broken.

// clickDroppedNoSlot are the STATIC widget types whose Mendix counterpart has no
// click action at all, so the property is meaningless rather than unimplemented.
// Checked against generated/metamodel — the arbiter per CLAUDE.md — by
// TestClickCapableTypesCarryClickActionInMetamodel.
var clickDroppedNoSlot = map[string]bool{
	"dataview":         true, // Pages$DataView          (the reported case)
	"dynamictext":      true, // Pages$DynamicText
	"title":            true, // Pages$Title
	"textbox":          true, // Pages$TextBox           (has OnChange/OnEnter/OnLeave, no click)
	"textarea":         true, // Pages$TextArea          (same)
	"datepicker":       true, // Pages$DatePicker        (same)
	"dropdown":         true, // Pages$DropDown          (same)
	"checkbox":         true, // Pages$CheckBox          (same)
	"radiobuttons":     true, // Pages$RadioButtonGroup  (same)
	"radiobuttongroup": true, // Pages$RadioButtonGroup  (alternate spelling)
	"groupbox":         true, // Pages$GroupBox
	"tabcontainer":     true, // Pages$TabContainer      (has ActivePageOnChangeAction, no click)
	"layoutgrid":       true, // Pages$LayoutGrid
	"snippetcall":      true, // Pages$SnippetCall
}

// clickCapableInMendix are the STATIC widget types Mendix DOES give a click
// action but mxcli does not write. Measured against generated/metamodel:
//
//	Pages$ListView.ClickAction
//	Pages$StaticImageViewer.ClickAction
//	Pages$DynamicImageViewer.ClickAction
//
// They earn a different sentence, because the remedy is different: the model
// can hold the action, mxcli simply has no writer for it, so moving the action
// to a container is a workaround rather than the correct modelling.
//
// Bare `image` is deliberately absent — it is not always the legacy viewer.
var clickCapableInMendix = map[string]bool{
	"listview":     true,
	"staticimage":  true,
	"dynamicimage": true,
}

// validateWidgetOnClick reports (MDL-WIDGET23) an `onclick:`/`action:` property
// that mxcli does not write for this widget type.
//
// A warning rather than an error, matching MDL-WIDGET20/21 — the same "silently
// dropped on write" family. Nothing fails the build here: the document is
// perfectly valid, the widget just does not do what the author asked, which is
// precisely why it needs saying out loud.
//
// Pluggable widgets are excluded by the caller: their action slots come from
// their own definition and are routed by the widget engine.
func validateWidgetOnClick(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || w.GetAction() == nil {
		return nil
	}
	typ := strings.ToLower(w.Type)
	if !clickDroppedNoSlot[typ] && !clickCapableInMendix[typ] {
		return nil
	}

	var message, suggestion string
	if clickCapableInMendix[typ] {
		message = fmt.Sprintf(
			"%s: widget `%s` (%s) has an on-click action, and Mendix does model one on %s — "+
				"but mxcli has no writer for it, so the value is dropped and the widget does nothing",
			locationPrefix, w.Name, w.Type, w.Type)
		suggestion = "Wrap the clickable part in a `container` and put the action there — a container's " +
			"on-click IS written (Pages$DivContainer.OnClickAction)"
	} else {
		message = fmt.Sprintf(
			"%s: widget `%s` (%s) has an on-click action, but Mendix models no click action on %s at all — "+
				"the value is dropped on write and the rendered element has no handler",
			locationPrefix, w.Name, w.Type, w.Type)
		suggestion = "Put the action on a `container` inside this widget (a container renders with " +
			"tabindex/role=\"button\" and its on-click is written), or use an `actionbutton`/`linkbutton`"
	}

	return []linter.Violation{{
		RuleID:     "MDL-WIDGET23",
		Severity:   linter.SeverityWarning,
		Message:    message,
		Suggestion: suggestion,
	}}
}
