// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// violationsByRule returns the set of rule IDs present in a violation slice.
func violationsByRule(vs []linter.Violation) map[string]linter.Violation {
	out := map[string]linter.Violation{}
	for _, v := range vs {
		if _, seen := out[v.RuleID]; !seen {
			out[v.RuleID] = v
		}
	}
	return out
}

// checkMicroflowSource parses one CREATE MICROFLOW statement and validates it.
func checkMicroflowSource(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	stmt, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("statement 0 is %T, want *ast.CreateMicroflowStmt", prog.Statements[0])
	}
	return ValidateMicroflow(stmt)
}

// upstream #893 item 1. `declare $X String;` with no initial value parsed, passed
// `mxcli check` and was written by `exec`; mxbuild then rejected the Create
// Variable activity with CE0038 "The 'Value' property is required."
//
// Measured on mxbuild 11.6.6 against a blank app whose baseline is 1 error:
// the probe microflow took it to 2, and supplying `= ”` took it back to 1.
func TestMDL061_DeclareWithoutValue(t *testing.T) {
	vs := checkMicroflowSource(t, `create microflow Synthetic.MF_BareDeclare ()
begin
  declare $X String;
  log info node 'x' 'y';
end;`)

	got, ok := violationsByRule(vs)["MDL061"]
	if !ok {
		t.Fatalf("expected MDL061 for a value-less declare, got %#v", vs)
	}
	if got.Severity != linter.SeverityError {
		t.Errorf("MDL061 severity = %v, want Error (mxbuild rejects it with CE0038)", got.Severity)
	}
	if !strings.Contains(got.Message, "X") {
		t.Errorf("MDL061 message should name the variable, got %q", got.Message)
	}
	// The suggestion must be actionable: the fix is a value, and `= ''` was
	// verified to build clean.
	if !strings.Contains(got.Suggestion, "=") {
		t.Errorf("MDL061 suggestion should show the initializer form, got %q", got.Suggestion)
	}
}

// The control for MDL061: an initialized primitive declare is exactly what the
// fix advice tells the user to write, so it must not be flagged — otherwise the
// rule has no escape and the syntax becomes unusable rather than fixable.
func TestMDL061_InitializedDeclareIsClean(t *testing.T) {
	for _, src := range []string{
		`create microflow Synthetic.MF_S () begin declare $X String = ''; end;`,
		`create microflow Synthetic.MF_I () begin declare $N Integer = 0; end;`,
		`create microflow Synthetic.MF_B () begin declare $B Boolean = false; end;`,
	} {
		vs := checkMicroflowSource(t, src)
		if _, bad := violationsByRule(vs)["MDL061"]; bad {
			t.Errorf("MDL061 fired on an initialized declare: %s", src)
		}
	}
}

// A list or object declare is already rejected by MDL040/MDL043, which say the
// activity cannot hold that type at all. Adding "and it needs a value" on top is
// noise pointing at the wrong fix — the user must not add `= empty`, they must
// stop declaring it.
func TestMDL061_DefersToTheTypeRules(t *testing.T) {
	vs := checkMicroflowSource(t, `create microflow Synthetic.MF_List ()
begin
  declare $Items list of Synthetic.Item;
end;`)
	ids := violationsByRule(vs)
	if _, ok := ids["MDL040"]; !ok {
		t.Fatalf("expected MDL040 for a list declare, got %#v", vs)
	}
	if _, bad := ids["MDL061"]; bad {
		t.Error("MDL061 must not pile onto a list declare — MDL040 already says the activity cannot hold a list")
	}
}

// upstream #893 item 2. A `return` inside a loop builds an End event inside the
// LoopedActivity, which Mendix forbids: CE0068 "End events cannot be placed
// inside a loop." Measured on mxbuild 11.6.6; replacing it with `break` took the
// same app back to its 1-error baseline.
func TestMDL062_ReturnInsideLoop(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "directly in a for-loop body (the reported case)",
			src: `create microflow Synthetic.MF_LoopReturn ()
begin
  retrieve $PartList from Synthetic.Part;
  loop $Part in $PartList
  begin
    return;
  end loop;
end;`,
		},
		{
			name: "in a while body",
			src: `create microflow Synthetic.MF_WhileReturn ()
begin
  declare $Go Boolean = true;
  while $Go
  begin
    return;
  end while;
end;`,
		},
		{
			name: "nested inside a branch inside the loop — still inside the loop",
			src: `create microflow Synthetic.MF_LoopIfReturn ()
begin
  retrieve $PartList from Synthetic.Part;
  loop $Part in $PartList
  begin
    if 1 = 1 then
      return;
    end if;
  end loop;
end;`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := violationsByRule(checkMicroflowSource(t, tc.src))["MDL062"]
			if !ok {
				t.Fatal("expected MDL062 for a return inside a loop")
			}
			if got.Severity != linter.SeverityError {
				t.Errorf("MDL062 severity = %v, want Error (mxbuild rejects it with CE0068)", got.Severity)
			}
			if !strings.Contains(got.Suggestion, "break") {
				t.Errorf("MDL062 should point at `break`, the construct that replaces it: %q", got.Suggestion)
			}
		})
	}
}

// The control for MDL062: a return AFTER the loop is the normal shape and must
// stay clean, or the rule makes every loop-bearing microflow unwritable.
func TestMDL062_ReturnAfterLoopIsClean(t *testing.T) {
	vs := checkMicroflowSource(t, `create microflow Synthetic.MF_ReturnAfter () returns Boolean
begin
  retrieve $PartList from Synthetic.Part;
  loop $Part in $PartList
  begin
    break;
  end loop;
  return true;
end;`)
	if _, bad := violationsByRule(vs)["MDL062"]; bad {
		t.Errorf("MDL062 fired on a return placed after the loop: %#v", vs)
	}
}

// upstream #893 item 6, generalised to the rule mxbuild actually enforces.
//
// A microflow's variable namespace is FLAT: branches and loops do not scope it.
// Measured on mxbuild 11.6.6, each of these is CE0111 "Duplicate variable name"
// and each was isolated in its own microflow to attribute the error:
//
//	declare $X + `$X = call microflow …`   (the reported case)
//	declare $X + declare $X                 same body
//	declare $X + declare $X                 one outside a loop, one inside it
//	declare $X + declare $X                 in two SIBLING if/else branches
//	retrieve $L + retrieve $L
//	parameter Name + declare $Name
//	loop iterator $I + declare $I
func TestMDL063_DuplicateVariableNames(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "declare then an activity output of the same name (the reported case)",
			src: `create microflow Synthetic.MF_Shadow ()
begin
  declare $Session String = '';
  $Session = call microflow Synthetic.Helper();
end;`,
		},
		{
			name: "two declares in the same body",
			src: `create microflow Synthetic.MF_TwoDeclares ()
begin
  declare $X String = 'a';
  declare $X String = 'b';
end;`,
		},
		{
			name: "a loop body does not open a new scope",
			src: `create microflow Synthetic.MF_LoopScope ()
begin
  declare $X String = 'a';
  retrieve $L from Synthetic.Item;
  loop $I in $L
  begin
    declare $X String = 'b';
  end loop;
end;`,
		},
		{
			name: "sibling branches do not isolate either",
			src: `create microflow Synthetic.MF_Branches ()
begin
  if 1 = 1 then
    declare $X String = 'a';
  else
    declare $X String = 'b';
  end if;
end;`,
		},
		{
			name: "two retrieves into the same name",
			src: `create microflow Synthetic.MF_TwoRetrieves ()
begin
  retrieve $L from Synthetic.Item;
  retrieve $L from Synthetic.Item;
end;`,
		},
		{
			name: "a parameter is a variable too",
			src: `create microflow Synthetic.MF_ParamClash (Name: String)
begin
  declare $Name String = 'x';
end;`,
		},
		{
			name: "so is a loop iterator",
			src: `create microflow Synthetic.MF_IterClash ()
begin
  retrieve $L from Synthetic.Item;
  loop $I in $L
  begin
    declare $I String = 'x';
  end loop;
end;`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := violationsByRule(checkMicroflowSource(t, tc.src))["MDL063"]
			if !ok {
				t.Fatal("expected MDL063 for a duplicated variable name")
			}
			if got.Severity != linter.SeverityError {
				t.Errorf("MDL063 severity = %v, want Error (mxbuild rejects it with CE0111)", got.Severity)
			}
		})
	}
}

// The control for MDL063, and the distinction the rule turns on: assigning to an
// existing variable is a Change Variable activity, which creates nothing and is
// exactly how a declared variable is meant to be updated. Measured clean on
// mxbuild 11.6.6. Getting this wrong would flag the normal idiom.
func TestMDL063_AssignmentToADeclaredVariableIsClean(t *testing.T) {
	vs := checkMicroflowSource(t, `create microflow Synthetic.MF_Assign ()
begin
  declare $X String = 'a';
  $X = 'b';
  $X = 'c';
end;`)
	if _, bad := violationsByRule(vs)["MDL063"]; bad {
		t.Errorf("MDL063 fired on a plain reassignment, which is a Change Variable and always valid: %#v", vs)
	}
}

// Distinct names in every producing position must stay clean — the rule keys on
// the NAME, so a bug that keyed on the statement kind instead would fire here.
func TestMDL063_DistinctNamesAreClean(t *testing.T) {
	vs := checkMicroflowSource(t, `create microflow Synthetic.MF_Distinct (In: String)
begin
  declare $A String = 'a';
  retrieve $L from Synthetic.Item;
  loop $I in $L
  begin
    declare $B String = 'b';
  end loop;
  $C = call microflow Synthetic.Helper();
end;`)
	if _, bad := violationsByRule(vs)["MDL063"]; bad {
		t.Errorf("MDL063 fired although every producer has a distinct name: %#v", vs)
	}
}

// MDL052 already owns iterator-vs-iterator, with a message about loop scoping
// that MDL063's generic wording would not improve on. Two rules firing on one
// line is noise, so MDL063 stands aside for that pair only.
func TestMDL063_LeavesIteratorPairsToMDL052(t *testing.T) {
	ids := violationsByRule(checkMicroflowSource(t, `create microflow Synthetic.MF_TwoLoops ()
begin
  retrieve $L from Synthetic.Item;
  loop $R in $L
  begin
    log info node 'x' 'y';
  end loop;
  loop $R in $L
  begin
    log info node 'x' 'y';
  end loop;
end;`))
	if _, ok := ids["MDL052"]; !ok {
		t.Fatal("expected MDL052 for two loops sharing an iterator name")
	}
	if _, bad := ids["MDL063"]; bad {
		t.Error("MDL063 must not double-report what MDL052 already covers")
	}
}

// The exemptions below are not judgement calls — each is a shape that mxbuild
// 11.6.6 accepts, found by running the rules over the shipped examples before
// wiring them in. Without them these rules would refuse MDL that builds clean,
// which is the failure mode execEnforcedMicroflowRules warns about with MDL009.

// #350's manual retry loop. `while true` is built as an ExclusiveMerge back-edge
// rather than a LoopedActivity, so there is no loop object for the End event to
// be inside. Measured: 0 errors above baseline, no CE0068.
func TestMDL062_ExemptsWhileTrue(t *testing.T) {
	vs := checkMicroflowSource(t, `create microflow Synthetic.MF_ManualRetry (Flag: Boolean)
begin
  while true
  begin
    if $Flag then
      return;
    end if;
    continue;
  end while;
end;`)
	if _, bad := violationsByRule(vs)["MDL062"]; bad {
		t.Errorf("MDL062 fired on a `while true` manual loop, which builds no LoopedActivity: %#v", vs)
	}
}

// ... but a `while` with a real condition IS a LoopedActivity. Measured CE0068,
// so the exemption must be keyed on the literal `true`, not on `while`.
func TestMDL062_WhileWithConditionIsNotExempt(t *testing.T) {
	vs := checkMicroflowSource(t, `create microflow Synthetic.MF_CondWhile (Flag: Boolean)
begin
  while $Flag
  begin
    return;
  end while;
end;`)
	if _, ok := violationsByRule(vs)["MDL062"]; !ok {
		t.Errorf("MDL062 must still fire for `while <cond>` — measured CE0068 on mxbuild: %#v", vs)
	}
}

// With `returns T as $Var` the builder takes the End event's value from the
// variable and none lands inside the loop. Measured: no CE0068 (that shape
// builds CE0109 instead, a different defect this rule must not mislabel).
func TestMDL062_ExemptsReturnsAsClause(t *testing.T) {
	vs := checkMicroflowSource(t, `create microflow Synthetic.MF_AsClause () returns Boolean as $Done
begin
  retrieve $L from Synthetic.Item;
  loop $I in $L
  begin
    return true;
  end loop;
end;`)
	if _, bad := violationsByRule(vs)["MDL062"]; bad {
		t.Errorf("MDL062 fired on a `returns … as $Var` microflow, where no End event lands in the loop: %#v", vs)
	}
}

// `set $Match = contains($Hay, $Needle)` on two Strings parses as a
// ListOperationStmt, but addListOperationAction rewrites it to a Change
// Variable exactly to avoid this CE0111 (ledger #53/#63). Both examples build
// at baseline; an earlier draft of MDL063 flagged them.
func TestMDL063_ExemptsStringOverloadedListOps(t *testing.T) {
	for _, src := range []string{
		`create microflow Synthetic.MF_C (Hay: String, Needle: String) returns Boolean
begin
  declare $Match Boolean = false;
  set $Match = contains($Hay, $Needle);
  return $Match;
end;`,
		`create microflow Synthetic.MF_F (Raw: String, Needle: String) returns Integer
begin
  declare $At Integer = 0;
  set $At = find($Raw, $Needle);
  return $At;
end;`,
	} {
		vs := checkMicroflowSource(t, src)
		if _, bad := violationsByRule(vs)["MDL063"]; bad {
			t.Errorf("MDL063 fired on a string function the builder rewrites to a Change Variable: %s\n%#v", src, vs)
		}
	}
}

// A genuine list operation still creates its output variable, so a declare in
// front of one is still CE0111 — the exemption is keyed on the input being a
// declared String, not on the operation name alone.
func TestMDL063_GenuineListOperationIsNotExempt(t *testing.T) {
	vs := checkMicroflowSource(t, `create microflow Synthetic.MF_ListC (Items: list of Synthetic.Item, One: Synthetic.Item) returns Boolean
begin
  declare $Found Boolean = false;
  set $Found = contains($Items, $One);
  return $Found;
end;`)
	if _, ok := violationsByRule(vs)["MDL063"]; !ok {
		t.Errorf("MDL063 must still fire for a real list operation feeding a pre-declared variable: %#v", vs)
	}
}

// mxbuild does not check an @excluded document: measured, the microflow that is
// CE0111 when included produces no error at all when excluded. Because these
// rules are error-severity they block `exec`, so flagging excluded scaffolding
// would undo #312 by another route.
func TestCEGapRules_SkipExcludedMicroflows(t *testing.T) {
	vs := checkMicroflowSource(t, `@excluded
create microflow Synthetic.MF_Excluded () returns String as $msg
begin
  declare $X String;
  declare $msg String = '';
  $msg = call microflow Synthetic.NoSuchTarget();
  retrieve $L from Synthetic.Item;
  loop $I in $L
  begin
    return $msg;
  end loop;
end;`)
	for _, id := range []string{"MDL061", "MDL062", "MDL063"} {
		if _, bad := violationsByRule(vs)[id]; bad {
			t.Errorf("%s fired on an @excluded microflow, which mxbuild never checks: %#v", id, vs)
		}
	}
}
