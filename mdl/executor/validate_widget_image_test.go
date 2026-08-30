// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Found while fixing the Image CE0463 (mxcli-formula1 FINDINGS §69/§142): with
// CE0463 out of the way, the bare form's real problem is visible.
//
//	image i (Responsive: false)
//	  → [error] "No image selected." at Image 'i'
//
// The Image widget's `datasource` defaults to `image`, which shows an image from
// an image collection — and MDL has no way to say WHICH image (the widget's
// `imageObject` property is of type `image`, an operation the widget engine does
// not implement). So the default spelling of `image` always writes a model
// mxbuild rejects.
//
// CE0463 was masking that, which is why it went unnoticed: the definition error
// fired first and `mx update-widgets` "fixed" the page by clearing it, leaving
// the real error behind. The same gap is what makes a describe → exec copy of an
// Atlas layout lose its brand image (§142).
//
// Reporting it at check time is the whole value: mxbuild reports it at the far
// end of a build, and `mxcli check` runs in a second.

func imageWidget(props map[string]any) *ast.WidgetV3 {
	return &ast.WidgetV3{Name: "img", Type: "image", Properties: props}
}

func imageViolations(vs []linter.Violation) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == "MDL-WIDGET22" {
			out = append(out, v)
		}
	}
	return out
}

// The reported case: the default data source with no image to show.
func TestImageWidget_DefaultSourceWithNoImageIsReported(t *testing.T) {
	got := imageViolations(validateImageSource(imageWidget(map[string]any{"Responsive": "false"}), "page X"))
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	if got[0].Severity != linter.SeverityError {
		t.Errorf("severity = %v, want error — mxbuild refuses the build", got[0].Severity)
	}
	if !strings.Contains(got[0].Message, "No image selected") {
		t.Errorf("the message should quote what mxbuild says: %s", got[0].Message)
	}
	// It must offer the spelling that does work, not just refuse.
	if !strings.Contains(got[0].Suggestion, "imageUrl") {
		t.Errorf("the suggestion should name the working form: %s", got[0].Suggestion)
	}
}

// …and with an explicit ImageType: image, which is the same model.
func TestImageWidget_ExplicitImageTypeWithNoImageIsReported(t *testing.T) {
	w := imageWidget(map[string]any{"ImageType": "image"})
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
}

// CONTROL 1: the URL form is complete and must pass. This is the shape the
// CE0463 fix was verified on, at 0 errors.
func TestImageWidget_UrlFormIsClean(t *testing.T) {
	w := imageWidget(map[string]any{"ImageType": "imageUrl", "ImageUrl": "https://example.com/x.png"})
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 0 {
		t.Errorf("the URL form builds at 0 errors and must not be reported: %+v", got)
	}
}

// CONTROL 2: an icon source needs no image either.
func TestImageWidget_IconFormIsClean(t *testing.T) {
	w := imageWidget(map[string]any{"ImageType": "icon"})
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 0 {
		t.Errorf("an icon source needs no image: %+v", got)
	}
}

// CONTROL 3: the rule is about the IMAGE widget only. A different widget with a
// stray ImageType property is none of its business.
func TestImageWidget_OtherWidgetsAreNotTouched(t *testing.T) {
	w := &ast.WidgetV3{Name: "dg", Type: "datagrid", Properties: map[string]any{}}
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 0 {
		t.Errorf("a datagrid was reported: %+v", got)
	}
}

// CONTROL 4: an ImageType nobody recognises is left alone rather than guessed
// at. Reporting on an unknown source would fire on a widget version this build
// does not know about.
func TestImageWidget_UnknownImageTypeIsLeftAlone(t *testing.T) {
	w := imageWidget(map[string]any{"ImageType": "somethingNew"})
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 0 {
		t.Errorf("an unrecognised ImageType was reported: %+v", got)
	}
}

// The URL form with an EMPTY url is the same incompleteness by the other route,
// and mxbuild rejects it too.
func TestImageWidget_EmptyUrlIsReported(t *testing.T) {
	w := imageWidget(map[string]any{"ImageType": "imageUrl", "ImageUrl": ""})
	if got := imageViolations(validateImageSource(w, "page X")); len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
}
