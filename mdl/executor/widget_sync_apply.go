// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/widgets"
	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// widget_sync_apply.go writes the reconciliation the planner describes.
//
// Reconciliation is delegated to widgets.AugmentTemplate (see applyToWidget), which
// makes the stored widget's TYPE byte-identical to `mx update-widgets` output. What
// remains are value-level migrations update-widgets also performs and this does not:
//
//   - Appearance/DesignProperties: an empty list needs the `[3]` marker
//   - LabelTemplate: written as an explicit null, not omitted
//   - Forms$GridSortBar: SortDirection -> SortOrder, list marker 3 -> 2
//   - a newly added TextTemplate property's value: null, not a populated template
//
// The last one is scoped carefully: nulling EVERY TextTemplate value in a synced
// project takes CE0463 from 33 to 127, because instances that legitimately carry a
// caption need it. Only properties this operation introduced may be nulled.
//
// Three of those four were TESTED IN ISOLATION against the fixture and NONE moved the
// count (33 before, 33 after each): adding the `[3]` marker to empty DesignProperties,
// writing LabelTemplate as an explicit null, and the GridSortBar SortDirection ->
// SortOrder + marker migration. They are real differences from update-widgets output
// but they are not what CE0463 is reacting to. Recorded so they are not retried.
//
// The remaining untested candidate is the scoped TextTemplate null (added properties
// only). If that also fails, the cause is not among the 25 value paths and the next
// move is the splice bisection in .claude/skills/diagnose-ce0463.md.
//
// # The pairing invariant
//
// A CustomWidgets$WidgetProperty in Object.Properties is bound to its
// CustomWidgets$WidgetPropertyType in Type.ObjectType.PropertyTypes by TypePointer.
// Removing one half without the other yields a project Mendix cannot LOAD (the
// StreamingBsonUnitReader "does not contain a constructor with a parameter of type
// WidgetValue" failure) — which `mx check` reports as "0 errors" because it never got
// far enough to check anything. Both halves move together here, keyed on the
// PropertyType's $ID.

// SyncResult reports what was written.
type SyncResult struct {
	UnitsChanged    int
	WidgetsChanged  int
	PropertiesFixed int
	Skipped         []string // changes the plan proposed that this step does not apply
}

// ApplyWidgetSync reconciles stored widget instances and writes the affected units.
func ApplyWidgetSync(b backend.RawUnitBackend, projectPath string, opts SyncOptions) (*SyncResult, *SyncPlan, error) {
	plan, err := PlanWidgetSync(b, projectPath, opts)
	if err != nil {
		return nil, nil, err
	}

	// Group by unit so each document is read, mutated and written exactly once.
	byUnit := map[string][]SyncWidgetPlan{}
	for _, w := range plan.Widgets {
		byUnit[w.ContainerID] = append(byUnit[w.ContainerID], w)
	}
	unitIDs := make([]string, 0, len(byUnit))
	for id := range byUnit {
		unitIDs = append(unitIDs, id)
	}
	sort.Strings(unitIDs)

	defs, err := installedWidgetDefs(projectPath)
	if err != nil {
		return nil, plan, err
	}

	res := &SyncResult{}
	for _, unitID := range unitIDs {
		raw, err := b.GetRawUnitBytes(model.ID(unitID))
		if err != nil {
			return nil, plan, fmt.Errorf("read unit %s: %w", unitID, err)
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			return nil, plan, fmt.Errorf("parse unit %s: %w", unitID, err)
		}

		wanted := map[string][]SyncPropertyChange{}
		widgetDef := map[string]*mpk.WidgetDefinition{}
		for _, w := range byUnit[unitID] {
			wanted[w.Widget] = append(wanted[w.Widget], w.Changes...)
			widgetDef[w.Widget] = defs[w.WidgetID]
		}

		changed := 0
		widgets := 0
		out := mapWidgets(doc, func(name string, widget bson.D) (bson.D, bool) {
			changes, ok := wanted[name]
			if !ok {
				return widget, false
			}
			def := widgetDef[name]
			if def == nil {
				return widget, false
			}
			updated, ok := applyToWidget(widget, def)
			if !ok {
				return widget, false
			}
			changed += len(changes)
			widgets++
			return updated, true
		})

		if changed == 0 {
			continue
		}

		// Never write a unit carrying a duplicate GUID. Mendix accepts one on load and
		// on `mx check`, then refuses to SAVE the project — and `mx update-widgets`
		// collapses mprcontents/ before it discovers that, leaving the project both
		// flattened and unloadable. A corruption that only surfaces later, after the
		// safety net has been destroyed, has to be caught before it reaches disk.
		outDoc, isDoc := out.(bson.D)
		if !isDoc {
			return nil, plan, fmt.Errorf("unit %s: reconciliation did not produce a document", unitID)
		}
		if dup, ok := widgetIDsAreUnique(outDoc); !ok {
			return nil, plan, fmt.Errorf(
				"unit %s (%s): reconciliation would write duplicate GUID %s — refusing to write, "+
					"no changes have been made to this unit; please report this project shape",
				unitID, bsonString(doc, "Name"), dup)
		}

		encoded, err := bson.Marshal(outDoc)
		if err != nil {
			return nil, plan, fmt.Errorf("encode unit %s: %w", unitID, err)
		}
		if err := b.UpdateRawUnit(unitID, encoded); err != nil {
			return nil, plan, fmt.Errorf("write unit %s: %w", unitID, err)
		}
		res.UnitsChanged++
		res.WidgetsChanged += widgets
		res.PropertiesFixed += changed
	}
	return res, plan, nil
}

// applyToWidget reconciles one stored CustomWidget node against its installed package.
//
// It delegates to widgets.AugmentTemplate rather than reimplementing the operations.
// That pass performs SIX reconciliations — enum option sets, property metadata
// (Caption/Category/DefaultValue), ValueType scalars, the AllowUpload envelope,
// PropertyType order, and definition attributes — plus add/remove of the property set
// itself. Hand-rolling only add/remove/attributes left 47 Captions, 32 Categories and
// every ValueType/Translations wrong on a synced DataGrid2, because those belong to
// passes that were never called.
//
// AugmentTemplate operates on a WidgetTemplate, which is exactly the (Type, Object)
// pair a stored instance carries — the shapes are the same, only the encoding differs.
func applyToWidget(widget bson.D, def *mpk.WidgetDefinition) (bson.D, bool) {
	typeDoc, ok := docField(widget, "Type")
	if !ok {
		return widget, false
	}
	objDoc, ok := docField(widget, "Object")
	if !ok {
		return widget, false
	}

	typeMap, ok := widgetToMap(typeDoc).(map[string]any)
	if !ok {
		return widget, false
	}
	objMap, ok := widgetToMap(objDoc).(map[string]any)
	if !ok {
		return widget, false
	}

	tmpl := &widgets.WidgetTemplate{
		WidgetID: bsonString(typeDoc, "WidgetId"),
		Type:     typeMap,
		Object:   objMap,
	}
	if err := widgets.AugmentTemplate(tmpl, def); err != nil {
		return widget, false
	}

	// AugmentTemplate mints placeholder IDs for anything it adds. Those are stable
	// strings, so writing them straight through would give every widget that gains the
	// same property an IDENTICAL $ID. Remap them to fresh UUIDs, consistently across
	// Type and Object together so TypePointer still binds its PropertyType.
	remap := map[string]string{}
	collectWidgetPlaceholders(tmpl.Type, remap)
	collectWidgetPlaceholders(tmpl.Object, remap)
	for k := range remap {
		remap[k] = types.GenerateID()
	}
	newType := rewriteWidgetIDs(tmpl.Type, remap)
	newObj := rewriteWidgetIDs(tmpl.Object, remap)

	// Remapping by VALUE is not sufficient: AugmentTemplate gives every entry of an
	// object-list property a copy of the same constructed node, so one placeholder
	// becomes N nodes that would all receive the same UUID. See ensureUniqueWidgetIDs
	// — this is the duplicate-Guid corruption reported against PR #89.
	seen := map[string]bool{}
	newType = ensureUniqueWidgetIDs(newType, seen)
	newObj = ensureUniqueWidgetIDs(newObj, seen)

	widget = setField(widget, "Type", mapToWidgetDoc(newType))
	widget = setField(widget, "Object", mapToWidgetDoc(newObj))
	return widget, true
}

// collectWidgetPlaceholders records every placeholder ID AugmentTemplate minted.
func collectWidgetPlaceholders(v any, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for _, val := range t {
			collectWidgetPlaceholders(val, out)
		}
	case []any:
		for _, item := range t {
			collectWidgetPlaceholders(item, out)
		}
	case string:
		if isWidgetPlaceholderID(t) {
			out[t] = ""
		}
	}
}

func rewriteWidgetIDs(v any, remap map[string]string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = rewriteWidgetIDs(val, remap)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = rewriteWidgetIDs(item, remap)
		}
		return out
	case string:
		if id, ok := remap[t]; ok && id != "" {
			return id
		}
	}
	return v
}

// isWidgetPlaceholderID matches the "aa"-prefixed IDs the template pipeline mints.
func isWidgetPlaceholderID(s string) bool {
	return len(s) == 32 && strings.HasPrefix(s, "aa0000000000000000000000")
}
