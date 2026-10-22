// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1616: every member assignment of a `create` action
// (`$S = create M.E (A1 = …, A2 = …)`) made exec and check list the modules
// and decode every domain model of the project, so 200 creates of 8 members
// went from 14 s to 294 s once 30 modules were added — and `mxcli test`, whose
// generated flows are mostly such creates, took minutes per test file. The
// member assignments of one flow must cost what one lookup of the entity's
// domain model costs, not one project scan each.

// createsFlow is the issue's generator: one microflow of n `create` actions,
// each assigning members (or none, for the control).
func createsFlow(name string, n int, members bool) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "mdl 1;\ncreate microflow MyFirstModule.%s () returns Boolean as $r\nbegin\n", name)
	for j := 1; j <= n; j++ {
		if members {
			fmt.Fprintf(&sb, "  $S%d = create MyFirstModule.Sale (A1 = 'a', A2 = 'b', A3 = 'c', A4 = 'd', A5 = 1, A6 = 2, A7 = true, A8 = [%%CurrentDateTime%%]);\n", j)
		} else {
			fmt.Fprintf(&sb, "  $S%d = create MyFirstModule.Sale;\n", j)
		}
	}
	sb.WriteString("  return true;\nend;\n")
	return sb.String()
}

func TestCreateMemberAssignments_DoNotScanTheProjectEach(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, `mdl 1;
create persistent entity MyFirstModule.Sale (
  A1: String(100), A2: String(100), A3: String(100), A4: String(100),
  A5: Integer, A6: Integer, A7: Boolean, A8: DateTime
);`); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	const creates = 20
	const assignments = creates * 8
	reads := func(src string) int64 {
		t.Helper()
		before := unitFileReads(t, exec)
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("exec: %v\n%s", err, out.String())
		}
		return unitFileReads(t, exec) - before
	}
	plain := reads(createsFlow("Plain", creates, false))
	withMembers := reads(createsFlow("WithMembers", creates, true))
	t.Logf("unit-file reads: %d without members, %d with %d member assignments", plain, withMembers, assignments)

	if extra := withMembers - plain; extra >= assignments {
		t.Errorf("%d member assignments cost %d extra unit-file reads (%d without members, %d with): at least one per assignment, so the cost grows with the project, not the flow",
			assignments, extra, plain, withMembers)
	}
}
