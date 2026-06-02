// SPDX-License-Identifier: Apache-2.0

package mprbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// makeStyleableWidget builds a widget with a Forms$Appearance sub-document and
// an empty (marker-only) DesignProperties array, matching serializeAppearance.
func makeStyleableWidget(name string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Pages$DivContainer"},
		{Key: "Name", Value: name},
		{Key: "Appearance", Value: bson.D{
			{Key: "$Type", Value: "Forms$Appearance"},
			{Key: "Class", Value: ""},
			{Key: "DesignProperties", Value: bson.A{int32(3)}},
			{Key: "DynamicClasses", Value: ""},
			{Key: "Style", Value: ""},
		}},
	}
}

// designPropEntries returns the DesignPropertyValue entries (marker stripped) for a widget.
func designPropEntries(t *testing.T, rawData bson.D, widgetName string) []any {
	t.Helper()
	result := findBsonWidget(rawData, widgetName)
	if result == nil {
		t.Fatalf("widget %q not found", widgetName)
	}
	app := dGetDoc(result.widget, "Appearance")
	if app == nil {
		t.Fatalf("widget %q has no Appearance", widgetName)
	}
	return dGetArrayElements(dGet(app, "DesignProperties"))
}

// findEntry returns the DesignPropertyValue entry with the given Key, or nil.
func findEntry(entries []any, key string) bson.D {
	for _, el := range entries {
		if entry, ok := el.(bson.D); ok && dGetString(entry, "Key") == key {
			return entry
		}
	}
	return nil
}

func TestSetDesignProperty_ToggleOn_Appends(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &mprPageMutator{rawData: rawData, widgetFinder: findBsonWidget}

	if err := m.SetDesignProperty("ctn1", "Full width", "toggle", ""); err != nil {
		t.Fatalf("SetDesignProperty failed: %v", err)
	}

	entries := designPropEntries(t, rawData, "ctn1")
	if len(entries) != 1 {
		t.Fatalf("expected 1 design property, got %d", len(entries))
	}
	entry := findEntry(entries, "Full width")
	if entry == nil {
		t.Fatal("expected entry for 'Full width'")
	}
	if dGetString(entry, "$Type") != designPropertyEntryType {
		t.Errorf("expected entry $Type=%q, got %q", designPropertyEntryType, dGetString(entry, "$Type"))
	}
	val := dGetDoc(entry, "Value")
	if val == nil || dGetString(val, "$Type") != toggleDesignPropertyType {
		t.Errorf("expected toggle value, got %#v", dGet(entry, "Value"))
	}
}

func TestSetDesignProperty_Option_Appends(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &mprPageMutator{rawData: rawData, widgetFinder: findBsonWidget}

	if err := m.SetDesignProperty("ctn1", "Spacing bottom", "option", "Large"); err != nil {
		t.Fatalf("SetDesignProperty failed: %v", err)
	}

	entry := findEntry(designPropEntries(t, rawData, "ctn1"), "Spacing bottom")
	if entry == nil {
		t.Fatal("expected entry for 'Spacing bottom'")
	}
	val := dGetDoc(entry, "Value")
	if val == nil || dGetString(val, "$Type") != optionDesignPropertyType {
		t.Fatalf("expected option value, got %#v", dGet(entry, "Value"))
	}
	if dGetString(val, "Option") != "Large" {
		t.Errorf("expected Option='Large', got %q", dGetString(val, "Option"))
	}
}

func TestSetDesignProperty_UpdatesExistingKeyInPlace(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &mprPageMutator{rawData: rawData, widgetFinder: findBsonWidget}

	// First set as toggle, then re-set the same key as an option.
	if err := m.SetDesignProperty("ctn1", "Mode", "toggle", ""); err != nil {
		t.Fatalf("first set failed: %v", err)
	}
	if err := m.SetDesignProperty("ctn1", "Mode", "option", "Compact"); err != nil {
		t.Fatalf("second set failed: %v", err)
	}

	entries := designPropEntries(t, rawData, "ctn1")
	if len(entries) != 1 {
		t.Fatalf("expected key updated in place (1 entry), got %d", len(entries))
	}
	val := dGetDoc(findEntry(entries, "Mode"), "Value")
	if dGetString(val, "$Type") != optionDesignPropertyType || dGetString(val, "Option") != "Compact" {
		t.Errorf("expected option 'Compact', got %#v", val)
	}
}

func TestSetDesignProperty_PreservesCustomKind(t *testing.T) {
	w := makeStyleableWidget("ctn1")
	// Seed an existing custom (ToggleButtonGroup/ColorPicker) design property.
	app := dGetDoc(w, "Appearance")
	dSetArray(app, "DesignProperties", []any{
		bson.D{
			{Key: "$Type", Value: designPropertyEntryType},
			{Key: "Key", Value: "Accent"},
			{Key: "Value", Value: bson.D{
				{Key: "$Type", Value: customDesignPropertyType},
				{Key: "Value", Value: "blue"},
			}},
		},
	})
	rawData := makeRawPage(w)
	m := &mprPageMutator{rawData: rawData, widgetFinder: findBsonWidget}

	if err := m.SetDesignProperty("ctn1", "Accent", "option", "red"); err != nil {
		t.Fatalf("SetDesignProperty failed: %v", err)
	}

	val := dGetDoc(findEntry(designPropEntries(t, rawData, "ctn1"), "Accent"), "Value")
	if dGetString(val, "$Type") != customDesignPropertyType {
		t.Errorf("expected custom kind preserved, got %q", dGetString(val, "$Type"))
	}
	if dGetString(val, "Value") != "red" {
		t.Errorf("expected custom Value='red', got %q", dGetString(val, "Value"))
	}
}

func TestRemoveDesignProperty(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &mprPageMutator{rawData: rawData, widgetFinder: findBsonWidget}
	_ = m.SetDesignProperty("ctn1", "A", "toggle", "")
	_ = m.SetDesignProperty("ctn1", "B", "toggle", "")

	if err := m.RemoveDesignProperty("ctn1", "A"); err != nil {
		t.Fatalf("RemoveDesignProperty failed: %v", err)
	}

	entries := designPropEntries(t, rawData, "ctn1")
	if len(entries) != 1 || findEntry(entries, "A") != nil || findEntry(entries, "B") == nil {
		t.Fatalf("expected only 'B' to remain, got %d entries", len(entries))
	}
}

func TestClearDesignProperties(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &mprPageMutator{rawData: rawData, widgetFinder: findBsonWidget}
	_ = m.SetDesignProperty("ctn1", "A", "toggle", "")
	_ = m.SetDesignProperty("ctn1", "B", "option", "X")

	if err := m.ClearDesignProperties("ctn1"); err != nil {
		t.Fatalf("ClearDesignProperties failed: %v", err)
	}

	if entries := designPropEntries(t, rawData, "ctn1"); len(entries) != 0 {
		t.Fatalf("expected all design properties cleared, got %d", len(entries))
	}
	// Marker must be preserved so the array still serializes correctly.
	result := findBsonWidget(rawData, "ctn1")
	arr := toBsonA(dGet(dGetDoc(result.widget, "Appearance"), "DesignProperties"))
	if len(arr) != 1 {
		t.Fatalf("expected marker-only array, got %d elements", len(arr))
	}
	if _, ok := arr[0].(int32); !ok {
		t.Errorf("expected int32 marker preserved, got %T", arr[0])
	}
}

func TestSetDesignProperty_WidgetNotFound(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &mprPageMutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetDesignProperty("nope", "A", "toggle", ""); err == nil {
		t.Fatal("expected error for nonexistent widget")
	}
}

func TestSetDesignProperty_NoAppearance(t *testing.T) {
	w := bson.D{
		{Key: "$Type", Value: "Pages$DivContainer"},
		{Key: "Name", Value: "ctn1"},
	}
	rawData := makeRawPage(w)
	m := &mprPageMutator{rawData: rawData, widgetFinder: findBsonWidget}
	if err := m.SetDesignProperty("ctn1", "A", "toggle", ""); err == nil {
		t.Fatal("expected error when widget has no Appearance")
	}
}
