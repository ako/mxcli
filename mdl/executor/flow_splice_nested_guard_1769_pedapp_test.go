// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
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
