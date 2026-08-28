// SPDX-License-Identifier: Apache-2.0

// Check-time validation for the `editable:` widget property.
//
// Mendix models editability on INPUT widgets only. Measured against
// generated/metamodel (the arbiter per CLAUDE.md), exactly eleven Pages types
// carry Editability / ConditionalEditabilitySettings — ten input widgets plus
// DataView — and not one of the fourteen button types does. A button has
// visibility, not editability.
//
// mxcli accepted `editable:` on any widget because the allow-lists behind
// MDL-WIDGET01 and MDL-WIDGET07 are widget-type-AGNOSTIC: isBuiltinPropName
// answers "is this a real MDL property name anywhere", and both validators read
// it as "is this valid on THIS widget". So `editable:` on a button passed check,
// passed exec, wrote nothing, and the button stayed enabled — a silent
// functional failure rather than a caught error. (issue #928)
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// editableWidgetTypes are the MDL widget types whose Mendix counterpart carries
// Editability / ConditionalEditabilitySettings.
//
// Kept in sync with generated/metamodel by
// TestEditableWidgetTypesMatchMetamodel, which fails if Mendix's set changes —
// the list cannot be derived at runtime because MDL type names are not the
// metamodel's type names.
var editableWidgetTypes = map[string]bool{
	"checkbox":                  true, // Pages$CheckBox
	"dataview":                  true, // Pages$DataView
	"datepicker":                true, // Pages$DatePicker
	"dropdown":                  true, // Pages$DropDown
	"filemanager":               true, // Pages$FileManager
	"imageuploader":             true, // Pages$ImageUploader
	"inputreferencesetselector": true, // Pages$InputReferenceSetSelector
	"radiobuttons":              true, // Pages$RadioButtonGroup
	"radiobuttongroup":          true, // Pages$RadioButtonGroup (alternate spelling)
	"referenceselector":         true, // Pages$ReferenceSelector
	"textarea":                  true, // Pages$TextArea
	"textbox":                   true, // Pages$TextBox
}

// ValidateWidgetEditability reports (MDL-WIDGET20) an `editable:` property on a
// widget type that has no editability in the Mendix model.
//
// It runs in the no-project pass: the answer is the widget's own type, already
// in the statement. A warning rather than an error, matching MDL-WIDGET07 — the
// same "silently dropped on write" family, and the same reason (a hard reject on
// a vocabulary that cannot be proven complete risks false positives).
//
// Pluggable widgets are excluded. Their property vocabulary comes from the
// widget's own definition and is checked by MDL-WIDGET01; `editable` on one is
// either a real property of that widget or already reported there.
func validateWidgetEditability(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	key, ok := widgetEditableKey(w)
	if !ok {
		return nil
	}
	if editableWidgetTypes[strings.ToLower(w.Type)] {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET20",
		Severity: linter.SeverityWarning,
		Message: fmt.Sprintf(
			"%s: widget `%s` (%s) has an `%s` property, but Mendix models editability on input "+
				"widgets only — %s has no Editability, so this is silently dropped on write and the "+
				"widget stays enabled",
			locationPrefix, w.Name, w.Type, key, w.Type,
		),
		Suggestion: "Use `visible: [ ... ]` to hide it conditionally (buttons do support conditional " +
			"visibility), or move the condition into the microflow the button calls",
	}}
}

// widgetEditableKey finds an editability property however the author spelled it.
// Both MDL forms have to be caught: `editable: 'false'` stays as `Editable`,
// while the bracket form `editable: [expr]` is lowered by the visitor to
// `EditableIf`. Missing the second would leave the shape the docs
// actually recommend silently dropped, which is the bug. The key is returned as
// spelled so the message quotes the author's casing.
func widgetEditableKey(w *ast.WidgetV3) (string, bool) {
	for key := range w.Properties {
		l := strings.ToLower(key)
		if l == "editable" || l == "editableif" {
			return key, true
		}
	}
	return "", false
}

// validatePluggableEditability (MDL-WIDGET21) reports a plain `editable:` on a
// PLUGGABLE widget, which mxcli parses, accepts, and does not write.
//
// This file used to state that pluggable widgets need no check because
// `editable` on one is "either a real property of that widget or already
// reported [by MDL-WIDGET01]". Measured on Mendix 11.13, neither holds:
//
//   - `combobox cmb (Editable: Never)` produces no diagnostic at all, while
//     `Bogusprop` on the same widget is an MDL-WIDGET01 error. So `editable` is
//     accepted — not because the widget defines it, but because the builtin
//     property allow-list is widget-type-AGNOSTIC, the same weakness #928
//     documented.
//   - No `.def.json` maps `editable` to anything, and the stored
//     CustomWidgets$CustomWidget carries no Editable key. The value reaches the
//     document nowhere.
//
// The result was the reported failure: in one `create page`, `editable: Never`
// persisted on the textboxes and vanished on the combobox beside them, with
// `mxcli check`, `mxcli exec` and `mx check` all clean
// (ako/mxcli-maintenance-2). Static input widgets now carry it; a pluggable one
// still cannot, so it is reported rather than dropped in silence.
//
// A warning, not an error, for the same reason as MDL-WIDGET07 and #928's own
// rule: the pluggable vocabulary cannot be proven complete, so a widget that
// genuinely wires its own editability must not be blocked.
func validatePluggableEditability(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	key, ok := widgetEditableKey(w)
	if !ok {
		return nil
	}
	// The bracket form lowers to EditableIf and rides on
	// ConditionalEditabilitySettings, which is a different path from the plain
	// enum — do not claim it is dropped without having measured it.
	if strings.EqualFold(key, "editableif") {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET21",
		Severity: linter.SeverityWarning,
		Message: fmt.Sprintf(
			"%s: widget `%s` (%s) is a pluggable widget and its `%s` property is not written — "+
				"the value is accepted here and reaches no stored property, so the widget stays editable",
			locationPrefix, w.Name, w.Type, key,
		),
		Suggestion: "Set the widget's own read-only property if it has one (`mxcli widget describe " +
			"<id>` lists them), bind the attribute through a data view that is itself not editable, " +
			"or use a static input widget where `editable:` is written.",
	}}
}
