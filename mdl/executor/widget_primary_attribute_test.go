// SPDX-License-Identifier: Apache-2.0

// The bare `Attribute:` keyword names ONE attribute — the widget's primary one.
//
// It used to fill every attribute-typed mapping, so a Slider's
// `Attribute: Score` landed on valueAttribute AND minAttribute, maxAttribute and
// stepAttribute. Studio Pro leaves those unset unless the matching *ValueType is
// "attribute" (default "static"), and a value in a pruned slot is CE0463 (#238).
package executor

import "testing"

func sliderLikeDef() *WidgetDefinition {
	return &WidgetDefinition{
		WidgetID: "com.mendix.widget.custom.slider.Slider",
		MDLName:  "SLIDER",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "valueAttribute", Source: "Attribute", Operation: "attribute"},
			{PropertyKey: "minAttribute", Source: "Attribute", Operation: "attribute"},
			{PropertyKey: "maxAttribute", Source: "Attribute", Operation: "attribute"},
			{PropertyKey: "stepAttribute", Source: "Attribute", Operation: "attribute"},
		},
	}
}

func TestIsPrimaryAttributeMapping(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}, currentDef: sliderLikeDef()}

	if !e.isPrimaryAttributeMapping(PropertyMapping{PropertyKey: "valueAttribute"}) {
		t.Error("the FIRST attribute mapping is the widget's primary one and must take the bare `Attribute:`")
	}
	for _, k := range []string{"minAttribute", "maxAttribute", "stepAttribute"} {
		if e.isPrimaryAttributeMapping(PropertyMapping{PropertyKey: k}) {
			t.Errorf("%s took the bare `Attribute:` — Studio Pro leaves it unset unless its *ValueType is \"attribute\", and a value in a pruned slot is CE0463", k)
		}
	}
}

// A widget with a single attribute property is the case the bare keyword was
// written for and must be untouched.
func TestIsPrimaryAttributeMapping_SingleAttributeWidget(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}, currentDef: &WidgetDefinition{
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "textAttribute", Source: "Attribute", Operation: "attribute"},
		},
	}}
	if !e.isPrimaryAttributeMapping(PropertyMapping{PropertyKey: "textAttribute"}) {
		t.Error("a single-attribute widget lost its `Attribute:` routing")
	}
}

// No definition to compare against: behave as before rather than silently
// dropping the attribute.
func TestIsPrimaryAttributeMapping_NoDefinition(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	if !e.isPrimaryAttributeMapping(PropertyMapping{PropertyKey: "anything"}) {
		t.Error("with no definition the mapping must still be filled")
	}
}
