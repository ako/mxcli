// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#856: a find or filter by expression may name a member of the
// list's entity bare — `find $L where Subject = 'x'` — and the builder stores
// it as `$currentObject/Subject = 'x'` (#1002: Mendix evaluates the predicate
// per item, and a bare name is CE0117). describe prints the stored form, so the
// statement as the script spells it never matched its own stored activity.
//
// A re-run that changed nothing was still Unchanged (the built flow is the
// stored one, ako/mxcli#859), but a re-run with a real change elsewhere went
// to the statement diff, where the find read as changed too: replaced with new
// $IDs at best, and next to the real change the two merged into one replace
// whose fragment declares $A while the stored find still does, which mdl 1
// refused ("the fragment declares $A, which the microflow already has").

const listOpWhereFlow = `mdl 1;
create or modify microflow MyFirstModule.ListOpWhere ($L: List of FeedbackModule.Feedback)
begin
  declare $N Integer = 0;
  $A = find $L where Subject = 'x';
  $B = filter $L where Subject != 'y' and ScreenWidth > 0;
  log info node 'T' toString($N);
end;`

func TestCreateOrModify_ListOperationWhereBareMemberIsItsStoredActivity(t *testing.T) {
	for _, header := range []string{"mdl 1;", ""} {
		t.Run("header "+header, func(t *testing.T) {
			exec, out, _ := openPedAppCopy(t)
			if err := agreeExec(t, exec, listOpWhereFlow); err != nil {
				t.Fatalf("setup: %v\n%s", err, out.String())
			}
			desc := describeFlow(t, exec, "ListOpWhere")
			if !strings.Contains(desc, "where $currentObject/Subject = 'x'") {
				t.Fatalf("the stored find is not described in its qualified spelling, so the case would not reach the bug:\n%s", desc)
			}
			before := flowObjectIDs(loopEditFlowOf(t, exec, "ListOpWhere").ObjectCollection)

			// One real change, next to the find; the find and filter are
			// restated exactly as the script first wrote them.
			edited := strings.Replace(listOpWhereFlow, "declare $N Integer = 0;", "declare $N Integer = 1;", 1)
			edited = strings.TrimPrefix(edited, "mdl 1;\n")
			if header != "" {
				edited = header + "\n" + edited
			}
			out.Reset()
			if err := agreeExec(t, exec, edited); err != nil {
				t.Fatalf("a one-line change next to a find by expression is refused: %v", err)
			}
			if strings.Contains(out.String(), "MDL-V1-REBUILD") {
				t.Errorf("the change fell back to rebuilding the flow:\n%s", out.String())
			}
			if !strings.Contains(out.String(), "Modified microflow: MyFirstModule.ListOpWhere (spliced: 1 edited in place)") {
				t.Errorf("want only the changed declare edited, got:\n%s", out.String())
			}
			if got := describeFlow(t, exec, "ListOpWhere"); !strings.Contains(got, "declare $N Integer = 1;") {
				t.Errorf("the change was not written:\n%s", got)
			}

			// Nothing is new: the find, the filter and the rest keep their
			// $IDs and places, and so does the declare, edited in place
			// (ako/mxcli#1761).
			after := flowObjectIDs(loopEditFlowOf(t, exec, "ListOpWhere").ObjectCollection)
			var gone, added []model.ID
			for id := range before {
				if _, ok := after[id]; !ok {
					gone = append(gone, id)
				}
			}
			for id, p := range after {
				if q, ok := before[id]; !ok {
					added = append(added, id)
				} else if p != q {
					t.Errorf("object %s moved from %v to %v", id, q, p)
				}
			}
			if len(gone) != 0 || len(added) != 0 {
				t.Errorf("want every object kept (0 gone, 0 new), got %d gone, %d new", len(gone), len(added))
			}
		})
	}
}

// Control: a predicate that does change is still a change, in either spelling.
func TestCreateOrModify_ListOperationWhereChangedPredicateIsAChange(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, listOpWhereFlow); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	for _, c := range []struct{ from, to string }{
		{"where Subject = 'x'", "where Subject = 'z'"},
		{"where Subject = 'x'", "where $currentObject/Description = 'x'"},
		{"where Subject != 'y' and ScreenWidth > 0", "where Subject != 'y' and ScreenHeight > 0"},
	} {
		out.Reset()
		if err := agreeExec(t, exec, strings.Replace(listOpWhereFlow, c.from, c.to, 1)); err != nil {
			t.Fatalf("%s → %s: %v", c.from, c.to, err)
		}
		if !strings.Contains(out.String(), "(spliced: 1 edited in place)") {
			t.Errorf("%s → %s: want the operation edited, got:\n%s", c.from, c.to, out.String())
		}
		// Back to the stored statement for the next case.
		if err := agreeExec(t, exec, listOpWhereFlow); err != nil {
			t.Fatalf("restore: %v", err)
		}
	}
}
