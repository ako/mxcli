// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"
)

// rawChildSlot is one child-slot property of a pluggable widget: a fixed
// property that holds widgets, as opposed to an object list that holds repeated
// items.
type rawChildSlot struct {
	// Keyword is the MDL container keyword, derived from the property key the
	// same way an object list's is (tagContentContainer -> tagcontentcontainer).
	Keyword string
	// PropertyKey is the stored key, kept so the emitted MDL can be matched back
	// to the document when debugging.
	PropertyKey string
	Widgets     []rawWidget
}

// extractChildSlots reconstructs every child slot of a pluggable widget.
//
// # Why
//
// DESCRIBE reconstructed a Gallery's `content` and `filtersPlaceholder` by
// asking for those property keys BY NAME (extractGalleryWidgetsByPropertyKey),
// and nothing at all for any other widget. So a child slot on an arbitrary
// pluggable widget was dropped from the describe output — silently, at exit 0.
//
// That was invisible while slices 2-3 were unwritten, because MDL could not
// express such a slot in the first place. Once it could, describe -> edit ->
// exec started DELETING a widget's body: measured on a page mxcli authored
// itself, `tagcontentcontainer body { dynamictext t }` was stored correctly and
// came back as a bare head.
//
// This generalises the Gallery reader in the way the rest of this work
// generalises everything else: a child slot is any property whose Value holds a
// `Widgets` array, read off the document rather than looked up in a table of
// known widgets. A widget nobody has thought about round-trips for free.
//
// # Empty slots are skipped
//
// getBsonArrayElements strips the leading typed-array marker, so an EMPTY slot
// is length 0 here and length 1 in the raw BSON. Emitting empty slots would put
// a `slot { }` block on nearly every pluggable widget — correct, and unreadable.
func extractChildSlots(ctx *ExecContext, w map[string]any, entityContext string) []rawChildSlot {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return nil
	}
	keyMap := buildPropertyTypeKeyMap(w, true)
	if len(keyMap) == 0 {
		return nil
	}

	var out []rawChildSlot
	for _, prop := range getBsonArrayElements(obj["Properties"]) {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		widgetsArr := getBsonArrayElements(value["Widgets"])
		if len(widgetsArr) == 0 {
			continue
		}
		key := keyMap[extractBinaryID(propMap["TypePointer"])]
		if key == "" {
			continue
		}

		var widgets []rawWidget
		for _, wgt := range widgetsArr {
			wgtMap, ok := wgt.(map[string]any)
			if !ok {
				continue
			}
			widgets = append(widgets, parseRawWidget(ctx, wgtMap, entityContext)...)
		}
		if len(widgets) == 0 {
			continue
		}

		kw := strings.ToLower(deriveObjectListKeyword(key))
		if kw == "" {
			kw = strings.ToLower(key)
		}
		out = append(out, rawChildSlot{Keyword: kw, PropertyKey: key, Widgets: widgets})
	}

	// Stable order: the BSON property order is not guaranteed to be meaningful,
	// and an unstable describe makes diffs unusable (the same reasoning as the
	// MDL-WIDGET07 ordering fix).
	sort.Slice(out, func(i, j int) bool { return out[i].Keyword < out[j].Keyword })
	return out
}

// outputChildSlots emits each reconstructed slot as an MDL container block.
//
// The slot NAME is synthesised. A child slot is a fixed property, not a named
// element — the document stores no name for it, and MDL requires one, so
// DESCRIBE WIDGET's usage example generates `slot1`, `slot2` for the same
// reason. Names are derived from the keyword so a re-describe is stable rather
// than renumbering on every run.
func outputChildSlots(ctx *ExecContext, slots []rawChildSlot, prefix string, indent int) {
	for _, s := range slots {
		fmt.Fprintf(ctx.Output, "%s%s %s {\n", prefix, s.Keyword, s.Keyword+"1")
		for _, child := range s.Widgets {
			outputWidgetMDLV3(ctx, child, indent+1)
		}
		fmt.Fprintf(ctx.Output, "%s}\n", prefix)
	}
}
