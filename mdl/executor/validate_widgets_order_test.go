// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// widgetWithUnknownProps builds a static widget carrying several properties no
// builder consumes, so every one produces an MDL-WIDGET07 warning. Eight keys
// make an accidental pass vanishingly unlikely: with unsorted map iteration the
// chance of Go handing back the same order twice is 1/8!.
func widgetWithUnknownProps() *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type: "container",
		Name: "c1",
		Properties: map[string]any{
			"zeta": "1", "alpha": "2", "mike": "3", "delta": "4",
			"omega": "5", "bravo": "6", "kilo": "7", "sierra": "8",
		},
	}
}

func messagesOf(vs []linter.Violation) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Message)
	}
	return out
}

// `mxcli check` printed the same warnings in a different order from one run to
// the next, because the validator iterated w.Properties directly. Nothing was
// wrong with the diagnostics, but it made a before/after diff of check output
// useless as a measurement — two runs of the same binary over mdl-examples/
// disagreed on 11 of 515 scripts, which was more noise than the grammar change
// being measured produced signal.
func TestStaticWidgetUnknownProps_OrderIsStable(t *testing.T) {
	first := messagesOf(validateStaticWidgetUnknownProps(widgetWithUnknownProps(), "page M.P"))
	if len(first) != 8 {
		t.Fatalf("got %d violations, want 8 — the fixture must produce one per unknown key, or this test proves nothing", len(first))
	}

	for i := 0; i < 50; i++ {
		got := messagesOf(validateStaticWidgetUnknownProps(widgetWithUnknownProps(), "page M.P"))
		if len(got) != len(first) {
			t.Fatalf("run %d produced %d violations, want %d", i, len(got), len(first))
		}
		for j := range first {
			if got[j] != first[j] {
				t.Fatalf("run %d differs at position %d:\n got %q\nwant %q\nwidget property warnings must not depend on map iteration order",
					i, j, got[j], first[j])
			}
		}
	}
}

// Stable is necessary but not sufficient — it must also be a stable order a
// reader can predict. Sorted by property key is the one the fix chose.
func TestStaticWidgetUnknownProps_OrderIsSorted(t *testing.T) {
	w := widgetWithUnknownProps()
	got := messagesOf(validateStaticWidgetUnknownProps(w, "page M.P"))

	keys := make([]string, 0, len(w.Properties))
	for k := range w.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if len(got) != len(keys) {
		t.Fatalf("got %d violations for %d properties", len(got), len(keys))
	}
	for i, k := range keys {
		if !strings.Contains(got[i], "`"+k+"`") {
			t.Errorf("violation %d = %q, want it to be about property %q — warnings should follow sorted key order", i, got[i], k)
		}
	}
}

// sortedPropertyKeys is the shared helper; a nil widget must not panic, since
// the validators are called on trees built from partial parses.
func TestSortedPropertyKeys(t *testing.T) {
	if got := sortedPropertyKeys(nil); got != nil {
		t.Errorf("sortedPropertyKeys(nil) = %v, want nil", got)
	}
	if got := sortedPropertyKeys(&ast.WidgetV3{}); len(got) != 0 {
		t.Errorf("sortedPropertyKeys(no properties) = %v, want empty", got)
	}
	w := &ast.WidgetV3{Properties: map[string]any{"b": 1, "a": 2, "c": 3}}
	got := sortedPropertyKeys(w)
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sortedPropertyKeys = %v, want %v", got, want)
			break
		}
	}
}
