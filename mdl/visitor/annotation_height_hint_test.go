// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"
)

// TestAnnotationHeightHint covers issue #1014. Mendix stores no annotation
// height — DomainModels$Annotation has exactly Caption, ExportLevel, Location
// and Width — so `Height: n` can never be accepted. The bare ANTLR error
// ("expecting {POSITION, CAPTION, WIDTH}") reads like a missing mxcli feature,
// which is what prompted the feature request; the hint says which it is and
// names the two levers that do exist.
func TestAnnotationHeightHint(t *testing.T) {
	script := `create or modify annotation in M (
  Caption: 'Section note',
  Position: (100, 100),
  Width: 420,
  Height: 200
);`
	_, errs := Build(script)
	if len(errs) == 0 {
		t.Fatal("expected a parse error for Height on an annotation")
	}
	joined := errsText(errs)
	for _, want := range []string{
		"does not store an annotation height",
		"auto-sizes",
		"Width: 300",
		"SET POSITION",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("hint does not mention %q; got:\n%s", want, joined)
		}
	}
}

// TestAnnotationUnknownPropertyHint: any other unsupported property gets the
// four-property statement without the height-specific advice.
func TestAnnotationUnknownPropertyHint(t *testing.T) {
	_, errs := Build("create annotation in M (\n  Caption: 'x',\n  Color: 'red'\n);")
	if len(errs) == 0 {
		t.Fatal("expected a parse error for Color on an annotation")
	}
	joined := errsText(errs)
	if !strings.Contains(joined, "stores only Caption, Position and Width") {
		t.Errorf("missing the four-property hint; got:\n%s", joined)
	}
	if strings.Contains(joined, "auto-sizes") {
		t.Errorf("height-specific advice leaked onto a non-height property; got:\n%s", joined)
	}
}

// TestWidgetHeightIsNotAnnotationHinted is the control that keeps the hint from
// keying on the word "Height". A page widget's Height is valid MDL, and the
// annotation expecting-set is what distinguishes the two.
func TestWidgetHeightIsNotAnnotationHinted(t *testing.T) {
	script := `create page M.P (Title: 'T', Layout: 'Atlas_Core.Atlas_Default') {
  IMAGE img (Image: 'M.Coll.Pic', Width: 100, Height: 200)
}`
	_, errs := Build(script)
	if len(errs) != 0 {
		t.Fatalf("a widget Height should parse cleanly; got: %s", errsText(errs))
	}
}
