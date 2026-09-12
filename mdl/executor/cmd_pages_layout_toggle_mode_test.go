// SPDX-License-Identifier: Apache-2.0

// A scroll-container region's ToggleMode, end to end.
//
// The property decides whether the region collapses. mxcli did not read it, did
// not write it and did not carry it, so `describe layout` → `create layout` of
// any Atlas layout with a sidebar returned a region pinned open at its full
// width — 232px of a 414px phone, with the app in what was left. Nothing
// reported it: the copy execs clean, `mx check` is 0 errors, and the only
// visible difference is on a screen narrow enough to care.
//
// Measured on a real 11.14.0 project, sidebar region of each Atlas layout:
//
//	Atlas_Default   ShrinkContentInitiallyClosed      copy: (absent)
//	Atlas_SideBar   ShrinkContentInitiallyOpen        copy: (absent)
//	Atlas_TopBar    SlideOverContent                  copy: (absent)
//	Phone_Sidebar   PushContentAside                  copy: (absent)
//
// Four behaviours, one outcome. The tests below pin each half of the round trip
// separately, because the loss was invisible precisely where the two meet.

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// layoutWithRegionProps builds `region left (…)` with the given properties, in
// the lowercased shape the parser produces.
func layoutWithRegionProps(props map[string]any) *ast.CreateLayoutStmt {
	return layoutStmt(
		map[string]any{"layouttype": "Responsive"},
		&ast.WidgetV3{
			Type: "scrollcontainer", Name: "layoutContainer",
			Children: []*ast.WidgetV3{
				{Type: "region", Name: "left", Properties: props},
				{Type: "region", Name: "center", Children: []*ast.WidgetV3{{Type: "placeholder", Name: "Main"}}},
			},
		},
	)
}

func leftRegion(t *testing.T, stmt *ast.CreateLayoutStmt) *pages.ScrollContainerRegion {
	t.Helper()
	l, err := newPopupPageBuilder().buildLayoutV3(stmt)
	if err != nil {
		t.Fatalf("buildLayoutV3: %v", err)
	}
	sc, ok := l.Widgets[0].(*pages.ScrollContainer)
	if !ok {
		t.Fatalf("root widget = %T, want *pages.ScrollContainer", l.Widgets[0])
	}
	return sc.Regions[0]
}

func TestBuildLayoutV3_RegionCarriesToggleMode(t *testing.T) {
	r := leftRegion(t, layoutWithRegionProps(map[string]any{
		"size": 232, "sizemode": "Pixels", "class": "region-sidebar",
		"togglemode": "ShrinkContentInitiallyClosed",
	}))
	if r.ToggleMode != "ShrinkContentInitiallyClosed" {
		t.Errorf("ToggleMode = %q, want ShrinkContentInitiallyClosed", r.ToggleMode)
	}
	// The neighbours are the control: they were already carried, so a failure
	// here would mean the whole region broke rather than this property.
	if r.Size != 232 || r.SizeMode != "Pixels" || r.Class != "region-sidebar" {
		t.Errorf("region = %d/%q/%q, want 232/Pixels/region-sidebar", r.Size, r.SizeMode, r.Class)
	}
}

// Every member, because the four Atlas layouts use four different ones — a
// version of this that only accepted the Shrink pair would have looked right
// against Atlas_Default and dropped Atlas_TopBar's sidebar.
func TestBuildLayoutV3_EveryToggleModeMemberIsAccepted(t *testing.T) {
	for _, mode := range pages.ScrollContainerToggleModes {
		t.Run(mode, func(t *testing.T) {
			if got := leftRegion(t, layoutWithRegionProps(map[string]any{"togglemode": mode})).ToggleMode; got != mode {
				t.Errorf("ToggleMode = %q, want %q", got, mode)
			}
		})
	}
}

// MDL is case-insensitive about property values elsewhere, and the stored
// member is not — so the canonical spelling is what reaches the document.
func TestBuildLayoutV3_ToggleModeIsCanonicalised(t *testing.T) {
	if got := leftRegion(t, layoutWithRegionProps(map[string]any{"togglemode": "shrinkcontentinitiallyopen"})).ToggleMode; got != "ShrinkContentInitiallyOpen" {
		t.Errorf("ToggleMode = %q, want the canonical ShrinkContentInitiallyOpen", got)
	}
}

// An unrecognised member is dropped when Mendix loads the document, so passing
// it through would write a layout that execs clean, builds clean and renders
// with no toggle behaviour at all — the exact silence this whole change is
// about. Studio Pro's caption is the value a person is most likely to type.
func TestBuildLayoutV3_UnknownToggleModeIsRefused(t *testing.T) {
	_, err := newPopupPageBuilder().buildLayoutV3(layoutWithRegionProps(
		map[string]any{"togglemode": "Shrink content (initially closed)"}))
	if err == nil {
		t.Fatal("a Studio Pro caption was accepted; want a refusal")
	}
	for _, want := range []string{"ShrinkContentInitiallyClosed", "region", "left"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

// The control for the rest of the file: a region that says nothing about
// toggling must stay empty here, so that the writer's own default (None, which
// is what Studio Pro stores on an untouched region) is the single place the
// value is decided.
func TestBuildLayoutV3_NoToggleModeStaysEmpty(t *testing.T) {
	if got := leftRegion(t, layoutWithRegionProps(map[string]any{"class": "region-sidebar"})).ToggleMode; got != "" {
		t.Errorf("ToggleMode = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// The read half: describe
// ---------------------------------------------------------------------------

func TestParseRawWidget_RegionCarriesToggleMode(t *testing.T) {
	ctx, _ := newMockCtx(t)

	raw := map[string]any{
		"$Type": "Forms$ScrollContainer",
		"Name":  "layoutContainer",
		"Left": map[string]any{
			"$Type":      "Forms$ScrollContainerRegion",
			"Appearance": map[string]any{"Class": "region-sidebar"},
			"Size":       int32(232),
			"SizeMode":   "Pixels",
			"ToggleMode": "ShrinkContentInitiallyClosed",
		},
	}

	region := parseRawWidget(ctx, raw)[0].Children[0]
	if region.RegionToggleMode != "ShrinkContentInitiallyClosed" {
		t.Errorf("RegionToggleMode = %q, want ShrinkContentInitiallyClosed", region.RegionToggleMode)
	}
}

func TestOutputWidgetMDLV3_RegionEmitsToggleMode(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}

	outputWidgetMDLV3(ctx, rawWidget{
		Type: "Forms$ScrollContainerRegion", Name: "left",
		RegionSize: 232, RegionSizeMode: "Pixels", RegionToggleMode: "ShrinkContentInitiallyClosed",
		Class: "region-sidebar",
	}, 0)

	if out := buf.String(); !strings.Contains(out, "ToggleMode: 'ShrinkContentInitiallyClosed'") {
		t.Errorf("describe output does not carry the toggle mode:\n%s", out)
	}
}

// None is every region's untouched value, so emitting it would put a line in
// every describe of every layout — noise that makes the meaningful ones harder
// to see, and re-executing without it lands on the same value anyway.
func TestOutputWidgetMDLV3_RegionOmitsDefaultToggleMode(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}

	outputWidgetMDLV3(ctx, rawWidget{
		Type: "Forms$ScrollContainerRegion", Name: "center",
		RegionToggleMode: "None", Class: "region-content",
	}, 0)

	if out := buf.String(); strings.Contains(out, "ToggleMode") {
		t.Errorf("describe output mentions the default toggle mode:\n%s", out)
	}
}

// The round trip is the point, so it gets its own test: what describe emits for
// an Atlas sidebar has to build back into the same region. Anything less is the
// defect — the copy looked identical in MDL and rendered differently.
func TestLayoutRegion_DescribeOutputRebuildsTheSameToggleMode(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}
	outputWidgetMDLV3(ctx, rawWidget{
		Type: "Forms$ScrollContainerRegion", Name: "left",
		RegionSize: 232, RegionSizeMode: "Pixels", RegionToggleMode: "ShrinkContentInitiallyClosed",
		Class: "region-sidebar",
	}, 0)

	// What describe wrote, read back as the parser would hand it to the builder.
	props := map[string]any{}
	for _, pair := range [][2]string{
		{"size", "232"}, {"sizemode", "Pixels"},
		{"togglemode", "ShrinkContentInitiallyClosed"}, {"class", "region-sidebar"},
	} {
		if !strings.Contains(buf.String(), pair[1]) {
			t.Fatalf("describe output is missing %q:\n%s", pair[1], buf.String())
		}
		props[pair[0]] = pair[1]
	}
	props["size"] = 232

	r := leftRegion(t, layoutWithRegionProps(props))
	if r.ToggleMode != "ShrinkContentInitiallyClosed" || r.Size != 232 || r.SizeMode != "Pixels" {
		t.Errorf("rebuilt region = %q/%d/%q, want ShrinkContentInitiallyClosed/232/Pixels",
			r.ToggleMode, r.Size, r.SizeMode)
	}
}

// ---------------------------------------------------------------------------
// The toggle button, which mxbuild requires wherever a region can toggle
// ---------------------------------------------------------------------------

// CE0611 — "A sidebar toggle button is required if a region can toggle" — is
// what makes this widget part of the same change rather than a follow-up.
// Measured on mxbuild 11.12.0: a layout with ShrinkContentInitiallyOpen and no
// toggle button takes a clean project to 1 error. Without the widget, carrying
// the region property would have turned every copy of an Atlas sidebar layout
// into a build failure.
func TestBuildSidebarToggleV3_BuildsTheWidget(t *testing.T) {
	w := &ast.WidgetV3{
		Type: "sidebartoggle", Name: "sidebarToggle",
		Properties: map[string]any{
			"buttonstyle": "Primary",
			"icon":        "Atlas_Core.Atlas_Filled.navigation-menu",
			"class":       "toggle-btn",
		},
	}
	// Through buildWidgetV3, not the builder directly: the keyword routing and
	// the central appearance pass are both part of what is being asserted.
	built, err := newPopupPageBuilder().buildWidgetV3(w)
	if err != nil {
		t.Fatalf("buildWidgetV3: %v", err)
	}
	btn, ok := built.(*pages.SidebarToggleButton)
	if !ok {
		t.Fatalf("widget = %T, want *pages.SidebarToggleButton", built)
	}
	if btn.TypeName != "Forms$SidebarToggleButton" {
		t.Errorf("TypeName = %q", btn.TypeName)
	}
	if btn.ButtonStyle != pages.ButtonStylePrimary {
		t.Errorf("ButtonStyle = %q, want Primary", btn.ButtonStyle)
	}
	if btn.Icon == nil || btn.Icon.Image != "Atlas_Core.Atlas_Filled.navigation-menu" {
		t.Errorf("Icon = %+v", btn.Icon)
	}
	if btn.Class != "toggle-btn" {
		t.Errorf("Class = %q", btn.Class)
	}
}

// It toggles the layout's togglable region — there is only ever one, which is
// why Mendix gives it its own type instead of an action. Accepting an Action
// would write a button that ignores it.
func TestBuildSidebarToggleV3_RefusesAnAction(t *testing.T) {
	w := &ast.WidgetV3{
		Type: "sidebartoggle", Name: "sbToggle",
		Properties: map[string]any{"Action": &ast.ActionV3{Type: "microflow", Target: "M.Flow"}},
	}
	_, err := newPopupPageBuilder().buildSidebarToggleV3(w)
	if err == nil {
		t.Fatal("an Action on a sidebartoggle was accepted; want a refusal")
	}
	if !strings.Contains(err.Error(), "actionbutton") {
		t.Errorf("error %q does not name the widget to use instead", err.Error())
	}
}

// Before this, describe emitted the button as a comment ending "NOT
// re-executable" — honest, and it meant a copied Atlas layout could not have a
// collapsing sidebar at all.
func TestOutputWidgetMDLV3_SidebarToggleIsReExecutable(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}

	outputWidgetMDLV3(ctx, rawWidget{
		Type: "Forms$SidebarToggleButton", Name: "sidebarToggle3",
		ButtonStyle: "Primary", Icon: "Atlas_Core.Atlas_Filled.navigation-menu",
		Class: "toggle-btn",
	}, 0)

	out := buf.String()
	if strings.Contains(out, "NOT re-executable") {
		t.Fatalf("the button is still emitted as a dropped widget:\n%s", out)
	}
	for _, want := range []string{"sidebartoggle sidebarToggle3", "ButtonStyle: Primary", "Atlas_Core.Atlas_Filled.navigation-menu"} {
		if !strings.Contains(out, want) {
			t.Errorf("describe output missing %q:\n%s", want, out)
		}
	}
}
