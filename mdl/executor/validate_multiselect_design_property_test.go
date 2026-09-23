// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#511.
//
// A design property declared `"multiSelect": true` is a SET of chosen options,
// and Mendix stores it as a compound — measured by decoding a Studio Pro-authored
// document already in a blank 11.12.2 project
// (mprcontents/b5/41/b541cfca-….mxunit, an Atlas page):
//
//	Forms$DesignPropertyValue
//	  Key:   "Hide on"
//	  Value: Forms$CompoundDesignPropertyValue
//	           Properties: [3,
//	             Forms$DesignPropertyValue{ Key: "Phone",
//	                                        Value: Forms$ToggleDesignPropertyValue }]
//
// Structurally identical to `Spacing`, which MDL already writes. So the capability
// is NOT missing — `DesignProperties: ['Hide on': ['Phone': on, 'Tablet': on]]`
// writes it, `describe styling` reads it back, and mxbuild reports 0 errors.
//
// What was broken is the FLAT spelling. `'Hide on': 'Phone'` looks reasonable,
// is in the declared option list, and so was written as a plain
// Forms$OptionDesignPropertyValue — which mxbuild refuses:
//
//	[CE6084] "Expected design property Hide on to be of type Toggle button group,
//	         but found Option." at List view 'lvThings'
//
// Measured: the same statement with 'Align self' (same declared type, NOT
// multiSelect) is 0 errors. So the flat form must be refused and told the
// spelling that works, not silently written.
func TestDesignProperty_FlatValueOnMultiSelectIsRefused(t *testing.T) {
	props := []ThemeProperty{{
		Name:        "Hide on",
		Type:        "ToggleButtonGroup",
		MultiSelect: true,
		Options:     []ThemeOption{{Name: "Phone"}, {Name: "Tablet"}},
	}}
	_, _, err := astDesignPropToValueChecked(
		ast.DesignPropertyEntryV3{Key: "Hide on", Value: "Phone"}, props)
	if err == nil {
		t.Fatal("a flat value on a multi-select design property was accepted; mxbuild refuses it with CE6084")
	}
	if !strings.Contains(err.Error(), "Hide on") {
		t.Errorf("the message does not name the property: %q", err)
	}
	// Naming the spelling that works is the whole point — the author cannot
	// derive it from "CE6084: found Option".
	if !strings.Contains(err.Error(), "'Phone': on") {
		t.Errorf("the message does not name the compound spelling that works: %q", err)
	}
}

// The control that keeps the refusal narrow: the SAME declared control type,
// without multiSelect, still writes a plain option. Refusing both would break
// every ToggleButtonGroup in every Atlas page.
func TestDesignProperty_FlatValueOnSingleSelectStillWrites(t *testing.T) {
	props := []ThemeProperty{{
		Name:    "Align self",
		Type:    "ToggleButtonGroup",
		Options: []ThemeOption{{Name: "Left"}, {Name: "Right"}},
	}}
	dp, ok, err := astDesignPropToValueChecked(
		ast.DesignPropertyEntryV3{Key: "Align self", Value: "Right"}, props)
	if err != nil {
		t.Fatalf("a single-select ToggleButtonGroup was refused: %v", err)
	}
	if !ok || dp.ValueType != "option" || dp.Option != "Right" {
		t.Errorf("got %+v (ok=%v), want an option value carrying Right", dp, ok)
	}
}

// The compound spelling is the one that works, so it must keep working. This is
// the shape decoded from the Studio Pro document above.
func TestDesignProperty_CompoundOnMultiSelectIsAccepted(t *testing.T) {
	props := []ThemeProperty{{
		Name: "Hide on", Type: "ToggleButtonGroup", MultiSelect: true,
		Options: []ThemeOption{{Name: "Phone"}, {Name: "Tablet"}},
	}}
	dp, ok, err := astDesignPropToValueChecked(ast.DesignPropertyEntryV3{
		Key: "Hide on",
		Nested: []ast.DesignPropertyEntryV3{
			{Key: "Phone", Value: "on"},
			{Key: "Tablet", Value: "on"},
		},
	}, props)
	if err != nil {
		t.Fatalf("the compound spelling was refused: %v", err)
	}
	if !ok || dp.ValueType != "compound" || len(dp.Compound) != 2 {
		t.Fatalf("got %+v (ok=%v), want a compound with 2 entries", dp, ok)
	}
	for _, sub := range dp.Compound {
		if sub.ValueType != "toggle" {
			t.Errorf("sub-entry %q has ValueType %q, want toggle (Forms$ToggleDesignPropertyValue)",
				sub.Key, sub.ValueType)
		}
	}
}

// With no theme metadata there is no multiSelect flag to read, so nothing can be
// refused on its strength. Silence, and write what was asked.
func TestDesignProperty_NoThemeMetadataWritesFlat(t *testing.T) {
	dp, ok, err := astDesignPropToValueChecked(
		ast.DesignPropertyEntryV3{Key: "Unknown", Value: "Whatever"}, nil)
	if err != nil {
		t.Fatalf("refused a property the theme says nothing about: %v", err)
	}
	if !ok || dp.ValueType != "option" {
		t.Errorf("got %+v (ok=%v), want a plain option", dp, ok)
	}
}

// The theme reader must actually parse the flag, or every test above passes
// against a registry where MultiSelect is false. Atlas declares it on `Hide on`.
func TestThemeReader_ParsesMultiSelect(t *testing.T) {
	const js = `{"Widget":[
	  {"name":"Hide on","type":"ToggleButtonGroup","multiSelect":true,
	   "options":[{"name":"Phone"},{"name":"Tablet"}]},
	  {"name":"Align self","type":"ToggleButtonGroup","options":[{"name":"Left"}]}]}`
	reg, err := parseDesignPropertiesJSON([]byte(js))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	byName := map[string]ThemeProperty{}
	for _, p := range reg["Widget"] {
		byName[p.Name] = p
	}
	if !byName["Hide on"].MultiSelect {
		t.Error("multiSelect was not parsed for `Hide on` — Atlas declares it")
	}
	if byName["Align self"].MultiSelect {
		t.Error("multiSelect was set for a property that does not declare it")
	}
}

// mustDesignPropValue is the two-result form the older tests were written
// against. It exists so they exercise astDesignPropToValueChecked — the function
// production calls — rather than a wrapper kept alive only by those tests.
func mustDesignPropValue(t *testing.T, p ast.DesignPropertyEntryV3, themeProps []ThemeProperty) (pages.DesignPropertyValue, bool) {
	t.Helper()
	dp, ok, err := astDesignPropToValueChecked(p, themeProps)
	if err != nil {
		t.Fatalf("design property %q: %v", p.Key, err)
	}
	return dp, ok
}
