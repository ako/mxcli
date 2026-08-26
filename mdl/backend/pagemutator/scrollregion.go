// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ScrollRegionSlots is the fixed slot order of a Forms$ScrollContainer, by BSON
// key. The centre one is stored as CenterRegion while its four siblings are bare
// positions — a spelling that has caught every walk written against this type.
var ScrollRegionSlots = []string{"Top", "Right", "Bottom", "Left", "CenterRegion"}

// scrollRegionKey maps the slot name MDL uses onto the BSON key.
//
// MDL says `center` because that is what the position is called everywhere
// else — in the CREATE LAYOUT body, in DESCRIBE output, and in Studio Pro's own
// UI. Only the stored key is CenterRegion.
func scrollRegionKey(slot string) (string, bool) {
	switch strings.ToLower(slot) {
	case "top":
		return "Top", true
	case "right":
		return "Right", true
	case "bottom":
		return "Bottom", true
	case "left":
		return "Left", true
	case "center", "centre":
		return "CenterRegion", true
	}
	return "", false
}

// insertIntoScrollRegion handles `INSERT INTO <container>.<slot>`.
//
// It reports handled=false when the ref is not a scroll-container region, so the
// caller falls through to the DataGrid2 column path — the dotted widgetRef
// serves both, and which one it means is decided by what the named widget
// actually is rather than by a separate syntax.
//
// An unoccupied slot is refused rather than created: an empty slot has no stored
// region document, and a region carries size, size mode and a class that a bare
// INSERT would have to invent. `create or replace layout` states them.
func (m *Mutator) insertIntoScrollRegion(containerRef, slot string, position backend.InsertPosition, widgets []pages.Widget) (bool, error) {
	result := m.widgetFinder(m.rawData, containerRef)
	if result == nil {
		return false, nil
	}
	if t := bsonnav.DGetString(result.widget, "$Type"); t != "Forms$ScrollContainer" && t != "Pages$ScrollContainer" {
		return false, nil
	}

	key, ok := scrollRegionKey(slot)
	if !ok {
		return true, fmt.Errorf("scroll container %q has no region %q (want top, right, bottom, left or center)", containerRef, slot)
	}
	// BEFORE/AFTER position a widget among its siblings; a region is not a
	// sibling of anything, so only INTO means something here. Silently treating
	// them as INTO would put widgets somewhere the script did not ask for.
	if !strings.EqualFold(string(position), "into") {
		return true, fmt.Errorf("a scroll container region can only be an INSERT INTO target; "+
			"to place a widget relative to another, name that widget: `insert %s <widgetName> { … }`",
			strings.ToLower(string(position)))
	}

	// Resolved before serializing, so a mistyped slot reports the slot rather
	// than whatever the widget serializer happens to say about widgets that were
	// never going anywhere.
	region := bsonnav.DGetDoc(result.widget, key)
	if region == nil {
		return true, fmt.Errorf("scroll container %q has no %s region; "+
			"add one with `create or replace layout` — an empty slot has no stored document to insert into",
			containerRef, strings.ToLower(slot))
	}

	newBsonWidgets, err := m.serializeWidgets(widgets)
	if err != nil {
		return true, fmt.Errorf("serialize widgets: %w", err)
	}

	// Reuses the container path, which already knows that an empty container
	// omits Widgets entirely and has to create it with the typed-array marker —
	// writing the list bare makes the project unloadable.
	newRegion, err := appendChildrenToContainer(region, containerRef+"."+slot, newBsonWidgets)
	if err != nil {
		return true, err
	}

	// bson.D is a slice: appendChildrenToContainer may have reallocated it, so
	// the grown region has to be written back into the scroll container, and the
	// scroll container back into its own parent array. Skipping either step is a
	// silent no-op that reports success.
	if !bsonnav.DSet(result.widget, key, newRegion) {
		return true, fmt.Errorf("scroll container %q: could not write region %s back", containerRef, slot)
	}
	result.parentArr[result.index] = result.widget
	bsonnav.DSetArray(result.parentDoc, result.parentKey, result.parentArr)
	return true, nil
}
