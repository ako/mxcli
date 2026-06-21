// SPDX-License-Identifier: Apache-2.0

package mpk

import (
	"path/filepath"
	"testing"
)

// chartsMPK is a bundled, multi-widget package (Charts.mpk ships 10 widgets).
// Before #679, ParseMPK/getWidgetIDFromMPK read only WidgetFiles[0], so only
// the first widget (AreaChart) was ever registered.
func chartsMPK(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", "testdata", "expr-checker", "widgets", "Charts.mpk")
	return p
}

func TestParseMPKAll_BundledPackage(t *testing.T) {
	ClearCache()
	defs, err := ParseMPKAll(chartsMPK(t))
	if err != nil {
		t.Fatalf("ParseMPKAll: %v", err)
	}
	if len(defs) < 2 {
		t.Fatalf("expected many widgets in a bundled .mpk, got %d", len(defs))
	}
	want := map[string]bool{
		"com.mendix.widget.web.areachart.AreaChart": false,
		"com.mendix.widget.web.barchart.BarChart":   false,
		"com.mendix.widget.web.piechart.PieChart":   false,
		"com.mendix.widget.web.linechart.LineChart": false,
	}
	for _, d := range defs {
		if _, ok := want[d.ID]; ok {
			want[d.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("bundled widget %s not parsed from Charts.mpk", id)
		}
	}
}

func TestParseMPKWidget_SelectsCorrectWidget(t *testing.T) {
	ClearCache()
	// ParseMPK returns the first widget (AreaChart) — back-compat.
	first, err := ParseMPK(chartsMPK(t))
	if err != nil {
		t.Fatalf("ParseMPK: %v", err)
	}
	if first.ID != "com.mendix.widget.web.areachart.AreaChart" {
		t.Errorf("ParseMPK first = %s, want AreaChart", first.ID)
	}
	// ParseMPKWidget selects a non-first bundled widget by id.
	bar, err := ParseMPKWidget(chartsMPK(t), "com.mendix.widget.web.barchart.BarChart")
	if err != nil {
		t.Fatalf("ParseMPKWidget(BarChart): %v", err)
	}
	if bar.ID != "com.mendix.widget.web.barchart.BarChart" {
		t.Errorf("ParseMPKWidget = %s, want BarChart", bar.ID)
	}
	if _, err := ParseMPKWidget(chartsMPK(t), "com.example.nope.Nope"); err == nil {
		t.Error("ParseMPKWidget for a missing id should error")
	}
}

func TestFindMPK_RegistersAllBundledWidgets(t *testing.T) {
	ClearCache()
	projectDir := filepath.Join("..", "..", "..", "testdata", "expr-checker")
	// A non-first widget in Charts.mpk must be discoverable.
	mpkPath, err := FindMPK(projectDir, "com.mendix.widget.web.barchart.BarChart")
	if err != nil {
		t.Fatalf("FindMPK: %v", err)
	}
	if mpkPath == "" {
		t.Error("FindMPK did not find BarChart (bundled in Charts.mpk past WidgetFiles[0])")
	}
}
