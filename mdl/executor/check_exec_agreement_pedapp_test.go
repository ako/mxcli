// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// check -p and exec must not disagree (ako/mxcli#906, mendixlabs/mxcli#1210 –
// #1213, ako/mxcli#558, #563). Each case below passed `check -p` and was then
// refused by exec — after the statements before it had been written — or was
// accepted by both and broke the project. Every test pairs the case with a
// control that both accept, on the Studio Pro-authored PedApp fixture.

// agreeCheck is what `mxcli check -p` reports as an error: the no-project rules,
// the reference pass, and the project conflicts (cmd/mxcli/cmd_check.go).
func agreeCheck(t *testing.T, exec *Executor, dir, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	var out []string
	for _, v := range ValidateProgram(prog, filepath.Join(dir, "PedApp.mpr")) {
		if v.Severity == linter.SeverityError {
			out = append(out, v.RuleID+": "+v.Message)
		}
	}
	refErrs, _ := exec.ValidateProgramWithWarnings(prog)
	refErrs = append(refErrs, exec.CheckProjectConflicts(prog)...)
	for _, e := range refErrs {
		out = append(out, e.Error())
	}
	return out
}

// agreeExec runs the script's statements as `exec --no-check` does, so what is
// asserted is exec's own verdict, not the pre-flight it shares with check.
func agreeExec(t *testing.T, exec *Executor, src string) error {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	return exec.ExecuteProgram(prog)
}

// assertAgree runs check, then exec, and asserts both refuse (want != "",
// matched in both outputs) or both accept.
func assertAgree(t *testing.T, exec *Executor, dir, src, want string) {
	t.Helper()
	checkErrs := agreeCheck(t, exec, dir, src)
	execErr := agreeExec(t, exec, src)
	checkSays := strings.Join(checkErrs, "\n")
	if want == "" {
		if len(checkErrs) != 0 || execErr != nil {
			t.Errorf("control: check and exec must both accept\ncheck: %s\nexec: %v", checkSays, execErr)
		}
		return
	}
	if !strings.Contains(checkSays, want) {
		t.Errorf("check -p must predict the refusal %q, reported:\n%s", want, checkSays)
	}
	if execErr == nil || !strings.Contains(execErr.Error(), want) {
		t.Errorf("exec must refuse with %q, got: %v", want, execErr)
	}
}

// ako/mxcli#906: an existing demo user. The control is `create or modify`.
func TestCheckExecAgree_DemoUserExists(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const create = "mdl 1;\ncreate demo user 'agree_demo' ( Password: 'Agree!23456789', UserRoles: (User) );"
	if err := agreeExec(t, exec, create); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	assertAgree(t, exec, dir, create, "demo user already exists")
	assertAgree(t, exec, dir,
		"mdl 1;\ncreate or modify demo user 'agree_demo' ( Password: 'Agree!23456789', UserRoles: (User) );", "")
}

// ako/mxcli#906: an existing jar dependency. The control changes its version,
// which exec does through the same applyJarDepAction.
func TestCheckExecAgree_JarDependencyExists(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	const add = `mdl 1;
alter module MyFirstModule
  add jar dependency ( group = 'org.example', artifact = 'agree-lib', version = '1.0.0', included = true );`
	if err := agreeExec(t, exec, add); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	assertAgree(t, exec, dir, add, "jar dependency 'org.example:agree-lib' already exists")
	assertAgree(t, exec, dir,
		"mdl 1;\nalter module MyFirstModule set jar dependency 'org.example:agree-lib' version '1.0.1';", "")
	// The replay follows the script: an add after a drop of the same coordinate
	// is fine, and check must not flag it from the stored state.
	assertAgree(t, exec, dir, `mdl 1;
alter module MyFirstModule drop jar dependency 'org.example:agree-lib';
alter module MyFirstModule
  add jar dependency ( group = 'org.example', artifact = 'agree-lib', version = '1.0.2', included = true );`, "")
}

// ako/mxcli#906: translations into the project's source language (PedApp's is
// en_US). The control is another language.
func TestCheckExecAgree_SourceLanguageTranslations(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, "mdl 1;\ncreate or modify translations in MyFirstModule for en_US (\n  'Hello' as 'Hi'\n);",
		"en_US is the project's source language")
	assertAgree(t, exec, dir, "mdl 1;\ncreate or modify translations in MyFirstModule for nl_NL (\n  'Hello' as 'Hallo'\n);", "")
}

// mendixlabs/mxcli#1211 / ako/mxcli#558: a task queue the script creates
// before the call that names it was "not found" by check and by exec's
// pre-flight. The other half: the same queue created AFTER the call (as
// `create or modify`, which MDL-ORDER01 does not judge) is refused by both.
func TestCheckExecAgree_TaskQueueCreatedInScript(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	// The reporter's script, headerless (mdl 0) as filed.
	assertAgree(t, exec, dir, `create module "ModQ";
create or modify queue "ModQ"."TQ_Work" (Parallelism: 1);
create microflow "ModQ"."SUB_Work" ()
begin
  log info node 'Repro' 'work';
end;
/
create microflow "ModQ"."ACT_Enqueue" ()
begin
  call microflow "ModQ"."SUB_Work"() in queue "ModQ"."TQ_Work";
end;
/`, "")

	exec2, _, dir2 := openPedAppCopy(t)
	assertAgree(t, exec2, dir2, `mdl 1;
create module ModQ2;
create microflow ModQ2.SUB_Work ()
begin
  log info node 'Repro' 'work';
end;
create microflow ModQ2.ACT_Enqueue ()
begin
  call microflow ModQ2.SUB_Work() in queue ModQ2.TQ_Work;
end;
create or modify task queue ModQ2.TQ_Work (Parallelism: 1);`, "ModQ2.TQ_Work")
}

// mendixlabs/mxcli#1212: a page whose button calls a microflow the script
// creates further down. Control: the microflow first — its `show page` of the
// later page resolves lazily, so that order works.
func TestCheckExecAgree_PageCallsLaterMicroflow(t *testing.T) {
	const page = `create page ModO.ItemPage (
  Title: 'Item',
  Layout: Atlas_Core.PopupLayout,
  Params: ( $Item: ModO.Item )
) {
  dataview dvItem (DataSource: $Item) {
    textbox txtName (Attribute: Name)
    actionbutton btnReopen (Caption: 'Reopen', Action: call microflow ModO.ACT_OpenItem(Item = $Item))
  }
};
`
	const flow = `create microflow ModO.ACT_OpenItem ($Item: ModO.Item)
begin
  show page ModO.ItemPage(Item = $Item);
end;
`
	const head = "mdl 1;\ncreate module ModO;\ncreate persistent entity ModO.Item (Name: String(100));\n"

	exec, _, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, head+page+flow, "ModO.ACT_OpenItem")

	exec2, _, dir2 := openPedAppCopy(t)
	assertAgree(t, exec2, dir2, head+flow+page, "")

	// The later definition as `create or modify`: no project-free rule can
	// judge it, the project tier must.
	exec3, _, dir3 := openPedAppCopy(t)
	assertAgree(t, exec3, dir3, head+page+strings.Replace(flow, "create microflow", "create or modify microflow", 1),
		"ModO.ACT_OpenItem")
}

// mendixlabs/mxcli#1210: a variable passed to a Microflow-typed Java action
// parameter was written as the reference's text and the project no longer
// loaded. Control: the literal name.
func TestCheckExecAgree_MicroflowTypedJavaActionParameter(t *testing.T) {
	const head = `mdl 1;
create module ModA;
create java action ModA.RunNamed (Target: Microflow not null) returns Boolean as $$
return true;
$$;
create microflow ModA.SUB_Target () returns Boolean as $Ok
begin
  declare $Ok Boolean = true;
  return $Ok;
end;
`
	exec, _, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, head+`create microflow ModA.SUB_Variable ($Name: String) returns Boolean as $R
begin
  $R = call java action ModA.RunNamed(Target = $Name);
  return $R;
end;`, "parameter Target is of type Microflow")

	exec2, _, dir2 := openPedAppCopy(t)
	assertAgree(t, exec2, dir2, head+`create microflow ModA.SUB_Literal () returns Boolean as $R
begin
  $R = call java action ModA.RunNamed(Target = 'ModA.SUB_Target');
  return $R;
end;`, "")

	// mendixlabs/mxcli#1282: a line break before `)` is not part of the name,
	// so both accept it, as they accept the same-line control above.
	exec3, _, dir3 := openPedAppCopy(t)
	assertAgree(t, exec3, dir3, head+`create microflow ModA.SUB_LineBreak () returns Boolean as $R
begin
  $R = call java action ModA.RunNamed(Target = ModA.SUB_Target
  );
  return $R;
end;`, "")
}

// ako/mxcli#563: `referenceselector` parses, so check passed it, and the page
// builder has no writer for it. Control: the same page with a text box.
func TestCheckExecAgree_ReferenceSelector(t *testing.T) {
	const head = `mdl 1;
create module ModR;
create persistent entity ModR.Sys (Name: String(100));
create persistent entity ModR.Req (Title: String(100));
create association ModR.Req_Sys from ModR.Req to ModR.Sys type Reference;
create page ModR.ReqPage (
  Title: 'Req',
  Layout: Atlas_Core.PopupLayout,
  Params: ( $Req: ModR.Req )
) {
  dataview dvReq (DataSource: $Req) {
    %s
  }
};`
	exec, _, dir := openPedAppCopy(t)
	assertAgree(t, exec, dir, strings.Replace(head, "%s",
		"referenceselector rsSystem (Label: 'System', Association: ModR.Req_Sys)", 1),
		"built-in Forms widget Forms$ReferenceSelector")

	exec2, _, dir2 := openPedAppCopy(t)
	assertAgree(t, exec2, dir2, strings.Replace(head, "%s", "textbox txtTitle (Attribute: Title)", 1), "")
}

// mendixlabs/mxcli#1213: retrieve constraints mxbuild rejects with CE0161 —
// measured on 11.13.0, each against the control beside it, which builds clean:
//
//	startsWith(Title, 'X')                 starts-with(Title, 'X')
//	ModX.Order_Customer != empty           ModX.Order_Customer/ModX.Customer
//	[NoSuchAttr = 'x']                     [Title = 'x']
//	[CreatedDate > …]                      [createdDate > …]
//
// MDL047/MDL091 are exec-enforced rules, so exec refuses those two itself; the
// members are resolved by the reference pass exec runs as its pre-flight.
func TestCheckExecAgree_RetrieveConstraint(t *testing.T) {
	const head = `mdl 1;
create module ModX;
create persistent entity ModX.Order (Title: String(100), CreatedDate: AutoCreatedDate);
create persistent entity ModX.Customer (Name: String(100));
create association ModX.Order_Customer from ModX.Order to ModX.Customer type Reference;
create microflow ModX.SUB_Find ()
begin
  retrieve $A from ModX.Order where %s;
end;`
	cases := []struct{ bad, want, good string }{
		{"startsWith(Title, 'X')", "MDL091", "starts-with(Title, 'X')"},
		{"ModX.Order_Customer != empty", "MDL047", "ModX.Order_Customer/ModX.Customer"},
		{"[NoSuchAttr = 'x']", `names "NoSuchAttr"`, "[Title = 'x']"},
		{"[CreatedDate > '[%CurrentDateTime%]']", "spelled `createdDate`", "[createdDate > '[%CurrentDateTime%]']"},
	}
	for _, c := range cases {
		exec, _, dir := openPedAppCopy(t)
		bad := strings.Replace(head, "%s", c.bad, 1)
		if got := strings.Join(agreeCheck(t, exec, dir, bad), "\n"); !strings.Contains(got, c.want) {
			t.Errorf("%s: check -p must report %q, reported:\n%s", c.bad, c.want, got)
		}
		assertAgree(t, exec, dir, strings.Replace(head, "%s", c.good, 1), "")
	}
}
