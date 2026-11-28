// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"
)

// ako/mxcli#1832, #2635: a canonical DESCRIBE judges a flow's positions against
// the project's style, the one `create or modify` rebuilds the description in.
// Nothing is written into the flow's documentation. Described after the project
// moves to another style, the same flow keeps every position that style would
// not derive, which is the control.

const styledFlowsMDL = `
create microflow MyFirstModule.ACT_Styled ($N: Integer) returns Boolean
begin
  log info 'start';
  if $N < 0 then
    log info 'negative';
    return false;
  end if;
  if $N > 100 then
    log info 'too big';
    return false;
  end if;
  if $N > 10 then
    log info 'big';
  else
    log info 'small 1';
    log info 'small 2';
  end if;
  if $N > 5 then
    log info 'over five';
  end if;
  log info 'ok';
  return true;
end;

create nanoflow MyFirstModule.NAV_Styled ($N: Integer) returns Boolean
begin
  if $N < 0 then
    return false;
  end if;
  if $N > 10 then
    log info 'big';
  else
    log info 'small 1';
    log info 'small 2';
  end if;
  return true;
end;
`

func TestDescribeJudgesPositionsAgainstTheProjectStyle(t *testing.T) {
	for _, preset := range []string{"true-right-false-down"} {
		for _, guard := range []bool{false, true} {
			style := preset
			if guard {
				style += "+guard"
			}
			t.Run(style, func(t *testing.T) {
				exec, out := describeExecutor(t)
				if err := exec.SetFlowLayoutStyle(preset); err != nil {
					t.Fatal(err)
				}
				if guard {
					l := *exec.flowLayoutOverride
					l.Guard = GuardTrueRight
					exec.flowLayoutOverride = &l
				}
				run(t, exec, styledFlowsMDL)
				describes := []string{"describe microflow MyFirstModule.ACT_Styled", "describe nanoflow MyFirstModule.NAV_Styled"}
				for _, d := range describes {
					mdl := describeText(t, exec, out, d)
					if strings.Contains(mdl, flowLayoutMarker) {
						t.Fatalf("%s: wrote a %s line:\n%s", d, flowLayoutMarker, mdl)
					}
					if got := layoutLines(mdl); len(got) > 0 {
						t.Errorf("%s printed layout annotations under the project style: %v\n%s", d, got, mdl)
					}
				}
				// Control: in a project on the default style the same flows are
				// not derivable, so their positions are printed.
				if err := exec.SetFlowLayoutStyle(""); err != nil {
					t.Fatal(err)
				}
				for _, d := range describes {
					if got := layoutLines(describeText(t, exec, out, d)); len(got) == 0 {
						t.Errorf("%s: no layout annotations under a style that did not draw it", d)
					}
				}
			})
		}
	}
}

const styledCaseSplitsMDL = `
create or modify persistent entity MyFirstModule.Animal ( "Label": String(50) );
create or modify persistent entity MyFirstModule.Dog generalization MyFirstModule.Animal ();
create or modify persistent entity MyFirstModule.Cat generalization MyFirstModule.Animal ();

create microflow MyFirstModule.ACT_Cases ($P: NanoflowCommons.GeocodingProvider, $A: MyFirstModule.Animal) returns String
begin
  case $P
    when MapQuest then
      log info 'map 1';
      log info 'map 2';
    when Google then
      log info 'google';
    when LocationIQ then
      log info 'iq 1';
      log info 'iq 2';
      log info 'iq 3';
    when Geocodio then
      log info 'geocodio';
    when (empty) then
      log info 'none';
  end case;
  split type $A
    when MyFirstModule.Dog then
      log info 'dog';
    when MyFirstModule.Cat then
      log info 'cat 1';
      log info 'cat 2';
    when MyFirstModule.Animal then
      log info 'animal';
  end split;
  return 'done';
end;
`

func TestDescribeKeepsCaseOrderInCaseColumns(t *testing.T) {
	exec, out := describeExecutor(t)
	if err := exec.SetFlowLayoutStyle("true-right-false-down"); err != nil {
		t.Fatal(err)
	}
	run(t, exec, styledCaseSplitsMDL)
	mdl := describeText(t, exec, out, "describe microflow MyFirstModule.ACT_Cases")
	if got := layoutLines(mdl); len(got) > 0 {
		t.Errorf("printed layout annotations under the project style: %v\n%s", got, mdl)
	}
	// The cases come back in the order they were written, not the order a
	// column's sides would sort them in.
	last := -1
	for _, want := range []string{"when MapQuest", "when Google", "when LocationIQ", "when Geocodio", "when (empty)", "when MyFirstModule.Dog", "when MyFirstModule.Cat", "when MyFirstModule.Animal"} {
		i := strings.Index(mdl, want)
		if i < 0 || i < last {
			t.Fatalf("%q missing or out of order in:\n%s", want, mdl)
		}
		last = i
	}
}

func TestDescribeJudgesWrapAgainstTheProjectStyle(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&body, "  log info 'step %d';\n", i)
	}
	mdl := "create microflow MyFirstModule.ACT_Long () returns Boolean begin\n" + body.String() + "  return true;\nend;\n"
	for _, l := range []flowLayout{{}, {Wrap: WrapOff}, {MaxRowWidth: 1200}} {
		t.Run(l.String(), func(t *testing.T) {
			exec, out := describeExecutor(t)
			style := l
			exec.flowLayoutOverride = &style
			run(t, exec, mdl)
			got := describeText(t, exec, out, "describe microflow MyFirstModule.ACT_Long")
			if strings.Contains(got, flowLayoutMarker) {
				t.Fatalf("wrote a marker for %s:\n%s", l, got)
			}
			if lines := layoutLines(got); len(lines) > 0 {
				t.Errorf("printed layout annotations under the project style: %v", lines)
			}
		})
	}
}
