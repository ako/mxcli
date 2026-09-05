// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// unreconstructedContainers names the container-shaped properties a pluggable
// widget carries in its stored BSON that DESCRIBE did not reproduce.
//
// # Why this exists
//
// Slices 2-3 (PROPOSAL_def_driven_widget_bodies.md) made a widget's object
// lists and child slots WRITABLE from MDL. DESCRIBE PAGE cannot yet read them
// back for an arbitrary pluggable widget: extractObjectLists reconstructs the
// chart-shaped lists it was built for and returns nothing for HTML Element's
// `attributes`, and nothing reconstructs a child slot at all.
//
// Measured on a page mxcli itself authored:
//
//	written   htmlelement frame { attribute a1 (…); tagcontentcontainer body { … } }
//	stored    BSON carries `attributes` with data-x AND `tagContentContainer`
//	          with its DynamicText — the write path is correct
//	described htmlelement frame (tagName: 'div', …)      <- body gone, silently
//
// So describe -> edit -> exec deleted a widget's body and said nothing. That is
// the #965 failure class (an annotation emptying the loop body it sits in), and
// it became reachable the moment the construct could be written.
//
// Reconstructing them is a separate piece of work. Until then the honest
// behaviour is the one slice 4's usage example already follows: say what was
// left out rather than pretend the output is complete. A visible gap is a
// nuisance; a silent one is data loss.
//
// # Detection is from the document, not from a list
//
// A container is a property whose Value holds an `Objects` array (object list)
// or a `Widgets` array (child slot). That is read off the stored BSON, so it
// covers widgets nobody has thought about — the same reason the rest of this
// work reads definitions rather than maintaining keyword tables.
//
// Anything already reconstructed is excluded, so a chart's series list — which
// DESCRIBE does emit — produces no note.
func unreconstructedContainers(w map[string]any, reconstructed []rawObjectList) []string {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return nil
	}
	keyMap := buildPropertyTypeKeyMap(w, true)
	if len(keyMap) == 0 {
		return nil
	}

	done := make(map[string]bool, len(reconstructed))
	for _, ol := range reconstructed {
		if ol.Keyword != "" {
			done[ol.Keyword] = true
		}
	}

	seen := make(map[string]bool)
	var out []string
	for _, prop := range getBsonArrayElements(obj["Properties"]) {
		propMap, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		value, ok := propMap["Value"].(map[string]any)
		if !ok {
			continue
		}
		// CHILD SLOTS ONLY, and only when they hold widgets.
		//
		// An object list is deliberately excluded even though it is equally
		// unreconstructed, because it cannot be reported without crying wolf: a
		// widget template ships DEFAULT entries in its lists, and they are
		// structurally identical to a user's. Measured on the probe page —
		// which wrote one `attribute` and no `event` at all — the stored
		// document carries one object in each, so warning on object lists named
		// `event` too. A note that fires on defaults is noise, and noise trains
		// people to ignore the notes that matter.
		//
		// A WIDGET inside a slot has no such ambiguity: a template never puts
		// one there, so its presence means someone did. That is the case where
		// a describe -> exec round trip destroys real work, and it is the case
		// worth interrupting for.
		//
		// (getBsonArrayElements strips the leading typed-array marker, so a
		// length of 0 here really is empty — the raw BSON array is length 1.)
		if len(getBsonArrayElements(value["Widgets"])) == 0 {
			continue
		}
		key := keyMap[extractBinaryID(propMap["TypePointer"])]
		if key == "" {
			continue
		}
		// Report the MDL keyword the author would write, not the raw property
		// key, so the note names something they can act on.
		kw := strings.ToLower(deriveObjectListKeyword(key))
		if kw == "" {
			kw = strings.ToLower(key)
		}
		if done[kw] || seen[kw] {
			continue
		}
		seen[kw] = true
		out = append(out, kw)
	}
	sort.Strings(out)
	return out
}

// writeOmittedContainerNote emits the gap as an MDL comment, so the output
// still parses and re-executes — it simply does not carry the body, and says
// so where the body would have been.
func writeOmittedContainerNote(out io.Writer, prefix string, omitted []string) {
	if len(omitted) == 0 {
		return
	}
	fmt.Fprintf(out, "%s-- NOT SHOWN: %s — this widget stores content DESCRIBE cannot yet\n",
		prefix, strings.Join(omitted, ", "))
	fmt.Fprintf(out, "%s-- reproduce. Re-running this script would DROP it. Inspect the widget with\n", prefix)
	fmt.Fprintf(out, "%s-- `mxcli widget describe <name>` and re-add the block by hand.\n", prefix)
}
