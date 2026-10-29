// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"
)

// ako/mxcli#905 (#897 item 4, rehearsal 3 class G2): under mdl 1, `create or
// modify` refused to splice a new activity whose custom error handler ends in
// its own `return` — "an error handler in the fragment ends at an end event of
// its own … a return inside an error handler is not spliced yet" — on every
// run. Created fresh, the same statement worked. On CapTrack this made a
// stub-then-real script set leave the export flow as its placeholder.
//
// The handler's return is a new end event of the flow, drawn where the builder
// drew it relative to the fragment, like a guard clause's (#888). Named
// TestSpliceRerun_ so that it runs in the parity suite (#870).

const handlerHelper = `create or modify microflow MyFirstModule.EH_Helper () returns Boolean as $Ok
begin
  return true;
end;
`

// An mxcli-authored stub grows by an activity whose handler returns, under
// both language versions — once as a replace of the stub's activity, once as an
// insert before a kept one — and the second run of the same script writes
// nothing.
func TestSpliceRerun_GrowByHandlerReturn(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	if err := h.exec("mdl 1;\n" + handlerHelper); err != nil {
		t.Fatalf("helper: %v\n%s", err, h.out.String())
	}

	for _, c := range []struct {
		name, header, stub, grown, spliced string
		ends                               int
	}{
		{"replace, mdl 1", "mdl 1;\n",
			`create or modify microflow MyFirstModule.EH_Replace1 ()
begin
  log info node 'EH' 'stub';
end;
`, `create or modify microflow MyFirstModule.EH_Replace1 ()
begin
  $Ok = call microflow MyFirstModule.EH_Helper() on error begin
    log error node 'EH' 'failed';
    return;
  end error;
  log info node 'EH' 'done';
end;
`, "(spliced: 1 replaced with new element IDs)", 2},
		{"insert, mdl 1, a returned value", "mdl 1;\n",
			`create or modify microflow MyFirstModule.EH_Insert1 () returns Boolean as $R
begin
  log info node 'EH' 'done';
  return true;
end;
`, `create or modify microflow MyFirstModule.EH_Insert1 () returns Boolean as $R
begin
  $Ok = call microflow MyFirstModule.EH_Helper() on error without rollback begin
    log error node 'EH' 'failed for ' + $currentUser/Name;
    return false;
  end error;
  log info node 'EH' 'done';
  return true;
end;
`, "(spliced: 1 inserted)", 2},
		{"insert, mdl 0", "",
			`create or modify microflow MyFirstModule.EH_Insert0 ()
begin
  log info node 'EH' 'done';
end;
`, `create or modify microflow MyFirstModule.EH_Insert0 ()
begin
  $Ok = call microflow MyFirstModule.EH_Helper() on error begin
    log error node 'EH' 'failed';
    return;
  end error;
  log info node 'EH' 'done';
end;
`, "(spliced: 1 inserted)", 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			// verdictsOf restores the fixture after a write, so the helper
			// and the stub are stated again after it.
			stored := func() {
				if err := h.exec("mdl 1;\n" + handlerHelper + c.stub); err != nil {
					t.Fatalf("stub: %v\n%s", err, h.out.String())
				}
			}
			stored()
			// check -p agrees with exec (#892): no refusal, no rebuild.
			if v := h.verdictsOf(c.header + handlerHelper + c.grown); agree(t, c.header != "", v) || v.checkError || v.checkWarning {
				t.Errorf("check/diff/exec predict a refusal or a rebuild: %+v", v)
			}
			stored()
			name := strings.Fields(strings.SplitN(c.stub, ".", 2)[1])[0]
			stub := h.flowUnit(t, name)

			if err := h.exec(c.header + c.grown); err != nil {
				t.Fatalf("grow: %v\n%s", err, h.out.String())
			}
			if !strings.Contains(h.out.String(), c.spliced) {
				t.Errorf("want %s, got:\n%s", c.spliced, h.out.String())
			}
			grown := h.flowUnit(t, name)
			if strings.Contains(c.spliced, "inserted") {
				requireKept(t, stub, grown, "")
			}
			if got := countType(t, grown, "Microflows$EndEvent"); got != c.ends {
				t.Errorf("%d end events after the grow, want %d", got, c.ends)
			}
			described := h.mustDescribeMdl0(t, "microflow MyFirstModule."+name)
			for _, want := range []string{"on error", "log error node 'EH'", "return", "'EH' 'done'"} {
				if !strings.Contains(described, want) {
					t.Errorf("the grown flow does not state %q:\n%s", want, described)
				}
			}

			// The twice-exec rule.
			settled := h.snapshot()
			if err := h.exec(c.header + c.grown); err != nil {
				t.Fatalf("second run: %v\n%s", err, h.out.String())
			}
			if changed := settled.diff(h.snapshot()); len(changed) != 0 {
				t.Errorf("the second run wrote: %s\n%s", strings.Join(changed, "; "), h.out.String())
			}
			// Control: a change after the new activity is a change, and is
			// written. (A change inside a stored handler is a replace of
			// its activity, which the splice does not make.)
			changed := strings.Replace(c.grown, "'EH' 'done'", "'EH' 'finished'", 1)
			if err := h.exec(c.header + changed); err != nil {
				t.Fatalf("changed flow: %v\n%s", err, h.out.String())
			}
			if len(settled.diff(h.snapshot())) == 0 {
				t.Errorf("a changed log message wrote nothing:\n%s", h.out.String())
			}
		})
	}
}

// A Studio Pro-authored flow grows by an activity whose handler returns: every
// stored element stays, the handler's return is a new end event, and the
// second run — and the grown flow's own description — writes nothing. The
// handler's message is an expression, which is stored as '{1}' with the
// expression as its parameter: the script's spelling and describe's are one
// activity, or the re-run would see the activity changed and refuse it.
func TestSpliceRerun_GrowStudioProFlowByHandlerReturn(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	if err := h.exec("mdl 1;\n" + handlerHelper); err != nil {
		t.Fatalf("helper: %v\n%s", err, h.out.String())
	}

	const target = "microflow Administration.ShowPasswordForm"
	described := withoutLayout(h.mustDescribeMdl0(t, target))
	const show = "  show page Administration.ChangePasswordForm("
	const call = "  $Ok = call microflow MyFirstModule.EH_Helper() on error begin\n" +
		"    log error node 'Pwd' 'no password form for ' + $Account/FullName;\n" +
		"    return;\n" +
		"  end error;\n"
	edited := strings.Replace(described, show, call+show, 1)
	if edited == described {
		t.Fatalf("describe output changed shape:\n%s", described)
	}
	if v := h.verdictsOf("mdl 1;\n" + handlerHelper + edited); agree(t, true, v) || v.checkError {
		t.Errorf("check/diff/exec predict a refusal: %+v", v)
	}
	if err := h.exec("mdl 1;\n" + handlerHelper); err != nil {
		t.Fatalf("helper: %v\n%s", err, h.out.String())
	}
	before := h.flowUnit(t, "ShowPasswordForm")
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("grow: %v\n%s", err, h.out.String())
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 inserted)") {
		t.Errorf("want one insert, got:\n%s", h.out.String())
	}
	after := h.flowUnit(t, "ShowPasswordForm")
	requireKept(t, before, after, "")
	if got := countType(t, after, "Microflows$EndEvent"); got != 2 {
		t.Errorf("%d end events after the grow, want 2", got)
	}

	settled := h.snapshot()
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("second run: %v\n%s", err, h.out.String())
	}
	if changed := settled.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the second run wrote: %s\n%s", strings.Join(changed, "; "), h.out.String())
	}
	again := h.mustDescribeMdl0(t, target)
	if err := h.exec("mdl 1;\n" + again); err != nil {
		t.Fatalf("re-exec of the description: %v\n%s", err, h.out.String())
	}
	if changed := settled.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("the description of the grown flow wrote: %s", strings.Join(changed, "; "))
	}
}

// Where the handler's return would be drawn across a stored flow there is no
// free room, and the grow is refused with that reason — by check as by exec —
// and writes nothing: the insert stretches the stored handler of the activity
// before it across the gap the new handler goes in.
func TestSpliceRerun_HandlerReturnWithNoRoomIsRefused(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	if err := h.exec("mdl 1;\n" + handlerHelper); err != nil {
		t.Fatalf("helper: %v\n%s", err, h.out.String())
	}
	const flow = `create or modify microflow MyFirstModule.EH_Room ()
begin
  log info node 'R' 'a' on error begin
    log error node 'R' 'x';
    log error node 'R' 'y';
    return;
  end error;
  log info node 'R' 'b';
end;
`
	if err := h.exec("mdl 1;\n" + flow); err != nil {
		t.Fatalf("create: %v\n%s", err, h.out.String())
	}
	grown := strings.Replace(flow, "  log info node 'R' 'b';\n",
		"  $Ok = call microflow MyFirstModule.EH_Helper() on error begin\n    log error node 'R' 'failed';\n    return;\n  end error;\n  log info node 'R' 'b';\n", 1)
	v := h.verdictsOf("mdl 1;\n" + handlerHelper + grown)
	if !agree(t, true, v) || !v.checkError {
		t.Fatalf("want the grow refused by check, diff and exec: %+v", v)
	}
	if !strings.Contains(v.execMessage, "no free room for the return") {
		t.Errorf("the refusal does not say there is no room:\n%s", v.execMessage)
	}
	if strings.Contains(v.execMessage, "not spliced yet") {
		t.Errorf("the refusal still says a handler's return is not spliced:\n%s", v.execMessage)
	}
}
