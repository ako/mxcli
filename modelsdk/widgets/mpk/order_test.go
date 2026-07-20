// SPDX-License-Identifier: Apache-2.0

package mpk

import (
	"encoding/xml"
	"testing"
)

// parseWidgetXML unmarshals a widget XML snippet into a WidgetDefinition using the
// same path as ParseMPK (xmlWidget → buildDefinition).
func parseWidgetXML(t *testing.T, x string) *WidgetDefinition {
	t.Helper()
	var w xmlWidget
	if err := xml.Unmarshal([]byte(x), &w); err != nil {
		t.Fatalf("unmarshal widget xml: %v", err)
	}
	return buildDefinition(&w, "1.0.0")
}

// TestAllTopLevel_InterleavesSystemProps asserts that AllTopLevel preserves the
// widget XML's declared order with <systemProperty> elements interleaved among
// regular <property> elements — including a mixed group where a systemProperty is
// declared ahead of a regular property (the ComboBox "Editability" group shape).
// mxbuild emits the WidgetType's PropertyTypes in this order and CE0463 checks it.
func TestAllTopLevel_InterleavesSystemProps(t *testing.T) {
	x := `<widget id="x.Y" pluginWidget="true">
	  <name>Y</name>
	  <properties>
	    <propertyGroup caption="Group">
	      <property key="alpha" type="string"><caption>Alpha</caption></property>
	      <propertyGroup caption="Label"><systemProperty key="Label"/></propertyGroup>
	      <propertyGroup caption="Conditional visibility"><systemProperty key="Visibility"/></propertyGroup>
	      <propertyGroup caption="Editability">
	        <systemProperty key="Editability"/>
	        <property key="customEditability" type="enumeration" defaultValue="default"><caption>Editable</caption></property>
	      </propertyGroup>
	      <property key="omega" type="string"><caption>Omega</caption></property>
	    </propertyGroup>
	  </properties>
	</widget>`
	def := parseWidgetXML(t, x)

	got := make([]string, 0, len(def.AllTopLevel))
	for _, p := range def.AllTopLevel {
		got = append(got, p.Key)
	}
	want := []string{"alpha", "Label", "Visibility", "Editability", "customEditability", "omega"}
	if len(got) != len(want) {
		t.Fatalf("AllTopLevel = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AllTopLevel[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}

	// System entries must be flagged IsSystem; regular ones must not.
	sys := map[string]bool{"Label": true, "Visibility": true, "Editability": true}
	for _, p := range def.AllTopLevel {
		if p.IsSystem != sys[p.Key] {
			t.Errorf("%s: IsSystem=%v, want %v", p.Key, p.IsSystem, sys[p.Key])
		}
	}

	// def.Properties (regular only) excludes system props — it is the group-walk
	// order (regulars-then-subgroups), which is why AllTopLevel, not def.Properties,
	// is the authoritative reorder key: the old reorder-by-def.Properties left the
	// system props stranded at the end.
	reg := map[string]bool{}
	for _, p := range def.Properties {
		reg[p.Key] = true
	}
	for _, k := range []string{"alpha", "customEditability", "omega"} {
		if !reg[k] {
			t.Errorf("def.Properties missing regular key %q", k)
		}
	}
	for _, k := range []string{"Label", "Visibility", "Editability"} {
		if reg[k] {
			t.Errorf("def.Properties must not contain system prop %q", k)
		}
	}
}

// TestReturnTypeAssignableTo asserts the parser captures a
// <returnType assignableTo="../other"/> reference (used by ComboBox's
// staticDataSourceValue), distinct from a concrete <returnType type="String"/>.
func TestReturnTypeAssignableTo(t *testing.T) {
	x := `<widget id="x.Y" pluginWidget="true">
	  <name>Y</name>
	  <properties>
	    <propertyGroup caption="Group">
	      <property key="typed" type="expression"><caption>T</caption><returnType type="String"/></property>
	      <property key="assignable" type="expression"><caption>A</caption><returnType assignableTo="../other"/></property>
	    </propertyGroup>
	  </properties>
	</widget>`
	def := parseWidgetXML(t, x)

	byKey := map[string]PropertyDef{}
	for _, p := range def.Properties {
		byKey[p.Key] = p
	}
	if p := byKey["typed"]; p.ReturnType != "String" || p.ReturnTypeAssignableTo != "" {
		t.Errorf("typed: ReturnType=%q AssignableTo=%q, want String/empty", p.ReturnType, p.ReturnTypeAssignableTo)
	}
	if p := byKey["assignable"]; p.ReturnType != "" || p.ReturnTypeAssignableTo != "../other" {
		t.Errorf("assignable: ReturnType=%q AssignableTo=%q, want empty/../other", p.ReturnType, p.ReturnTypeAssignableTo)
	}
}
