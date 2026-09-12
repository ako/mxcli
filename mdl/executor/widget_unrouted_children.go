// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// A pluggable widget's body is distributed over four passes in buildPluggable —
// declared child slots, object lists, and two auto-discovery passes — and every
// one of them SKIPS a child it does not recognise. Nothing then reported the
// skip, so a child that matched none of the four was built and thrown away:
//
//	datagrid dg (...) {
//	  column c (attribute: Name) filter f { textfilter tf (attribute: Name) }
//	}
//
// `filter` is a Gallery keyword (widgetSlotKeywordOverrides spells the same
// DataGrid property `controlbar`), and the grammar reads that line as a COLUMN
// with no body followed by a SIBLING filter widget — so it landed among the
// grid's own children, matched nothing, and vanished. `mxcli check` was silent,
// `exec` succeeded, and `DESCRIBE PAGE` showing a column with no filter was the
// only way to find out. That spelling is what `mxcli syntax page widgets`
// documented, so it was also the likeliest thing for an author to write.
//
// The drop is not datagrid-specific — it is one `continue` per pass, shared by
// every pluggable widget — which is why this is a rule about children and slots
// rather than a special case for filters.

// unroutedPluggableChildren returns the direct children of w that the engine
// has nowhere to put, given the parent's definition.
//
// It is deliberately CONSERVATIVE: it reports only what it can prove from the
// definition alone, so that `check` (which has the definition) never claims a
// drop that `exec` (which has the definition AND the widget template) would not
// make. The bail-outs below are each a case where a child may still be routed
// by information this function cannot see.
func unroutedPluggableChildren(def *WidgetDefinition, w *ast.WidgetV3) []*ast.WidgetV3 {
	if def == nil || w == nil || len(w.Children) == 0 {
		return nil
	}
	// No declared child slots: applyChildSlots returns early and the auto pass
	// hands every unmatched child to the first widgets-typed property it finds.
	// Something may well take them, and this function cannot see what.
	if len(def.ChildSlots) == 0 {
		return nil
	}
	// A `template` slot IS the catch-all — applyChildSlots assigns leftover
	// children to it (defaultSlotContainer). A Gallery has one, which is why an
	// arbitrary widget in a gallery body is fine and the same widget in a
	// datagrid body is not.
	if defHasDefaultChildSlot(def) {
		return nil
	}
	var out []*ast.WidgetV3
	for _, c := range w.Children {
		if c == nil || defRoutesChild(def, c) {
			continue
		}
		out = append(out, c)
	}
	return out
}

func defHasDefaultChildSlot(def *WidgetDefinition) bool {
	for _, cs := range def.ChildSlots {
		if strings.EqualFold(cs.MDLContainer, defaultSlotContainer) {
			return true
		}
	}
	return false
}

// defRoutesChild mirrors the matching done by applyObjectLists, applyChildSlots
// and the auto-discovery passes. Matching is case-insensitive here even though
// the registry lower-cases MDLContainer on load, so that a hand-written
// .def.json cannot turn a casing slip into a false "this is dropped".
func defRoutesChild(def *WidgetDefinition, c *ast.WidgetV3) bool {
	for _, ol := range def.ObjectLists {
		// By container keyword (`column c { … }`), or by name against the
		// property key (the auto-discovery pass matches a child's NAME).
		if strings.EqualFold(ol.MDLContainer, c.Type) || strings.EqualFold(ol.PropertyKey, c.Name) {
			return true
		}
	}
	isContainer := strings.EqualFold(c.Type, "container")
	for _, cs := range def.ChildSlots {
		if strings.EqualFold(cs.MDLContainer, c.Type) || strings.EqualFold(cs.PropertyKey, c.Name) {
			return true
		}
		// `container <slotName> { … }` — routed by the container's NAME, the
		// authoring form applyChildSlots supports for slots whose keyword reads
		// badly as a widget type.
		if isContainer && strings.EqualFold(cs.MDLContainer, c.Name) {
			return true
		}
	}
	return false
}

// unroutedChildMessage explains one dropped child, naming what the parent does
// declare. The "spelled X on this widget" half is the answer in the case that
// prompted the rule: `filter` and `controlbar` are the SAME property under two
// widgets' conventions, so an author copying a gallery example onto a datagrid
// needs the other spelling, not a list to hunt through.
func unroutedChildMessage(def *WidgetDefinition, child *ast.WidgetV3) string {
	msg := fmt.Sprintf("`%s` is not a container or slot of %s, so it would be dropped on write",
		strings.ToLower(child.Type), parentLabel(def))
	if alt := sameSlotUnderAnotherName(def, child.Type); alt != "" {
		msg += fmt.Sprintf(" — on this widget that slot is spelled `%s`", alt)
	}
	return msg + declaredContainers(def)
}

// sameSlotUnderAnotherName finds the keyword THIS widget uses for the property
// that some other widget spells `keyword`. Derived from
// widgetSlotKeywordOverrides, which is where the convention already lives, so
// the two cannot drift.
func sameSlotUnderAnotherName(def *WidgetDefinition, keyword string) string {
	var propertyKey string
	for widgetID, slots := range widgetSlotKeywordOverrides {
		if widgetID == def.WidgetID {
			continue
		}
		for key, kw := range slots {
			if strings.EqualFold(kw, keyword) {
				propertyKey = key
			}
		}
	}
	if propertyKey == "" {
		return ""
	}
	for _, cs := range def.ChildSlots {
		if strings.EqualFold(cs.PropertyKey, propertyKey) {
			return strings.ToLower(cs.MDLContainer)
		}
	}
	return ""
}

// validateUnroutedChildren is the check-time half (MDL-WIDGET29). It sits beside
// MDL-WIDGET26, which covers the neighbouring case: a container KEYWORD (`group`,
// `series` — words that are not widgets at all) under a parent that does not
// declare it. This one covers a real WIDGET in the same position, which
// MDL-WIDGET26 cannot report because a widget resolves perfectly well on its own.
func validateUnroutedChildren(w *ast.WidgetV3, def *WidgetDefinition, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, child := range unroutedPluggableChildren(def, w) {
		out = append(out, linter.Violation{
			RuleID:     "MDL-WIDGET29",
			Severity:   linter.SeverityError,
			Message:    fmt.Sprintf("%s: %s", locationPrefix, unroutedChildMessage(def, child)),
			Suggestion: "move it into one of the parent's containers, or out of the widget's body",
		})
	}
	return out
}

// refuseUnroutedChildren is the exec-time half. `check` can be skipped
// (`exec --no-check`), and a write that silently discards part of what it was
// given is the failure this whole rule exists to stop — so the writer refuses
// rather than trusting that the checker ran.
func refuseUnroutedChildren(def *WidgetDefinition, w *ast.WidgetV3) error {
	dropped := unroutedPluggableChildren(def, w)
	if len(dropped) == 0 {
		return nil
	}
	return mdlerrors.NewValidationf("widget `%s`: %s", w.Name, unroutedChildMessage(def, dropped[0]))
}
