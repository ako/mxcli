// SPDX-License-Identifier: Apache-2.0

// The Timeline's hide rules (ako/mxcli#1762, mendixlabs/mxcli#1668).
//
// A pluggable widget's enumeration that its editorConfig hides must be stored at
// its XML default, or mx check reports CE0463. mxcli refuses that for the one
// Timeline rule it recognised (`groupEvents: false`), and passed the rest:
// `mxcli widget describe timeline` reported "2 of 6 editor hide-rules
// recognized". Measured on mxbuild 11.13.0 with Timeline 3.2.2, one widget per
// variant, nothing else hidden on the page:
//
//	customVisualization: true,  groupByDayOptions: 'dayMonth'                 → CE0463  (1)
//	customVisualization: true,  groupByKey: 'month', groupByMonthOptions: 'monthYear' → CE0463  (1)
//	groupByKey: 'day',   groupByMonthOptions: 'monthYear'                     → CE0463  (4)
//	groupByKey: 'month', groupByDayOptions: 'fullDate'                        → CE0463  (5)
//	groupByKey: 'year',  groupByDayOptions: 'dayMonth'                        → CE0463  (6)
//	groupByKey: 'year',  groupByMonthOptions: 'monthYear'                     → CE0463  (6)
//	controls — custom mode with neither set; 'day' + groupByDayOptions;
//	           'month' + groupByMonthOptions                                  → 0 errors
//
// That matrix is this file's oracle. Two shapes went unread. terser folds the
// statements before the `switch` into its head, `switch(A ? hide : hide, B ||
// hide, e.groupByKey)`, so rule (1)'s guard is bounded by the `switch(` paren,
// which the guard parser took for a conditional group. And the `case`/`default`
// arms of a switch had no reading at all.
package executor

import (
	"slices"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// timelineGetProperties is Timeline 3.2.3's getProperties, verbatim from the
// marketplace package (3.2.2 differs only in the namespace identifier).
const timelineGetProperties = `exports.getProperties=function(e,t){switch(e.customVisualization?I.hidePropertiesIn(t,e,["title","description","icon","timeIndication","groupByDayOptions","groupByMonthOptions"]):I.hidePropertiesIn(t,e,["customIcon","customTitle","customEventDateTime","customDescription","customGroupHeader"]),e.groupEvents||I.hidePropertiesIn(t,e,["groupAttribute","groupByKey","groupByDayOptions","groupByMonthOptions","ungroupedEventsPosition","customGroupHeader"]),e.groupByKey){case"day":I.hidePropertyIn(t,e,"groupByMonthOptions");break;case"month":I.hidePropertyIn(t,e,"groupByDayOptions");break;default:I.hidePropertiesIn(t,e,["groupByDayOptions","groupByMonthOptions"])}return t};`

// timelineCase is one configuration and whether prop is hidden under it.
type timelineCase struct {
	prop   string
	cv     string // customVisualization
	ge     string // groupEvents
	key    string // groupByKey
	hidden bool
}

var timelineCases = []timelineCase{
	// Rule (1), the ternary's THEN branch inside the switch head.
	{"groupByDayOptions", "true", "true", "day", true},     // measured: CE0463
	{"groupByMonthOptions", "true", "true", "month", true}, // measured: CE0463
	{"title", "true", "true", "day", true},
	{"description", "true", "true", "day", true},
	{"timeIndication", "true", "true", "day", true},
	{"icon", "true", "true", "day", true},
	{"title", "false", "true", "day", false},
	{"icon", "false", "true", "day", false},
	// Rule (2), its ELSE branch — recognised before; the control.
	{"customTitle", "false", "true", "day", true},
	{"customTitle", "true", "true", "day", false},
	// Rule (3) — recognised before; the control.
	{"ungroupedEventsPosition", "false", "false", "day", true},
	{"ungroupedEventsPosition", "false", "true", "day", false},
	// Rules (4)–(6), the switch's arms.
	{"groupByMonthOptions", "false", "true", "day", true},    // (4) measured: CE0463
	{"groupByDayOptions", "false", "true", "month", true},    // (5) measured: CE0463
	{"groupByDayOptions", "false", "true", "year", true},     // (6) measured: CE0463
	{"groupByMonthOptions", "false", "true", "year", true},   // (6) measured: CE0463
	{"groupByDayOptions", "false", "true", "day", false},     // measured: clean
	{"groupByMonthOptions", "false", "true", "month", false}, // measured: clean
}

func checkTimelineCases(t *testing.T, rules []types.WidgetVisibilityRule) {
	t.Helper()
	for _, c := range timelineCases {
		values := map[string]string{"customVisualization": c.cv, "groupEvents": c.ge, "groupByKey": c.key}
		hidden, found := ruleHidesUnder(rules, c.prop, values)
		if !found && c.hidden {
			t.Errorf("%s: no rule at all; want hidden under %v", c.prop, values)
			continue
		}
		if hidden != c.hidden {
			t.Errorf("%s under %v: hidden=%v, want %v", c.prop, values, hidden, c.hidden)
		}
	}
}

func TestTimelineHideRules_AllSixRecognised(t *testing.T) {
	rules, stats := extractVisibilityRulesFromJS(timelineGetProperties)
	if stats.TotalHideCalls != 6 || stats.Recognized != 6 {
		t.Errorf("coverage %d of %d; want 6 of 6", stats.Recognized, stats.TotalHideCalls)
	}
	checkTimelineCases(t, rules)
}

// The same through the committed package (testdata/expr-checker ships Timeline
// 3.2.2), which is what `widget describe` and the .def.json generator read.
func TestTimelineHideRules_FromTheInstalledPackage(t *testing.T) {
	const mpkPath = "../../testdata/expr-checker/widgets/com.mendix.widget.web.Timeline.mpk"
	rules, recognized, total := ExtractWidgetVisibilityStats(mpkPath, "com.mendix.widget.web.timeline.Timeline")
	if total != 6 || recognized != 6 {
		t.Errorf("%d of %d editor hide-rules recognized; want 6 of 6", recognized, total)
	}
	checkTimelineCases(t, rules)
}

// A switch's arms, including the shapes Timeline does not use: grouped labels,
// fall-through into the next arm, a default arm that is not last, comma and
// semicolon siblings.
func TestSwitchArms_FallThroughAndDefault(t *testing.T) {
	js := `exports.getProperties=function(e,t){switch(e.k){` +
		`case"a":x.hidePropertyIn(t,e,"p1");` + // falls through into b/c
		`case"b":case"c":x.hidePropertyIn(t,e,"p2");break;` +
		`case"d":x.hidePropertyIn(t,e,"p3"),x.hidePropertyIn(t,e,"p4");x.hidePropertyIn(t,e,"p5");break;` +
		`default:x.hidePropertyIn(t,e,"p6");` + // falls through into e
		`case"e":x.hidePropertyIn(t,e,"p7")` +
		`}return t}`
	rules, stats := extractVisibilityRulesFromJS(js)
	if stats.Recognized != stats.TotalHideCalls {
		t.Errorf("coverage %d of %d", stats.Recognized, stats.TotalHideCalls)
	}
	hiddenFor := map[string][]string{
		"p1": {"a"},
		"p2": {"a", "b", "c"},
		"p3": {"d"}, "p4": {"d"}, "p5": {"d"},
		"p6": {"z"},      // only a value no case names
		"p7": {"e", "z"}, // its own case, and the default that falls into it
	}
	for prop, want := range hiddenFor {
		for _, k := range []string{"a", "b", "c", "d", "e", "z"} {
			hidden, _ := ruleHidesUnder(rules, prop, map[string]string{"k": k})
			if hidden != slices.Contains(want, k) {
				t.Errorf("%s with k=%q: hidden=%v, want %v", prop, k, hidden, slices.Contains(want, k))
			}
		}
	}
}

// Shapes a switch rule cannot state soundly are lifted as nothing — the safe
// direction, which is what each was before.
func TestSwitchArms_UnrepresentableShapesProduceNoRule(t *testing.T) {
	cases := map[string]string{
		// the arm's hide carries its own guard: hidden on `k=="a" AND flag`
		"q1": `function(e,t){switch(e.k){case"a":e.flag&&x.hidePropertyIn(t,e,"q1");break}return t}`,
		// a label that is not a literal makes the default arm's complement unknown
		"q2": `function(e,t){switch(e.k){case n:x.hidePropertyIn(t,e,"q0");break;default:x.hidePropertyIn(t,e,"q2")}return t}`,
		// a switch under an `if` runs only when the if holds
		"q3": `function(e,t){if(e.on){switch(e.k){case"a":x.hidePropertyIn(t,e,"q3")}}return t}`,
		// the discriminant is not one of the widget's properties (the platform)
		"q4": `function(e,t,r){switch(r){case"web":x.hidePropertyIn(t,e,"q4")}return t}`,
		// an arm every value reaches is unconditional, which no condition states
		"q5": `function(e,t){switch(e.k){default:x.hidePropertyIn(t,e,"q5")}return t}`,
		// whether the arm before falls through cannot be read off its last
		// statement — a block, a conditional break — so q6's arm is not known
		"q6": `function(e,t){switch(e.k){case"a":{x.hidePropertyIn(t,e,"q0");break}case"b":x.hidePropertyIn(t,e,"q6")}return t}`,
		"q7": `function(e,t){switch(e.k){case"a":if(e.f)break;x.hidePropertyIn(t,e,"q0");case"b":x.hidePropertyIn(t,e,"q7")}return t}`,
	}
	for prop, js := range cases {
		rules, _ := extractVisibilityRulesFromJS(js)
		if r := findRule(rules, prop); r != nil {
			t.Errorf("%s: lifted %+v from %s; want no rule", prop, r, js)
		}
	}
}

// A switch inside an object list's forEach reads the item.
func TestSwitchArms_NestedItemScope(t *testing.T) {
	js := `exports.getProperties=function(e,t){e.items.forEach((function(r,n){switch(r.mode){` +
		`case"a":x.hideNestedPropertiesIn(t,e,"items",n,["s1"]);break;` +
		`default:x.hidePropertyIn(t,e,"items",n,"s2")}}));return t}`
	rules, _ := extractVisibilityRulesFromJS(js)
	s1, s2 := findRule(rules, "s1"), findRule(rules, "s2")
	if s1 == nil || s1.ListPropertyKey != "items" || s1.HiddenWhen == nil ||
		*s1.HiddenWhen != (types.WidgetVisibilityCondition{PropertyKey: "mode", Operator: "eq", Value: "a", Scope: types.ConditionScopeItem}) {
		t.Errorf("s1 = %+v; want items/s1 hidden when the item's mode is \"a\"", s1)
	}
	if s2 == nil || s2.ListPropertyKey != "items" || s2.HiddenWhen == nil ||
		*s2.HiddenWhen != (types.WidgetVisibilityCondition{PropertyKey: "mode", Operator: "ne", Value: "a", Scope: types.ConditionScopeItem}) {
		t.Errorf("s2 = %+v; want items/s2 hidden when the item's mode is not \"a\"", s2)
	}
}

// terser folds statements into an `if(` head the same way it does a `switch(`;
// a CALL's paren is different, because the call can itself be conditional.
func TestStatementHeadParenIsNotAGroup(t *testing.T) {
	js := `exports.getProperties=function(e,t){` +
		`if(e.a?x.hidePropertyIn(t,e,"r1"):x.hidePropertyIn(t,e,"r2"),e.b)return t;` +
		`e.y&&foo(e.x&&x.hidePropertyIn(t,e,"r3"));return t}`
	rules, _ := extractVisibilityRulesFromJS(js)
	if h, _ := ruleHidesUnder(rules, "r1", map[string]string{"a": "true"}); !h {
		t.Error("r1: want hidden when a is set")
	}
	if h, _ := ruleHidesUnder(rules, "r1", map[string]string{"a": "false"}); h {
		t.Error("r1: hidden when a is not set")
	}
	if h, _ := ruleHidesUnder(rules, "r2", map[string]string{"a": "false"}); !h {
		t.Error("r2 (the else branch, the control): want hidden when a is not set")
	}
	if r := findRule(rules, "r3"); r != nil {
		t.Errorf("r3: lifted %+v; a guard inside a call's arguments is one conjunct of `e.y && …`", r)
	}
}

// fileUploaderGetProperties is File Uploader 2.5.0's getProperties, verbatim from
// the marketplace package (content 235351).
const fileUploaderGetProperties = `exports.getProperties=function(e,t){return e.readOnlyMode?A.hidePropertiesIn(t,e,["files"===e.uploadMode?"associatedImages":"associatedFiles","createImageAction","createFileAction","allowedFileFormats","maxFilesPerUpload","maxFilesPerBatch","maxFileSize","objectCreationTimeout"]):"files"===e.uploadMode?(A.hidePropertiesIn(t,e,["associatedImages","createImageAction"]),function(e,t){var r;e.allowedFileFormats.forEach(function(r,n){"simple"===r.configMode?A.hideNestedPropertiesIn(t,e,"allowedFileFormats",n,["mimeType","extensions","typeFormatDescription"]):A.hideNestedPropertiesIn(t,e,"allowedFileFormats",n,["predefinedType"])});var n=null===(r=t[0].properties)||void 0===r?void 0:r.find(function(e){return"allowedFileFormats"===e.key});if(n){var o;n.objectHeaders=["File type",""],null===(o=n.objects)||void 0===o||o.forEach(function(t,r){var n=e.allowedFileFormats[r];"simple"===n.configMode?t.captions=[E[n.predefinedType].description," "]:t.captions=[n.typeFormatDescription,"".concat(z(n.mimeType,n.extensions))]})}}(e,t)):A.hidePropertiesIn(t,e,["associatedFiles","createFileAction","allowedFileFormats"]),"files"===e.uploadMode?A.hidePropertiesIn(t,e,["onUploadSuccessImage","onUploadFailureImage"]):A.hidePropertiesIn(t,e,["onUploadSuccessFile","onUploadFailureFile"]),e.enableCustomButtons?e.customButtons.forEach(function(r,n){A.hideNestedPropertiesIn(t,e,"customButtons",n,"files"===e.uploadMode?["buttonActionImage"]:["buttonActionFile"])}):A.hidePropertiesIn(t,e,["customButtons"]),t};`

// ako/mxcli#258 item 1: the ternary-wrapped `hidePropertiesIn` that gates the
// create* actions on uploadMode. It was unread when filed ("6 of 9"); the
// else-if chain reading added for mendixlabs/mxcli#1583 lifts it, so this pins
// that, and that the switch/statement-head changes for #1762 leave it alone.
func TestFileUploaderUploadModeGate_258(t *testing.T) {
	rules, _ := extractVisibilityRulesFromJS(fileUploaderGetProperties)
	cases := []struct {
		prop, readOnly, mode string
		hidden               bool
	}{
		{"createImageAction", "false", "files", true},
		{"createImageAction", "false", "images", false},
		{"createImageAction", "true", "images", true},
		{"createFileAction", "false", "images", true},
		{"createFileAction", "false", "files", false},
		{"createFileAction", "true", "files", true},
		{"onUploadSuccessImage", "false", "files", true},
		{"onUploadSuccessImage", "false", "images", false},
		{"onUploadSuccessFile", "false", "images", true},
		{"onUploadSuccessFile", "false", "files", false},
	}
	for _, c := range cases {
		values := map[string]string{"readOnlyMode": c.readOnly, "uploadMode": c.mode}
		if hidden, _ := ruleHidesUnder(rules, c.prop, values); hidden != c.hidden {
			t.Errorf("%s under %v: hidden=%v, want %v", c.prop, values, hidden, c.hidden)
		}
	}
}
