// SPDX-License-Identifier: Apache-2.0

package backend

import "sort"

// scrollRegionSlots is the fixed slot order of a Forms$ScrollContainer. The
// centre one is stored as CenterRegion while its four siblings are bare
// positions — a spelling that has caught every walk written against this type.
var scrollRegionSlots = []string{"Top", "Right", "Bottom", "Left", "CenterRegion"}

func skipInPlaceholderWalk(key string) bool {
	switch key {
	case "$Type", "$ID", "Name", "Top", "Right", "Bottom", "Left", "CenterRegion":
		return true
	}
	return false
}

// LayoutPlaceholderNames walks a layout's decoded BSON and returns the names of
// every Forms$Placeholder it declares, in document order.
//
// It is a free function on the raw document rather than a method on either
// backend because both engines read layouts the same way — the placeholder names
// are the layout's public API and neither gen nor the legacy parser exposes them
// as a list, so both implementations would otherwise be the same walk twice.
//
// Two shapes it has to know about, and one it must not assume:
//
//   - The tree hangs off Content (a Forms$WebLayoutContent or
//     Forms$NativeLayoutContent), never off the layout element. Reading a
//     "Widget" key returns nothing for every layout ever written.
//   - A ScrollContainer holds its children in five named slots — Top, Right,
//     Bottom, Left and CenterRegion, the last spelled unlike its siblings — not
//     in a list.
//
// Everything else is walked generically: a placeholder can sit inside a
// container, a layout grid, a group box or a tab page, and enumerating the
// containers that may hold one is a list that goes stale. The walk descends into
// every nested document and array instead, and reports every Forms$Placeholder
// it meets.
func LayoutPlaceholderNames(raw map[string]any) []string {
	content, ok := raw["Content"].(map[string]any)
	if !ok {
		return nil
	}
	var out []string
	collectPlaceholders(content, &out)
	return out
}

func collectPlaceholders(node any, out *[]string) {
	switch n := node.(type) {
	case map[string]any:
		if t, _ := n["$Type"].(string); t == "Forms$Placeholder" || t == "Pages$Placeholder" {
			if name, _ := n["Name"].(string); name != "" {
				*out = append(*out, name)
			}
			// A placeholder has no children worth descending into, but falling
			// through costs nothing and keeps the walk uniform.
		}
		// The decoder hands back a map, so sibling-key order is already lost and
		// iterating it directly would make the result vary between runs — the
		// kind of output that reads as a flaky diff. The five region slots are
		// walked in their fixed order first (which is the order a reader expects
		// to see a layout's placeholders reported), then the remaining keys
		// sorted. Within a region the widgets are an array, so document order is
		// preserved where it exists.
		for _, slot := range scrollRegionSlots {
			if child, ok := n[slot]; ok {
				collectPlaceholders(child, out)
			}
		}
		rest := make([]string, 0, len(n))
		for key := range n {
			if !skipInPlaceholderWalk(key) {
				rest = append(rest, key)
			}
		}
		sort.Strings(rest)
		for _, key := range rest {
			collectPlaceholders(n[key], out)
		}
	case []any:
		for _, item := range n {
			collectPlaceholders(item, out)
		}
	}
}
