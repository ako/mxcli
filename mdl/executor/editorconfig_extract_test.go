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

// TestExtractVisibility_NestedIsScopedToItsList confirms an object-list-nested
// hide (hidePropertyIn(...,"columns",n,"key")) is lifted as a NESTED rule and
// never as a top-level one: `sortable` is a property of a column, not of the
// grid, so a consumer evaluating it against the grid would read an absent key.
//
// The rules used to be skipped entirely (#574 Phase 1), which is what let an
// Accordion group carry a value its widget hides all the way to a CE0463 that
// only `mx create-module-package` reported (upstream #931).
func TestExtractVisibility_NestedIsScopedToItsList(t *testing.T) {
	js := `g=function(e,t){e.columns.forEach(function(r,n){e.columnsSortable||_.hidePropertyIn(t,e,"columns",n,"sortable")})}`
	rules, _ := extractVisibilityRulesFromJS(js)
	if findRule(rules, "sortable") != nil && findRule(rules, "sortable").ListPropertyKey == "" {
		t.Error("nested column hide must not produce a top-level rule")
	}
	r := findRule(rules, "sortable")
	if r == nil {
		t.Fatalf("nested column hide not lifted at all; got %+v", rules)
	}
	if r.ListPropertyKey != "columns" {
		t.Errorf("listPropertyKey = %q, want columns", r.ListPropertyKey)
	}
	if r.HiddenWhen == nil || r.HiddenWhen.PropertyKey != "columnsSortable" || r.HiddenWhen.Operator != "falsy" {
		t.Errorf("condition = %+v, want columnsSortable falsy", r.HiddenWhen)
	}
	// `columnsSortable` is read off the GRID, so the condition stays widget-scoped.
	if r.HiddenWhen.Scope != "" {
		t.Errorf("scope = %q, want widget scope", r.HiddenWhen.Scope)
	}
}

// TestExtractVisibility_NamespaceAndReturn covers the guard forms the filter /
// Gallery / TreeNode editorConfigs use that a `_.`-only, no-`return` parser missed:
// arbitrary namespace prefixes (D./M./j.), a leading `return`, grouping parens
// `cond&&(hide,…)`, and `||` falsy guards.
func TestExtractVisibility_NamespaceAndReturn(t *testing.T) {
	cases := []struct {
		name, js, key, condKey, op, val string
	}{
		{"return + || falsy, namespace j.", `f=function(e,t){return e.adjustable||j.hidePropertyIn(t,e,"screenReaderButtonCaption")}`,
			"screenReaderButtonCaption", "adjustable", "falsy", ""},
		{"eq && namespace j.", `f=function(e,t){x,"auto"===e.attrChoice&&j.hidePropertyIn(t,e,"attributes")}`,
			"attributes", "attrChoice", "eq", "auto"},
		{"return + eq, minified no space, namespace D.", `f=function(e,t){return"none"===e.showEmptyPlaceholder&&D.hidePropertyIn(t,e,"emptyPlaceholder")}`,
			"emptyPlaceholder", "showEmptyPlaceholder", "eq", "none"},
		{"grouping paren cond&&(hide,…), namespace D.", `f=function(e,t){y,"None"===e.itemSelection&&(D.hidePropertyIn(t,e,"autoSelect"),D.hidePropertyIn(t,e,"x"))}`,
			"autoSelect", "itemSelection", "eq", "None"},
		{"ternary truthy, namespace M.", `f=function(e,t){z,e.customVisualization?M.hidePropertiesIn(t,e,["title"]):null}`,
			"title", "customVisualization", "truthy", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rules, _ := extractVisibilityRulesFromJS(c.js)
			r := findRule(rules, c.key)
			if r == nil || r.HiddenWhen == nil {
				t.Fatalf("no rule for %s (rules=%+v)", c.key, rules)
			}
			if r.HiddenWhen.PropertyKey != c.condKey || r.HiddenWhen.Operator != c.op || r.HiddenWhen.Value != c.val {
				t.Errorf("%s: got {%s %s %q}, want {%s %s %q}", c.key,
					r.HiddenWhen.PropertyKey, r.HiddenWhen.Operator, r.HiddenWhen.Value, c.condKey, c.op, c.val)
			}
		})
	}
}

// TestTernaryElseBranchHideRule covers the `cond ? (…) : hidePropertiesIn([…])`
// shape, where the hide fires when the condition is FALSY and the condition sits
// before the matching `?`, past the whole then-branch.
//
// The snippet is ProgressCircle 3.3.2's real getProperties, minified, INCLUDING
// the switch statement that precedes the ternary. That preamble is not
// decoration: a first attempt at this walked back past the `?` to the start of
// the function and handed the guard parser a fragment with an unbalanced `}`,
// which parsed to nothing and looked exactly like "unsupported shape".
//
// Without the rule, labelText read as visible whenever labelType was "text" —
// its default — even with showLabel false, and the widget failed CE0463 either
// way round (ledger #104 follow-on).
func TestTernaryElseBranchHideRule(t *testing.T) {
	js := `function getProperties(e,t,r){` +
		`switch(e.type){case"dynamic":a.hidePropertiesIn(t,e,[].concat(n(b.static),n(b.expression)));break;` +
		`case"static":a.hidePropertiesIn(t,e,[].concat(n(b.dynamic),n(b.expression)));break;` +
		`case"expression":a.hidePropertiesIn(t,e,[].concat(n(b.static),n(b.dynamic)))}` +
		`return e.showLabel?("custom"!==e.labelType&&a.hidePropertyIn(t,e,"customLabel"),` +
		`"text"!==e.labelType&&a.hidePropertyIn(t,e,"labelText")):` +
		`a.hidePropertiesIn(t,e,["customLabel","labelText","labelType"]),t}`

	rules, _ := extractVisibilityRulesFromJS(js)

	want := map[string]bool{"customLabel": false, "labelText": false, "labelType": false}
	for _, r := range rules {
		if r.HiddenWhen != nil && r.HiddenWhen.PropertyKey == "showLabel" && r.HiddenWhen.Operator == "falsy" {
			if _, ok := want[r.PropertyKey]; ok {
				want[r.PropertyKey] = true
			}
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("missing rule: %s hidden when showLabel is falsy", key)
		}
	}

	// The then-branch rule must survive too — a property can carry several rules,
	// and labelText is hidden by EITHER gate.
	var sawInner bool
	for _, r := range rules {
		if r.PropertyKey == "labelText" && r.HiddenWhen != nil &&
			r.HiddenWhen.PropertyKey == "labelType" && r.HiddenWhen.Operator == "ne" &&
			r.HiddenWhen.Value == "text" {
			sawInner = true
		}
	}
	if !sawInner {
		t.Error("the inner `\"text\" !== labelType` rule was lost")
	}
}

// TestTernaryConditionRejectsNonTernaryColon checks the safe direction: a `:`
// that is not a ternary (an object literal, a label) must yield no rule rather
// than a guessed one, since a wrong rule hides a property the user set.
func TestTernaryConditionRejectsNonTernaryColon(t *testing.T) {
	for _, s := range []string{`{foo:`, `return{a:1,b:`, ``} {
		if got, ok := ternaryCondition(s); ok {
			t.Errorf("ternaryCondition(%q) = %q, true; want no match", s, got)
		}
	}
	// And a real ternary is still found, past a nested one.
	if got, ok := ternaryCondition(`x?a?b:c:`[:len(`x?a?b:c:`)-1]); !ok || got != "x" {
		t.Errorf("nested ternary: got %q, %v; want %q, true", got, ok, "x")
	}
}
