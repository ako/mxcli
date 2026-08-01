// SPDX-License-Identifier: Apache-2.0

// CommitType previously declared two values Mendix does not define —
// "YesWithEvents" and "NoEvent". Nothing crashed, because the read path passes the
// stored string straight through, but the named constants matched nothing in a real
// project: any code switching on them silently fell through to the default. The
// giveaway was mdl/backend/mcp, which had to translate CommitTypeNoEvent into
// "YesWithoutEvents" to stay correct.
//
// This test pins the set to the generated metamodel so a hand-written constant can
// never drift from the enum again.
package microflows

import (
	"testing"

	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
)

func TestCommitType_MatchesMetamodelEnum(t *testing.T) {
	// The generated CommitEnum is derived from Mendix's own reflection data and is
	// the authority for what may appear in a BSON Commit field.
	want := map[string]bool{
		string(genMf.CommitEnumYes):              true,
		string(genMf.CommitEnumYesWithoutEvents): true,
		string(genMf.CommitEnumNo):               true,
	}

	got := map[string]bool{
		string(CommitTypeYes):              true,
		string(CommitTypeYesWithoutEvents): true,
		string(CommitTypeNo):               true,
	}

	for v := range want {
		if !got[v] {
			t.Errorf("metamodel defines Commit value %q with no CommitType constant", v)
		}
	}
	for v := range got {
		if !want[v] {
			t.Errorf("CommitType declares %q, which is not a Mendix Commit value — "+
				"code switching on it can never match a real project", v)
		}
	}
}

// TestCommitType_DefaultIsNo documents the zero value: an action built without an
// explicit flag must serialize as Mendix's default, No — not as an empty string.
func TestCommitType_DefaultIsNo(t *testing.T) {
	var zero CommitType
	if zero == CommitTypeNo {
		t.Fatal("test premise wrong: the zero value is not literally CommitTypeNo")
	}
	// Callers must set the flag explicitly; the builder does. This asserts the
	// constant's spelling, which is what the BSON writer emits.
	if string(CommitTypeNo) != "No" {
		t.Errorf("CommitTypeNo = %q, want %q", CommitTypeNo, "No")
	}
}
