// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#573: `describe layout Atlas_Core.Phone_BottomBar` said
//
//	region bottom (Class: 'region-bottombar') {
//	  -- Forms$SimpleMenuBar (simpleMenuBar1)  -- NOT re-executable: mxcli cannot
//	  --   author this widget, so re-running this script would drop it
//	}
//
// so a phone layout could not carry a bottom bar from MDL at all, and a copy of
// Atlas's lost it. The widget renders a MENU DOCUMENT, and which one is the whole
// point of it — emitting the keyword without the reference would turn a visible
// note into a silent drop.

// storedSimpleMenuBar is the widget exactly as a blank 11.14.0 project stores it
// in Atlas_Core.Phone_BottomBar.
func storedSimpleMenuBar(orientation string, source map[string]any) map[string]any {
	return map[string]any{
		"$Type": "Forms$SimpleMenuBar",
		"Name":  "simpleMenuBar1",
		"Appearance": map[string]any{
			"$Type":          "Forms$Appearance",
			"Class":          "bottom-nav-text-icons",
			"Style":          "",
			"DynamicClasses": "",
		},
		"MenuSource":  source,
		"Orientation": orientation,
		"TabIndex":    int32(0),
	}
}

func rebuildMenuWidget(t *testing.T, widgetMDL string) pages.Widget {
	t.Helper()
	src := "create page Mod.P (Title: 'T') {\n" + widgetMDL + "\n}"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("DESCRIBE emitted MDL that does not parse (%q): %v", widgetMDL, errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	if len(page.Widgets) == 0 {
		t.Fatalf("DESCRIBE emitted no widget, only:\n%s", widgetMDL)
	}
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	widget, err := pb.buildWidgetV3(page.Widgets[0])
	if err != nil {
		t.Fatalf("building %q: %v", widgetMDL, err)
	}
	return widget
}

func TestDescribeSimpleMenuBar_RoundTripsTheMenuDocument(t *testing.T) {
	got := describeStoredWidget(t, storedSimpleMenuBar("Horizontal", map[string]any{
		"$Type": "Forms$MenuDocumentSource",
		"Menu":  "Atlas_Core.Phone_Menu",
	}))
	if strings.Contains(got, "NOT re-executable") {
		t.Fatalf("simple menu bar still described as unauthorable:\n%s", got)
	}
	for _, want := range []string{"simplemenubar simpleMenuBar1", "Menu: Atlas_Core.Phone_Menu", "Class: 'bottom-nav-text-icons'"} {
		if !strings.Contains(got, want) {
			t.Errorf("description lacks %q:\n%s", want, got)
		}
	}

	bar, ok := rebuildMenuWidget(t, got).(*pages.SimpleMenuBar)
	if !ok {
		t.Fatalf("replay did not build a simple menu bar from:\n%s", got)
	}
	if bar.Menu != "Atlas_Core.Phone_Menu" {
		t.Errorf("replayed Menu = %q, want Atlas_Core.Phone_Menu", bar.Menu)
	}
	if bar.NavigationProfile != "" {
		t.Errorf("replayed NavigationProfile = %q, want none — the source is a menu document", bar.NavigationProfile)
	}
	if bar.Orientation != pages.MenuOrientationHorizontal {
		t.Errorf("replayed Orientation = %q, want Horizontal", bar.Orientation)
	}
}

// Vertical is the non-default, so it is the one a describer that forgets the
// property loses.
func TestDescribeSimpleMenuBar_RoundTripsVerticalAndAProfile(t *testing.T) {
	got := describeStoredWidget(t, storedSimpleMenuBar("Vertical", map[string]any{
		"$Type":             "Forms$NavigationSource",
		"NavigationProfile": "Phone",
	}))
	bar, ok := rebuildMenuWidget(t, got).(*pages.SimpleMenuBar)
	if !ok {
		t.Fatalf("replay did not build a simple menu bar from:\n%s", got)
	}
	if bar.Orientation != pages.MenuOrientationVertical {
		t.Errorf("replayed Orientation = %q, want Vertical; described as:\n%s", bar.Orientation, got)
	}
	if bar.NavigationProfile != "Phone" || bar.Menu != "" {
		t.Errorf("replayed source = profile %q / menu %q, want profile Phone; described as:\n%s", bar.NavigationProfile, bar.Menu, got)
	}
}

// A menu bar or navigation tree pointed at a menu document described with its
// profile slot empty — `menubar m` — and replayed onto the Responsive profile.
func TestDescribeMenuBar_KeepsAMenuDocumentSource(t *testing.T) {
	got := describeStoredWidget(t, map[string]any{
		"$Type":      "Forms$MenuBar",
		"Name":       "topMenu",
		"MenuSource": map[string]any{"$Type": "Forms$MenuDocumentSource", "Menu": "Mod.TopMenu"},
	})
	bar, ok := rebuildMenuWidget(t, got).(*pages.MenuBar)
	if !ok {
		t.Fatalf("replay did not build a menu bar from:\n%s", got)
	}
	if bar.Menu != "Mod.TopMenu" || bar.NavigationProfile != "" {
		t.Errorf("replayed source = menu %q / profile %q, want menu Mod.TopMenu; described as:\n%s", bar.Menu, bar.NavigationProfile, got)
	}
}

func TestSimpleMenuBar_RejectsBothSourcesAndABadOrientation(t *testing.T) {
	for _, src := range []string{
		"simplemenubar b (Menu: Mod.M, Profile: 'Phone')",
		"simplemenubar b (Menu: Mod.M, Orientation: Diagonal)",
	} {
		prog, errs := visitor.Build("create page Mod.P (Title: 'T') {\n" + src + "\n}")
		if len(errs) > 0 {
			t.Fatalf("%q does not parse: %v", src, errs)
		}
		pb := &pageBuilder{widgetScope: map[string]model.ID{}}
		if _, err := pb.buildWidgetV3(prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]); err == nil {
			t.Errorf("%q was accepted", src)
		}
	}
}
