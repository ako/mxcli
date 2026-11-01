// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"path/filepath"
	"testing"
)

// ako/mxcli#1832: `wrap: off` keeps the main line on one row however long it
// gets; `max-row-width` moves where a row wraps.

func rowYs(t *testing.T, layout *flowLayout, n int) map[int]int {
	t.Helper()
	fb := buildWithLayout(t, logStatements(n), layout)
	ys := map[int]int{}
	for _, p := range activityPositions(fb.objects) {
		ys[p.Y]++
	}
	return ys
}

func TestWrap_DefaultWrapsALongFlow(t *testing.T) {
	// Control: 40 activities are far past MaxRowWidth and wrap by default.
	if ys := rowYs(t, nil, 40); len(ys) < 2 {
		t.Fatalf("default layout put 40 activities on %d row(s), want them wrapped", len(ys))
	}
}

func TestWrap_OffKeepsOneRow(t *testing.T) {
	if ys := rowYs(t, &flowLayout{Wrap: WrapOff}, 40); len(ys) != 1 {
		t.Fatalf("wrap: off put 40 activities on %d rows, want 1", len(ys))
	}
}

func TestWrap_MaxRowWidthMovesTheBreak(t *testing.T) {
	def := rowYs(t, nil, 40)
	narrow := rowYs(t, &flowLayout{MaxRowWidth: 1200}, 40)
	if len(narrow) <= len(def) {
		t.Fatalf("max-row-width 1200 gave %d rows, default %d: want more rows", len(narrow), len(def))
	}
	fb := buildWithLayout(t, logStatements(40), &flowLayout{MaxRowWidth: 1200})
	acts := activityPositions(fb.objects)
	rowStart := acts[0].X
	for _, p := range acts {
		if p.X-rowStart > 1200+RowOverhang {
			t.Errorf("activity at x=%d is %d px into its row, past max-row-width 1200", p.X, p.X-rowStart)
		}
	}
	assertNoOverlap(t, fb.objects)
}

func TestWrap_Config(t *testing.T) {
	dir := t.TempDir()
	l, err := LoadFlowLayoutConfig(writeLayoutConfig(t, dir, "flows:\n  wrap: off\n  max-row-width: 2000\n"))
	if err != nil {
		t.Fatal(err)
	}
	if l.Wrap != WrapOff || l.MaxRowWidth != 2000 {
		t.Fatalf("got %+v", *l)
	}
	if got, want := l.markerValue(), "max-row-width=2000; wrap=off"; got != want {
		t.Fatalf("marker %q, want %q", got, want)
	}
	back, err := parseFlowLayoutMarker(l.markerValue())
	if err != nil || back != *l {
		t.Fatalf("marker reads back as %+v, %v", back, err)
	}
	// The default width is the default: it is not recorded.
	if v := (flowLayout{MaxRowWidth: MaxRowWidth}).markerValue(); v != "" {
		t.Fatalf("explicit default width recorded as %q", v)
	}
	for _, bad := range []string{"wide", "-5", "100"} {
		if _, err := LoadFlowLayoutConfig(writeLayoutConfig(t, filepath.Join(t.TempDir()), "flows:\n  max-row-width: "+bad+"\n")); err == nil {
			t.Errorf("max-row-width %q accepted", bad)
		}
	}
}
