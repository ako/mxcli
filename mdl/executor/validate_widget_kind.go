// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// Slice 0 of PROPOSAL_def_driven_widget_bodies.md: teach the validator what a
// widget is.
//
// Until now the GRAMMAR was the widget-kind validator — `widgetTypeV3` is an
// allow-list, so an unknown kind could not parse and the validator never needed
// an independent notion of one. Two mistakes already slip past that, because
// neither is a keyword the parser checks, and both reached `exec` with `check`
// reporting success:
//
//	pluggablewidget 'com.acme.NotAWidget' w1        -- the id is a string literal
//	group g1 (…) inside HTML Element                -- a real keyword, wrong parent
//
// Reporting them is worth doing on its own. It is also what makes the
// def-driven body (slices 2-3) safe: those give up the parser's enforcement, so
// the semantic check has to exist first.

// validateWidgetKind reports a widget whose kind mxcli cannot resolve, and a
// container keyword the parent's definition does not declare.
func validateWidgetKind(w *ast.WidgetV3, registry *WidgetRegistry, parentDef *WidgetDefinition,
	parentObjectLists map[string]*ObjectListMapping, locationPrefix string) []linter.Violation {
	if w == nil || registry == nil {
		return nil
	}

	// An explicit widget id that resolves to nothing. Only reachable through the
	// `pluggablewidget '<id>'` / `customwidget '<id>'` forms, where the id is a
	// string literal the parser cannot check.
	if id, ok := w.Properties["WidgetType"].(string); ok && id != "" {
		if _, known := registry.GetByWidgetID(id); !known && !packageInstalledFor(registry, id) {
			return []linter.Violation{{
				RuleID:   "MDL-WIDGET25",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf("%s: widget `%s` has no definition for %q%s",
					locationPrefix, w.Name, id, nearestWidgetIDs(registry, id)),
				Suggestion: "install the widget from the Marketplace so its .mpk lands in widgets/, or check the id for a typo",
			}}
		}
		return nil
	}

	// A container keyword used where the parent does not declare it. These
	// keywords mean nothing on their own — `group` is not a widget — so one
	// outside a parent that declares it can only be a mistake.
	if !isObjectListContainerKeyword(w.Type, registry) {
		return nil
	}
	// Never judge a container against a parent that cannot be resolved. In a
	// project that has not run `widget init` the registry knows only the
	// embedded widgets, so every real parent looks container-less and every
	// container would be reported. Silence is the honest answer there.
	if parentDef == nil {
		return nil
	}
	if parentObjectLists[strings.ToUpper(w.Type)] != nil || parentDeclaresSlot(parentDef, w.Type) {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET26",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf("%s: `%s` is not a container of %s%s",
			locationPrefix, strings.ToLower(w.Type), parentLabel(parentDef), declaredContainers(parentDef)),
		Suggestion: "use one of the parent widget's own containers, or move this out of the widget's body",
	}}
}

// isObjectListContainerKeyword reports whether a keyword is an object-list
// container rather than a widget in its own right.
//
// Derived from the registry — the union of every known definition's container
// keywords — rather than restated. A hardcoded list here was already wrong:
// isUniversalObjectListKeyword names seven where the grammar has nine, missing
// SCALECOLOR, CUSTOMBUTTON and ALLOWEDFILEFORMAT, which is the same list-drift
// this proposal exists to remove.
func isObjectListContainerKeyword(widgetType string, registry *WidgetRegistry) bool {
	if widgetType == "" || registry == nil {
		return false
	}
	up := strings.ToUpper(widgetType)
	// A keyword that names a widget is a widget, whatever else it may be.
	if _, isWidget := registry.Get(up); isWidget {
		return false
	}
	for _, def := range registry.All() {
		for _, ol := range def.ObjectLists {
			if strings.EqualFold(ol.MDLContainer, widgetType) {
				return true
			}
		}
	}
	return isUniversalObjectListKeyword(widgetType)
}

func parentLabel(parentDef *WidgetDefinition) string {
	if parentDef == nil {
		return "this widget"
	}
	if parentDef.MDLName != "" {
		return "`" + strings.ToLower(parentDef.MDLName) + "`"
	}
	return "`" + parentDef.WidgetID + "`"
}

// declaredContainers names what the parent DOES declare, so the reader is not
// left to guess. A parent with none says so, which is the more useful answer.
func declaredContainers(parentDef *WidgetDefinition) string {
	if parentDef == nil {
		return ""
	}
	var names []string
	for _, ol := range parentDef.ObjectLists {
		names = append(names, strings.ToLower(ol.MDLContainer))
	}
	for _, cs := range parentDef.ChildSlots {
		names = append(names, strings.ToLower(cs.MDLContainer))
	}
	if len(names) == 0 {
		return ", which declares no containers"
	}
	sort.Strings(names)
	return " — it declares: " + strings.Join(names, ", ")
}

// nearestWidgetIDs suggests known ids sharing the unknown one's last segment,
// which is where a typo or a wrong vendor prefix usually shows.
func nearestWidgetIDs(registry *WidgetRegistry, id string) string {
	last := id
	if i := strings.LastIndex(id, "."); i >= 0 {
		last = id[i+1:]
	}
	var hits []string
	for _, def := range registry.All() {
		if strings.EqualFold(def.WidgetID, id) {
			continue
		}
		if strings.Contains(strings.ToLower(def.WidgetID), strings.ToLower(last)) {
			hits = append(hits, def.WidgetID)
		}
	}
	if len(hits) == 0 {
		return ""
	}
	sort.Strings(hits)
	if len(hits) > 3 {
		hits = hits[:3]
	}
	return " — did you mean " + strings.Join(hits, ", ") + "?"
}

// packageInstalledFor reports whether the project has a widget package for this
// id, even though no definition has been extracted from it yet.
//
// Load-bearing against a false-positive storm. LoadWidgetRegistry reads only
// `.mxcli/widgets/*.def.json`; unlike the page builder's registry it does NOT
// refresh those from installed .mpk files. So in a project that has never run
// `widget init`, the validator knows the nine embedded widgets and nothing else
// — and calling every real project widget "unknown" would be worse than the
// silence this rule replaces.
//
// Asking whether the package is installed is the same question slice 1's error
// message asks, and it separates "mxcli has not looked at this widget yet" from
// "this widget does not exist".
func packageInstalledFor(registry *WidgetRegistry, widgetID string) bool {
	dir := registryProjectPath(registry)
	if dir == "" {
		return false
	}
	found, err := mpk.FindMPK(filepath.Dir(dir), widgetID)
	return err == nil && found != ""
}

// parentDeclaresSlot reports whether the parent declares a CHILD SLOT by this
// name. Object lists alone are not the whole vocabulary of a widget body, and
// treating a declared slot as undeclared would report correct MDL.
func parentDeclaresSlot(parentDef *WidgetDefinition, keyword string) bool {
	if parentDef == nil {
		return false
	}
	for _, cs := range parentDef.ChildSlots {
		if strings.EqualFold(cs.MDLContainer, keyword) {
			return true
		}
	}
	return false
}
