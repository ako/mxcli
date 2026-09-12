// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"
)

// `describe widget` is where an author is sent to find out what a widget's body
// takes — the MDL-WIDGET26/29 messages say so, and so do the quick reference and
// the syntax topics. It listed a widget's containers and, for an object list,
// that item's SCALAR sub-properties. It did not list the item's widgets-typed
// SLOTS.
//
// For a Data Grid 2 that omission had teeth. The only filter-shaped thing it
// named was `controlbar` — the grid-wide filter bar — while `column`'s own
// `filter` slot, the actual home of a column filter, went unmentioned. So the
// command answered "where does the filter go?" with the one place that renders
// "Unable to get filter store" at runtime (ako/view-entity-examples FINDINGS §2).
//
// These tests hold the answer in place.

func columnSlots(t *testing.T) map[string]DescribedItemSlot {
	t.Helper()
	var col *DescribedContainer
	for _, c := range describeContainers(datagridLikeDef()) {
		if c.Keyword == "column" {
			cc := c
			col = &cc
		}
	}
	if col == nil {
		t.Fatal("no `column` container described for a datagrid-shaped definition")
	}
	out := map[string]DescribedItemSlot{}
	for _, s := range col.ItemSlots {
		out[s.PropertyKey] = s
	}
	return out
}

func TestDescribeWidget_NamesTheSlotsInsideAnObjectListItem(t *testing.T) {
	slots := columnSlots(t)
	if _, ok := slots["filter"]; !ok {
		t.Fatalf("a column's `filter` slot is not described; got %v", slots)
	}
	if _, ok := slots["content"]; !ok {
		t.Errorf("a column's `content` slot is not described; got %v", slots)
	}
}

// The accepted types are the useful half — they are what tells the reader the
// filter is written directly in the column's braces rather than in a nested
// block. Compared against the engine's own table, so the description cannot
// drift from the routing it describes.
func TestDescribeWidget_ItemSlotAcceptedTypesComeFromTheEngineTable(t *testing.T) {
	want := itemSlotAcceptedChildTypes["com.mendix.widget.web.datagrid.Datagrid"]["columns"]["filter"]
	if len(want) == 0 {
		t.Fatal("the engine table no longer routes any widget type into a column's filter slot — " +
			"if that is deliberate, this test and the description have to follow it")
	}
	got := columnSlots(t)["filter"].Accepts
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("described accepts = %v, engine routes %v", got, want)
	}
}

// Exactly one slot per item is the default, and which one is decided by the
// engine (defaultItemSlotKey), not restated here.
func TestDescribeWidget_MarksTheDefaultItemSlot(t *testing.T) {
	slots := columnSlots(t)
	var defaults []string
	for key, s := range slots {
		if s.Default {
			defaults = append(defaults, key)
		}
	}
	if len(defaults) != 1 {
		t.Fatalf("want exactly one default slot, got %v", defaults)
	}
	if defaults[0] != "content" {
		t.Errorf("default slot = %s, want content", defaults[0])
	}
}

// Control: a widget whose containers are plain child slots must report no item
// slots at all — the new field cannot start inventing them.
func TestDescribeWidget_ChildSlotsHaveNoItemSlots(t *testing.T) {
	for _, c := range describeContainers(datagridLikeDef()) {
		if c.Kind == "child slot" && len(c.ItemSlots) > 0 {
			t.Errorf("child slot %s reported item slots %v", c.Keyword, c.ItemSlots)
		}
	}
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	def, ok := reg.Get("GALLERY")
	if !ok {
		t.Skip("gallery definition not embedded in this build")
	}
	for _, c := range describeContainers(def) {
		if len(c.ItemSlots) > 0 {
			t.Errorf("gallery container %s reported item slots %v — it has no object lists",
				c.Keyword, c.ItemSlots)
		}
	}
}

func TestPrintWidgetDescription_SaysHowToReachAnItemSlot(t *testing.T) {
	var buf bytes.Buffer
	PrintWidgetDescription(&buf, WidgetDescription{
		WidgetID:   "com.mendix.widget.web.datagrid.Datagrid",
		MDLName:    "DATAGRID",
		Kind:       "pluggable",
		Containers: describeContainers(datagridLikeDef()),
	})
	out := buf.String()
	if !strings.Contains(out, "slot filter -> filter:") {
		t.Errorf("rendered output does not name the column's filter slot:\n%s", out)
	}
	if !strings.Contains(out, "textfilter") {
		t.Errorf("rendered output does not say which widgets route into it:\n%s", out)
	}
	// The default slot reads as a sentence, not as an empty accepts list.
	if !strings.Contains(out, "any other widget in the item body") {
		t.Errorf("rendered output does not explain the default slot:\n%s", out)
	}
}
