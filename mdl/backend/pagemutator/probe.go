// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"sort"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// A dry run, so `mxcli check` can refuse what `exec` refuses.
//
// The property names an ALTER PAGE `SET` accepts are not a list that exists
// anywhere. A first-class name is whatever setRawWidgetPropertyMut's switch
// happens to handle; a pluggable one is whatever the STORED widget's own
// PropertyTypes declare, which is specific to the widget package the project has
// installed — no registry, and no table in this repo, can answer it for an
// arbitrary project.
//
// So the check does not re-derive the rule. It runs the real setter against a
// copy of the document and keeps only the error. That matters more than the code
// it saves: a check that re-implemented the resolution would drift from the
// setter, and the drift is silent in exactly the direction that hurts — a check
// that passes what exec then refuses, which is the gap this closes. The copy's
// Save is refused, so a mistake in a caller cannot turn a check into a write.
//
// Probe returns a copy of this mutator over its own deep copy of the document.
// Writes to it are visible nowhere: not in this mutator, and not on disk.
func (m *Mutator) Probe() (backend.PageMutator, error) {
	raw, err := bson.Marshal(m.rawData)
	if err != nil {
		return nil, fmt.Errorf("probe %s: marshal: %w", m.containerType, err)
	}
	var cloned bson.D
	if err := bson.Unmarshal(raw, &cloned); err != nil {
		return nil, fmt.Errorf("probe %s: unmarshal: %w", m.containerType, err)
	}
	return &Mutator{
		rawData:       cloned,
		containerType: m.containerType,
		unitID:        m.unitID,
		deps:          m.deps,
		widgetFinder:  m.widgetFinder,
		probe:         true,
	}, nil
}

// ResolvesTarget reports whether the stored document carries what this
// reference names — a widget, or a grid column when columnRef is set. It
// answers the question the setters answer first, so a caller can tell a target
// that is missing from one whose property is wrong, without reading an error
// message to find out which.
func (m *Mutator) ResolvesTarget(widgetRef, columnRef string) bool {
	if widgetRef == "" {
		return true // page-level SET addresses the document itself
	}
	if columnRef != "" {
		_, err := findBsonColumn(m.rawData, widgetRef, columnRef, m.widgetFinder)
		return err == nil
	}
	return m.widgetFinder(m.rawData, widgetRef) != nil
}

// WidgetPropertyKeys returns the property names the STORED widget declares — a
// pluggable widget's own template keys, or a DataGrid 2 column's when columnRef
// names one. It is the vocabulary a failed `SET` should be measured against, so
// the author sees what the widget has rather than only that their spelling is
// not it.
//
// A built-in widget returns nothing: its vocabulary is the setter's switch, not
// anything the document carries, and reporting an empty list as "this widget has
// no properties" would be worse than saying nothing.
func (m *Mutator) WidgetPropertyKeys(widgetRef, columnRef string) []string {
	byID := map[string]string{}
	if columnRef != "" {
		result, err := findBsonColumn(m.rawData, widgetRef, columnRef, m.widgetFinder)
		if err != nil || result == nil {
			return nil
		}
		byID = result.colPropKeys
	} else {
		result := m.widgetFinder(m.rawData, widgetRef)
		if result == nil {
			return nil
		}
		byID = result.colPropKeys
		if len(byID) == 0 {
			byID = buildPropKeyMap(result.widget)
		}
	}
	seen := make(map[string]bool, len(byID))
	keys := make([]string, 0, len(byID))
	for _, key := range byID {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
