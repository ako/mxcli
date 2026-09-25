// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// upstream #978, the third symptom. The row/column half was fixed by
// widgetKindsWithoutStoredNames; the report also listed
//
//	- duplicate widget name 'template1' (used 4 times) — Mendix requires unique widget names per page (CE0495)
//
// DESCRIBE writes every gallery's content as `template template1 { … }` and its
// filters as `filter filter1 { … }`, so a page with more than one gallery
// described into MDL that mxcli's own check rejected.
//
// Those blocks are CHILD SLOTS of the gallery's definition (gallery.def.json:
// TEMPLATE → `content`, FILTER → `filtersPlaceholder`). applyChildSlots builds
// only the block's children into the slot property; the block's own name is
// discarded, so the model holds nothing that could collide under CE0495. That is
// also why DESCRIBE has to synthesise `template1` — there is no stored name to
// read back.

func galleryRegistry(t *testing.T) *WidgetRegistry {
	t.Helper()
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("load embedded widget registry: %v", err)
	}
	if _, ok := reg.Get("GALLERY"); !ok {
		t.Fatal("embedded registry has no gallery definition")
	}
	return reg
}

// describedGallery is the shape DESCRIBE emits for a gallery with filters and
// content (cmd_pages_describe_output.go, the `widgetType == "gallery"` branch).
func describedGallery(name, text string) *ast.WidgetV3 {
	return &ast.WidgetV3{Type: "gallery", Name: name, Children: []*ast.WidgetV3{
		{Type: "filter", Name: "filter1", Children: []*ast.WidgetV3{
			{Type: "textfilter", Name: "tf_" + name},
		}},
		{Type: "template", Name: "template1", Children: []*ast.WidgetV3{
			{Type: "dynamictext", Name: text},
		}},
	}}
}

// The reported case: four galleries, as DESCRIBE renders them.
func TestCheckDuplicateWidgetNames_GallerySlotContainersAreNotWidgets(t *testing.T) {
	page := []*ast.WidgetV3{
		describedGallery("g1", "t1"),
		describedGallery("g2", "t2"),
		describedGallery("g3", "t3"),
		describedGallery("g4", "t4"),
	}
	if errs := checkDuplicateWidgetNames(page, galleryRegistry(t)); len(errs) != 0 {
		t.Errorf("a child-slot container's name is not stored, so it cannot be a CE0495 duplicate; got:\n  %s",
			strings.Join(errs, "\n  "))
	}
}

// The control: the widgets INSIDE a slot are real, stored widgets. Two of them
// sharing a name across galleries is CE0495 and must still be reported — a fix
// that skipped the slot's whole subtree would pass the test above.
func TestCheckDuplicateWidgetNames_WidgetsInsideSlotContainersStillCount(t *testing.T) {
	page := []*ast.WidgetV3{
		describedGallery("g1", "dup"),
		describedGallery("g2", "dup"),
	}
	errs := checkDuplicateWidgetNames(page, galleryRegistry(t))
	if len(errs) != 1 || !strings.Contains(errs[0], "'dup'") {
		t.Errorf("a duplicate widget inside a gallery template was not reported: %v", errs)
	}
}

// The second control: `template` outside a widget that declares it as a slot is
// buildTemplateV3 — a Forms$DivContainer that DOES store its name. Two of those
// sharing a name are a real duplicate.
func TestCheckDuplicateWidgetNames_StandaloneTemplateStillCounts(t *testing.T) {
	page := []*ast.WidgetV3{
		{Type: "template", Name: "template1"},
		{Type: "container", Name: "c", Children: []*ast.WidgetV3{
			{Type: "template", Name: "template1"},
		}},
	}
	errs := checkDuplicateWidgetNames(page, galleryRegistry(t))
	if len(errs) != 1 || !strings.Contains(errs[0], "'template1'") {
		t.Errorf("a standalone template stores its name; a duplicate must be reported: %v", errs)
	}
}
