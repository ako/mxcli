// SPDX-License-Identifier: Apache-2.0

package types

// WidgetVisibilityRule declares that a pluggable widget property is hidden
// under certain configurations of the same widget. Pluggable widgets express
// this in their compiled editorConfig.js via Mendix's hidePropertyIn /
// hidePropertiesIn helpers; this struct is the structured form mxcli stores in
// a widget's .def.json and evaluates at BSON serialization time.
//
// When a TextTemplate-typed property is hidden, Studio Pro nulls its
// TextTemplate; emitting the template's populated default instead triggers
// CE0463 ("the definition of this widget has changed"). See issue #574.
type WidgetVisibilityRule struct {
	PropertyKey string `json:"propertyKey"`
	// ListPropertyKey names the object-list property whose ITEMS carry
	// PropertyKey (e.g. an Accordion group's `initialCollapsedState` lives under
	// list `groups`). Empty for a rule about a top-level widget property.
	//
	// Nested rules are evaluated per list item, so consumers that only understand
	// top-level properties MUST skip a rule with a non-empty ListPropertyKey —
	// its PropertyKey does not name a property of the widget itself.
	ListPropertyKey string                     `json:"listPropertyKey,omitempty"`
	HiddenWhen      *WidgetVisibilityCondition `json:"hiddenWhen,omitempty"`
}

// Nested reports whether the rule targets an item sub-property of an object list
// rather than a top-level widget property.
func (r WidgetVisibilityRule) Nested() bool { return r.ListPropertyKey != "" }

// WidgetVisibilityCondition is a single predicate evaluated against the
// widget's current property values. Operators cover the dominant patterns
// observed in marketplace editorConfig.js files:
//
//	eq      — the named property equals Value
//	ne      — the named property differs from Value
//	truthy  — the named property is set / non-empty / not "false"/"0"
//	falsy   — the named property is unset / empty / "false" / "0"
//
// Conditions that don't fit (composite logic, runtime data lookups) are left
// unset, which evaluates to "not hidden" so serialization falls back to the
// template default.
type WidgetVisibilityCondition struct {
	PropertyKey string `json:"propertyKey"`
	Operator    string `json:"operator"`
	Value       string `json:"value,omitempty"`
	// Scope says which object PropertyKey belongs to. "" (the default) means the
	// widget itself; ConditionScopeItem means the sibling sub-property of the
	// same object-list item, which only makes sense on a nested rule. An
	// Accordion has both: its group properties are hidden when the WIDGET's
	// `collapsible` is off, and `initiallyCollapsed` is hidden unless the
	// GROUP's own `initialCollapsedState` is "dynamic".
	Scope string `json:"scope,omitempty"`
}

// ConditionScopeItem marks a condition evaluated against the object-list item
// that carries the rule's property, rather than against the widget.
const ConditionScopeItem = "item"

// Hidden reports whether the condition matches given the widget's current
// property values (keyed by property key, each value the property's primitive
// string form). An unset or unrecognized operator is treated as "not hidden".
func (c *WidgetVisibilityCondition) Hidden(values map[string]string) bool {
	if c == nil {
		return false
	}
	current := values[c.PropertyKey]
	switch c.Operator {
	case "eq":
		return current == c.Value
	case "ne":
		return current != c.Value
	case "truthy":
		return isTruthyPrimitive(current)
	case "falsy":
		return !isTruthyPrimitive(current)
	default:
		return false
	}
}

// isTruthyPrimitive mirrors how Mendix treats a boolean/enum primitive in
// editorConfig.js: empty, "false", and "0" are falsy; everything else truthy.
func isTruthyPrimitive(v string) bool {
	switch v {
	case "", "false", "0":
		return false
	default:
		return true
	}
}
