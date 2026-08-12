// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// upstream #830, second half: the drop-down filter's ASSOCIATION mode
// (`baseType: 'ref'`) was unauthorable. Every ref-mode property was unmapped in
// dropdownfilter.def.json, so `mxcli check` reported MDL-WIDGET01 "has no
// property `refEntity`" and exec dropped the lot — leaving no MDL way to filter
// a grid by a reference at all, which is what the issue asked for.
//
// It is now a def.json mode entered by giving the filter a `datasource:` (the
// OPTION list), mirroring the ComboBox's association mode. The four properties
// pinned here are the ones mxbuild reads; verified on 11.13.0 via `mx dump-mpr`:
// baseType="ref", refEntity=IndirectEntityRef{steps:[Order_Customer → Customer]},
// refOptions=XPathSource{Customer}, refCaption=AttributeRef{ZKT38.Customer.Name}.
func TestDropdownFilter_AssociationModeMappings(t *testing.T) {
	reg := LoadWidgetRegistry("")
	if reg == nil {
		t.Fatal("built-in widget registry not available")
	}
	def, ok := reg.Get("DROPDOWNFILTER")
	if !ok {
		t.Fatal("no DROPDOWNFILTER definition in the built-in registry")
	}
	engine := &PluggableWidgetEngine{}

	assoc := &ast.WidgetV3{
		Name: "ddf",
		Type: "dropdownfilter",
		Properties: map[string]any{
			"Association":      "ZKT38.Order_Customer",
			"CaptionAttribute": "Name",
			"DataSource":       &ast.DataSourceV3{Type: "database", Reference: "ZKT38.Customer"},
		},
	}
	mappings, _, err := engine.selectMappings(def, assoc)
	if err != nil {
		t.Fatalf("selectMappings: %v", err)
	}
	got := map[string]PropertyMapping{}
	for _, m := range mappings {
		got[m.PropertyKey] = m
	}
	// The reference is stored on refEntity via the `association` operation — the
	// WidgetValue has no association-valued field, so this writes an EntityRef
	// with association steps, exactly like the ComboBox.
	for key, wantOp := range map[string]string{
		"baseType":   "primitive",
		"refOptions": "datasource",
		"refEntity":  "association",
		"refCaption": "attribute",
	} {
		m, ok := got[key]
		if !ok {
			t.Errorf("association mode does not map %q — the property is unwritable without it", key)
			continue
		}
		if m.Operation != wantOp {
			t.Errorf("%q operation = %q, want %q", key, m.Operation, wantOp)
		}
	}
	if got["baseType"].Value != "ref" {
		t.Errorf("baseType = %q, want the literal \"ref\" — anything else leaves the widget in attribute mode",
			got["baseType"].Value)
	}

	// A filter with no datasource is an ordinary column filter and must keep the
	// attribute-mode mappings; the mode split must not disturb it.
	plain := &ast.WidgetV3{Name: "ddf", Type: "dropdownfilter", Properties: map[string]any{}}
	plainMappings, _, err := engine.selectMappings(def, plain)
	if err != nil {
		t.Fatalf("selectMappings (attribute mode): %v", err)
	}
	sawAttrChoice := false
	for _, m := range plainMappings {
		if m.PropertyKey == "baseType" {
			t.Error("a datasource-less filter must stay in attribute mode, but baseType is being written")
		}
		if m.PropertyKey == "attrChoice" {
			sawAttrChoice = true
		}
	}
	if !sawAttrChoice {
		t.Error("attribute mode lost its attrChoice mapping")
	}
}
