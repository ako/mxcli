// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#1832: a flow laid out in a style records it, and a canonical
// DESCRIBE judges its positions against that style — whatever style the project
// is in when it is described. With the record gone the same description keeps
// every position it can no longer derive, which is the control.

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

func TestDescribeUsesTheRecordedLayoutStyle(t *testing.T) {
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
				for _, stmt := range strings.Split(strings.TrimSpace(styledFlowsMDL), "end;\n\n") {
					if !strings.HasSuffix(stmt, "end;") {
						stmt += "end;"
					}
					run(t, exec, stmt)
				}
				// The project is back on the default style; the flows record theirs.
				if err := exec.SetFlowLayoutStyle(""); err != nil {
					t.Fatal(err)
				}
				for _, d := range []string{"describe microflow MyFirstModule.ACT_Styled", "describe nanoflow MyFirstModule.NAV_Styled"} {
					mdl := describeText(t, exec, out, d)
					if !strings.Contains(mdl, flowLayoutMarker) {
						t.Fatalf("%s: no %s line:\n%s", d, flowLayoutMarker, mdl)
					}
					if got := layoutLines(mdl); len(got) > 0 {
						t.Errorf("%s printed layout annotations under the recorded style: %v\n%s", d, got, mdl)
					}
				}
			})
		}
	}
}
