// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// describe → check → exec on a Timeline, once the hide rules are known
// (ako/mxcli#1762). One of the two format enumerations is ALWAYS hidden — by
// customVisualization, or by the groupByKey switch — and a generic pluggable
// widget was described with every stored primitive, so each Timeline came back
// with a hidden property at its default and drew an MDL-WIDGET10 warning on
// mxcli's own output. Such a property carries nothing: exec writes a hidden
// property the script does not name at that same default.
func TestTimelineDescribe_HiddenDefaultsAreNotPrinted(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, timelineSetup)
	variants := []struct{ name, props, mustPrint, mustNotPrint string }{
		{"tlB", `customVisualization: true`, "customVisualization", "groupByDayOptions"},
		{"tlS", `groupByKey: 'year'`, "groupByKey: 'year'", "groupByMonthOptions"},
		{"tlM", `groupByKey: 'month', groupByMonthOptions: 'monthYear'`, "groupByMonthOptions: 'monthYear'", "groupByDayOptions"},
	}
	for _, v := range variants {
		run(t, exec, timelinePage(v.name, v.props))
	}
	for _, v := range variants {
		id := unitIDByName(t, exec, "page", "P_"+v.name)
		before := storedBytes(t, exec, id)

		out.Reset()
		run(t, exec, "describe page ModTL.P_"+v.name)
		described := out.String()
		if !strings.Contains(described, v.mustPrint) {
			t.Errorf("%s: describe lost %q:\n%s", v.name, v.mustPrint, described)
		}
		if strings.Contains(described, v.mustNotPrint) {
			t.Errorf("%s: describe printed the hidden %s:\n%s", v.name, v.mustNotPrint, described)
		}

		prog, errs := visitor.Build(described)
		if len(errs) > 0 {
			t.Fatalf("%s: describe output does not parse: %v\n%s", v.name, errs, described)
		}
		if got := widget10For(ValidateWidgetProperties(prog, exec.backend.Path()), v.name); len(got) > 0 {
			t.Errorf("%s: check of describe's own output reports %+v", v.name, got)
		}

		if err := runErr(t, exec, described); err != nil {
			t.Fatalf("%s: exec of the description: %v", v.name, err)
		}
		if !bytes.Equal(before, storedBytes(t, exec, id)) {
			t.Errorf("%s: describe → exec changed the stored page", v.name)
		}
	}
}

func storedBytes(t *testing.T, exec *Executor, unitID string) []byte {
	t.Helper()
	raw, err := exec.backend.GetRawUnitBytes(model.ID(unitID))
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), raw...)
}
