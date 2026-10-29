// SPDX-License-Identifier: Apache-2.0

// A Timeline enumeration its editorConfig hides, set to a non-default value
// (ako/mxcli#1762, mendixlabs/mxcli#1668). Reported: `mxcli check` and `exec`
// both pass, and mx check reports "[CE0463] The definition of this widget has
// changed" at Timeline 'tlD' and 'tlG'.
//
// The check already refused this shape for the one rule it recognised —
// `groupEvents: false` plus a non-default hidden value is an MDL-WIDGET10
// error, "a non-default value there fails the build with CE0463" — so the
// remaining rules get the same verdict rather than a second policy: refused
// when the script names the value, written at the default when it does not.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// timelineVariants mirrors the measurement in editorconfig_switch_1762_test.go:
// one widget per configuration, with what mx check said about it on 11.13.0.
var timelineVariants = []struct {
	name, props string
	hiddenProp  string // "" for a control mx check passed
}{
	{"tlB", `customVisualization: true`, ""},
	{"tlD", `customVisualization: true, groupByKey: 'day', groupByDayOptions: 'dayMonth'`, "groupByDayOptions"},
	{"tlDm", `customVisualization: true, groupByKey: 'month', groupByMonthOptions: 'monthYear'`, "groupByMonthOptions"},
	{"tlC", `groupByKey: 'day', groupByDayOptions: 'dayMonth'`, ""},
	{"tlM", `groupByKey: 'month', groupByMonthOptions: 'monthYear'`, ""},
	{"tlG", `groupByKey: 'day', groupByMonthOptions: 'monthYear'`, "groupByMonthOptions"},
	{"tlF", `groupByKey: 'month', groupByDayOptions: 'fullDate'`, "groupByDayOptions"},
	{"tlY", `groupByKey: 'year', groupByDayOptions: 'dayMonth'`, "groupByDayOptions"},
	{"tlYm", `groupByKey: 'year', groupByMonthOptions: 'monthYear'`, "groupByMonthOptions"},
}

func timelinePage(name, props string) string {
	return "create page ModTL.P_" + name + " (Title: 'T', Layout: Atlas_Core.Atlas_Default) {\n" +
		"  timeline " + name + " (data: MICROFLOW ModTL.DS_Events, groupAttribute: At, " + props + ")\n};\n"
}

func widget10For(vs []linter.Violation, widget string) []linter.Violation {
	var out []linter.Violation
	for _, v := range vs {
		if v.RuleID == "MDL-WIDGET10" && strings.Contains(v.Message, "widget `"+widget+"`") {
			out = append(out, v)
		}
	}
	return out
}

// check -p against the installed Timeline 3.2.2 agrees with mxbuild on every
// measured variant.
func TestTimelineHiddenEnumeration_CheckAgreesWithMxbuild(t *testing.T) {
	project := projectFixture(t)
	var src strings.Builder
	for _, v := range timelineVariants {
		src.WriteString(timelinePage(v.name, v.props))
	}
	prog, errs := visitor.Build(src.String())
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	all := ValidateWidgetProperties(prog, project)

	for _, v := range timelineVariants {
		got := widget10For(all, v.name)
		if v.hiddenProp == "" {
			for _, g := range got {
				if g.Severity == linter.SeverityError {
					t.Errorf("%s (mx check: 0 errors): refused with %s", v.name, g.Message)
				}
			}
			continue
		}
		refused := false
		for _, g := range got {
			if g.Severity == linter.SeverityError && strings.Contains(g.Message, "`"+v.hiddenProp+"`") &&
				strings.Contains(g.Message, "CE0463") {
				refused = true
			}
		}
		if !refused {
			t.Errorf("%s (mx check: CE0463): want an MDL-WIDGET10 error naming `%s`, got %+v", v.name, v.hiddenProp, got)
		}
	}
	// The default arm's rule says which values hide it, not "matches".
	if got := widget10For(all, "tlY"); len(got) == 0 ||
		!strings.Contains(got[0].Message, "`groupByKey` is none of \"day\", \"month\"") {
		t.Errorf("tlY: message %+v; want it to name the values the default arm covers", got)
	}
}

// The value split the check already makes: a hidden enumeration set to its
// default is redundant, not a build failure, so it stays a warning.
func TestTimelineHiddenEnumeration_DefaultIsOnlyAWarning(t *testing.T) {
	project := projectFixture(t)
	prog, errs := visitor.Build(timelinePage("tlDef", `customVisualization: true, groupByDayOptions: 'dayName'`))
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	got := widget10For(ValidateWidgetProperties(prog, project), "tlDef")
	if len(got) != 1 || got[0].Severity != linter.SeverityWarning {
		t.Fatalf("got %+v; want one MDL-WIDGET10 warning (the value will be ignored)", got)
	}
}

const timelineSetup = `create module ModTL;
create non-persistent entity ModTL.Ev ( Line: String(200), At: DateTime );
create microflow ModTL.DS_Events () returns List of ModTL.Ev as $Events
begin
  $Events = create list of ModTL.Ev;
  return $Events;
end;
`

// The other half of the policy: a hidden enumeration the script does NOT name is
// written at its default (hiddenUnnamedProperties), now that the rules hiding
// it are known. exec's refusal of a named one is its pre-flight running this
// same check — see cmd/mxcli/exec_preflight_timeline_1762_test.go.
func TestTimelineHiddenEnumeration_UnnamedOnesAreWrittenAtTheirDefault(t *testing.T) {
	exec, _ := pageFixtureExecutor(t)
	run(t, exec, timelineSetup)
	if err := runProgErr(t, exec, timelinePage("tlB", `customVisualization: true`)); err != nil {
		t.Fatalf("exec of the variant mx check passed: %v", err)
	}
	id := unitIDByName(t, exec, "page", "P_tlB")
	for key, want := range map[string]string{"groupByDayOptions": "dayName", "groupByMonthOptions": "month"} {
		if got := storedPrimitive854(t, exec, id, "tlB", key); got != want {
			t.Errorf("%s stored %q; a hidden enumeration must be stored at its default %q", key, got, want)
		}
	}
}
