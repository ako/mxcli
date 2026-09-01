// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// mxcli-formula1 FINDINGS §142: a describe → rename → exec copy of an Atlas
// layout comes out with no brand image.
//
// `describe` emitted the Image widget as the built-in shorthand with no image
// reference at all —
//
//	image staticImage1 (Responsive: false)
//
// — because MDL had no way to name an image collection entry, so there was
// nothing to emit. Wiring the `image` operation gives the writer that
// capability; without the READ half the round trip still loses it, and the copy
// still renders blank.
//
// The stored form is a plain string on the WidgetValue, measured on a Studio
// Pro-authored widget: `Image: "MyFirstModule.Images._1"`, three parts,
// Module.Collection.Image.

// imagePropWidget is one stored CustomWidget carrying an image-typed property.
// The TypePointer indirection is the whole reason a dedicated extractor exists:
// the property is found by resolving its pointer to a PropertyKey, not by
// position.
func imagePropWidget(propertyKey, stored string) map[string]any {
	return map[string]any{
		"Type": map[string]any{
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					map[string]any{
						"$ID":         "pt-1",
						"$Type":       "CustomWidgets$WidgetPropertyType",
						"PropertyKey": propertyKey,
					},
				},
			},
		},
		"Object": map[string]any{
			"Properties": []any{
				map[string]any{
					"$Type":       "CustomWidgets$WidgetProperty",
					"TypePointer": "pt-1",
					"Value": map[string]any{
						"$Type":          "CustomWidgets$WidgetValue",
						"PrimitiveValue": "",
						"Image":          stored,
					},
				},
			},
		},
	}
}

func TestExtractCustomWidgetPropertyImage_ReadsTheReference(t *testing.T) {
	ctx, _ := newMockCtx(t)
	got := extractCustomWidgetPropertyImage(ctx, imagePropWidget("imageObject", "MyFirstModule.Images._1"), "imageObject")
	if got != "MyFirstModule.Images._1" {
		t.Errorf("got %q, want the stored qualified name", got)
	}
}

// CONTROL 1: an unset image reads as empty, so DESCRIBE emits nothing rather
// than an `Image: ”` that would not re-execute.
func TestExtractCustomWidgetPropertyImage_UnsetIsEmpty(t *testing.T) {
	ctx, _ := newMockCtx(t)
	if got := extractCustomWidgetPropertyImage(ctx, imagePropWidget("imageObject", ""), "imageObject"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// CONTROL 2: it must resolve the property by KEY, not take the first Image it
// finds. The Image widget has two image-typed properties — `imageObject` and
// `defaultImageDynamic` — so a positional read would return the wrong one.
func TestExtractCustomWidgetPropertyImage_ResolvesByPropertyKey(t *testing.T) {
	ctx, _ := newMockCtx(t)
	w := imagePropWidget("defaultImageDynamic", "Mod.Coll.Fallback")
	if got := extractCustomWidgetPropertyImage(ctx, w, "imageObject"); got != "" {
		t.Errorf("got %q for imageObject, but only defaultImageDynamic is set", got)
	}
	if got := extractCustomWidgetPropertyImage(ctx, w, "defaultImageDynamic"); got != "Mod.Coll.Fallback" {
		t.Errorf("got %q, want the fallback image", got)
	}
}

// The end §142 is about: what DESCRIBE prints. An image widget with an entry
// must emit `Image:` so describe → exec keeps it.
func TestDescribeImageWidget_EmitsTheImageReference(t *testing.T) {
	w := rawWidget{Name: "imgLogo"}
	w.ImageObject = "MyFirstModule.Images._1"

	out := describeImageWidgetProps(w)
	if !strings.Contains(strings.Join(out, ", "), "Image: 'MyFirstModule.Images._1'") {
		t.Errorf("describe did not emit the image reference: %v", out)
	}
}

// CONTROL: a widget with no entry emits no `Image:` at all. An empty one would
// re-execute into a reference to nothing.
func TestDescribeImageWidget_OmitsAnAbsentReference(t *testing.T) {
	w := rawWidget{Name: "imgLogo"}

	for _, p := range describeImageWidgetProps(w) {
		if strings.HasPrefix(p, "Image:") {
			t.Errorf("emitted %q for a widget with no image", p)
		}
	}
}
