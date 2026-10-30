// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1489 (ako/mxcli#1345): under `mdl 1`, an activity with a
// custom error handler could not be changed in place. `create or modify` with
// one call argument changed was refused, advising `alter`; the advised
// `alter … replace` was refused too, advising create or modify:
//
//	cannot replace the ActionActivity at (360, 200): it has an error handler,
//	which would be left with no activity; rewrite the flow with create or
//	modify instead
//
// and a change inside the handler, the call itself unchanged, was refused with
// the same message. The activity is now swapped in place when its replacement
// states the same handler, the stored handler and the merge it rejoins at
// staying as drawn; a change inside the handler is a change of the handler's
// own activity.

const handlerFlowSrc = `mdl 1;
create or modify microflow MyFirstModule.HSub ($P: String)
begin
  return;
end;
create or modify microflow MyFirstModule.HFlow ($P: String)
begin
  call microflow MyFirstModule.HSub(P = $P) on error without rollback begin
    log warning node 'Repro' 'failed';
  end error;
  log info node 'Repro' 'after';
  return;
end;`

// handlerRetSrc is the same with a handler that ends the flow (T4).
const handlerRetSrc = `mdl 1;
create or modify microflow MyFirstModule.HRet ($P: String)
begin
  call microflow MyFirstModule.HSub(P = $P) on error without rollback begin
    log warning node 'Repro' 'failed';
    return;
  end error;
  log info node 'Repro' 'after';
  return;
end;`

// handlerObjects returns the flow's objects by $ID, with where each is drawn,
// and the call activity's $ID.
func handlerObjects(t *testing.T, exec *Executor, name string) (map[model.ID]model.Point, model.ID) {
	t.Helper()
	mf := loopEditFlowOf(t, exec, name)
	var call model.ID
	for _, obj := range mf.ObjectCollection.Objects {
		if a, ok := obj.(*microflows.ActionActivity); ok {
			if _, ok := a.Action.(*microflows.MicroflowCallAction); ok {
				call = a.ID
			}
		}
	}
	return flowObjectIDs(mf.ObjectCollection), call
}

// spliceStep runs src — check first, then exec — and asserts it is spliced
// with exactly `changed` objects replaced by new ones and `added` new ones,
// nothing else rebuilt or moved, that describe shows want, and that a second
// run is Unchanged. An activity changed into the same kind of activity is
// edited in place and keeps its $ID (ako/mxcli#1761), so it is not among the
// `changed`: describe showing want is what says it was written.
func spliceStep(t *testing.T, exec *Executor, out *bytes.Buffer, name, src string, changed, added int, want ...string) {
	t.Helper()
	before, _ := handlerObjects(t, exec, name)
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatal(errs[0])
	}
	for _, v := range exec.CheckFlowVerdicts(prog) {
		t.Errorf("check -p predicts a refusal: %s", v.Message)
	}
	out.Reset()
	if err := agreeExec(t, exec, src); err != nil {
		t.Fatalf("refused: %v", err)
	}
	if !strings.Contains(out.String(), "MyFirstModule."+name+" (spliced") {
		t.Errorf("want the change spliced, got:\n%s", out.String())
	}
	after, _ := handlerObjects(t, exec, name)
	var gone int
	for id, p := range before {
		q, ok := after[id]
		if !ok {
			gone++
			continue
		}
		if p != q {
			t.Errorf("object %s moved from %v to %v", id, p, q)
		}
	}
	if gone != changed || len(after) != len(before)+added {
		t.Errorf("%d objects replaced (%d -> %d objects), want %d replaced and %d added", gone, len(before), len(after), changed, added)
	}
	desc := describeFlow(t, exec, name)
	for _, w := range want {
		if !strings.Contains(desc, w) {
			t.Errorf("describe lacks %q:\n%s", w, desc)
		}
	}
	out.Reset()
	if err := agreeExec(t, exec, src); err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if !strings.Contains(out.String(), "Unchanged microflow: MyFirstModule."+name) {
		t.Errorf("re-run is not Unchanged:\n%s", out.String())
	}
}

func TestCreateOrModify_ActivityWithErrorHandlerIsSplicedInPlace(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, handlerFlowSrc); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	if err := agreeExec(t, exec, handlerRetSrc); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	flow := func(src string) string {
		// The HFlow statement alone, so the HSub statement is not re-run.
		return "mdl 1;\n" + src[strings.Index(src, "create or modify microflow MyFirstModule.HFlow"):]
	}

	// The call is edited in place (ako/mxcli#1761): it keeps its $ID, and
	// describe shows the new argument.
	t.Run("T1 call argument changed, handler unchanged", func(t *testing.T) {
		_, call := handlerObjects(t, exec, "HFlow")
		spliceStep(t, exec, out, "HFlow", flow(strings.Replace(handlerFlowSrc, "HSub(P = $P) on", "HSub(P = 'x') on", 1)), 0, 0,
			"HSub(P = 'x') on error without rollback begin", "log warning node 'Repro' 'failed';")
		if _, now := handlerObjects(t, exec, "HFlow"); now != call {
			t.Error("the call got a new $ID for a changed argument")
		}
	})
	t.Run("T3 only the handler's log changed", func(t *testing.T) {
		_, call := handlerObjects(t, exec, "HFlow")
		src := strings.Replace(handlerFlowSrc, "HSub(P = $P) on", "HSub(P = 'x') on", 1)
		src = strings.Replace(src, "'failed';", "'failed again';", 1)
		spliceStep(t, exec, out, "HFlow", flow(src), 0, 0, "'failed again'", "HSub(P = 'x') on error")
		if _, now := handlerObjects(t, exec, "HFlow"); now != call {
			t.Error("the call was replaced for a change inside its handler")
		}
	})
	t.Run("call and handler changed together", func(t *testing.T) {
		src := strings.Replace(handlerFlowSrc, "HSub(P = $P) on", "HSub(P = 'y') on", 1)
		src = strings.Replace(src, "'failed';", "'failed twice';", 1)
		spliceStep(t, exec, out, "HFlow", flow(src), 0, 0, "HSub(P = 'y') on error", "'failed twice'")
	})
	// A note on the activity is part of it: added or reworded with the
	// activity unchanged otherwise, it is written, not dropped with the
	// handler's objects the fragment leaves out.
	t.Run("note added to the call", func(t *testing.T) {
		src := strings.Replace(handlerFlowSrc, "HSub(P = $P) on", "HSub(P = 'y') on", 1)
		src = strings.Replace(src, "'failed';", "'failed twice';", 1)
		src = strings.Replace(src, "  call microflow MyFirstModule.HSub(", "  @annotation 'calls the sub'\n  call microflow MyFirstModule.HSub(", 1)
		spliceStep(t, exec, out, "HFlow", flow(src), 0, 1, "@annotation 'calls the sub'", "HSub(P = 'y') on error")
	})
	t.Run("note on the call reworded", func(t *testing.T) {
		src := strings.Replace(handlerFlowSrc, "HSub(P = $P) on", "HSub(P = 'y') on", 1)
		src = strings.Replace(src, "'failed';", "'failed twice';", 1)
		src = strings.Replace(src, "  call microflow MyFirstModule.HSub(", "  @annotation 'calls the sub, once'\n  call microflow MyFirstModule.HSub(", 1)
		// The stored note goes out, the reworded one comes in: one object
		// replaced. The call itself is the stored one.
		spliceStep(t, exec, out, "HFlow", flow(src), 1, 0, "@annotation 'calls the sub, once'")
		if desc := describeFlow(t, exec, "HFlow"); strings.Contains(desc, "'calls the sub'") {
			t.Errorf("the old note is still there:\n%s", desc)
		}
	})
	t.Run("T4 handler that returns", func(t *testing.T) {
		spliceStep(t, exec, out, "HRet", strings.Replace(handlerRetSrc, "HSub(P = $P) on", "HSub(P = 'x') on", 1), 0, 0,
			"HSub(P = 'x') on error without rollback begin")
	})

	// Control: the handler's kind is part of the activity's shape; another
	// kind is still refused, with advice that is not create or modify.
	t.Run("control: another kind of handler is refused", func(t *testing.T) {
		before, _ := handlerObjects(t, exec, "HFlow")
		src := flow(strings.Replace(handlerFlowSrc, "on error without rollback begin", "on error begin", 1))
		err := agreeExec(t, exec, src)
		if err == nil {
			t.Fatal("want a refusal")
		}
		if after, _ := handlerObjects(t, exec, "HFlow"); len(after) != len(before) {
			t.Error("the refused statement wrote")
		}
	})
}

func TestAlter_ReplaceOfActivityWithErrorHandler(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, handlerFlowSrc); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	alter := func(body string) string {
		return "mdl 1;\nalter microflow MyFirstModule.HFlow {\n  replace call microflow MyFirstModule.HSub(*) with begin\n" +
			body + "\n  end;\n};"
	}
	unchangedSince := func(t *testing.T, before map[model.ID]model.Point) {
		t.Helper()
		after, _ := handlerObjects(t, exec, "HFlow")
		if len(after) != len(before) {
			t.Fatalf("the refused alter wrote")
		}
		for id := range before {
			if _, ok := after[id]; !ok {
				t.Fatalf("the refused alter wrote")
			}
		}
	}

	// The refusals first: none may advise create or modify, which refuses
	// the same change.
	for _, tc := range []struct{ name, body, want string }{
		{"no handler stated", "    call microflow MyFirstModule.HSub(P = 'x');", "states none"},
		{"another handler stated",
			"    call microflow MyFirstModule.HSub(P = 'x') on error without rollback begin\n" +
				"      log warning node 'Repro' 'something else';\n    end error;",
			"another error handler"},
	} {
		t.Run("refused: "+tc.name, func(t *testing.T) {
			before, _ := handlerObjects(t, exec, "HFlow")
			src := alter(tc.body)
			checkSays(t, exec, src, tc.want)
			err := agreeExec(t, exec, src)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want a refusal with %q, got %v", tc.want, err)
			}
			if strings.Contains(err.Error(), "create or modify") {
				t.Errorf("the refusal advises create or modify: %v", err)
			}
			unchangedSince(t, before)
		})
	}

	t.Run("T2 the same handler stated", func(t *testing.T) {
		before, call := handlerObjects(t, exec, "HFlow")
		src := alter("    call microflow MyFirstModule.HSub(P = 'x') on error without rollback begin\n" +
			"      log warning node 'Repro' 'failed';\n    end error;")
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("refused: %v", err)
		}
		// The same kind of activity: the stored call, edited in place
		// (ako/mxcli#1761) — every object keeps its $ID, and describe shows
		// the new argument.
		after, now := handlerObjects(t, exec, "HFlow")
		if now != call || len(after) != len(before) {
			t.Errorf("want the call edited in place: call %s -> %s, %d -> %d objects", call, now, len(before), len(after))
		}
		for id := range before {
			if _, ok := after[id]; !ok {
				t.Errorf("object %s got a new $ID", id)
			}
		}
		for id, p := range before {
			if q, ok := after[id]; ok && p != q {
				t.Errorf("object %s moved from %v to %v", id, p, q)
			}
		}
		desc := describeFlow(t, exec, "HFlow")
		if !strings.Contains(desc, "HSub(P = 'x') on error without rollback begin") ||
			!strings.Contains(desc, "log warning node 'Repro' 'failed';") {
			t.Errorf("describe after the alter:\n%s", desc)
		}
	})
}
