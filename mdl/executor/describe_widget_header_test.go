// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// Item 6 of slice 2: DESCRIBE emits the widget's own MDL name now that the
// keyword form parses for every widget with a definition.
func TestPluggableWidgetHeader_UsesTheMDLName(t *testing.T) {
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Fatal("no registry — the embedded definitions must load, or this test proves nothing")
	}

	def, ok := registry.Get("COMBOBOX")
	if !ok || def == nil || def.WidgetID == "" {
		t.Fatal("COMBOBOX is not an embedded definition; pick another widget for this test")
	}

	got := pluggableWidgetHeader(registry, def.WidgetID, "cmb1")
	want := "combobox cmb1"
	if got != want {
		t.Errorf("header = %q, want %q", got, want)
	}
}

// The fallbacks. Each must produce the id form, which always round-trips.
func TestPluggableWidgetHeader_FallsBackToTheIDForm(t *testing.T) {
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Fatal("no registry")
	}

	cases := []struct {
		name     string
		registry *WidgetRegistry
		widgetID string
		why      string
	}{
		{"unknown id", registry, "com.acme.NotInstalled.Thing",
			"an id with no definition has no MDL name to emit; inventing one would not resolve on the way back in"},
		{"no registry", nil, "com.mendix.widget.web.combobox.Combobox",
			"without definitions there is nothing to resolve against"},
		{"empty id", registry, "",
			"nothing to look up"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pluggableWidgetHeader(tc.registry, tc.widgetID, "w1")
			if !strings.HasPrefix(got, "pluggablewidget '") {
				t.Errorf("header = %q, want the pluggablewidget id form — %s", got, tc.why)
			}
		})
	}
}

// The ambiguity guard. Two definitions sharing an MDL name make the name
// unusable: the builder resolves it with registry.Get, which can return only
// one of them, so emitting the name could silently retarget the widget onto a
// different one. An MDL name is the last segment of a widget id, so two vendors
// shipping `….Slider` is not hypothetical.
func TestPluggableWidgetHeader_AmbiguousNameFallsBack(t *testing.T) {
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Fatal("no registry")
	}
	def, ok := registry.Get("COMBOBOX")
	if !ok {
		t.Fatal("COMBOBOX missing")
	}

	// Control first: unique today, so the name IS emitted.
	if got := pluggableWidgetHeader(registry, def.WidgetID, "w1"); !strings.HasPrefix(got, "combobox ") {
		t.Fatalf("control: header = %q, want the name form before a collision is introduced", got)
	}

	// Introduce a second definition claiming the same MDL name. This is what a
	// colliding install looks like in the registry: byWidgetID keeps both,
	// byMDLName keeps only the last, so the two lookups disagree.
	other := &WidgetDefinition{WidgetID: "com.acme.other.Combobox", MDLName: "combobox"}
	registry.byWidgetID[other.WidgetID] = other
	registry.byMDLName["COMBOBOX"] = other
	if got := pluggableWidgetHeader(registry, def.WidgetID, "w1"); !strings.HasPrefix(got, "pluggablewidget '") {
		t.Errorf("header = %q, want the id form once two definitions claim the MDL name — "+
			"emitting the ambiguous name could rebuild the page onto the other widget", got)
	}
}

// unreconstructedContainers turns silent data loss into a visible gap.
//
// DESCRIBE cannot yet read a child slot back for an arbitrary pluggable widget,
// so describe -> exec deleted a widget's body and said nothing. Measured on a
// page mxcli authored itself: the stored BSON carried `tagContentContainer`
// with a DynamicText, and the describe output was a bare head.
func TestUnreconstructedContainers(t *testing.T) {
	// A minimal pluggable widget document: two properties, one child slot with
	// a widget in it and one empty. Arrays carry the leading typed-array marker,
	// which is why an EMPTY container is length 1 and not length 0 — getting
	// that wrong reports every widget as lossy.
	widget := map[string]any{
		"Type": map[string]any{
			"PropertyTypes": []any{
				int32(3),
				map[string]any{"$ID": "t1", "PropertyKey": "tagContentContainer"},
				map[string]any{"$ID": "t2", "PropertyKey": "emptySlot"},
			},
		},
		"Object": map[string]any{
			"Properties": []any{
				int32(3),
				map[string]any{
					"TypePointer": "t1",
					"Value": map[string]any{
						"Widgets": []any{int32(3), map[string]any{"$Type": "Forms$DynamicText"}},
					},
				},
				map[string]any{
					"TypePointer": "t2",
					"Value":       map[string]any{"Widgets": []any{int32(3)}},
				},
			},
		},
	}

	got := unreconstructedContainers(widget, nil)

	// The populated slot must be reported.
	var sawPopulated, sawEmpty bool
	for _, g := range got {
		if strings.Contains(g, "tagcontent") {
			sawPopulated = true
		}
		if strings.Contains(g, "empty") {
			sawEmpty = true
		}
	}
	if !sawPopulated {
		t.Errorf("a child slot holding a widget was not reported; got %v — "+
			"this is the case where describe -> exec destroys real work", got)
	}
	// The control: an EMPTY slot must not be reported, or the note fires on
	// every widget and stops being read.
	if sawEmpty {
		t.Errorf("an empty child slot was reported as lost; got %v — an array of length 1 "+
			"is the typed-array marker alone, i.e. no content", got)
	}
}

// Nothing to report on a document with no containers at all.
func TestUnreconstructedContainers_Empty(t *testing.T) {
	if got := unreconstructedContainers(map[string]any{}, nil); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}
