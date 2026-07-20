// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// find returns the first rule for the given property key, or nil.
func findRule(rules []types.WidgetVisibilityRule, key string) *types.WidgetVisibilityRule {
	for i := range rules {
		if rules[i].PropertyKey == key {
			return &rules[i]
		}
	}
	return nil
}

// TestExtractVisibility_DataGrid runs the extractor against the real
// (minified) Data Widgets 3.10.0 DataGrid2 editorConfig.js and asserts it lifts
// the three top-level textTemplate hides that drive the #600 CE0463 — the ones
// the hand-transcribed table never covered — with the correct conditions.
func TestExtractVisibility_DataGrid(t *testing.T) {
	js, err := os.ReadFile("testdata/datagrid.editorConfig.js")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	rules, stats := extractVisibilityRulesFromJS(string(js))
	t.Logf("coverage: %d/%d hide calls recognized (nested=%d complex=%d), %d rules",
		stats.Recognized, stats.TotalHideCalls, stats.SkippedNested, stats.SkippedComplex, len(rules))

	want := []struct {
		key, condKey, op, val string
	}{
		{"clearSelectionButtonLabel", "itemSelection", "ne", "Multi"},
		{"singleSelectionColumnLabel", "itemSelection", "ne", "Single"},
		{"loadMoreButtonCaption", "pagination", "ne", "loadMore"},
	}
	for _, w := range want {
		r := findRule(rules, w.key)
		if r == nil || r.HiddenWhen == nil {
			t.Errorf("%s: no rule extracted", w.key)
			continue
		}
		c := r.HiddenWhen
		if c.PropertyKey != w.condKey || c.Operator != w.op || c.Value != w.val {
			t.Errorf("%s: got {%s %s %q}, want {%s %s %q}",
				w.key, c.PropertyKey, c.Operator, c.Value, w.condKey, w.op, w.val)
		}
	}

	// A couple more expected top-level lifts (non-textTemplate, but proves the
	// idioms generalize): emptyPlaceholder hidden when showEmptyPlaceholder=="none".
	if r := findRule(rules, "emptyPlaceholder"); r == nil || r.HiddenWhen == nil ||
		r.HiddenWhen.PropertyKey != "showEmptyPlaceholder" || r.HiddenWhen.Operator != "eq" || r.HiddenWhen.Value != "none" {
		t.Errorf("emptyPlaceholder rule missing/wrong: %+v", r)
	}
}

// TestExtractVisibility_VideoPlayerPattern reproduces the hand-transcribed
// VideoPlayer rule: `"expression"===e.type && hidePropertiesIn(t,e,["videoUrl","posterUrl"])`.
func TestExtractVisibility_VideoPlayerPattern(t *testing.T) {
	js := `exports.getProperties=function(e,t){"expression"===e.type&&_.hidePropertiesIn(t,e,["videoUrl","posterUrl"]);return t}`
	rules, _ := extractVisibilityRulesFromJS(js)
	for _, key := range []string{"videoUrl", "posterUrl"} {
		r := findRule(rules, key)
		if r == nil || r.HiddenWhen == nil || r.HiddenWhen.PropertyKey != "type" ||
			r.HiddenWhen.Operator != "eq" || r.HiddenWhen.Value != "expression" {
			t.Errorf("%s: got %+v, want hidden when type==expression", key, r)
		}
	}
}

// TestExtractVisibility_TimelinePattern reproduces the hand-transcribed Timeline
// rule, which uses a ternary truthy guard:
// `e.customVisualization ? hidePropertiesIn(t,e,["title","description"]) : x`.
func TestExtractVisibility_TimelinePattern(t *testing.T) {
	js := `exports.getProperties=function(e,t){e.customVisualization?_.hidePropertiesIn(t,e,["title","description","timeIndication"]):null;return t}`
	rules, _ := extractVisibilityRulesFromJS(js)
	for _, key := range []string{"title", "description", "timeIndication"} {
		r := findRule(rules, key)
		if r == nil || r.HiddenWhen == nil || r.HiddenWhen.PropertyKey != "customVisualization" ||
			r.HiddenWhen.Operator != "truthy" {
			t.Errorf("%s: got %+v, want hidden when customVisualization truthy", key, r)
		}
	}
}

// TestExtractVisibility_ScopedAlias proves alias resolution is scoped: the same
// identifier `r` aliased to different properties in two functions must not leak.
func TestExtractVisibility_ScopedAlias(t *testing.T) {
	js := `f1=function(e,t){var r=Object.keys(e);return r};` +
		`f2=function(e,t){var r=t.itemSelection;"Multi"!==r&&_.hidePropertyIn(e,t,"clearSelectionButtonLabel")}`
	rules, _ := extractVisibilityRulesFromJS(js)
	r := findRule(rules, "clearSelectionButtonLabel")
	if r == nil || r.HiddenWhen == nil || r.HiddenWhen.PropertyKey != "itemSelection" ||
		r.HiddenWhen.Operator != "ne" || r.HiddenWhen.Value != "Multi" {
		t.Fatalf("scoped alias not resolved: %+v", r)
	}
}

// TestExtractVisibility_SkipsNested confirms object-list-nested hides
// (hidePropertyIn(...,"columns",n,"key")) are not lifted as top-level rules.
func TestExtractVisibility_SkipsNested(t *testing.T) {
	js := `g=function(e,t){e.columns.forEach(function(r,n){e.columnsSortable||_.hidePropertyIn(t,e,"columns",n,"sortable")})}`
	rules, stats := extractVisibilityRulesFromJS(js)
	if findRule(rules, "sortable") != nil {
		t.Error("nested column hide must not produce a top-level rule")
	}
	if stats.SkippedNested == 0 {
		t.Error("expected the nested hide to be counted as skipped")
	}
}
