// SPDX-License-Identifier: Apache-2.0

// Ledger finding #104: DESCRIBE PAGE does not round-trip a pluggable widget.
//
// Two independent defects, and fixing either one alone makes things worse:
//
//   - Every explicit property was emitted with a raw %s, so a string value lost
//     its quotes. A JSON spec then broke the re-parse at its first '{'.
//   - extractExplicitProperties skipped any value of "true"/"false" as a
//     "common default", so booleans never reached the output at all.
//
// Fix the quoting alone and the description re-parses cleanly while silently
// dropping a property — a worse failure than one that errors. So both halves
// are pinned here, and by the widget's DECLARED property type rather than by
// the shape of the value: a String property whose value happens to be "30" or
// "true" must still come back quoted.
package executor

import (
	"testing"
)

// buildVegaLikeWidget mirrors the ledger's probe: one pluggable widget carrying
// a string, an enumeration, an integer and a boolean, with the declared
// ValueTypes the widget package ships in Type.ObjectType.PropertyTypes.
func buildVegaLikeWidget() map[string]any {
	const (
		idSpec        = "type-id-spec"
		idDatasetName = "type-id-dataset"
		idRenderer    = "type-id-renderer"
		idHeight      = "type-id-height"
		idShowActions = "type-id-showactions"
	)

	widgetType := map[string]any{
		"WidgetId": "ledger.widget.web.vegachart.VegaChart",
		"ObjectType": map[string]any{
			"PropertyTypes": []any{
				map[string]any{"$ID": idSpec, "PropertyKey": "spec",
					"ValueType": map[string]any{"Type": "String"}},
				map[string]any{"$ID": idDatasetName, "PropertyKey": "datasetName",
					"ValueType": map[string]any{"Type": "String"}},
				map[string]any{"$ID": idRenderer, "PropertyKey": "renderer",
					"ValueType": map[string]any{"Type": "Enumeration"}},
				map[string]any{"$ID": idHeight, "PropertyKey": "chartHeight",
					"ValueType": map[string]any{"Type": "Integer"}},
				map[string]any{"$ID": idShowActions, "PropertyKey": "showActions",
					"ValueType": map[string]any{"Type": "Boolean"}},
			},
		},
	}

	properties := []any{
		map[string]any{"TypePointer": idSpec,
			"Value": map[string]any{"PrimitiveValue": `{"a": 1}`}},
		map[string]any{"TypePointer": idDatasetName,
			"Value": map[string]any{"PrimitiveValue": "table"}},
		map[string]any{"TypePointer": idRenderer,
			"Value": map[string]any{"PrimitiveValue": "svg"}},
		map[string]any{"TypePointer": idHeight,
			"Value": map[string]any{"PrimitiveValue": "30"}},
		map[string]any{"TypePointer": idShowActions,
			"Value": map[string]any{"PrimitiveValue": "true"}},
	}

	return map[string]any{
		"Type":   widgetType,
		"Object": map[string]any{"Properties": properties},
	}
}

// TestExplicitPropertiesKeepBooleans is the read half. A boolean is a value the
// author wrote, not noise: "true" was discarded on the way out, so DESCRIBE
// could not show it and a round-trip silently turned it off.
func TestExplicitPropertiesKeepBooleans(t *testing.T) {
	props := extractExplicitProperties(nil, buildVegaLikeWidget())

	byKey := map[string]rawExplicitProp{}
	for _, p := range props {
		byKey[p.Key] = p
	}

	got, ok := byKey["showActions"]
	if !ok {
		t.Fatalf("showActions was dropped; got keys %v", explicitKeys(byKey))
	}
	if got.Value != "true" {
		t.Errorf("showActions = %q, want %q", got.Value, "true")
	}
	if got.ValueType != "Boolean" {
		t.Errorf("showActions ValueType = %q, want %q — the emitter needs it to leave the literal bare",
			got.ValueType, "Boolean")
	}
	// Everything the author set must survive, not just the boolean.
	for _, want := range []string{"spec", "datasetName", "renderer", "chartHeight"} {
		if _, ok := byKey[want]; !ok {
			t.Errorf("%s was dropped; got keys %v", want, explicitKeys(byKey))
		}
	}
}

// TestExplicitValueQuotingByDeclaredType is the emit half. Strings and
// enumerations are quoted; numbers and booleans are not; and an attribute
// reference stays bare because it is an identifier, not a literal.
func TestExplicitValueQuotingByDeclaredType(t *testing.T) {
	cases := []struct {
		name string
		in   rawExplicitProp
		want string
	}{
		{"string", rawExplicitProp{Key: "datasetName", Value: "table", ValueType: "String"}, "'table'"},
		{"json string", rawExplicitProp{Key: "spec", Value: `{"a": 1}`, ValueType: "String"}, `'{"a": 1}'`},
		{"enumeration", rawExplicitProp{Key: "renderer", Value: "svg", ValueType: "Enumeration"}, "'svg'"},
		{"integer", rawExplicitProp{Key: "chartHeight", Value: "30", ValueType: "Integer"}, "30"},
		{"decimal", rawExplicitProp{Key: "ratio", Value: "1.5", ValueType: "Decimal"}, "1.5"},
		{"boolean", rawExplicitProp{Key: "showActions", Value: "true", ValueType: "Boolean"}, "true"},
		{"attribute ref", rawExplicitProp{Key: "value", Value: "Amount", IsRef: true}, "Amount"},

		// A String property whose value looks like a literal is exactly why the
		// declared type is used instead of the value's shape.
		{"numeric-looking string", rawExplicitProp{Key: "label", Value: "30", ValueType: "String"}, "'30'"},
		{"boolean-looking string", rawExplicitProp{Key: "label", Value: "true", ValueType: "String"}, "'true'"},

		// No declared type: fall back to the value's shape, quoting anything that
		// is not plainly numeric or boolean, since an unquoted arbitrary string
		// may not parse at all.
		{"untyped text", rawExplicitProp{Key: "k", Value: "svg"}, "'svg'"},
		{"untyped number", rawExplicitProp{Key: "k", Value: "30"}, "30"},
		{"untyped bool", rawExplicitProp{Key: "k", Value: "false"}, "false"},

		// A quote inside a value must be escaped, not emitted raw.
		{"embedded quote", rawExplicitProp{Key: "k", Value: "it's", ValueType: "String"}, "'it''s'"},
	}

	for _, tc := range cases {
		if got := explicitPropValue(tc.in); got != tc.want {
			t.Errorf("%s: explicitPropValue(%q/%q) = %s, want %s",
				tc.name, tc.in.Value, tc.in.ValueType, got, tc.want)
		}
	}
}

func explicitKeys(m map[string]rawExplicitProp) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
