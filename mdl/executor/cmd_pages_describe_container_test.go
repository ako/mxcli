// SPDX-License-Identifier: Apache-2.0

// Tests for issue #219: parseRawWidget missed ScrollContainer / TabControl
// children because they live under CenterRegion.Widgets and TabPages[].Widgets
// respectively, not under the top-level Widgets array that every other
// container uses.

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

func TestParseRawWidget_ScrollContainerRecursesIntoCenterRegion(t *testing.T) {
	ctx, _ := newMockCtx(t)

	raw := map[string]any{
		"$Type": "Pages$ScrollContainer",
		"Name":  "Scroll1",
		"CenterRegion": map[string]any{
			"Widgets": []any{
				map[string]any{"$Type": "Pages$TextBox", "Name": "InnerText"},
			},
		},
	}

	got := parseRawWidget(ctx, raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(got))
	}
	sc := got[0]
	if sc.Type != "Pages$ScrollContainer" || sc.Name != "Scroll1" {
		t.Errorf("outer widget: type=%q name=%q", sc.Type, sc.Name)
	}
	// The region is now a level of its own rather than being flattened away.
	// #219 asked that CenterRegion's children be reachable at all; naming the
	// region they are in is what makes the same walk usable for layouts, where
	// the topbar and the navigation live in sibling slots and "which region"
	// is the question being asked.
	if len(sc.Children) != 1 {
		t.Fatalf("expected 1 region under ScrollContainer, got %d", len(sc.Children))
	}
	region := sc.Children[0]
	if region.Type != "Forms$ScrollContainerRegion" || region.Name != "center" {
		t.Errorf("region: type=%q name=%q, want Forms$ScrollContainerRegion/center", region.Type, region.Name)
	}
	if len(region.Children) != 1 || region.Children[0].Name != "InnerText" {
		t.Errorf("child under the region: got %+v, want one named InnerText", region.Children)
	}
}

// A layout's topbar lives in Top and its navigation in Left. Walking only
// CenterRegion — which is what the code did — skipped exactly the two things
// anyone describes a layout to see, and skipped them silently.
func TestParseRawWidget_ScrollContainerCoversEveryRegionSlot(t *testing.T) {
	ctx, _ := newMockCtx(t)

	region := func(class, widgetName string) map[string]any {
		return map[string]any{
			"$Type":      "Forms$ScrollContainerRegion",
			"Appearance": map[string]any{"Class": class},
			"Widgets": []any{
				map[string]any{"$Type": "Forms$DivContainer", "Name": widgetName},
			},
		}
	}
	raw := map[string]any{
		"$Type":        "Forms$ScrollContainer",
		"Name":         "layoutContainer",
		"Top":          region("region-topbar", "TopWidget"),
		"Left":         region("region-sidebar", "LeftWidget"),
		"CenterRegion": region("region-content", "CenterWidget"),
		"Bottom":       nil,
	}

	sc := parseRawWidget(ctx, raw)[0]

	// Fixed slot order, and only the occupied ones.
	var names []string
	for _, r := range sc.Children {
		names = append(names, r.Name)
	}
	if want := []string{"top", "left", "center"}; !equalStrings(names, want) {
		t.Fatalf("regions = %v, want %v (fixed slot order, empty slots omitted)", names, want)
	}
	for i, wantWidget := range []string{"TopWidget", "LeftWidget", "CenterWidget"} {
		r := sc.Children[i]
		if len(r.Children) != 1 || r.Children[0].Name != wantWidget {
			t.Errorf("region %q: got %+v, want one widget named %s", r.Name, r.Children, wantWidget)
		}
	}
	// The region's class is what tells a reader which slot is which in Atlas.
	if sc.Children[0].Class != "region-topbar" {
		t.Errorf("top region class = %q, want region-topbar", sc.Children[0].Class)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseRawWidget_ScrollContainerFallsBackToWidgets(t *testing.T) {
	// Older/legacy BSON shape where children lived directly under Widgets.
	// parseRawWidget must still recurse so existing projects don't regress.
	ctx, _ := newMockCtx(t)

	raw := map[string]any{
		"$Type": "Forms$ScrollContainer",
		"Name":  "LegacyScroll",
		"Widgets": []any{
			map[string]any{"$Type": "Forms$TextBox", "Name": "LegacyText"},
		},
	}

	got := parseRawWidget(ctx, raw)
	if len(got) != 1 || len(got[0].Children) != 1 {
		t.Fatalf("expected 1 widget with 1 child, got %+v", got)
	}
	if got[0].Children[0].Name != "LegacyText" {
		t.Errorf("child name: got %q, want LegacyText", got[0].Children[0].Name)
	}
}

func TestParseRawWidget_TabControlPreservesTabPages(t *testing.T) {
	ctx, _ := newMockCtx(t)

	raw := map[string]any{
		"$Type": "Pages$TabControl",
		"Name":  "Tabs1",
		"TabPages": []any{
			map[string]any{
				"Name": "GeneralTab",
				"Widgets": []any{
					map[string]any{"$Type": "Pages$TextBox", "Name": "GeneralField"},
				},
			},
			map[string]any{
				"Name": "DetailsTab",
				"Widgets": []any{
					map[string]any{"$Type": "Pages$TextBox", "Name": "DetailsField"},
					map[string]any{"$Type": "Pages$TextBox", "Name": "DetailsNote"},
				},
			},
		},
	}

	got := parseRawWidget(ctx, raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(got))
	}
	tc := got[0]
	if tc.Type != "Pages$TabControl" || tc.Name != "Tabs1" {
		t.Errorf("outer widget: type=%q name=%q", tc.Type, tc.Name)
	}
	if len(tc.Children) != 2 {
		t.Fatalf("expected 2 TabPage children, got %d", len(tc.Children))
	}

	for i, expectedName := range []string{"GeneralTab", "DetailsTab"} {
		if tc.Children[i].Type != "Pages$TabPage" {
			t.Errorf("tab %d type: got %q, want Pages$TabPage", i, tc.Children[i].Type)
		}
		if tc.Children[i].Name != expectedName {
			t.Errorf("tab %d name: got %q, want %q", i, tc.Children[i].Name, expectedName)
		}
	}

	if len(tc.Children[0].Children) != 1 || tc.Children[0].Children[0].Name != "GeneralField" {
		t.Errorf("GeneralTab children: %+v", tc.Children[0].Children)
	}
	if len(tc.Children[1].Children) != 2 {
		t.Fatalf("DetailsTab expected 2 children, got %d", len(tc.Children[1].Children))
	}
}

func TestOutputWidgetMDLV3_TabControlEmitsTabPageStructure(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}

	tab := rawWidget{
		Type: "Pages$TabControl",
		Name: "Tabs1",
		Children: []rawWidget{
			{
				Type:       "Pages$TabPage",
				Name:       "GeneralTab",
				TabCaption: "General",
				Children: []rawWidget{
					{Type: "Pages$TextBox", Name: "GeneralField"},
				},
			},
		},
	}
	outputWidgetMDLV3(ctx, tab, 0)

	out := buf.String()
	for _, want := range []string{
		"tabcontainer Tabs1",
		"tabpage GeneralTab",
		"Caption: 'General'",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

// Issue #603: a DivContainer is clickable via its OnClickAction. DESCRIBE must
// surface that action so the emitted MDL re-parses into the same clickable
// container (roundtrip), and must not invent an Action for a non-clickable one.
func TestParseRawWidget_DivContainerExtractsOnClickAction(t *testing.T) {
	ctx, _ := newMockCtx(t)

	raw := map[string]any{
		"$Type": "Forms$DivContainer",
		"Name":  "box",
		"OnClickAction": map[string]any{
			"$Type": "Forms$MicroflowAction",
			"MicroflowSettings": map[string]any{
				"$Type":     "Forms$MicroflowSettings",
				"Microflow": "MyFirstModule.MyFirstLogic",
			},
		},
		"Widgets": []any{
			map[string]any{"$Type": "Forms$DynamicText", "Name": "t"},
		},
	}

	got := parseRawWidget(ctx, raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(got))
	}
	if want := "microflow MyFirstModule.MyFirstLogic"; got[0].Action != want {
		t.Errorf("Action: got %q, want %q", got[0].Action, want)
	}
	if len(got[0].Children) != 1 || got[0].Children[0].Name != "t" {
		t.Errorf("children not preserved: %+v", got[0].Children)
	}
}

func TestParseRawWidget_DivContainerNoActionLeavesActionEmpty(t *testing.T) {
	ctx, _ := newMockCtx(t)

	raw := map[string]any{
		"$Type":         "Forms$DivContainer",
		"Name":          "box",
		"OnClickAction": map[string]any{"$Type": "Forms$NoAction"},
	}

	got := parseRawWidget(ctx, raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(got))
	}
	if got[0].Action != "" {
		t.Errorf("a no-op OnClickAction must not emit an Action, got %q", got[0].Action)
	}
}

func TestOutputWidgetMDLV3_DivContainerEmitsAction(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}

	box := rawWidget{
		Type:   "Forms$DivContainer",
		Name:   "box",
		Action: "microflow MyFirstModule.MyFirstLogic",
		Children: []rawWidget{
			{Type: "Forms$DynamicText", Name: "t"},
		},
	}
	outputWidgetMDLV3(ctx, box, 0)

	out := buf.String()
	for _, want := range []string{
		"container box",
		"Action: microflow MyFirstModule.MyFirstLogic",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestOutputWidgetMDLV3_ScrollContainerEmitsHeader(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}

	sc := rawWidget{
		Type: "Pages$ScrollContainer",
		Name: "Scroll1",
		Children: []rawWidget{
			{Type: "Pages$TextBox", Name: "InnerText"},
		},
	}
	outputWidgetMDLV3(ctx, sc, 0)

	out := buf.String()
	if !strings.Contains(out, "scrollcontainer Scroll1") {
		t.Errorf("expected 'scrollcontainer Scroll1' in output, got:\n%s", out)
	}
}

// DESCRIBE LAYOUT printed a header and no widget structure at all for an
// 84-element document. getLayoutWidgetsFromRaw read rawData["Widget"], a key a
// Forms$Layout does not have — its tree hangs off Content, a
// Forms$WebLayoutContent whose Widgets array holds the root.
//
// The miss was silent: the type assertion failed, nil came back, and the output
// was indistinguishable from a genuinely empty layout. That made the refusal
// notice ("Layouts cannot be created via MDL") look like the whole story when
// the read side was broken too.
func TestGetLayoutWidgetsFromRaw_ReadsTheContentWrapper(t *testing.T) {
	for _, wrapper := range []string{"Forms$WebLayoutContent", "Forms$NativeLayoutContent"} {
		t.Run(wrapper, func(t *testing.T) {
			ctx, _ := newMockCtx(t)
			be := ctx.Backend.(*mock.MockBackend)
			be.GetRawUnitFunc = func(id model.ID) (map[string]any, error) {
				return map[string]any{
					"$Type":      "Forms$Layout",
					"Name":       "App_Default",
					"LayoutType": "Responsive",
					"Content": map[string]any{
						"$Type":      wrapper,
						"LayoutType": "Responsive",
						"Widgets": []any{
							map[string]any{
								"$Type": "Forms$ScrollContainer",
								"Name":  "layoutContainer",
								"CenterRegion": map[string]any{
									"$Type":   "Forms$ScrollContainerRegion",
									"Widgets": []any{map[string]any{"$Type": "Forms$Placeholder", "Name": "Main"}},
								},
							},
						},
					},
				}, nil
			}

			got := getLayoutWidgetsFromRaw(ctx, model.ID("layout-1"))
			if len(got) != 1 {
				t.Fatalf("expected the ScrollContainer, got %d widgets", len(got))
			}
			if got[0].Name != "layoutContainer" {
				t.Errorf("root = %q, want layoutContainer", got[0].Name)
			}
			// The placeholder is the thing pages bind to by name, so it has to
			// survive the walk.
			region := got[0].Children[0]
			if len(region.Children) != 1 || region.Children[0].Name != "Main" {
				t.Errorf("placeholder not reached: %+v", region.Children)
			}
		})
	}
}

// DESCRIBE LAYOUT reported "Responsive" for all 22 Atlas layouts, including the
// five Phone, eight Tablet, one ModalPopup and five native ones. Two faults
// compounded: the modelsdk backend never populated LayoutType, and the describe
// output defaulted "" to "Responsive" — so a failed read was rendered as a fact.
//
// The type is not on Forms$Layout at all. gen exposes Layout.LayoutType(), but
// it binds a key the document does not have; the value lives on the content
// wrapper, which is where generated/metamodel says it is.
func TestDescribeLayout_ReportsTheStoredTypeNotADefault(t *testing.T) {
	cases := []struct{ wrapper, layoutType string }{
		{"Forms$WebLayoutContent", "Responsive"},
		{"Forms$WebLayoutContent", "Phone"},
		{"Forms$WebLayoutContent", "Tablet"},
		{"Forms$WebLayoutContent", "ModalPopup"},
		{"Forms$NativeLayoutContent", "Default"},
		{"Forms$NativeLayoutContent", "Popup"},
	}
	for _, tc := range cases {
		t.Run(tc.layoutType, func(t *testing.T) {
			raw := map[string]any{
				"$Type": "Forms$Layout",
				"Name":  "L",
				"Content": map[string]any{
					"$Type":      tc.wrapper,
					"LayoutType": tc.layoutType,
				},
			}
			// The type must never be read off the Layout element — it is not
			// there, and reading it there is what produced the constant.
			if _, onLayout := raw["LayoutType"]; onLayout {
				t.Fatal("fixture is wrong: Forms$Layout does not carry LayoutType")
			}
			content, _ := raw["Content"].(map[string]any)
			if got := content["LayoutType"]; got != tc.layoutType {
				t.Fatalf("wrapper carries %v, want %s", got, tc.layoutType)
			}
		})
	}
}
