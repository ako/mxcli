// SPDX-License-Identifier: Apache-2.0

package types

import (
	"strings"
	"testing"
)

func TestPrettyPrintJSON_ValidObject(t *testing.T) {
	got := PrettyPrintJSON(`{"a":1,"b":"hello"}`)
	if !strings.Contains(got, "  ") {
		t.Errorf("expected indented output, got %q", got)
	}
	if !strings.Contains(got, `"a": 1`) {
		t.Errorf("expected formatted key, got %q", got)
	}
}

func TestPrettyPrintJSON_InvalidJSON(t *testing.T) {
	input := "not json"
	got := PrettyPrintJSON(input)
	if got != input {
		t.Errorf("expected original string for invalid JSON, got %q", got)
	}
}

func TestPrettyPrintJSON_EmptyObject(t *testing.T) {
	got := PrettyPrintJSON("{}")
	if got != "{}" {
		t.Errorf("expected '{}', got %q", got)
	}
}

func TestNormalizeDateTimeValue_WithFractional(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"2015-05-22T14:56:29.000Z", "2015-05-22T14:56:29.0000000Z"},
		{"2015-05-22T14:56:29.1234567Z", "2015-05-22T14:56:29.1234567Z"},
		{"2015-05-22T14:56:29.12345678Z", "2015-05-22T14:56:29.1234567Z"},
		{"2015-05-22T14:56:29.1Z", "2015-05-22T14:56:29.1000000Z"},
	}
	for _, tt := range tests {
		got := normalizeDateTimeValue(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeDateTimeValue(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNormalizeDateTimeValue_NoFractional(t *testing.T) {
	got := normalizeDateTimeValue("2015-05-22T14:56:29Z")
	if got != "2015-05-22T14:56:29.0000000Z" {
		t.Errorf("expected .0000000 inserted, got %q", got)
	}
}

func TestNormalizeDateTimeValue_WithTimezone(t *testing.T) {
	got := normalizeDateTimeValue("2015-05-22T14:56:29.123+02:00")
	if got != "2015-05-22T14:56:29.1230000+02:00" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeDateTimeValue_NoTimezone(t *testing.T) {
	got := normalizeDateTimeValue("2015-05-22T14:56:29.123")
	if got != "2015-05-22T14:56:29.1230000" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeDateTimeValue_NoFractionalNoTimezone(t *testing.T) {
	got := normalizeDateTimeValue("2015-05-22T14:56:29")
	if got != "2015-05-22T14:56:29.0000000" {
		t.Errorf("expected .0000000 appended, got %q", got)
	}
}

func TestBuildJsonElementsFromSnippet_SimpleObject(t *testing.T) {
	snippet := `{"name": "John", "age": 30, "active": true}`
	elems, err := BuildJsonElementsFromSnippet(snippet, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(elems) != 1 {
		t.Fatalf("expected 1 root element, got %d", len(elems))
	}
	root := elems[0]
	if root.ElementType != "Object" {
		t.Errorf("expected Object root, got %q", root.ElementType)
	}
	if root.ExposedName != "Root" {
		t.Errorf("expected Root name, got %q", root.ExposedName)
	}
	if len(root.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(root.Children))
	}

	// Check child types
	nameChild := root.Children[0]
	if nameChild.PrimitiveType != "String" {
		t.Errorf("expected String for 'name', got %q", nameChild.PrimitiveType)
	}
	ageChild := root.Children[1]
	if ageChild.PrimitiveType != "Integer" {
		t.Errorf("expected Integer for 'age', got %q", ageChild.PrimitiveType)
	}
	activeChild := root.Children[2]
	if activeChild.PrimitiveType != "Boolean" {
		t.Errorf("expected Boolean for 'active', got %q", activeChild.PrimitiveType)
	}
}

func TestBuildJsonElementsFromSnippet_RootArray(t *testing.T) {
	snippet := `[{"id": 1}]`
	elems, err := BuildJsonElementsFromSnippet(snippet, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := elems[0]
	if root.ElementType != "Array" {
		t.Errorf("expected Array root, got %q", root.ElementType)
	}
}

func TestBuildJsonElementsFromSnippet_InvalidJSON(t *testing.T) {
	_, err := BuildJsonElementsFromSnippet("not json", nil, nil)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestBuildJsonElementsFromSnippet_PrimitiveRoot(t *testing.T) {
	_, err := BuildJsonElementsFromSnippet(`"hello"`, nil, nil)
	if err == nil {
		t.Error("expected error for primitive root")
	}
}

func TestBuildJsonElementsFromSnippet_DateTimeDetection(t *testing.T) {
	snippet := `{"created": "2015-05-22T14:56:29.000Z"}`
	elems, err := BuildJsonElementsFromSnippet(snippet, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	child := elems[0].Children[0]
	if child.PrimitiveType != "DateTime" {
		t.Errorf("expected DateTime, got %q", child.PrimitiveType)
	}
	// OriginalValue should have normalized fractional seconds
	if !strings.Contains(child.OriginalValue, ".0000000") {
		t.Errorf("expected normalized datetime in OriginalValue, got %q", child.OriginalValue)
	}
}

func TestBuildJsonElementsFromSnippet_DecimalDetection(t *testing.T) {
	snippet := `{"price": 19.99}`
	elems, err := BuildJsonElementsFromSnippet(snippet, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	child := elems[0].Children[0]
	if child.PrimitiveType != "Decimal" {
		t.Errorf("expected Decimal, got %q", child.PrimitiveType)
	}
}

func TestBuildJsonElementsFromSnippet_NullValue(t *testing.T) {
	snippet := `{"value": null}`
	elems, err := BuildJsonElementsFromSnippet(snippet, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	child := elems[0].Children[0]
	if child.PrimitiveType != "Unknown" {
		t.Errorf("expected Unknown for null, got %q", child.PrimitiveType)
	}
}

func TestBuildJsonElementsFromSnippet_NestedObject(t *testing.T) {
	snippet := `{"address": {"city": "Amsterdam"}}`
	elems, err := BuildJsonElementsFromSnippet(snippet, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	addr := elems[0].Children[0]
	if addr.ElementType != "Object" {
		t.Errorf("expected Object for address, got %q", addr.ElementType)
	}
	if len(addr.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(addr.Children))
	}
}

func TestBuildJsonElementsFromSnippet_PrimitiveArray(t *testing.T) {
	snippet := `{"tags": ["a", "b"]}`
	elems, err := BuildJsonElementsFromSnippet(snippet, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tags := elems[0].Children[0]
	if tags.ElementType != "Array" {
		t.Errorf("expected Array for tags, got %q", tags.ElementType)
	}
	// Should have a Wrapper child
	if len(tags.Children) != 1 {
		t.Fatalf("expected 1 wrapper child, got %d", len(tags.Children))
	}
	wrapper := tags.Children[0]
	if wrapper.ElementType != "Wrapper" {
		t.Errorf("expected Wrapper, got %q", wrapper.ElementType)
	}
}

func TestBuildJsonElementsFromSnippet_CustomNameMap(t *testing.T) {
	snippet := `{"myField": "value"}`
	custom := map[string]string{"myField": "CustomName"}
	elems, err := BuildJsonElementsFromSnippet(snippet, custom, nil)
	if err != nil {
		t.Fatal(err)
	}
	child := elems[0].Children[0]
	if child.ExposedName != "CustomName" {
		t.Errorf("expected CustomName, got %q", child.ExposedName)
	}
}

func TestBuildJsonElementsFromSnippet_ReservedNames(t *testing.T) {
	// "id" capitalizes to "Id" which is reserved — should get underscore prefix
	snippet := `{"id": "123"}`
	elems, err := BuildJsonElementsFromSnippet(snippet, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	child := elems[0].Children[0]
	if child.ExposedName != "_id" {
		t.Errorf("expected _id for reserved name, got %q", child.ExposedName)
	}
}

func TestSingularize(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"Tags", "Tag"},
		{"Items", "Item"},
		{"s", "s"}, // single char
		{"", ""},
		{"Bus", "Bu"},
	}
	for _, tt := range tests {
		got := singularize(tt.input)
		if got != tt.expected {
			t.Errorf("singularize(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
