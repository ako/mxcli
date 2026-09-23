// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#512.
//
// `clickCapableInMendix` in validate_widget_onclick.go named three widget types
// Mendix gives a click action and mxcli did not write:
//
//	Pages$ListView.ClickAction
//	Pages$StaticImageViewer.ClickAction
//	Pages$DynamicImageViewer.ClickAction
//
// The author got a warning and a workaround (wrap it in a `container`), which is
// honest but is modelling the app around a tool gap.
//
// It turned out the gap was one line per widget. The MODEL already carries the
// field (pages.ListView.ClickAction, pages.StaticImage/DynamicImage.OnClickAction)
// and the WRITER already serialises it — widget_write.go calls
// clientActionToGen(x.ClickAction) for the list view, and
// widget_write_legacy_gaps.go does the same for both images. Only the builders
// never read the action off the AST, so the field was always nil.
func TestBuildWidget_ClickActionIsCarried(t *testing.T) {
	// A close action: it needs no microflow resolution, so the test exercises the
	// builder wiring rather than the fixture's backend.
	action := &ast.ActionV3{Type: "close"}

	t.Run("listview", func(t *testing.T) {
		pb := newVehiclePB()
		lv := listViewWidget()
		lv.Properties["Action"] = action
		built, err := pb.buildListViewV3(lv)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if built.ClickAction == nil {
			t.Fatal("ClickAction is nil — the action was dropped, and the widget does nothing")
		}
		if _, ok := built.ClickAction.(*pages.ClosePageClientAction); !ok {
			t.Fatalf("ClickAction is %T, want *pages.ClosePageClientAction", built.ClickAction)
		}
	})

	t.Run("staticimage", func(t *testing.T) {
		pb := newVehiclePB()
		w := &ast.WidgetV3{Type: "staticimage", Name: "img",
			Properties: map[string]any{"Action": action}}
		built, err := pb.buildStaticImageV3(w)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if built.OnClickAction == nil {
			t.Fatal("OnClickAction is nil — the action was dropped")
		}
	})

	t.Run("dynamicimage", func(t *testing.T) {
		pb := newVehiclePB()
		w := &ast.WidgetV3{Type: "dynamicimage", Name: "img",
			Properties: map[string]any{"Action": action}}
		built, err := pb.buildDynamicImageV3(w)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if built.OnClickAction == nil {
			t.Fatal("OnClickAction is nil — the action was dropped")
		}
	})
}

// The control: a widget with no action must not grow one. A builder that always
// produced an action would make the test above pass and write a click handler
// onto every list view in every page.
func TestBuildWidget_NoActionStaysNil(t *testing.T) {
	pb := newVehiclePB()
	built, err := pb.buildListViewV3(listViewWidget())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if built.ClickAction != nil {
		t.Errorf("ClickAction = %#v on a list view with no action, want nil", built.ClickAction)
	}
}

// MDL-WIDGET23 must stop warning about what is now written, or the tool reports
// a gap it no longer has. The rule's own list is the thing under test: leaving a
// name in it is exactly how the warning outlives the gap.
func TestClickCapableInMendix_NoLongerNamesWhatIsWritten(t *testing.T) {
	for _, typ := range []string{"listview", "staticimage", "dynamicimage"} {
		if clickCapableInMendix[typ] {
			t.Errorf("%s is still listed as click-capable-but-unwritten, and its action IS now written", typ)
		}
	}
}

// The control for that: a type Mendix models NO click action on must keep
// warning. Emptying the map entirely would silence a real gap.
func TestClickDroppedNoSlot_StillWarns(t *testing.T) {
	if !clickDroppedNoSlot["dataview"] {
		t.Error("dataview no longer reports a dropped on-click; Mendix models none on it")
	}
	got := validateWidgetOnClick(&ast.WidgetV3{
		Type: "dataview", Name: "dv",
		Properties: map[string]any{"Action": &ast.ActionV3{Type: "close"}},
	}, "page X")
	if len(got) != 1 {
		t.Fatalf("got %d violations for an on-click on a dataview, want 1", len(got))
	}
}

// The read half. The write half landed on its own first and the action vanished
// on the next describe → exec: valid BSON, mxbuild at 0 errors, construct
// silently gone. That is the half-shell trap this repo keeps recording, and it
// is why a write fix is not done until DESCRIBE reproduces it.
//
// Measured end to end on a blank 11.12.2 project: `describe page` emits
// `Action: microflow MyFirstModule.ACT_Nothing`, that output re-checks clean,
// and re-executing it reports `Unchanged page` — so the emitted MDL rebuilds
// the stored document exactly.
func TestDescribeListView_EmitsClickAction(t *testing.T) {
	w := rawWidget{
		Type: "Forms$ListView", Name: "lvClick",
		Action: "microflow Pages.DoIt",
	}
	out := captureWidgetMDL(t, w)
	if !strings.Contains(out, "Action: microflow Pages.DoIt") {
		t.Errorf("describe dropped the list view's click action: %q", out)
	}
	// Editable is printed by the shared property formatter from the same field;
	// emitting it in the ListView branch too produced `Editable: true` twice in
	// one widget, which the parse side's own comment predicted. Caught by
	// round-tripping a DIFFERENT page than the one under test.
	if strings.Count(captureWidgetMDL(t, rawWidget{
		Type: "Forms$ListView", Name: "lv", Editable: "true",
	}), "Editable: true") != 1 {
		t.Error("Editable is emitted more than once for one list view")
	}
}

// captureWidgetMDL renders one widget through the real describe formatter.
func captureWidgetMDL(t *testing.T, w rawWidget) string {
	t.Helper()
	var buf bytes.Buffer
	outputWidgetMDLV3(&ExecContext{Output: &buf}, w, 0)
	return buf.String()
}
