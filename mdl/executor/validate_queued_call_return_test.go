// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func parseQueuedCallProgram(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return prog
}

func runQueuedCallCheck(t *testing.T, src string) []string {
	t.Helper()
	var out []string
	for _, v := range ValidateQueuedCallReturnType(parseQueuedCallProgram(t, src)) {
		if v.RuleID == "MDL088" {
			out = append(out, v.Message)
		}
	}
	return out
}

// mendixlabs/mxcli#1064, verbatim: `CALL MICROFLOW M.F(…) IN QUEUE M.Q` where F
// returns Boolean — `mxcli check --references` says "Check passed!" and `mx
// check` says CE7033 "A microflow used for background execution must have a
// Microflow return type of 'Nothing'."
func TestQueuedCall_MicroflowReturningBooleanIsReported(t *testing.T) {
	got := runQueuedCallCheck(t, `
create microflow M.F () returns boolean
begin
  return true;
end;
/
create microflow M.Caller ()
begin
  call microflow M.F() in queue M.Q;
end;
/
`)
	if len(got) != 1 {
		t.Fatalf("MDL088 violations = %v, want exactly one", got)
	}
	for _, want := range []string{"M.F", "Boolean", "CE7033", "M.Q"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("message %q does not mention %q", got[0], want)
		}
	}
}

// CONTROL: the identical call on a microflow that returns nothing is what
// Mendix accepts, and must stay silent — otherwise the test above would pass
// against a rule that flags every queued call.
func TestQueuedCall_VoidMicroflowIsClean(t *testing.T) {
	if got := runQueuedCallCheck(t, `
create microflow M.F ()
begin
  log info node 'M' 'x';
end;
/
create microflow M.Caller ()
begin
  call microflow M.F() in queue M.Q;
end;
/
`); len(got) != 0 {
		t.Errorf("a void queued microflow was rejected: %v", got)
	}
}

// CONTROL: the same Boolean microflow called WITHOUT a queue is an ordinary call.
func TestQueuedCall_UnqueuedCallIsNotChecked(t *testing.T) {
	if got := runQueuedCallCheck(t, `
create microflow M.F () returns boolean
begin
  return true;
end;
/
create microflow M.Caller ()
begin
  $ok = call microflow M.F();
end;
/
`); len(got) != 0 {
		t.Errorf("an unqueued call was type-checked: %v", got)
	}
}

// The queued call can sit anywhere in the body — a hand-written switch over
// statement kinds misses whichever nesting was added last.
func TestQueuedCall_NestedCallIsFound(t *testing.T) {
	got := runQueuedCallCheck(t, `
create microflow M.F () returns string
begin
  return 'x';
end;
/
create microflow M.Caller ($b: boolean)
begin
  if $b then
    while $b
    begin
      call microflow M.F() in queue M.Q;
    end while;
  end if;
end;
/
`)
	if len(got) != 1 || !strings.Contains(got[0], "String") {
		t.Errorf("MDL088 violations = %v, want one naming String", got)
	}
}

// A target the script does not define needs the project; the project-less pass
// must not guess.
func TestQueuedCall_UndefinedTargetIsLeftToReferences(t *testing.T) {
	if got := runQueuedCallCheck(t, `
create microflow M.Caller ()
begin
  call microflow Other.F() in queue M.Q;
end;
/
`); len(got) != 0 {
		t.Errorf("an unknown target was reported without a project: %v", got)
	}
}

// The --references half: a target already stored in the project, with the
// return type the backend lists for it.
func TestQueuedCall_StoredTarget(t *testing.T) {
	body := parseQueuedCallProgram(t, `
create microflow M.Caller ()
begin
  call microflow M.Stored() in queue M.Q;
end;
/
`).Statements[0].(*ast.CreateMicroflowStmt).Body

	for _, tc := range []struct {
		stored  string
		wantErr bool
	}{
		{"Boolean", true},
		{"Object", true},
		{"Void", false}, // how a stored "returns nothing" reads back
		{"", false},     // ReturnType absent altogether
	} {
		errs := validateQueuedMicroflowTargets(body, map[string]string{"M.Stored": tc.stored}, newScriptContext())
		if (len(errs) > 0) != tc.wantErr {
			t.Errorf("stored return %q: errors = %v, wantErr %v", tc.stored, errs, tc.wantErr)
		}
	}

	// CONTROL: return type unavailable → nothing is known to be wrong.
	if errs := validateQueuedMicroflowTargets(body, nil, newScriptContext()); len(errs) != 0 {
		t.Errorf("unknown return type was reported: %v", errs)
	}

	// A target the script itself (re)creates is the project-less pass's to
	// report; reporting it here too would print it twice.
	sc := newScriptContext()
	sc.microflows["M.Stored"] = true
	if errs := validateQueuedMicroflowTargets(body, map[string]string{"M.Stored": "Boolean"}, sc); len(errs) != 0 {
		t.Errorf("script-defined target reported on the project path too: %v", errs)
	}
}
