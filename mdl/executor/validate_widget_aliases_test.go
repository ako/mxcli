// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// A property mapping's MdlAliases are the names a person is meant to WRITE.
// They must be accepted by the property validator, or the validator rejects the
// only spelling the documentation offers.
//
// # The defect
//
// A PieChart binds its data at the widget level, so its def.json carries
//
//	{"propertyKey": "seriesValueAttribute", "source": "Attribute",
//	 "operation": "attribute", "mdlAliases": ["ValueAttribute"]}
//
// and the builder resolves `ValueAttribute:` through that alias — measured: the
// stored page comes back from DESCRIBE as `seriesValueAttribute: Total`, so the
// value persists. allowedWidgetProperties built its set from PropertyKey and
// Source only, so `ValueAttribute` was unknown and MDL-WIDGET01 fired. Since
// exec refuses to run a script with errors, the false positive did not merely
// warn — it blocked the page from being written at all.
//
// This is the "two lists, nothing comparing them" class again, and the sibling
// list in widget_defs.go (`mapped`, for knownProperties) already walks
// MdlAliases — so the two disagreed about the same def.json.
func TestAllowedWidgetProperties_IncludesMdlAliases(t *testing.T) {
	def := &WidgetDefinition{
		WidgetID: "com.acme.Test",
		MDLName:  "test",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "seriesValueAttribute", Source: "Attribute",
				Operation: "attribute", MdlAliases: []string{"ValueAttribute"}},
		},
	}

	allowed, keys := allowedWidgetProperties(def)

	if !allowed["valueattribute"] {
		t.Errorf("the alias `ValueAttribute` is not an allowed property; allowed keys: %v", keys)
	}
	// The control: the schema key must STILL be allowed. A fix that swapped one
	// name for the other would pass the assertion above and break every script
	// written against the schema key — including DESCRIBE's own output, which
	// emits `seriesValueAttribute`.
	if !allowed["seriesvalueattribute"] {
		t.Errorf("the schema key `seriesValueAttribute` stopped being allowed; allowed keys: %v", keys)
	}
	// The suggestion list is what "did you mean" reads, so a typo'd alias should
	// point back at the alias, not only at the schema key.
	var sawAlias bool
	for _, k := range keys {
		if k == "ValueAttribute" {
			sawAlias = true
		}
	}
	if !sawAlias {
		t.Errorf("`ValueAttribute` missing from the suggestion list %v — a typo would be told to "+
			"use the internal name instead of the documented one", keys)
	}
}

// Mode-scoped mappings carry aliases too, and go through the same helper.
//
// The alias here is deliberately NOT a case variant of the property key. The
// PieChart's real pair is `seriesName` / `SeriesName`, which collapses to one
// entry once lowercased — so a test using it passes against the broken code and
// proves nothing.
func TestAllowedWidgetProperties_IncludesMdlAliasesInModes(t *testing.T) {
	def := &WidgetDefinition{
		WidgetID: "com.acme.Test",
		MDLName:  "test",
		Modes: []WidgetMode{{
			PropertyMappings: []PropertyMapping{
				{PropertyKey: "seriesSortAttribute", Source: "Attribute",
					Operation: "attribute", MdlAliases: []string{"SortAttribute"}},
			},
		}},
	}
	allowed, keys := allowedWidgetProperties(def)
	if !allowed["sortattribute"] {
		t.Errorf("mode-scoped alias `SortAttribute` not allowed; keys: %v", keys)
	}
}

// The end-to-end shape of the bug, against the real embedded/installed
// definitions rather than a hand-built one: a PieChart written the documented
// way must not produce MDL-WIDGET01.
//
// Skips when the PieChart definition is not available (it ships in Charts.mpk,
// not in the embedded set), so this is a bonus assertion — the two above are the
// ones that always run.
func TestPieChartValueAttributeIsAccepted(t *testing.T) {
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Skip("no registry")
	}
	def, ok := registry.GetByWidgetID("com.mendix.widget.web.piechart.PieChart")
	if !ok || def == nil {
		t.Skip("PieChart definition not available without a project")
	}
	allowed, keys := allowedWidgetProperties(def)
	if !allowed["valueattribute"] {
		t.Errorf("PieChart rejects `ValueAttribute:`, the name propertyAliases registers "+
			"and the builder resolves; allowed: %s", strings.Join(keys, ", "))
	}
}
