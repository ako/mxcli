// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

func TestNoteOnATallElementSitsAboveIt(t *testing.T) {
	// A note is placed above its element's TOP EDGE. Offset from the centre, a
	// note on a loop — a box several hundred pixels tall — landed inside the box,
	// on top of the body it describes.
	loopHeight := 400
	pos, size := defaultAnnotationGeometry(model.Point{X: 500, Y: 300}, 0, loopHeight)
	boxTop := 300 - loopHeight/2
	if pos.Y+size.Height/2 >= boxTop {
		t.Fatalf("note bottom at %d, box top at %d: the note is inside the box",
			pos.Y+size.Height/2, boxTop)
	}
	// An ordinary activity keeps the geometry it has always had.
	plain, _ := defaultAnnotationGeometry(model.Point{X: 500, Y: 300}, 0, ActivityHeight)
	if plain.Y != 200 {
		t.Fatalf("note on an activity moved to y=%d, want 200", plain.Y)
	}
}
