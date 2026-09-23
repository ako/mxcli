// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1057: a Selection helper with `renderStyle: 'custom'` has
// three mandatory child slots (customAllSelected, customSomeSelected,
// customNoneSelected) and Studio Pro fills each with a Forms$StaticImageViewer.
// DESCRIBE PAGE emitted
//
//	-- Forms$StaticImageViewer (staticImage1)  -- NOT re-executable: mxcli
//	cannot author this widget, so re-running this script would drop it
//
// so `describe page` -> `create or replace page` emptied all three slots, and
// the build came back with CE0642 ("a required property has no value"). The
// reporter's workaround was to put an empty `dynamictext` in each slot, which
// satisfies the structure and loses the icons.
//
// Two halves, and the second is the one that hides: the widget had no describer
// AND `staticimage` had no spelling for WHICH image it shows
// (staticImageToGen: "MDL cannot name an image"). Emitting the keyword without
// the reference would have turned a visible note into a silent drop — the same
// trap #512 recorded for `Action:` on this very widget, and #142 for the
// pluggable `image`.

// storedStaticImage is one Studio Pro-shaped Forms$StaticImageViewer.
func storedStaticImage(name, image string) map[string]any {
	w := map[string]any{
		"$Type":      "Forms$StaticImageViewer",
		"Name":       name,
		"Image":      image,
		"Responsive": true,
		"Width":      int32(100),
		"Height":     int32(100),
	}
	return w
}

// describeStoredWidget runs the real describe path over a stored widget map.
func describeStoredWidget(t *testing.T, w map[string]any) string {
	t.Helper()
	ctx, _ := newMockCtx(t)
	var buf bytes.Buffer
	ctx.Output = &buf
	for _, rw := range parseRawWidget(ctx, w) {
		outputWidgetMDLV3(ctx, rw, 0)
	}
	return buf.String()
}

// rebuildStaticImage replays one emitted widget line through the real parser and
// the real page builder, returning what exec would store.
func rebuildStaticImage(t *testing.T, widgetMDL string) *pages.StaticImage {
	t.Helper()
	src := "create page Mod.P (Title: 'T') {\n" + widgetMDL + "\n}"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("DESCRIBE emitted MDL that does not parse (%q): %v", widgetMDL, errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	widget, err := pb.buildWidgetV3(page.Widgets[0])
	if err != nil {
		t.Fatalf("building %q: %v", widgetMDL, err)
	}
	img, ok := widget.(*pages.StaticImage)
	if !ok {
		t.Fatalf("replay built %T, want *pages.StaticImage", widget)
	}
	return img
}

// TestDescribeStaticImage_RoundTripsTheImageReference is the reported symptom at
// the layer it lives in: the widget must describe as re-executable MDL, and
// replaying that MDL must rebuild the SAME image reference — not merely
// something that parses.
func TestDescribeStaticImage_RoundTripsTheImageReference(t *testing.T) {
	const image = "Atlas_UI_Resources.Atlas_Icons.checkbox_checked"

	got := describeStoredWidget(t, storedStaticImage("imgAllSelected", image))
	if strings.Contains(got, "NOT re-executable") {
		t.Fatalf("an authorable widget is still flagged as unauthorable:\n%s", got)
	}
	if !strings.Contains(got, "staticimage imgAllSelected") {
		t.Fatalf("describe did not emit the staticimage keyword:\n%s", got)
	}
	if !strings.Contains(got, "Image: '"+image+"'") {
		t.Fatalf("describe did not emit the image reference, so replay drops it:\n%s", got)
	}

	replayed := rebuildStaticImage(t, strings.TrimRight(got, "\n"))
	if replayed.ImageName != image {
		t.Errorf("replay rebuilt ImageName %q, want %q — the round trip loses the image",
			replayed.ImageName, image)
	}
}

// TestDescribeStaticImage_InCustomWidgetSlot is the reporter's actual shape: the
// image sits in a pluggable widget's child slot, which is where Studio Pro puts
// it and where an empty slot becomes CE0642.
func TestDescribeStaticImage_InCustomWidgetSlot(t *testing.T) {
	const image = "Atlas_UI_Resources.Atlas_Icons.checkbox_checked"
	selectionHelper := map[string]any{
		"$Type": "CustomWidgets$CustomWidget",
		"Name":  "selectionHelper1",
		"Type": map[string]any{
			"WidgetId": "com.mendix.widget.web.selectionhelper.SelectionHelper",
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					int32(3),
					map[string]any{"$ID": "t1", "PropertyKey": "customAllSelected"},
				},
			},
		},
		"Object": map[string]any{
			"Properties": []any{
				int32(3),
				map[string]any{
					"TypePointer": "t1",
					"Value": map[string]any{"Widgets": []any{
						int32(3), storedStaticImage("imgAllSelected", image),
					}},
				},
			},
		},
	}

	got := describeStoredWidget(t, selectionHelper)
	if strings.Contains(got, "NOT re-executable") {
		t.Fatalf("the slot's static image is still unauthorable — the reported symptom:\n%s", got)
	}
	if !strings.Contains(got, "Image: '"+image+"'") {
		t.Fatalf("the slot round-trips empty, which is CE0642 on rebuild:\n%s", got)
	}
}

// TestDescribeStaticImage_RoundTripsSizeAndResponsive covers what emitting the
// keyword would otherwise have cost. Before this change a stored static image
// described as a visible "NOT re-executable" note; after it, anything the
// emitter leaves out is normalised away on replay WITHOUT a note — trading a
// loud loss for a quiet one. The writer hardcoded both units to "Auto" and the
// builder hardcoded Responsive to true, so a 300px non-responsive image came
// back auto-sized and responsive.
func TestDescribeStaticImage_RoundTripsSizeAndResponsive(t *testing.T) {
	stored := storedStaticImage("imgFixed", "Mod.Images.logo")
	stored["WidthUnit"] = "Pixels"
	stored["HeightUnit"] = "Percentage"
	stored["Responsive"] = false

	got := describeStoredWidget(t, stored)
	for _, want := range []string{"WidthUnit: pixels", "HeightUnit: percentage", "Responsive: false"} {
		if !strings.Contains(got, want) {
			t.Fatalf("describe did not emit %q, so replay normalises it away:\n%s", want, got)
		}
	}

	replayed := rebuildStaticImage(t, strings.TrimRight(got, "\n"))
	if replayed.WidthUnit != "pixels" {
		t.Errorf("replay rebuilt WidthUnit %q, want %q", replayed.WidthUnit, "pixels")
	}
	if replayed.HeightUnit != "percentage" {
		t.Errorf("replay rebuilt HeightUnit %q, want %q", replayed.HeightUnit, "percentage")
	}
	if replayed.Responsive {
		t.Error("replay rebuilt Responsive true — a non-responsive image came back responsive")
	}
}

// TestDescribeStaticImage_DefaultsStaySilent is the CONTROL for the test above:
// Auto units and a responsive image are Studio Pro's defaults, and the writer
// re-derives them, so emitting them would put clauses in the author's script
// that they never wrote — the "invents" half of the describe failure class.
func TestDescribeStaticImage_DefaultsStaySilent(t *testing.T) {
	stored := storedStaticImage("imgAuto", "Mod.Images.logo")
	stored["WidthUnit"] = "Auto"
	stored["HeightUnit"] = "Auto"

	got := describeStoredWidget(t, stored)
	for _, unwanted := range []string{"WidthUnit", "HeightUnit", "Responsive"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("describe emitted a default %q the writer re-derives:\n%s", unwanted, got)
		}
	}

	// And the default survives replay, which is what makes the silence correct
	// rather than merely quiet.
	if replayed := rebuildStaticImage(t, strings.TrimRight(got, "\n")); !replayed.Responsive {
		t.Error("an image described with no Responsive clause replayed as non-responsive")
	}
}

// TestDescribeStaticImage_NoImageIsSilent is the CONTROL. An image widget with
// no reference stored must still describe as a `staticimage`, with no `Image:`
// clause and no note — otherwise the assertions above could be satisfied by
// emitting the clause unconditionally.
func TestDescribeStaticImage_NoImageIsSilent(t *testing.T) {
	got := describeStoredWidget(t, map[string]any{
		"$Type": "Forms$StaticImageViewer",
		"Name":  "imgEmpty",
		"Image": "",
	})
	if !strings.Contains(got, "staticimage imgEmpty") {
		t.Fatalf("describe did not emit the staticimage keyword:\n%s", got)
	}
	if strings.Contains(got, "Image:") {
		t.Errorf("an unset image was emitted as a reference:\n%s", got)
	}
	if strings.Contains(got, "NOT re-executable") {
		t.Errorf("an authorable widget is flagged as unauthorable:\n%s", got)
	}
}
