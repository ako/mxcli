// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1319: an activity already IN the model that stores Studio
// Pro's First over an object-rooted mapping. check's MDL-MAP04 only sees
// scripts, so a model written by v0.24.0 or with `exec --no-check` reported
// "No issues found" from lint and 0 errors from mx check, then threw
//   key not found: Path(QName(None,),None,)
// when the microflow ran.

// reproShapes is the issue's three mappings: two object-rooted, one list-rooted.
func reproShapes(name string) (list, known bool) {
	switch name {
	case "G59.IMM_Order", "G59.IMM_Flat":
		return false, true
	case "G59.IMM_Lines":
		return true, true
	}
	return false, false
}

func importActivity(mapping string, force, rangeFirst bool) *microflows.ActionActivity {
	return &microflows.ActionActivity{Action: &microflows.ImportXmlAction{
		ResultHandling: &microflows.ResultHandlingMapping{
			MappingID:             model.ID(mapping),
			ResultVariable:        "Order",
			SingleObject:          true,
			ForceSingleOccurrence: &force,
			RangeSingleObject:     &rangeFirst,
		},
	}}
}

func lintImportRange(objects []microflows.MicroflowObject) []linter.Violation {
	var out []linter.Violation
	r := NewImportRangeObjectMappingRule()
	findFirstOnObjectMappings(objects, testMicroflow(), reproShapes, r, &out)
	return out
}

// The issue's table of stored values, row by row.
func TestImportRangeObjectMapping_IssueTable(t *testing.T) {
	cases := []struct {
		flow, mapping     string
		force, rangeFirst bool
		want              bool
	}{
		{"MF_Order_First", "G59.IMM_Order", true, true, true},
		{"MF_Flat_First", "G59.IMM_Flat", true, true, true},
		{"MF_Order_NoRange", "G59.IMM_Order", false, false, false},
		{"MF_Order_All", "G59.IMM_Order", false, false, false},
		{"MF_Flat_NoRange", "G59.IMM_Flat", false, false, false},
		// Same flag pair as the failing rows: Studio Pro's legitimate First
		// over a list-rooted mapping. The rule keys on the mapping, not the flags.
		{"MF_Lines_First", "G59.IMM_Lines", true, true, false},
		{"MF_Lines_All", "G59.IMM_Lines", false, false, false},
		// Either flag alone is the runtime failure (#242 measured FSO=true /
		// Range=false failing too), so either one is reported.
		{"force only", "G59.IMM_Order", true, false, true},
		{"range only", "G59.IMM_Order", false, true, true},
		// A mapping whose shape cannot be established is left alone.
		{"unknown mapping", "G59.IMM_Xml", true, true, false},
	}
	for _, tc := range cases {
		got := lintImportRange([]microflows.MicroflowObject{
			importActivity(tc.mapping, tc.force, tc.rangeFirst),
		})
		if tc.want && (len(got) != 1 || got[0].RuleID != "MDL-MAP04") {
			t.Errorf("%s: want one MDL-MAP04, got %v", tc.flow, got)
		}
		if !tc.want && len(got) != 0 {
			t.Errorf("%s: want no violation, got %s", tc.flow, got[0].Message)
		}
	}
}

func TestImportRangeObjectMapping_NamesMappingAndError(t *testing.T) {
	got := lintImportRange([]microflows.MicroflowObject{
		importActivity("G59.IMM_Order", true, true),
	})
	if len(got) != 1 {
		t.Fatalf("want one violation, got %d", len(got))
	}
	v := got[0]
	if v.Severity != linter.SeverityError {
		t.Errorf("severity = %v, want error", v.Severity)
	}
	for _, want := range []string{"G59.IMM_Order", "key not found: Path(QName(None,),None,)"} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message does not name %q: %s", want, v.Message)
		}
	}
	if v.Location.DocumentName != "ACT_Process" || v.Location.Module != "MyModule" {
		t.Errorf("location = %+v", v.Location)
	}
}

func TestImportRangeObjectMapping_InsideLoop(t *testing.T) {
	got := lintImportRange([]microflows.MicroflowObject{
		&microflows.LoopedActivity{ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{
				importActivity("G59.IMM_Flat", true, true),
			},
		}},
	})
	if len(got) != 1 {
		t.Fatalf("want one violation for the import inside the loop, got %d", len(got))
	}
}

// A REST call's `returns mapping … as Entity` stores the same ImportMappingCall
// and fails the same way on an object-rooted mapping (ako/mxcli#242).
func TestImportRangeObjectMapping_RestCall(t *testing.T) {
	force, first := true, true
	rest := &microflows.ActionActivity{Action: &microflows.RestCallAction{
		ResultHandling: &microflows.ResultHandlingMapping{
			MappingID:             "G59.IMM_Order",
			SingleObject:          true,
			ForceSingleOccurrence: &force,
			RangeSingleObject:     &first,
		},
	}}
	if got := lintImportRange([]microflows.MicroflowObject{rest}); len(got) != 1 {
		t.Fatalf("want one violation for the REST call, got %d", len(got))
	}
	force, first = false, false
	if got := lintImportRange([]microflows.MicroflowObject{rest}); len(got) != 0 {
		t.Fatalf("REST call storing Studio Pro's object shape was reported: %s", got[0].Message)
	}
}
