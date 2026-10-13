// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// mendixlabs/mxcli#1464: `alter page … set (Image: 'M.Images.gear') on
// staticImage12` was refused with `property "Image" is not a property of this
// built-in widget` — the key fell through to the pluggable setter, although
// CREATE writes it and DESCRIBE prints it. A Forms$StaticImageViewer stores the
// image as a by-name reference: the qualified name, a plain string under
// `Image`; a Forms$ImageViewer (dynamic image) the same under `DefaultImage`.

func legacyImageWidget(typ, name, key, current string) bson.D {
	return bson.D{
		{Key: "$Type", Value: typ},
		{Key: "Name", Value: name},
		{Key: "Appearance", Value: bson.D{{Key: "$Type", Value: "Forms$Appearance"}}},
		{Key: key, Value: current},
	}
}

func TestSetImage_StaticImageViewer(t *testing.T) {
	w := legacyImageWidget("Forms$StaticImageViewer", "staticImage12", "Image", "MyModule.Images.cog")
	raw := makeRawPage(w)
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("staticImage12", "Image", "MyModule.Images.gear"); err != nil {
		t.Fatalf("set Image: %v", err)
	}
	got := findBsonWidget(raw, "staticImage12").widget
	if v, ok := bsonnav.DGet(got, "Image").(string); !ok || v != "MyModule.Images.gear" {
		t.Fatalf("Image = %#v, want the string \"MyModule.Images.gear\"", bsonnav.DGet(got, "Image"))
	}
	if bsonnav.DGet(got, "DefaultImage") != nil {
		t.Errorf("a static image gained a DefaultImage key")
	}
}

func TestSetDefaultImage_DynamicImageViewer(t *testing.T) {
	w := legacyImageWidget("Forms$ImageViewer", "dynImg", "DefaultImage", "")
	raw := makeRawPage(w)
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
	// Lower-case, as `set defaultimage` arrives: first-class keys match any case.
	if err := m.SetWidgetProperty("dynImg", "defaultimage", "MyModule.Images.gear"); err != nil {
		t.Fatalf("set DefaultImage: %v", err)
	}
	got := findBsonWidget(raw, "dynImg").widget
	if v, ok := bsonnav.DGet(got, "DefaultImage").(string); !ok || v != "MyModule.Images.gear" {
		t.Fatalf("DefaultImage = %#v, want the string \"MyModule.Images.gear\"", bsonnav.DGet(got, "DefaultImage"))
	}
	if bsonnav.DGet(got, "Image") != nil {
		t.Errorf("a dynamic image gained an Image key")
	}
}

// Clearing is "", the shape CREATE writes for an unset image — never null.
func TestSetImage_EmptyClears(t *testing.T) {
	w := legacyImageWidget("Forms$StaticImageViewer", "si", "Image", "MyModule.Images.cog")
	raw := makeRawPage(w)
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("si", "Image", ""); err != nil {
		t.Fatalf("set Image '': %v", err)
	}
	if v, ok := bsonnav.DGet(findBsonWidget(raw, "si").widget, "Image").(string); !ok || v != "" {
		t.Fatalf("Image = %#v, want \"\"", bsonnav.DGet(findBsonWidget(raw, "si").widget, "Image"))
	}
}

// The property belongs to its widget: DefaultImage on a static image (and
// Image on a dynamic one) is refused rather than written as a stray key.
func TestSetImage_WrongViewerIsRefused(t *testing.T) {
	w := legacyImageWidget("Forms$StaticImageViewer", "si", "Image", "MyModule.Images.cog")
	m := &Mutator{rawData: makeRawPage(w), widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("si", "DefaultImage", "MyModule.Images.gear"); err == nil {
		t.Fatal("set DefaultImage on a static image returned nil")
	}
	d := legacyImageWidget("Forms$ImageViewer", "di", "DefaultImage", "")
	m = &Mutator{rawData: makeRawPage(d), widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("di", "Image", "MyModule.Images.gear"); err == nil {
		t.Fatal("set Image on a dynamic image returned nil")
	}
	if bsonnav.DGet(findBsonWidget(m.rawData, "di").widget, "Image") != nil {
		t.Error("a refused set still wrote an Image key")
	}
}

// A non-string value is refused, not stringified into the reference.
func TestSetImage_NonStringIsRefused(t *testing.T) {
	w := legacyImageWidget("Forms$StaticImageViewer", "si", "Image", "MyModule.Images.cog")
	m := &Mutator{rawData: makeRawPage(w), widgetFinder: findBsonWidget}
	err := m.SetWidgetProperty("si", "Image", []string{"a", "b"})
	if err == nil || !strings.Contains(err.Error(), "Image") {
		t.Fatalf("err = %v, want a refusal naming Image", err)
	}
}

// Control: the pluggable Image widget keeps its own path — `set Image` resolves
// against its template keys and writes into Object, never a top-level key.
func TestSetImage_PluggableImageUnchanged(t *testing.T) {
	w := kindedPluggableWidget("image", "", "", false)
	if err := setRawWidgetPropertyMut(w, "Image", "MyModule.Images.gear"); err != nil {
		t.Fatalf("set Image on pluggable: %v", err)
	}
	if got := bsonnav.DGetString(kindedValue(t, w), "PrimitiveValue"); got != "MyModule.Images.gear" {
		t.Errorf("pluggable PrimitiveValue = %q", got)
	}
	if bsonnav.DGet(w, "Image") != nil {
		t.Error("pluggable widget gained a top-level Image key")
	}
}
