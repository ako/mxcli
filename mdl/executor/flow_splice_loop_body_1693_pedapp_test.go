// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1693: `create or modify` and `alter` refused any change of a loop
// body's shape — an activity inserted, dropped or moved — with "the Loop …
// changes inside its body", and the way out was `alter … replace loop`, which
// re-minted every identity in the loop. Inside a body the splice now inserts,
// drops and replaces plain activities, laid out in the loop box's
// coordinates, the box growing when the body no longer fits; only the changed
// activities get new $IDs. Anything else in a body — a decision, a nested
// loop, a return — is still refused, naming `alter … replace body of` the
// loop, which keeps the loop and rebuilds only what is inside it.

const lb1693Flow = `mdl 1;
create or modify microflow MyFirstModule.%s ($Items: List of FeedbackModule.Feedback)
begin
  declare $S string = '';
  loop $It in $Items
  begin
    set $S = $S + 'a';
    log info node 'X' 'middle';
    commit $It;
  end loop;
  log info node 'X' $S;
end;`

// lb1693Body is the body of lb1693Flow as it is written there.
const lb1693Body = "    set $S = $S + 'a';\n    log info node 'X' 'middle';\n    commit $It;\n"

// lb1693Script is lb1693Flow for the flow name with its loop body replaced.
func lb1693Script(name, body string) string {
	return strings.Replace(fmt.Sprintf(lb1693Flow, name), lb1693Body, body, 1)
}

// unitBytesOf returns the stored bytes of MyFirstModule.name.
func unitBytesOf(t *testing.T, exec *Executor, name string) []byte {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	raw, err := ctx.Backend.GetRawUnitBytes(loopEditFlowOf(t, exec, name).ID)
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), raw...)
}

// loopOf returns the first top-level loop of the stored flow.
func loopOf(t *testing.T, mf *microflows.Microflow) *microflows.LoopedActivity {
	t.Helper()
	for _, obj := range mf.ObjectCollection.Objects {
		if l, ok := obj.(*microflows.LoopedActivity); ok {
			return l
		}
	}
	t.Fatalf("%s has no loop", mf.Name)
	return nil
}

// idChurn compares two flowObjectIDs maps: the $IDs gone and the ones added.
func idChurn(before, after map[model.ID]model.Point) (gone, added int) {
	for id := range before {
		if _, ok := after[id]; !ok {
			gone++
		}
	}
	for id := range after {
		if _, ok := before[id]; !ok {
			added++
		}
	}
	return gone, added
}

// assertLoopBodies checks what Studio Pro needs of every loop body in the
// flow: its nodes form one chain — one node no flow enters, which starts an
// iteration, one no flow leaves — and every node of it is drawn inside the
// loop box.
func assertLoopBodies(t *testing.T, mf *microflows.Microflow) {
	t.Helper()
	flows := mf.ObjectCollection.Flows
	var walk func(oc *microflows.MicroflowObjectCollection)
	walk = func(oc *microflows.MicroflowObjectCollection) {
		for _, obj := range oc.Objects {
			l, ok := obj.(*microflows.LoopedActivity)
			if !ok || l.ObjectCollection == nil {
				continue
			}
			inBody := map[model.ID]bool{}
			var nodes []microflows.MicroflowObject
			for _, b := range l.ObjectCollection.Objects {
				if _, note := b.(*microflows.Annotation); note {
					continue
				}
				inBody[b.GetID()] = true
				nodes = append(nodes, b)
				p := b.GetPosition()
				var sz model.Size
				if s, ok := b.(interface{ GetSize() model.Size }); ok {
					sz = s.GetSize()
				}
				if p.X-sz.Width/2 < 0 || p.Y-sz.Height/2 < 0 || p.X+sz.Width/2 > l.Size.Width || p.Y+sz.Height/2 > l.Size.Height {
					t.Errorf("a body node at %v (size %v) is drawn outside its loop box %v", p, sz, l.Size)
				}
			}
			in, out := map[model.ID]int{}, map[model.ID]int{}
			for _, f := range flows {
				if inBody[f.OriginID] != inBody[f.DestinationID] {
					t.Errorf("flow %s runs between the loop body and the rest of the flow", f.ID)
				}
				if inBody[f.OriginID] {
					out[f.OriginID]++
					in[f.DestinationID]++
				}
			}
			var starts, ends int
			for _, n := range nodes {
				if in[n.GetID()] == 0 {
					starts++
				}
				if out[n.GetID()] == 0 {
					ends++
				}
			}
			if len(nodes) > 0 && (starts != 1 || ends < 1) {
				t.Errorf("the loop body has %d nodes no flow enters and %d no flow leaves; want one start", starts, ends)
			}
			walk(l.ObjectCollection)
		}
	}
	walk(mf.ObjectCollection)
}

// agreeVerdict runs check's flow verdict (CheckFlowVerdicts, what `check -p`
// reports for a flow change) and then exec, and asserts both refuse with want
// — or, for want "", both accept.
func agreeVerdict(t *testing.T, exec *Executor, src, want string) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v\n%s", errs[0], src)
	}
	var says []string
	for _, v := range exec.CheckFlowVerdicts(prog) {
		says = append(says, v.Message)
	}
	execErr := exec.ExecuteProgram(prog)
	checkSays := strings.Join(says, "\n")
	if want == "" {
		if checkSays != "" || execErr != nil {
			t.Errorf("check and exec must both accept\ncheck: %s\nexec: %v", checkSays, execErr)
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

// bodyOrder asserts the loop body of the flow's description holds the given
// texts in order.
func bodyOrder(t *testing.T, desc string, want ...string) {
	t.Helper()
	start := strings.Index(desc, "loop $It in $Items")
	end := strings.Index(desc, "end loop;")
	if start < 0 || end < start {
		t.Fatalf("no loop in:\n%s", desc)
	}
	body := desc[start:end]
	at := 0
	for _, w := range want {
		i := strings.Index(body[at:], w)
		if i < 0 {
			t.Fatalf("the loop body does not hold %q (in order) — got:\n%s", w, body)
		}
		at += i + len(w)
	}
}

func TestSpliceLoopBody1693_CreateOrModify(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	cases := []struct {
		name, body, summary string
		order               []string
		gone, added         int
	}{
		{"LB1693Mid", "    set $S = $S + 'a';\n    log info node 'X' 'new';\n    log info node 'X' 'middle';\n    commit $It;\n",
			"spliced: 1 inserted", []string{"set $S", "'new'", "'middle'", "commit $It"}, 0, 1},
		{"LB1693First", "    log info node 'X' 'new';\n" + lb1693Body,
			"spliced: 1 inserted", []string{"'new'", "set $S", "'middle'", "commit $It"}, 0, 1},
		{"LB1693Last", lb1693Body + "    log info node 'X' 'new';\n",
			"spliced: 1 inserted", []string{"set $S", "'middle'", "commit $It", "'new'"}, 0, 1},
		{"LB1693Two", "    set $S = $S + 'a';\n    log info node 'X' 'new 1';\n    log info node 'X' 'new 2';\n    log info node 'X' 'middle';\n    commit $It;\n",
			"spliced: 1 inserted", []string{"set $S", "'new 1'", "'new 2'", "'middle'", "commit $It"}, 0, 2},
		{"LB1693DropMid", "    set $S = $S + 'a';\n    commit $It;\n",
			"spliced: 1 dropped", []string{"set $S", "commit $It"}, 1, 0},
		{"LB1693DropFirst", "    log info node 'X' 'middle';\n    commit $It;\n",
			"spliced: 1 dropped", []string{"'middle'", "commit $It"}, 1, 0},
		{"LB1693DropLast", "    set $S = $S + 'a';\n    log info node 'X' 'middle';\n",
			"spliced: 1 dropped", []string{"set $S", "'middle'"}, 1, 0},
		{"LB1693Swap", "    log info node 'X' 'middle';\n    set $S = $S + 'a';\n    commit $It;\n",
			"", []string{"'middle'", "set $S", "commit $It"}, 1, 1},
		{"LB1693ReplaceByTwo", "    set $S = $S + 'a';\n    log info node 'X' 'one';\n    log info node 'X' 'two';\n    commit $It;\n",
			"spliced: 1 replaced with new element IDs", []string{"set $S", "'one'", "'two'", "commit $It"}, 1, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := agreeExec(t, exec, fmt.Sprintf(lb1693Flow, tc.name)); err != nil {
				t.Fatalf("setup: %v\n%s", err, out.String())
			}
			stored := loopEditFlowOf(t, exec, tc.name)
			before := flowObjectIDs(stored.ObjectCollection)
			loopID := loopOf(t, stored).ID

			src := lb1693Script(tc.name, tc.body)
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatal(errs[0])
			}
			for _, v := range exec.CheckFlowVerdicts(prog) {
				t.Errorf("check -p predicts a refusal for a plain-activity change in the loop body: %s", v.Message)
			}
			out.Reset()
			if err := agreeExec(t, exec, src); err != nil {
				t.Fatalf("a plain-activity change in the loop body is refused: %v", err)
			}
			if !strings.Contains(out.String(), "Modified microflow: MyFirstModule."+tc.name+" (") ||
				(tc.summary != "" && !strings.Contains(out.String(), tc.summary)) {
				t.Errorf("want the change reported as %q, got:\n%s", tc.summary, out.String())
			}
			bodyOrder(t, describeFlow(t, exec, tc.name), tc.order...)

			mf := loopEditFlowOf(t, exec, tc.name)
			assertLoopBodies(t, mf)
			after := flowObjectIDs(mf.ObjectCollection)
			if loopOf(t, mf).ID != loopID {
				t.Error("the loop itself was replaced")
			}
			if gone, added := idChurn(before, after); gone != tc.gone || added != tc.added {
				t.Errorf("want %d $IDs gone and %d added (only the changed activities), got %d and %d",
					tc.gone, tc.added, gone, added)
			}

			// The re-run is a fixed point: nothing written.
			stored2 := unitBytesOf(t, exec, tc.name)
			out.Reset()
			if err := agreeExec(t, exec, src); err != nil {
				t.Fatalf("re-run: %v", err)
			}
			if !strings.Contains(out.String(), "Unchanged") || !bytes.Equal(stored2, unitBytesOf(t, exec, tc.name)) {
				t.Errorf("re-running the script wrote again:\n%s", out.String())
			}
		})
	}

	// The control for the $ID assertions above: the route the refusal used to
	// name — `alter … replace loop` — re-mints every identity in the loop for
	// the same change, so a comparison that saw nothing change would have
	// passed against it.
	t.Run("control: replace loop re-mints the body", func(t *testing.T) {
		const name = "LB1693Control"
		if err := agreeExec(t, exec, fmt.Sprintf(lb1693Flow, name)); err != nil {
			t.Fatalf("setup: %v", err)
		}
		before := flowObjectIDs(loopEditFlowOf(t, exec, name).ObjectCollection)
		if err := agreeExec(t, exec, "mdl 1;\nalter microflow MyFirstModule."+name+" {\n"+
			"  replace loop $It in $Items with begin loop $It in $Items begin\n"+
			"    set $S = $S + 'a';\n    log info node 'X' 'new';\n    log info node 'X' 'middle';\n    commit $It;\n"+
			"  end loop; end;\n};"); err != nil {
			t.Fatal(err)
		}
		gone, added := idChurn(before, flowObjectIDs(loopEditFlowOf(t, exec, name).ObjectCollection))
		if gone < 4 || added < 5 {
			t.Errorf("replace loop kept identities (%d gone, %d added); the comparison would not detect churn", gone, added)
		}
	})
}

// A body edit that needs more room than the loop box has grows the box, and
// what is drawn past the loop moves along, as past an inserted activity.
func TestSpliceLoopBody1693_BoxGrows(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const name = "LB1693Grow"
	if err := agreeExec(t, exec, fmt.Sprintf(lb1693Flow, name)); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	mf := loopEditFlowOf(t, exec, name)
	loop := loopOf(t, mf)
	pos, size := loop.Position, loop.Size
	var after model.Point // the log after the loop
	for _, obj := range mf.ObjectCollection.Objects {
		if obj.GetPosition().X > pos.X+size.Width/2 {
			if _, ok := obj.(*microflows.ActionActivity); ok {
				after = obj.GetPosition()
			}
		}
	}
	if err := agreeExec(t, exec, lb1693Script(name, lb1693Body+"    log info node 'X' 'new 1';\n    log info node 'X' 'new 2';\n")); err != nil {
		t.Fatal(err)
	}
	mf = loopEditFlowOf(t, exec, name)
	assertLoopBodies(t, mf)
	grown := loopOf(t, mf)
	if grown.Size.Width <= size.Width || grown.Size.Height != size.Height {
		t.Errorf("want the box wider and as high, got %v from %v", grown.Size, size)
	}
	if left := grown.Position.X - grown.Size.Width/2; left != pos.X-size.Width/2 {
		t.Errorf("the box's left edge moved from %d to %d; it grows on its far side", pos.X-size.Width/2, left)
	}
	if grown.Position.Y != pos.Y {
		t.Errorf("the box moved off its line: %v -> %v", pos, grown.Position)
	}
	moved := false
	for _, obj := range mf.ObjectCollection.Objects {
		if p := obj.GetPosition(); p.Y == after.Y && p.X == after.X+(grown.Size.Width-size.Width) {
			moved = true
		}
	}
	if !moved {
		t.Errorf("the activity after the loop did not move along by %d", grown.Size.Width-size.Width)
	}
}

// The body of a nested loop is edited the same way, inside its own box; the
// outer box grows to fit a grown inner one.
func TestSpliceLoopBody1693_NestedLoop(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	src := `mdl 1;
create or modify microflow MyFirstModule.LB1693Nested ($Items: List of FeedbackModule.Feedback, $Others: List of FeedbackModule.Feedback)
begin
  loop $It in $Items
  begin
    log info node 'X' 'outer';
    loop $Other in $Others
    begin
      log info node 'X' 'inner';
    end loop;
  end loop;
  log info node 'X' 'done';
end;`
	if err := agreeExec(t, exec, src); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	before := flowObjectIDs(loopEditFlowOf(t, exec, "LB1693Nested").ObjectCollection)
	edited := strings.Replace(src, "      log info node 'X' 'inner';\n",
		"      log info node 'X' 'inner';\n      log info node 'X' 'inner 2';\n      commit $Other;\n", 1)
	out.Reset()
	if err := agreeExec(t, exec, edited); err != nil {
		t.Fatalf("an insert in a nested loop body is refused: %v", err)
	}
	if !strings.Contains(out.String(), "spliced: 1 inserted") {
		t.Errorf("want one insert, got:\n%s", out.String())
	}
	mf := loopEditFlowOf(t, exec, "LB1693Nested")
	assertLoopBodies(t, mf)
	if gone, added := idChurn(before, flowObjectIDs(mf.ObjectCollection)); gone != 0 || added != 2 {
		t.Errorf("want 0 $IDs gone and 2 added, got %d and %d", gone, added)
	}
	out.Reset()
	if err := agreeExec(t, exec, edited); err != nil || !strings.Contains(out.String(), "Unchanged") {
		t.Errorf("the re-run is not Unchanged: %v\n%s", err, out.String())
	}
}

func TestSpliceLoopBody1693_Alter(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	cases := []struct {
		name, op    string
		order       []string
		gone, added int
	}{
		{"LBA1693InsAfter", "insert after set $S = $S + 'a' begin log info node 'X' 'new'; end;",
			[]string{"set $S", "'new'", "'middle'", "commit $It"}, 0, 1},
		{"LBA1693InsAfterLast", "insert after commit $It begin log info node 'X' 'new'; end;",
			[]string{"set $S", "'middle'", "commit $It", "'new'"}, 0, 1},
		{"LBA1693InsBeforeFirst", "insert before set $S = $S + 'a' begin log info node 'X' 'new'; end;",
			[]string{"'new'", "set $S", "'middle'", "commit $It"}, 0, 1},
		{"LBA1693Drop", "drop log node 'X' 'middle';",
			[]string{"set $S", "commit $It"}, 1, 0},
		{"LBA1693DropLast", "drop commit $It;",
			[]string{"set $S", "'middle'"}, 1, 0},
		{"LBA1693ReplaceTwo", "replace log node 'X' 'middle' with begin log info node 'X' 'one'; log info node 'X' 'two'; end;",
			[]string{"set $S", "'one'", "'two'", "commit $It"}, 1, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := agreeExec(t, exec, fmt.Sprintf(lb1693Flow, tc.name)); err != nil {
				t.Fatalf("setup: %v\n%s", err, out.String())
			}
			stored := loopEditFlowOf(t, exec, tc.name)
			before := flowObjectIDs(stored.ObjectCollection)
			loopID := loopOf(t, stored).ID
			src := "mdl 1;\nalter microflow MyFirstModule." + tc.name + " {\n  " + tc.op + "\n};"
			// check and exec agree: both accept.
			agreeVerdict(t, exec, src, "")
			bodyOrder(t, describeFlow(t, exec, tc.name), tc.order...)
			mf := loopEditFlowOf(t, exec, tc.name)
			assertLoopBodies(t, mf)
			if loopOf(t, mf).ID != loopID {
				t.Error("the loop itself was replaced")
			}
			if gone, added := idChurn(before, flowObjectIDs(mf.ObjectCollection)); gone != tc.gone || added != tc.added {
				t.Errorf("want %d $IDs gone and %d added, got %d and %d", tc.gone, tc.added, gone, added)
			}
		})
	}
}

// What changes a body's shape is still refused — by check and exec alike,
// writing nothing — and the refusal names `replace body of` the loop.
func TestSpliceLoopBody1693_StillRefused(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const withIf = `mdl 1;
create or modify microflow MyFirstModule.LB1693If ($Items: List of FeedbackModule.Feedback)
begin
  loop $It in $Items
  begin
    if $It/Subject = empty then
      log info node 'X' 'no email';
    end if;
    commit $It;
  end loop;
end;`
	for _, setup := range []string{fmt.Sprintf(lb1693Flow, "LB1693Refused"), withIf} {
		if err := agreeExec(t, exec, setup); err != nil {
			t.Fatalf("setup: %v\n%s", err, out.String())
		}
	}
	const advice = "replace body of loop $It in $Items with begin"
	cases := []struct {
		name, flow, src, want string
	}{
		{"create or modify adds a decision", "LB1693Refused",
			lb1693Script("LB1693Refused", "    if $It/Subject = empty then\n      log info node 'X' 'no email';\n    end if;\n"+lb1693Body),
			"a decision is added to it"},
		{"create or modify changes a decision in the body", "LB1693If",
			strings.Replace(withIf, "$It/Subject = empty", "$It/Subject != empty", 1),
			"a decision in it"},
		{"create or modify takes a decision out of the body", "LB1693If",
			strings.Replace(withIf, "    if $It/Subject = empty then\n      log info node 'X' 'no email';\n    end if;\n", "", 1),
			"a decision in it"},
		{"create or modify adds a nested loop", "LB1693Refused",
			lb1693Script("LB1693Refused", lb1693Body+"    loop $Inner in $Items\n    begin\n      commit $Inner;\n    end loop;\n"),
			"a loop is added to it"},
		{"alter inserts a decision", "LB1693Refused",
			"mdl 1;\nalter microflow MyFirstModule.LB1693Refused {\n  insert after commit $It begin if $It/Subject = empty then log info node 'X' 'e'; end if; end;\n};",
			"the fragment holds a decision"},
		{"alter drops a decision in the body", "LB1693If",
			"mdl 1;\nalter microflow MyFirstModule.LB1693If {\n  drop '$It/Subject = empty';\n};",
			"it is a decision"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stored := unitBytesOf(t, exec, tc.flow)
			agreeVerdict(t, exec, tc.src, tc.want)
			err := agreeExec(t, exec, tc.src)
			if err == nil || !strings.Contains(err.Error(), advice) {
				t.Errorf("the refusal must name %q, got: %v", advice, err)
			}
			if !bytes.Equal(stored, unitBytesOf(t, exec, tc.flow)) {
				t.Error("the refused statement wrote")
			}
		})
	}

	// In a nested loop the refusal names the loop whose body changes.
	t.Run("nested loop names the inner loop", func(t *testing.T) {
		src := `mdl 1;
create or modify microflow MyFirstModule.LB1693NestedRefused ($Items: List of FeedbackModule.Feedback, $Others: List of FeedbackModule.Feedback)
begin
  loop $It in $Items
  begin
    loop $Other in $Others
    begin
      commit $Other;
    end loop;
  end loop;
end;`
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("setup: %v", err)
		}
		edited := strings.Replace(src, "      commit $Other;\n", "      if $Other/Subject = empty then\n        commit $Other;\n      end if;\n", 1)
		agreeVerdict(t, exec, edited, "replace body of loop $Other in $Others with begin")
		agreeVerdict(t, exec, "mdl 1;\nalter microflow MyFirstModule.LB1693NestedRefused {\n"+
			"  insert after commit $Other begin if $Other/Subject = empty then log info node 'X' 'e'; end if; end;\n};",
			"replace body of loop $Other in $Others with begin")
	})
}

func TestAlterReplaceBody1693(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const name = "LB1693ReplaceBody"
	if err := agreeExec(t, exec, fmt.Sprintf(lb1693Flow, name)); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	mf := loopEditFlowOf(t, exec, name)
	before := flowObjectIDs(mf.ObjectCollection)
	loop := loopOf(t, mf)
	bodyIDs := flowObjectIDs(loop.ObjectCollection)

	// A body with a decision, which the splice does not put in a body: the
	// loop keeps its $ID and everything outside it stays.
	const replace = `mdl 1;
alter microflow MyFirstModule.LB1693ReplaceBody {
  replace body of loop $It in $Items with begin
    if $It/Subject = empty then
      log info node 'X' 'no email';
    end if;
    commit $It;
  end;
};`
	agreeVerdict(t, exec, replace, "")
	mf = loopEditFlowOf(t, exec, name)
	assertLoopBodies(t, mf)
	after := flowObjectIDs(mf.ObjectCollection)
	got := loopOf(t, mf)
	if got.ID != loop.ID {
		t.Fatal("replace body replaced the loop itself")
	}
	if src, ok := got.LoopSource.(*microflows.IterableList); !ok || src.VariableName != "It" || src.ListVariableName != "Items" {
		t.Errorf("the loop's iterator or list changed: %#v", got.LoopSource)
	}
	for id, p := range before {
		if _, inBody := bodyIDs[id]; inBody {
			if _, ok := after[id]; ok {
				t.Errorf("a stored body element %s survived replace body", id)
			}
			continue
		}
		if q, ok := after[id]; !ok {
			t.Errorf("element %s outside the loop was taken out", id)
		} else if id != loop.ID && q != p && q.Y == p.Y && q.X < p.X {
			t.Errorf("element %s outside the loop moved back from %v to %v", id, p, q)
		}
	}
	desc := describeFlow(t, exec, name)
	bodyOrder(t, desc, "if $It/Subject = empty then", "'no email'", "end if;", "commit $It")
	if strings.Contains(desc, "'middle'") || strings.Contains(desc, "set $S") {
		t.Errorf("the stored body is still there:\n%s", desc)
	}

	// Round trip: the flow's own description, executed back, is Unchanged.
	t.Run("describe round-trips", func(t *testing.T) {
		stored := unitBytesOf(t, exec, name)
		out.Reset()
		if err := agreeExec(t, exec, "mdl 1;\n"+strings.TrimPrefix(desc, "mdl 1;\n")); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Unchanged") || !bytes.Equal(stored, unitBytesOf(t, exec, name)) {
			t.Errorf("describe -> exec wrote:\n%s", out.String())
		}
	})

	// A body that fits the box keeps its size and position.
	t.Run("a body that fits keeps the box", func(t *testing.T) {
		l := loopOf(t, loopEditFlowOf(t, exec, name))
		if err := agreeExec(t, exec, "mdl 1;\nalter microflow MyFirstModule."+name+" {\n"+
			"  replace body of loop $It in $Items with begin commit $It; end;\n};"); err != nil {
			t.Fatal(err)
		}
		got := loopOf(t, loopEditFlowOf(t, exec, name))
		if got.ID != l.ID || got.Position != l.Position || got.Size != l.Size {
			t.Errorf("want the loop kept as it is, got %v %v from %v %v", got.Position, got.Size, l.Position, l.Size)
		}
		if body := describeFlow(t, exec, name); !strings.Contains(body, "commit $It;") || strings.Contains(body, "no email") {
			t.Errorf("the body was not replaced:\n%s", body)
		}
	})

	refusals := []struct{ name, op, want string }{
		{"not a loop", "replace body of $S with begin commit $It; end;",
			"replace body of addresses a loop"},
		{"reads an undeclared variable", "replace body of loop $It in $Items with begin commit $Nope; end;",
			"not declared on the path"},
		{"declares a variable the flow has", "replace body of loop $It in $Items with begin declare $S string = 'x'; end;",
			"already has outside the loop"},
	}
	for _, r := range refusals {
		t.Run("refused: "+r.name, func(t *testing.T) {
			stored := unitBytesOf(t, exec, name)
			agreeVerdict(t, exec, "mdl 1;\nalter microflow MyFirstModule."+name+" {\n  "+r.op+"\n};", r.want)
			if !bytes.Equal(stored, unitBytesOf(t, exec, name)) {
				t.Error("the refused statement wrote")
			}
		})
	}

	t.Run("while loop", func(t *testing.T) {
		if err := agreeExec(t, exec, `mdl 1;
create or modify microflow MyFirstModule.LB1693While ()
begin
  declare $N integer = 0;
  while $N < 3
  begin
    set $N = $N + 1;
  end while;
end;`); err != nil {
			t.Fatalf("setup: %v", err)
		}
		l := loopOf(t, loopEditFlowOf(t, exec, "LB1693While"))
		agreeVerdict(t, exec, "mdl 1;\nalter microflow MyFirstModule.LB1693While {\n"+
			"  replace body of while $N < 3 with begin set $N = $N + 2; log info node 'X' 'step'; end;\n};", "")
		mf := loopEditFlowOf(t, exec, "LB1693While")
		assertLoopBodies(t, mf)
		if loopOf(t, mf).ID != l.ID {
			t.Error("the while loop itself was replaced")
		}
		if d := describeFlow(t, exec, "LB1693While"); !strings.Contains(d, "set $N = $N + 2;") || !strings.Contains(d, "'step'") {
			t.Errorf("the body was not replaced:\n%s", d)
		}
	})

	t.Run("nested loop", func(t *testing.T) {
		src := `mdl 1;
create or modify microflow MyFirstModule.LB1693RBNested ($Items: List of FeedbackModule.Feedback, $Others: List of FeedbackModule.Feedback)
begin
  loop $It in $Items
  begin
    loop $Other in $Others
    begin
      commit $Other;
    end loop;
  end loop;
end;`
		if err := agreeExec(t, exec, src); err != nil {
			t.Fatalf("setup: %v", err)
		}
		mf := loopEditFlowOf(t, exec, "LB1693RBNested")
		outer := loopOf(t, mf)
		var inner *microflows.LoopedActivity
		for _, obj := range outer.ObjectCollection.Objects {
			if l, ok := obj.(*microflows.LoopedActivity); ok {
				inner = l
			}
		}
		agreeVerdict(t, exec, "mdl 1;\nalter microflow MyFirstModule.LB1693RBNested {\n"+
			"  replace body of loop $Other in $Others with begin\n"+
			"    if $Other/Subject = empty then log info node 'X' 'none'; end if;\n"+
			"    commit $Other; log info node 'X' 'done';\n  end;\n};", "")
		mf = loopEditFlowOf(t, exec, "LB1693RBNested")
		assertLoopBodies(t, mf)
		ids := flowObjectIDs(mf.ObjectCollection)
		if _, ok := ids[outer.ID]; !ok {
			t.Error("the outer loop was replaced")
		}
		if _, ok := ids[inner.ID]; !ok {
			t.Error("the inner loop was replaced")
		}
		out.Reset()
		if err := agreeExec(t, exec, "mdl 1;\n"+strings.TrimPrefix(describeFlow(t, exec, "LB1693RBNested"), "mdl 1;\n")); err != nil ||
			!strings.Contains(out.String(), "Unchanged") {
			t.Errorf("describe -> exec is not Unchanged: %v\n%s", err, out.String())
		}
	})

	// A stored loop whose body is empty has no activity to insert next to;
	// create or modify fills it with replace body, which loses nothing there.
	t.Run("create or modify fills an empty body", func(t *testing.T) {
		empty := `mdl 1;
create or modify microflow MyFirstModule.LB1693Empty ($Items: List of FeedbackModule.Feedback)
begin
  loop $It in $Items
  begin
  end loop;
  log info node 'X' 'done';
end;`
		if err := agreeExec(t, exec, empty); err != nil {
			t.Fatalf("setup: %v", err)
		}
		before := flowObjectIDs(loopEditFlowOf(t, exec, "LB1693Empty").ObjectCollection)
		filled := strings.Replace(empty, "  begin\n  end loop;", "  begin\n    commit $It;\n    log info node 'X' 'one';\n  end loop;", 1)
		out.Reset()
		if err := agreeExec(t, exec, filled); err != nil {
			t.Fatalf("filling an empty loop body is refused: %v", err)
		}
		mf := loopEditFlowOf(t, exec, "LB1693Empty")
		assertLoopBodies(t, mf)
		if gone, added := idChurn(before, flowObjectIDs(mf.ObjectCollection)); gone != 0 || added != 2 {
			t.Errorf("want 0 $IDs gone and 2 added, got %d and %d", gone, added)
		}
		out.Reset()
		if err := agreeExec(t, exec, filled); err != nil || !strings.Contains(out.String(), "Unchanged") {
			t.Errorf("the re-run is not Unchanged: %v\n%s", err, out.String())
		}
	})
}

// A body restated from describe states every node's @position: under mdl 1
// the splice moves none of them (as at top level, mendixlabs/mxcli#1598), so
// an insert that needs room among them is refused rather than written in a
// shape the next run would read as moved nodes. The control is the same insert
// in the script that states no positions, which is spliced.
func TestSpliceLoopBody1693_StatedPositionsArePinned(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	const name = "LB1693Pinned"
	if err := agreeExec(t, exec, fmt.Sprintf(lb1693Flow, name)); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	var buf bytes.Buffer
	saved := exec.output
	exec.output = &buf
	if err := afRun(t, exec, "describe microflow MyFirstModule."+name+" with handles;"); err != nil {
		t.Fatal(err)
	}
	exec.output = saved
	restated := strings.Replace(buf.String(), "    set $S = $S + 'a';\n",
		"    set $S = $S + 'a';\n    log info node 'X' 'new';\n", 1)
	if restated == buf.String() {
		t.Fatalf("the restated script has no set statement to insert after:\n%s", buf.String())
	}
	stored := unitBytesOf(t, exec, name)
	agreeVerdict(t, exec, restated, "draws where it is")
	if !bytes.Equal(stored, unitBytesOf(t, exec, name)) {
		t.Error("the refused statement wrote")
	}
	agreeVerdict(t, exec, lb1693Script(name, "    set $S = $S + 'a';\n    log info node 'X' 'new';\n    log info node 'X' 'middle';\n    commit $It;\n"), "")
}
