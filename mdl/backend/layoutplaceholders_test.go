// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"strings"
	"testing"
)

// atlasShapedLayout mirrors what a decoded Forms$Layout actually looks like:
// the tree hangs off Content, and the scroll container's children sit in five
// named slots rather than in a list.
func atlasShapedLayout() map[string]any {
	region := func(class string, widgets ...any) map[string]any {
		return map[string]any{
			"$Type":   "Forms$ScrollContainerRegion",
			"Class":   class,
			"Widgets": widgets,
		}
	}
	placeholder := func(name string) map[string]any {
		return map[string]any{"$Type": "Forms$Placeholder", "Name": name}
	}
	return map[string]any{
		"$Type": "Forms$Layout",
		"Name":  "App_Default",
		"Content": map[string]any{
			"$Type":      "Forms$WebLayoutContent",
			"LayoutType": "Responsive",
			"Widgets": []any{
				map[string]any{
					"$Type": "Forms$ScrollContainer",
					"Name":  "layoutContainer",
					"Top": region("region-topbar",
						// Nested one container deep, which is how Atlas_SideBar
						// carries its Topbar placeholder.
						map[string]any{
							"$Type":   "Forms$DivContainer",
							"Name":    "headerRow",
							"Widgets": []any{placeholder("HeaderLeft"), placeholder("HeaderRight")},
						},
					),
					"Left":         region("region-sidebar", map[string]any{"$Type": "Forms$NavigationTree", "Name": "navMenu"}),
					"CenterRegion": region("region-content", placeholder("Main")),
				},
			},
		},
	}
}

func TestLayoutPlaceholderNames_FindsThemInEverySlotAndNested(t *testing.T) {
	got := LayoutPlaceholderNames(atlasShapedLayout())
	want := "HeaderLeft,HeaderRight,Main"
	if strings.Join(got, ",") != want {
		t.Errorf("placeholders = %v, want %s", got, want)
	}
}

// Reading rawData["Widget"] is what made DESCRIBE LAYOUT print nothing for an
// 84-element document; the same mistake here would report every layout as
// having no placeholders, which silently disables the repoint check.
func TestLayoutPlaceholderNames_ReadsThroughContent(t *testing.T) {
	noContent := map[string]any{
		"$Type": "Forms$Layout",
		"Widget": map[string]any{
			"Widgets": []any{map[string]any{"$Type": "Forms$Placeholder", "Name": "Main"}},
		},
	}
	if got := LayoutPlaceholderNames(noContent); len(got) != 0 {
		t.Errorf("a layout with no Content must report nothing, got %v", got)
	}

	// Control: the same placeholder under Content is found, so the empty result
	// above is about the missing wrapper and not about the walk.
	if got := LayoutPlaceholderNames(atlasShapedLayout()); len(got) == 0 {
		t.Fatal("control failed: a real layout shape reported no placeholders")
	}
}

// The five slots are reported in their fixed order — Top, Right, Bottom, Left,
// CenterRegion — not alphabetically. That ordering is where the CenterRegion
// spelling is load-bearing: a table that said "Center" would leave the centre
// slot to the generic key walk, which sorts, so Main would come out first
// instead of last.
//
// (The generic walk is why a wrong spelling would not *lose* the placeholder —
// it descends into every remaining key. The order is what gives it away, and the
// order is what a user reads.)
func TestLayoutPlaceholderNames_ReportsSlotsInFixedOrder(t *testing.T) {
	region := func(name string) map[string]any {
		return map[string]any{
			"$Type":   "Forms$ScrollContainerRegion",
			"Widgets": []any{map[string]any{"$Type": "Forms$Placeholder", "Name": name}},
		}
	}
	l := map[string]any{
		"$Type": "Forms$Layout",
		"Content": map[string]any{
			"$Type": "Forms$WebLayoutContent",
			"Widgets": []any{map[string]any{
				"$Type":        "Forms$ScrollContainer",
				"Name":         "layoutContainer",
				"Top":          region("InTop"),
				"Right":        region("InRight"),
				"Bottom":       region("InBottom"),
				"Left":         region("InLeft"),
				"CenterRegion": region("Main"),
			}},
		},
	}
	got := strings.Join(LayoutPlaceholderNames(l), ",")
	want := "InTop,InRight,InBottom,InLeft,Main"
	if got != want {
		t.Errorf("placeholders = %s, want %s (slot order, not alphabetical)", got, want)
	}
}

func TestLayoutPlaceholderNames_IsDeterministic(t *testing.T) {
	first := strings.Join(LayoutPlaceholderNames(atlasShapedLayout()), ",")
	for i := 0; i < 20; i++ {
		if got := strings.Join(LayoutPlaceholderNames(atlasShapedLayout()), ","); got != first {
			t.Fatalf("run %d gave %q, first run gave %q — map iteration order is leaking into the output", i, got, first)
		}
	}
}
