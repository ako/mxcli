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
// The Image widget's `datasource` property selects where the image comes from,
// and it defaults to `image` — an entry from an image collection, named on the
// widget's `imageObject` property. A widget with that source and no entry is a
// model mxbuild refuses:
//
//	[error] "No image selected." at Image 'i'
//
// This was invisible until the CE0463 fix landed. Every mxcli-authored Image
// failed the build with CE0463 "the definition of this widget has changed" —
// which fired first, and which `mx update-widgets` cleared, leaving the real
// error behind it (mxcli-formula1 FINDINGS §69/§142).
//
// When this rule was first written MDL could not name an image collection entry
// at all, so its only advice was to switch source. `Image:` is now authorable
// (see the `image` operation in widget_engine.go), so the rule reports a
// genuinely incomplete widget rather than a missing capability — and the remedy
// it offers is the one that keeps the author's intent.
//
// Reported at check time because that costs a second rather than a build. It is
// an ERROR: the build does not pass, so accepting it would be mxcli waving
// through a script it knows produces a broken app.
const imageSourceRule = "MDL-WIDGET22"

// imageSourceNeedsImage lists the `datasource` values that require an image
// collection entry. Anything else — including a value this build does not
// recognise — is left alone rather than guessed at.
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
		if strings.TrimSpace(w.GetStringProp("Image")) != "" {
			return nil // an entry is named — the widget is complete
		}
		return []linter.Violation{{
			RuleID:   imageSourceRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` (image) shows an image from an image collection "+
					"(the default `ImageType: image`) but names no image — mxbuild rejects the "+
					"build with \"No image selected.\"",
				locationPrefix, w.Name),
			Location: linter.Location{DocumentType: "page"},
			Suggestion: fmt.Sprintf(
				"Name the entry: `image %s (Image: 'Module.Collection.ImageName')` — "+
					"`show image collections` lists them, and `describe image collection "+
					"Module.Collection` lists the images inside one. Or switch source: "+
					"`ImageType: imageUrl, ImageUrl: 'https://…'`, or `ImageType: icon`.",
				w.Name),
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
