// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1106: under `mdl 1`, `create or modify` of a flow whose only
// change is one expression inside a loop body was refused —
//
//	the Loop at (…) changes inside its body; the splice does not edit inside
//	a loop, and replacing the whole loop would rebuild every node it holds.
//	Nothing was written.  [MDL-V1-REBUILD]
//
// so one script could not both provision and update a flow that loops. A body
// of the same shape — same statements in the same order, each either as stored
// or the same kind of activity with a new value — needs no layout: each
// changed activity is replaced in place, inside the loop, where it is drawn.

const loopEditFlow = `mdl 1;
create or modify microflow MyFirstModule.LoopEdit ($Items: List of FeedbackModule.Feedback)
begin
  declare $S string = '';
  loop $It in $Items
  begin
    set $S = $S + 'a';
    log info node 'X' 'in loop';
    commit $It;
  end loop;
  log info node 'X' $S;
end;`

// loopEditFlowOf returns the stored LoopEdit microflow, or nil.
func loopEditFlowOf(t *testing.T, exec *Executor, name string) *microflows.Microflow {
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
		if m.Name == name && h.GetModuleName(h.FindModuleID(m.ContainerID)) == "MyFirstModule" {
			mf, err := ctx.Backend.GetMicroflow(m.ID)
			if err != nil {
				t.Fatal(err)
			}
			return mf
		}
	}
	t.Fatalf("MyFirstModule.%s not found", name)
	return nil
}

// flowObjectIDs returns every object's $ID in the flow, loop bodies included,
// with where it is drawn.
func flowObjectIDs(oc *microflows.MicroflowObjectCollection) map[model.ID]model.Point {
	out := map[model.ID]model.Point{}
	var walk func(*microflows.MicroflowObjectCollection)
	walk = func(oc *microflows.MicroflowObjectCollection) {
		if oc == nil {
			return
		}
		for _, obj := range oc.Objects {
			out[obj.GetID()] = obj.GetPosition()
			if l, ok := obj.(*microflows.LoopedActivity); ok {
				walk(l.ObjectCollection)
			}
		}
	}
	walk(oc)
	return out
}

func describeFlow(t *testing.T, exec *Executor, name string) string {
	t.Helper()
	var buf bytes.Buffer
	saved := exec.output
	exec.output = &buf
	defer func() { exec.output = saved }()
	if err := afRun(t, exec, "describe microflow MyFirstModule."+name+";"); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestCreateOrModify_EditInsideLoopBody(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, loopEditFlow); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	before := flowObjectIDs(loopEditFlowOf(t, exec, "LoopEdit").ObjectCollection)

	edited := strings.Replace(loopEditFlow, "set $S = $S + 'a';", "set $S = $S + toString(length($S));", 1)
	if prog, errs := visitor.Build(edited); len(errs) > 0 {
		t.Fatal(errs[0])
	} else {
		for _, v := range exec.CheckFlowVerdicts(prog) {
			t.Errorf("check -p predicts a refusal for an in-loop expression edit: %s", v.Message)
		}
	}
	out.Reset()
	if err := agreeExec(t, exec, edited); err != nil {
		t.Fatalf("an expression edit inside a loop body is refused: %v", err)
	}
	if !strings.Contains(out.String(), "Modified microflow: MyFirstModule.LoopEdit (spliced: 1 edited in place)") {
		t.Errorf("want the edit reported as one splice, got:\n%s", out.String())
	}
	desc := describeFlow(t, exec, "LoopEdit")
	if !strings.Contains(desc, "toString(length($S))") || strings.Contains(desc, "+ 'a'") {
		t.Errorf("the edit was not written:\n%s", desc)
	}

	// Every object keeps its $ID and its place: the loop, its other
	// activity, and the edited activity itself, which is the stored one
	// with a new expression (ako/mxcli#1761) — the same kind of activity.
	after := flowObjectIDs(loopEditFlowOf(t, exec, "LoopEdit").ObjectCollection)
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
		t.Fatalf("want the edited activity kept, got %d gone, %d added", len(gone), len(added))
	}

	// The re-run is a fixed point.
	out.Reset()
	if err := agreeExec(t, exec, edited); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Unchanged") {
		t.Errorf("re-running the edited script wrote again:\n%s", out.String())
	}

	// An edit that reads the loop's iterator: in scope inside the body
	// (the scope check walked only the flow around the loop).
	t.Run("reads the iterator", func(t *testing.T) {
		src := strings.Replace(edited, "commit $It;", "commit $It without events;", 1)
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("refused: %v", err)
		}
		if desc := describeFlow(t, exec, "LoopEdit"); !strings.Contains(desc, "commit $It without events;") {
			t.Errorf("the edit was not written:\n%s", desc)
		}
		edited = src
	})

	// Controls: a change to the body's shape the splice does not make — a
	// decision or a loop added in it — still refuses, writing nothing. (An
	// activity inserted, dropped or moved is spliced since ako/mxcli#1693,
	// flow_splice_loop_body_1693_pedapp_test.go.)
	stored := loopEditBytes(t, exec)
	for name, src := range map[string]string{
		"decision": strings.Replace(edited, "log info node 'X' 'in loop';",
			"if $It/Subject = empty then\n      log info node 'X' 'in loop';\n    end if;", 1),
		"loop": strings.Replace(edited, "log info node 'X' 'in loop';",
			"loop $Inner in $Items\n    begin\n      log info node 'X' 'in loop';\n    end loop;", 1),
	} {
		t.Run("control: "+name+" refused", func(t *testing.T) {
			if src == edited {
				t.Fatal("control edit did not apply")
			}
			err := agreeExec(t, exec, src)
			if err == nil || !strings.Contains(err.Error(), "changes inside its body") {
				t.Fatalf("want the in-loop refusal, got %v", err)
			}
			if !bytes.Equal(stored, loopEditBytes(t, exec)) {
				t.Error("the refused statement wrote")
			}
		})
	}

	// A change next to the loop and one inside it in the same run.
	t.Run("edit next to the loop and inside it", func(t *testing.T) {
		src := strings.Replace(edited, "declare $S string = '';", "declare $S string = 'start';", 1)
		src = strings.Replace(src, "'in loop'", "'still in loop'", 1)
		out.Reset()
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("refused: %v", err)
		}
		if !strings.Contains(out.String(), "2 edited in place") {
			t.Errorf("want two in-place edits, got:\n%s", out.String())
		}
		desc := describeFlow(t, exec, "LoopEdit")
		if !strings.Contains(desc, "'still in loop'") || !strings.Contains(desc, "'start'") {
			t.Errorf("the edits were not written:\n%s", desc)
		}
	})
}

// A nested loop's body is edited the same way, at any depth.
func TestCreateOrModify_EditInsideNestedLoopBody(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	src := `mdl 1;
create or modify microflow MyFirstModule.LoopEditNested ($Items: List of FeedbackModule.Feedback, $Others: List of FeedbackModule.Feedback)
begin
  loop $It in $Items
  begin
    loop $Other in $Others
    begin
      log info node 'X' 'inner';
    end loop;
  end loop;
end;`
	if err := agreeExec(t, exec, src); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	edited := strings.Replace(src, "'inner'", "'inner, edited'", 1)
	if err := agreeExec(t, exec, edited); err != nil {
		t.Fatalf("an edit in a nested loop body is refused: %v", err)
	}
	if desc := describeFlow(t, exec, "LoopEditNested"); !strings.Contains(desc, "'inner, edited'") {
		t.Errorf("the edit was not written:\n%s", desc)
	}
}

func loopEditBytes(t *testing.T, exec *Executor) []byte {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	raw, err := ctx.Backend.GetRawUnitBytes(loopEditFlowOf(t, exec, "LoopEdit").ID)
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), raw...)
}
