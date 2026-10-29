// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1769 (mendixlabs/mxcli#1673) on the Studio Pro-authored PedApp,
// through the real codec: the reported alter, which replaces a loop with its
// own text, wrote the find in its body as Microflows$FindByExpression `Code =
// $Lookup/Code` — check and exec silent, CE0117 from mx check — where `create
// microflow` writes Microflows$Find on MyModule.OrderLine.Code.

const matchLinesSetup = `mdl 1;
create module MyModule;
create persistent entity MyModule.Order (Code: String(50));
create persistent entity MyModule.OrderLine (Code: String(50), Status: String(20));
create association MyModule.OrderLine_Order from MyModule.OrderLine to MyModule.Order;
create non-persistent entity MyModule.Lookup (Code: String(50));
create microflow MyModule.SUB_MatchLines ($Order: MyModule.Order, $Lookups: List of MyModule.Lookup) returns Integer as $Found
begin
  declare $Found Integer = 0;
  retrieve $Lines from $Order/MyModule.OrderLine_Order;
  $Open = filter $Lines by Status = 'open';
  loop $Lookup in $Lookups
  begin
    $Line = find $Lines by Code = $Lookup/Code;
    if $Line != empty then
      set $Found = $Found + 1;
    end if;
  end loop;
  return $Found;
end;`

// matchLinesFindIn returns, for each way an alter can write a find, the
// statement that writes one in list: the reported replace of the loop with
// its own text, a replace of its body, and plain inserts in the stored body
// and on the main line, which the guard of ako/mxcli#891 always covered.
func matchLinesFindIn(list string) map[string]string {
	find := `$Line = find ` + list + ` by Code = $Lookup/Code;`
	return map[string]string{
		"replace loop": `mdl 1;
alter microflow MyModule.SUB_MatchLines {
  replace loop $Lookup in $Lookups with begin
    loop $Lookup in $Lookups
    begin
      ` + find + `
      if $Line != empty then
        set $Found = $Found + 1;
      end if;
    end loop;
  end;
};`,
		"replace body of loop": `mdl 1;
alter microflow MyModule.SUB_MatchLines {
  replace body of loop $Lookup in $Lookups with begin
    ` + find + `
    if $Line != empty then
      set $Found = $Found + 1;
    end if;
  end;
};`,
		"insert in the loop body": `mdl 1;
alter microflow MyModule.SUB_MatchLines {
  insert after $Line begin $Again = find ` + list + ` by Code = $Lookup/Code; end;
};`,
		"insert on the main line": `mdl 1;
alter microflow MyModule.SUB_MatchLines {
  insert after $Open begin $First = find ` + list + ` by Code = 'x'; end;
};`,
	}
}

func openMatchLinesFixture(t *testing.T) (*Executor, *bytes.Buffer, string) {
	t.Helper()
	exec, out, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, matchLinesSetup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	return exec, out, dir
}

// TestAlterMicroflow_FindOverUntypedListIsRefused: over a list whose entity
// the fragment does not know — the stored flow does not type a list
// operation's output — every alter is refused and writes nothing. On the main
// line and in a stored loop body it always was (ako/mxcli#891); in a loop the
// fragment brings, it was written as a find by expression. Check does not
// pass it silently: like the main-line refusal, it names the change as not
// predicted with exec's reason.
func TestAlterMicroflow_FindOverUntypedListIsRefused(t *testing.T) {
	const want = `cannot qualify member "Code"`
	for name, src := range matchLinesFindIn("$Open") {
		t.Run(name, func(t *testing.T) {
			exec, _, dir := openMatchLinesFixture(t)
			before := projectFiles(t, dir)
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v\n%s", errs[0], src)
			}
			r := exec.CheckFlowVerdictsReport(prog)
			var said []string
			for _, u := range r.Unpredicted {
				said = append(said, u.Reason)
			}
			for _, v := range r.Violations {
				said = append(said, v.Message)
			}
			if !strings.Contains(strings.Join(said, "\n"), want) {
				t.Errorf("check must name the refusal %q, said: %v", want, said)
			}
			if err := exec.ExecuteProgram(prog); err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("exec must refuse with %q, got: %v", want, err)
			}
			if changed := diffProjectFiles(before, projectFiles(t, dir)); len(changed) != 0 {
				t.Errorf("a refused alter wrote %v", changed)
			}
		})
	}
}

// matchLinesFlow returns the stored MyModule.SUB_MatchLines.
func matchLinesFlow(t *testing.T, exec *Executor) *microflows.Microflow {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Name == "SUB_MatchLines" && h.GetModuleName(h.FindModuleID(m.ContainerID)) == "MyModule" {
			mf, err := ctx.Backend.GetMicroflow(m.ID)
			if err != nil {
				t.Fatal(err)
			}
			return mf
		}
	}
	t.Fatal("MyModule.SUB_MatchLines not found")
	return nil
}

// TestAlterMicroflow_FindOverAssociationListIsByAttribute: every alter that
// writes `find $Lines by Code = …`, where $Lines is retrieved over an
// association before it, writes Microflows$Find on MyModule.OrderLine.Code,
// as `create microflow` does; check predicts it and exec writes it.
func TestAlterMicroflow_FindOverAssociationListIsByAttribute(t *testing.T) {
	for name, src := range matchLinesFindIn("$Lines") {
		t.Run(name, func(t *testing.T) {
			exec, _, _ := openMatchLinesFixture(t)
			predictedAgree(t, exec, src, "")
			var finds int
			for _, op := range fragmentListOperations(matchLinesFlow(t, exec).ObjectCollection.Objects) {
				switch f := op.(type) {
				case *microflows.FindOperation:
					t.Errorf("a find over $%s was stored by the expression %q", f.ListVariable, f.Expression)
				case *microflows.FindByAttributeOperation:
					finds++
					if f.Attribute != "MyModule.OrderLine.Code" {
						t.Errorf("a find over $%s is on %q, want MyModule.OrderLine.Code", f.ListVariable, f.Attribute)
					}
				}
			}
			if finds == 0 {
				t.Errorf("no find by attribute was stored")
			}
		})
	}
}

// The reported alter writes what describe prints back unchanged: exec of the
// describe output reports Unchanged.
func TestAlterMicroflow_ReplacedLoopDescribeRoundTrips(t *testing.T) {
	exec, out, _ := openMatchLinesFixture(t)
	predictedAgree(t, exec, matchLinesFindIn("$Lines")["replace loop"], "")
	var desc bytes.Buffer
	saved := exec.output
	exec.output = &desc
	err := afRun(t, exec, "describe microflow MyModule.SUB_MatchLines;")
	exec.output = saved
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(desc.String(), "find $Lines by Code = $Lookup/Code;") {
		t.Errorf("describe does not print the find by attribute:\n%s", desc.String())
	}
	out.Reset()
	if err := afRun(t, exec, "mdl 1;\n"+desc.String()); err != nil {
		t.Fatalf("exec of the describe output: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Unchanged") {
		t.Errorf("exec of the describe output must report Unchanged, got:\n%s", out.String())
	}
}

// A stored loop's iterator is typed from its list, so a member of it in a
// fragment inserted in the loop body is qualified; it was refused.
func TestAlterMicroflow_ChangeOnStoredIteratorIsQualified(t *testing.T) {
	exec, _, _ := openMatchLinesFixture(t)
	predictedAgree(t, exec, `mdl 1;
alter microflow MyModule.SUB_MatchLines {
  insert after $Line begin change $Lookup (Code = 'x'); end;
};`, "")
	var changed []string
	var walk func(objs []microflows.MicroflowObject)
	walk = func(objs []microflows.MicroflowObject) {
		for _, obj := range objs {
			switch o := obj.(type) {
			case *microflows.ActionActivity:
				if ch, ok := o.Action.(*microflows.ChangeObjectAction); ok {
					for _, m := range ch.Changes {
						changed = append(changed, m.AttributeQualifiedName)
					}
				}
			case *microflows.LoopedActivity:
				walk(o.ObjectCollection.Objects)
			}
		}
	}
	walk(matchLinesFlow(t, exec).ObjectCollection.Objects)
	if strings.Join(changed, ",") != "MyModule.Lookup.Code" {
		t.Errorf("the change was stored on %v, want [MyModule.Lookup.Code]", changed)
	}
}
