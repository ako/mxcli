// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// `dynamicimage` is the sibling gap left open by mendixlabs/mxcli#1057, and it
// is worse than the static image's was — TWO defects, only one of which is a
// round trip.
//
// Measured on a blank Mendix 11.12.1 project, mxbuild 11.12.1, before this
// change. mxcli authors the widget, and the build refuses it:
//
//	[error] [CE0489] "Select an entity for the data source of this dynamic
//	                  image." at Dynamic image 'imgPhoto'
//
// because dynamicImageToGen called imageViewerSourceToGen() with no arguments
// and wrote a Forms$ImageViewerSource carrying nothing but its own `$ID` and
// `EntityRef: null`. So every dynamic image mxcli has ever written is broken at
// build time, not merely lossy.
//
// And, exactly as for the static image, DESCRIBE had no case for the type:
//
//	-- Forms$ImageViewer (imgPhoto)  -- NOT re-executable: mxcli cannot author
//	this widget, so re-running this script would drop it
//
// There is NO Studio Pro-authored dynamic image in the fixture (measured: 1
// Forms$ImageViewer in a blank 11.12.1 app, and it is mxcli's own), so the
// document shape here is metamodel-derived. What IS pinned to Studio Pro is the
// part that matters: `DomainModels$DirectEntityRef{Entity: "Module.Entity"}`,
// 20 of 20 instances in the same app, which is the element CE0489 is asking for.

// storedDynamicImage is one Forms$ImageViewer bound to an entity.
func storedDynamicImage(name, entity string) map[string]any {
	src := map[string]any{"$Type": "Forms$ImageViewerSource"}
	if entity != "" {
		src["EntityRef"] = map[string]any{
			"$Type":  "DomainModels$DirectEntityRef",
			"Entity": entity,
		}
	}
	return map[string]any{
		"$Type":      "Forms$ImageViewer",
		"Name":       name,
		"DataSource": src,
		"Responsive": true,
		"Width":      int32(200),
		"Height":     int32(200),
	}
}

// rebuildDynamicImage replays one emitted widget through the real parser and the
// real page builder, returning what exec would store.
func rebuildDynamicImage(t *testing.T, widgetMDL string) *pages.DynamicImage {
	t.Helper()
	src := "create page Mod.P (Title: 'T') {\n" + widgetMDL + "\n}"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("DESCRIBE emitted MDL that does not parse (%q): %v", widgetMDL, errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	// A real domain model, because buildDataSourceV3 RESOLVES the entity it is
	// given — the binding is checked, not copied through, which is the whole
	// point of writing an EntityRef rather than a name.
	pb := &pageBuilder{
		widgetScope: map[string]model.ID{},
		execCache: &executorCache{
			hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{
				model.ID("mod-my"): "MyFirstModule",
			}},
			domainModels: []*domainmodel.DomainModel{{
				ContainerID: model.ID("mod-my"),
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-photo")}, Name: "Photo"},
				},
			}},
		},
	}
	widget, err := pb.buildWidgetV3(page.Widgets[0])
	if err != nil {
		t.Fatalf("building %q: %v", widgetMDL, err)
	}
	img, ok := widget.(*pages.DynamicImage)
	if !ok {
		t.Fatalf("replay built %T, want *pages.DynamicImage", widget)
	}
	return img
}

// TestDescribeDynamicImage_RoundTripsTheDataSource is the CE0489 half and the
// describe half at once: the entity the widget is bound to has to survive
// describe -> exec, because it is the one property without which the build
// fails outright.
func TestDescribeDynamicImage_RoundTripsTheDataSource(t *testing.T) {
	const entity = "MyFirstModule.Photo"

	got := describeStoredWidget(t, storedDynamicImage("imgPhoto", entity))
	if strings.Contains(got, "NOT re-executable") {
		t.Fatalf("an authorable widget is still flagged as unauthorable:\n%s", got)
	}
	if !strings.Contains(got, "dynamicimage imgPhoto") {
		t.Fatalf("describe did not emit the dynamicimage keyword:\n%s", got)
	}
	if !strings.Contains(got, "database from "+entity) {
		t.Fatalf("describe did not emit the data source, so replay rebuilds a widget CE0489 refuses:\n%s", got)
	}

	replayed := rebuildDynamicImage(t, strings.TrimRight(got, "\n"))
	db, ok := replayed.DataSource.(*pages.DatabaseSource)
	if !ok {
		t.Fatalf("replay rebuilt DataSource %T, want *pages.DatabaseSource", replayed.DataSource)
	}
	if db.EntityName != entity {
		t.Errorf("replay rebuilt entity %q, want %q — the round trip loses the binding", db.EntityName, entity)
	}
}

// TestDescribeDynamicImage_RoundTripsDefaultImageAndDisplay covers the rest of
// what the widget stores and the writer hardcoded: the fallback image, the size
// units, Responsive, and the two display flags.
func TestDescribeDynamicImage_RoundTripsDefaultImageAndDisplay(t *testing.T) {
	const fallback = "MyFirstModule.Images.placeholder"
	stored := storedDynamicImage("imgPhoto", "MyFirstModule.Photo")
	stored["DefaultImage"] = fallback
	stored["WidthUnit"] = "Pixels"
	stored["HeightUnit"] = "Percentage"
	stored["Responsive"] = false
	stored["ShowAsThumbnail"] = true
	stored["OnClickEnlarge"] = true

	got := describeStoredWidget(t, stored)
	for _, want := range []string{
		"DefaultImage: '" + fallback + "'",
		"WidthUnit: pixels",
		"HeightUnit: percentage",
		"Responsive: false",
		"DisplayAs: thumbnail",
		"OnClickType: enlarge",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("describe did not emit %q, so replay normalises it away:\n%s", want, got)
		}
	}

	replayed := rebuildDynamicImage(t, strings.TrimRight(got, "\n"))
	if replayed.DefaultImageName != fallback {
		t.Errorf("replay rebuilt DefaultImageName %q, want %q", replayed.DefaultImageName, fallback)
	}
	if replayed.WidthUnit != "pixels" || replayed.HeightUnit != "percentage" {
		t.Errorf("replay rebuilt units %q/%q, want pixels/percentage", replayed.WidthUnit, replayed.HeightUnit)
	}
	if replayed.Responsive {
		t.Error("replay rebuilt Responsive true — a non-responsive image came back responsive")
	}
	if !replayed.ShowAsThumbnail {
		t.Error("replay rebuilt ShowAsThumbnail false — a thumbnail came back full size")
	}
	if !replayed.OnClickEnlarge {
		t.Error("replay rebuilt OnClickEnlarge false — the enlarge-on-click behaviour is gone")
	}
}

// TestDescribeDynamicImage_DefaultsStaySilent is the CONTROL for the test above.
// Auto units, a responsive image, a full-size image and no enlarge are Mendix's
// defaults that the writer re-derives, so emitting them would put clauses in the
// author's script they never wrote — the "invents" half of the describe failure
// class, where each round trip accumulates another default.
func TestDescribeDynamicImage_DefaultsStaySilent(t *testing.T) {
	stored := storedDynamicImage("imgAuto", "MyFirstModule.Photo")
	stored["WidthUnit"] = "Auto"
	stored["HeightUnit"] = "Auto"
	stored["ShowAsThumbnail"] = false
	stored["OnClickEnlarge"] = false

	got := describeStoredWidget(t, stored)
	for _, unwanted := range []string{"WidthUnit", "HeightUnit", "Responsive", "DisplayAs", "OnClickType", "DefaultImage"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("describe emitted a default %q the writer re-derives:\n%s", unwanted, got)
		}
	}
	if replayed := rebuildDynamicImage(t, strings.TrimRight(got, "\n")); !replayed.Responsive {
		t.Error("an image described with no Responsive clause replayed as non-responsive")
	}
}

// TestDescribeDynamicImage_NoSourceIsStillDescribable is the second CONTROL: a
// widget whose source has no entity — which is exactly what mxcli used to write
// — must still describe as a `dynamicimage`, with no DataSource clause invented
// for it. Emitting one would guess an entity, and a wrong guess is CE0489's
// sibling rather than a fix for it.
func TestDescribeDynamicImage_NoSourceIsStillDescribable(t *testing.T) {
	got := describeStoredWidget(t, storedDynamicImage("imgUnbound", ""))
	if !strings.Contains(got, "dynamicimage imgUnbound") {
		t.Fatalf("describe did not emit the dynamicimage keyword:\n%s", got)
	}
	if strings.Contains(got, "DataSource") || strings.Contains(got, "database from") {
		t.Errorf("an unbound source was emitted as a binding:\n%s", got)
	}
	if strings.Contains(got, "NOT re-executable") {
		t.Errorf("an authorable widget is flagged as unauthorable:\n%s", got)
	}
}
