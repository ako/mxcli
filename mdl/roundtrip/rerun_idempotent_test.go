// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#859: under mdl 1, executing a script a second time must write
// nothing. The beta rehearsal found flows mxcli had itself just created from a
// script that the identical second run re-spliced every time, or refused.
//
// These are the classes whose cause is not the control-flow shape of the flow:
//
//   - S3: a member named fully qualified (`sort by M.Dept.SortOrder`,
//     `create M.Emp (M.Emp_Dept = $d)`, `find $L by M.Emp_Dept = $d`) is
//     stored as the member itself and described short, so the declared
//     statement never matched the stored one: "spliced: 1 replaced" on every
//     run, under mdl 0 as well. Inside a loop body that is S6's refusal ("the
//     Loop changes inside its body"), and next to an end event S7's ("no free
//     room for the fragment"); two phantoms either side of a declaration or a
//     guard's return merged into one fragment that S5 ("the fragment declares
//     $X, which the microflow already has") and S4 ("the fragment returns")
//     refused.
//   - S8: a retrieve whose XPath has a nested predicate was described without
//     its outer brackets, which its own parser rejects, so the stored flow
//     could not be compared at all.
//
// Every case runs the script twice on a fresh copy and requires the second run
// to report Unchanged with no unit written. Each has a control: an edit of the
// same construct is written, so the comparison is not simply blind to it.

const rerunDomain = `create or modify persistent entity MyFirstModule.Dept (
  Name: String(100),
  SortOrder: Integer
);
create or modify persistent entity MyFirstModule.Emp (
  Name: String(100),
  Rank: Integer
);
create or modify association MyFirstModule.Emp_Dept from MyFirstModule.Emp to MyFirstModule.Dept;
`

type rerunCase struct {
	name, flow, target string
	// edit rewrites the flow into one that stores something else; "" = no
	// control for this case (the construct is covered by another's).
	from, to string
}

var rerunCases = []rerunCase{
	{
		name:   "S3 retrieve sort by a qualified attribute",
		target: "MyFirstModule.Rerun_Sort",
		flow: `create or modify microflow MyFirstModule.Rerun_Sort ()
returns List of MyFirstModule.Dept as $Departments
begin
  retrieve $Departments from MyFirstModule.Dept sort by MyFirstModule.Dept.SortOrder asc, MyFirstModule.Dept.Name asc;
  return $Departments;
end;
`,
		from: "MyFirstModule.Dept.SortOrder asc", to: "MyFirstModule.Dept.Name desc",
	},
	{
		name:   "S3 create with a qualified association and attribute",
		target: "MyFirstModule.Rerun_Create",
		flow: `create or modify microflow MyFirstModule.Rerun_Create ()
returns MyFirstModule.Emp as $New
begin
  retrieve $First from MyFirstModule.Dept first;
  $New = create MyFirstModule.Emp (MyFirstModule.Emp.Name = 'x', MyFirstModule.Emp_Dept = $First);
  return $New;
end;
`,
		from: "MyFirstModule.Emp.Name = 'x'", to: "MyFirstModule.Emp.Rank = 1",
	},
	{
		name:   "S3 change with a bare association and a qualified attribute",
		target: "MyFirstModule.Rerun_Change",
		flow: `create or modify microflow MyFirstModule.Rerun_Change ($Emp: MyFirstModule.Emp, $Dept: MyFirstModule.Dept)
begin
  change $Emp (MyFirstModule.Emp.Name = 'x', Emp_Dept = $Dept);
end;
`,
		from: "MyFirstModule.Emp.Name = 'x'", to: "MyFirstModule.Emp.Rank = 1",
	},
	{
		name:   "S3 find by a qualified association",
		target: "MyFirstModule.Rerun_Find",
		flow: `create or modify microflow MyFirstModule.Rerun_Find ($Dept: MyFirstModule.Dept, $Emps: List of MyFirstModule.Emp)
returns MyFirstModule.Emp as $Emp
begin
  $Emp = find $Emps by MyFirstModule.Emp_Dept = $Dept;
  return $Emp;
end;
`,
		from: "find $Emps by MyFirstModule.Emp_Dept = $Dept", to: "find $Emps by MyFirstModule.Emp.Name = $Dept/Name",
	},
	{
		// The statement that declares the variable does not name its entity:
		// a microflow call's result, a retrieve over an association, a loop
		// over that list. Only the builder knows what they hold.
		name:   "S3 qualified members of variables whose entity is not stated",
		target: "MyFirstModule.Rerun_Unstated",
		flow: `create or modify microflow MyFirstModule.Rerun_NewEmp ()
returns MyFirstModule.Emp as $E
begin
  $E = create MyFirstModule.Emp (Name = 'a');
  return $E;
end;
create or modify microflow MyFirstModule.Rerun_Unstated ($Start: MyFirstModule.Emp)
returns Boolean
begin
  $Made = call microflow MyFirstModule.Rerun_NewEmp();
  change $Made (MyFirstModule.Emp.Rank = 3);
  retrieve $Dept from $Start/MyFirstModule.Emp_Dept;
  change $Dept (MyFirstModule.Dept.SortOrder = 1);
  retrieve $Colleagues from $Dept/MyFirstModule.Emp_Dept;
  $Named = find $Colleagues by MyFirstModule.Emp.Name = 'x';
  loop $Colleague in $Colleagues
  begin
    change $Colleague (MyFirstModule.Emp.Rank = 2);
  end loop;
  return true;
end;
`,
		from: "change $Made (MyFirstModule.Emp.Rank = 3)", to: "change $Made (MyFirstModule.Emp.Name = 'y')",
	},
	{
		name:   "S5 qualified sort and create around a declaration",
		target: "MyFirstModule.Rerun_SortThenCreate",
		flow: `create or modify microflow MyFirstModule.Rerun_SortThenCreate ()
returns MyFirstModule.Emp as $New
begin
  retrieve $First from MyFirstModule.Dept sort by MyFirstModule.Dept.SortOrder asc first;
  $New = create MyFirstModule.Emp (Name = 'x', MyFirstModule.Emp_Dept = $First);
  return $New;
end;
`,
	},
	{
		name:   "S4 qualified find and create straddling a guard's return",
		target: "MyFirstModule.Rerun_FragmentReturns",
		flow: `create or modify microflow MyFirstModule.Rerun_FragmentReturns ($Dept: MyFirstModule.Dept, $Emps: List of MyFirstModule.Emp)
returns MyFirstModule.Emp as $Result
begin
  $Emp = find $Emps by MyFirstModule.Emp_Dept = $Dept;
  if $Emp = empty then
    $New = create MyFirstModule.Emp (Name = 'new', MyFirstModule.Emp_Dept = $Dept);
    return $New;
  end if;
  change $Emp (Name = 'x');
  return $Emp;
end;
`,
	},
	{
		name:   "S6 a qualified member inside a loop body",
		target: "MyFirstModule.Rerun_LoopBody",
		flow: `create or modify microflow MyFirstModule.Rerun_LoopBody ($Depts: List of MyFirstModule.Dept)
returns List of MyFirstModule.Emp as $Rows
begin
  $Rows = create list of MyFirstModule.Emp;
  loop $Dept in $Depts
  begin
    $Row = create MyFirstModule.Emp (Name = $Dept/Name, MyFirstModule.Emp_Dept = $Dept);
    add $Row to $Rows;
  end loop;
  return $Rows;
end;
`,
	},
	{
		name:   "S7 guard, qualified sort and a loop with a qualified create",
		target: "MyFirstModule.Rerun_NoFreeRoom",
		flow: `create or modify microflow MyFirstModule.Rerun_NoFreeRoom ($Guard: Boolean)
returns List of MyFirstModule.Emp as $Rows
begin
  $Rows = create list of MyFirstModule.Emp;
  if $Guard then
    return $Rows;
  end if;
  retrieve $Depts from MyFirstModule.Dept sort by MyFirstModule.Dept.SortOrder asc;
  loop $Dept in $Depts
  begin
    declare $Label String = '-';
    if $Dept/Name != empty then
      set $Label = $Dept/Name;
    end if;
    $Row = create MyFirstModule.Emp (Name = $Label, MyFirstModule.Emp_Dept = $Dept);
    add $Row to $Rows;
  end loop;
  return $Rows;
end;
`,
	},
	{
		name:   "S8 retrieve with a nested XPath predicate",
		target: "MyFirstModule.Rerun_NestedPredicate",
		flow: `create or modify microflow MyFirstModule.Rerun_NestedPredicate ()
returns List of MyFirstModule.Emp as $Missing
begin
  retrieve $Missing from MyFirstModule.Emp
    where [MyFirstModule.Emp_Dept/MyFirstModule.Dept[Name = empty]];
  return $Missing;
end;
`,
		from: "[Name = empty]", to: "[Name = 'x']",
	},
}

func TestFlowRerun_IdenticalScriptWritesNothing(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	for _, c := range rerunCases {
		for _, header := range []string{"mdl 1;\n", ""} {
			dialect := map[string]string{"": "mdl 0", "mdl 1;\n": "mdl 1"}[header]
			t.Run(c.name+"/"+dialect, func(t *testing.T) {
				h.restore()
				script := header + rerunDomain + c.flow
				if err := h.exec(script); err != nil {
					t.Fatalf("first run: %v\n%s", err, h.out.String())
				}
				first := h.snapshot()
				if err := h.exec(script); err != nil {
					t.Fatalf("the identical second run is refused: %v\n%s", err, h.out.String())
				}
				out := h.out.String()
				if strings.Contains(out, "MDL-V1-REBUILD") {
					t.Errorf("the second run did not match the stored flow and rebuilt it:\n%s", out)
				}
				// Unchanged statements collapse into the run's summary.
				if strings.Contains(out, "Modified") || strings.Contains(out, "Created") ||
					!strings.Contains(out, "documents already in sync") {
					t.Errorf("the second run did not report every statement Unchanged:\n%s", out)
				}
				if changed := first.diff(h.snapshot()); len(changed) != 0 {
					t.Errorf("the second run wrote %d unit(s):\n  %s", len(changed), strings.Join(changed, "\n  "))
				}
				if c.from == "" {
					return
				}
				// Control: the same construct, edited, is a change.
				edited := strings.Replace(script, c.from, c.to, 1)
				if edited == script {
					t.Fatalf("the flow has no %q", c.from)
				}
				if err := h.exec(edited); err != nil {
					t.Fatalf("an edited %s: %v\n%s", c.name, err, h.out.String())
				}
				if !strings.Contains(h.out.String(), "Modified microflow: "+c.target) {
					t.Errorf("an edit did not report Modified:\n%s", h.out.String())
				}
				if len(first.diff(h.snapshot())) == 0 {
					t.Error("an edit wrote nothing")
				}
			})
		}
	}
}

// ako/mxcli#859 (rehearsal W1): `create or modify view entity` on an existing
// view entity rebuilt it from the statement alone and dropped its access
// rules. Every re-run of a script wrote the domain model and its `grant` wrote
// the rule back; a re-run without the grant silently left the entity with no
// access at all.
func TestViewEntityRerun_KeepsAccessRules(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const view = `create or modify view entity MyFirstModule.RerunYear (
  Yr: Integer,
  TxCount: Integer
) as (
  select
    datepart(YEAR, t.TxDate) as Yr,
    count(*) as TxCount
  from MyFirstModule.RerunTx as t
  group by datepart(YEAR, t.TxDate)
);
`
	const grant = "grant read * on entity MyFirstModule.RerunYear to MyFirstModule.User;\n"
	const script = `mdl 1;
create or modify persistent entity MyFirstModule.RerunTx (
  TxDate: DateTime,
  Amount: Decimal
);
` + view + grant
	if err := h.exec(script); err != nil {
		t.Fatalf("first run: %v\n%s", err, h.out.String())
	}
	first := h.snapshot()
	if err := h.exec(script); err != nil {
		t.Fatalf("second run: %v\n%s", err, h.out.String())
	}
	if strings.Contains(h.out.String(), "Modified view entity") {
		t.Errorf("the identical second run modified the view entity:\n%s", h.out.String())
	}
	if changed := first.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the identical second run wrote %d unit(s):\n  %s", len(changed), strings.Join(changed, "\n  "))
	}

	// The view entity alone, without the grant, keeps the rule.
	if err := h.exec("mdl 1;\n" + view); err != nil {
		t.Fatalf("the view entity alone: %v\n%s", err, h.out.String())
	}
	if changed := first.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the unchanged view entity wrote %d unit(s):\n  %s", len(changed), strings.Join(changed, "\n  "))
	}
	if got := h.mustDescribeMdl0(t, "entity MyFirstModule.RerunYear"); !strings.Contains(got, strings.TrimSpace(grant)) {
		t.Errorf("the access rule is gone:\n%s", got)
	}

	// Control: a changed view entity is written, and keeps its rule too.
	edited := strings.Replace(view, "count(*) as TxCount", "count(t.Amount) as TxCount", 1)
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("an edited view entity: %v\n%s", err, h.out.String())
	}
	if len(first.diff(h.snapshot())) == 0 {
		t.Error("an edited view entity wrote nothing")
	}
	if got := h.mustDescribeMdl0(t, "entity MyFirstModule.RerunYear"); !strings.Contains(got, "count(t.Amount)") ||
		!strings.Contains(got, strings.TrimSpace(grant)) {
		t.Errorf("an edited view entity: want the new query and the rule kept:\n%s", got)
	}
}

// ako/mxcli#859 (rehearsal W2): `create or modify demo user` for a demo user
// that already holds exactly what the statement states removed and re-added
// it, so project security was written and "Modified demo user" reported on
// every run.
func TestDemoUserRerun_WritesNothing(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const user = "mdl 1;\ncreate or modify demo user 'rerun_a' ( Password: 'Rerun2027!pw', Entity: Administration.Account, UserRoles: (User) );\n"
	if err := h.exec(user); err != nil {
		t.Fatalf("first run: %v\n%s", err, h.out.String())
	}
	first := h.snapshot()
	if err := h.exec(user); err != nil {
		t.Fatalf("second run: %v\n%s", err, h.out.String())
	}
	if !strings.Contains(h.out.String(), "Unchanged demo user: rerun_a") {
		t.Errorf("the identical second run did not report Unchanged:\n%s", h.out.String())
	}
	if changed := first.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the identical second run wrote %d unit(s):\n  %s", len(changed), strings.Join(changed, "\n  "))
	}

	// Controls: another password, and another role, are each a change.
	for _, edit := range [][2]string{{"Rerun2027!pw", "Rerun2028!pw"}, {"(User)", "(User, Administrator)"}} {
		if err := h.exec(strings.Replace(user, edit[0], edit[1], 1)); err != nil {
			t.Fatalf("edited %s: %v\n%s", edit[1], err, h.out.String())
		}
		if !strings.Contains(h.out.String(), "Modified demo user: rerun_a") {
			t.Errorf("edited %s: want Modified:\n%s", edit[1], h.out.String())
		}
		if len(first.diff(h.snapshot())) == 0 {
			t.Errorf("edited %s: nothing written", edit[1])
		}
		first = h.snapshot()
	}
}

// ako/mxcli#859 (rehearsal V1): a view entity attribute declared Long over an
// AutoNumber source column was accepted when the source entity was created in
// the same run (its type could not be inferred yet) and refused on every run
// after, as a reference error — so the script could never be re-run. An
// AutoNumber is a Long the database fills; the model mx check accepts states
// the view attribute as Long.
func TestViewEntityRerun_LongOverAutoNumber(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const script = `create or modify persistent entity MyFirstModule.RerunLine (
  LineNo: AutoNumber default 1
);
create or modify view entity MyFirstModule.RerunVLine (
  LineNo: Long
) as (
  select l.LineNo as LineNo
  from MyFirstModule.RerunLine as l
);
`
	for _, header := range []string{"", "mdl 1;\n"} {
		if err := h.exec(header + script); err != nil {
			t.Fatalf("run under %q: %v\n%s", header, err, h.out.String())
		}
		if errs := h.checkReferences(header + script); len(errs) > 0 {
			t.Errorf("check --references under %q once the source exists: %v", header, errs)
		}
	}
	// Control: a type an AutoNumber is not is still a mismatch.
	if errs := h.checkReferences(strings.Replace(script, "LineNo: Long", "LineNo: Boolean", 1)); len(errs) == 0 {
		t.Error("a Boolean over an AutoNumber column was accepted")
	}
}

// checkReferences runs the reference validation `mxcli exec` runs before it
// executes a script (`check --references`), against the working copy.
func (h *harness) checkReferences(script string) []error {
	h.t.Helper()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		h.t.Fatalf("parse: %v", errs[0])
	}
	return h.exe.ValidateProgram(prog)
}

// ako/mxcli#859 (rehearsal M4): the splice's scope checks found variables by
// matching `$name` in an activity's text, string literals included, so
// replacing `set $Url = '…?$filter=' + $F` was refused under mdl 1 as "the
// fragment uses $filter, which is not declared". A `$` inside a string is text.
func TestFlowModify_DollarInsideAStringIsNotAVariable(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const flow = `create or modify microflow MyFirstModule.Rerun_StringDollar ($Filter: String)
returns String as $Url
begin
  declare $Url String = '';
  set $Url = '/odata/v1?$filter=' + $Filter;
  return $Url;
end;
`
	if err := h.exec(flow); err != nil {
		t.Fatalf("create: %v\n%s", err, h.out.String())
	}
	edited := "mdl 1;\n" + strings.Replace(flow, "/odata/v1", "/odata/v2", 1)
	for run := 1; run <= 2; run++ {
		before := h.snapshot()
		if err := h.exec(edited); err != nil {
			t.Fatalf("run %d of the edited flow: %v\n%s", run, err, h.out.String())
		}
		written := len(before.diff(h.snapshot()))
		switch {
		case run == 1 && (written == 0 || !strings.Contains(h.out.String(), "(spliced: 1 edited in place)")):
			t.Errorf("run 1: want the edited set spliced in, %d unit(s) written:\n%s", written, h.out.String())
		case run == 2 && written != 0:
			t.Errorf("run 2 of the same script wrote %d unit(s):\n%s", written, h.out.String())
		}
	}
	// Control: a `$name` outside a string that is not declared is refused.
	undeclared := "mdl 1;\n" + strings.Replace(flow, "'/odata/v1?$filter=' + $Filter", "'/odata/v3?' + $Missing", 1)
	if err := h.exec(undeclared); err == nil || !strings.Contains(err.Error(), "$Missing") {
		t.Errorf("an undeclared variable outside a string: got %v, want a refusal naming $Missing", err)
	}
}
