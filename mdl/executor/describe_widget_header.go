// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
)

// pluggableWidgetHeader returns the MDL header DESCRIBE PAGE should emit for a
// pluggable widget: its own MDL name where that round-trips, and the explicit
// `pluggablewidget '<id>'` form otherwise.
//
// Item 6 of slice 2 in PROPOSAL_def_driven_widget_bodies.md. Until slices 2-3
// the keyword form did not parse for most widgets, so DESCRIBE had no choice.
// Now that it does, emitting
//
//	htmlelement frame (tagName: 'div')
//
// instead of
//
//	pluggablewidget 'com.mendix.widget.web.htmlelement.HTMLElement' frame (…)
//
// makes describe → edit → exec produce the form a person would have written,
// which is the point of DESCRIBE being re-executable at all.
//
// # It falls back rather than guessing
//
// The bar is not "shorter" but "rebuilds the SAME widget". Two cases fail that
// and take the id form:
//
//  1. **The id resolves to no definition.** Without a project the registry
//     holds only the embedded widgets, so most real widgets are unknown here —
//     and an MDL name mxcli invented would not resolve on the way back in.
//  2. **Two definitions share an MDL name.** Then the name is ambiguous: the
//     builder resolves it by `registry.Get(ToUpper(name))`, which can only
//     return one of them, so emitting the name would silently retarget the
//     widget. The id is unambiguous by construction.
//
// Case 2 is not hypothetical in principle — an MDL name is the last segment of
// a widget id, and two vendors can ship `…​.Slider`. It costs one map to rule
// out, and the alternative failure is a describe that rewrites a page onto a
// different widget.
func pluggableWidgetHeader(registry *WidgetRegistry, widgetID, name string) string {
	idForm := fmt.Sprintf("pluggablewidget '%s' %s", widgetID, mdlIdent(name))
	if registry == nil || widgetID == "" {
		return idForm
	}
	def, ok := registry.GetByWidgetID(widgetID)
	if !ok || def == nil || def.MDLName == "" {
		return idForm
	}
	// Does emitting this name rebuild the SAME widget? Ask, rather than infer.
	//
	// The builder resolves a bare name with registry.Get(ToUpper(name)), and the
	// registry is keyed BY MDL NAME — so when two definitions claim one name the
	// map keeps only the last, while GetByWidgetID keeps both. Get and
	// GetByWidgetID then disagree, and emitting the name would rebuild the page
	// onto the other widget. An MDL name is the last segment of a widget id, so
	// two vendors shipping `….Slider` is not hypothetical.
	//
	// Counting definitions cannot see this (All() iterates the by-name map, so
	// the loser is already gone). Round-tripping the name through the same
	// lookup the builder uses can.
	back, ok := registry.Get(def.MDLName)
	if !ok || back == nil || back.WidgetID != widgetID {
		return idForm
	}
	return fmt.Sprintf("%s %s", strings.ToLower(def.MDLName), mdlIdent(name))
}
