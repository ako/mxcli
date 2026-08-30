// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// MDL-WIDGET22: an IMAGE widget with nothing to show.
//
// The Image widget's `datasource` property defaults to `image` — an image from
// an image collection — and MDL has no way to say WHICH image: the widget's
// `imageObject` property is of type `image`, an operation the pluggable widget
// engine does not implement. So the default spelling
//
//	image i (Responsive: false)
//
// writes a model mxbuild refuses:
//
//	[error] "No image selected." at Image 'i'
//
// This was invisible until the CE0463 fix landed. Every mxcli-authored Image
// failed the build with CE0463 "the definition of this widget has changed" —
// which fired first, and which `mx update-widgets` cleared, leaving the real
// error behind it (mxcli-formula1 FINDINGS §69/§142). It is also why a
// describe → exec copy of an Atlas layout loses its brand image.
//
// Reported at check time because that is where it costs a second rather than a
// build. It is an ERROR: the build does not pass, so accepting it would be
// mxcli passing a script it knows produces a broken app.
const imageSourceRule = "MDL-WIDGET22"

// imageSourceNeedsImage lists the `datasource` values that require an image
// reference MDL cannot yet author. Anything else — including a value this build
// does not recognise — is left alone rather than guessed at.
var imageSourceNeedsImage = map[string]bool{
	"":      true, // absent: the widget's own default is "image"
	"image": true,
}

// validateImageSource reports an IMAGE widget whose configured source has no
// value to render.
func validateImageSource(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || !strings.EqualFold(w.Type, "image") {
		return nil
	}
	source := strings.TrimSpace(w.GetStringProp("ImageType"))

	switch {
	case imageSourceNeedsImage[strings.ToLower(source)]:
		return []linter.Violation{{
			RuleID:   imageSourceRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` (image) shows an image from an image collection "+
					"(the default `ImageType: image`) but names no image — mxbuild rejects the "+
					"build with \"No image selected.\"",
				locationPrefix, w.Name),
			Location: linter.Location{DocumentType: "page"},
			Suggestion: "MDL cannot yet point an image widget at an image collection entry " +
				"(the widget's `imageObject` property is of a type the widget engine does not " +
				"author). Use the URL form instead — `image " + w.Name +
				" (ImageType: imageUrl, ImageUrl: 'https://…')` — or `ImageType: icon`, or set " +
				"the image in Studio Pro.",
		}}
	case strings.EqualFold(source, "imageUrl") && strings.TrimSpace(w.GetStringProp("ImageUrl")) == "":
		return []linter.Violation{{
			RuleID:   imageSourceRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` (image) has `ImageType: imageUrl` and no `ImageUrl` — "+
					"mxbuild rejects the build with \"No image selected.\"",
				locationPrefix, w.Name),
			Location:   linter.Location{DocumentType: "page"},
			Suggestion: "Give it a URL: `ImageUrl: 'https://…'`.",
		}}
	}
	return nil
}
