// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#1694: after another script spliced a decision into a microflow
// (`alter microflow … insert after $X begin if … end if; end`), re-running the
// original `create or modify` script means taking that decision out again,
// which was refused — "cannot drop the ExclusiveSplit … only an activity or a
// loop can be" — and so were `alter … drop` and `alter … replace` of it. The
// splice now takes out exactly the decision shape `alter … insert` produces: a
// split whose two paths hold plain activities or nothing and rejoin at one
// merge. The flow is reconnected from the split's incoming flow to the merge's
// outgoing one, and every other element keeps its $ID. Any other decision
// shape is still refused, by check and exec alike, saying which part of the
// shape is out of reach.

const dec1694Flow = `mdl 1;
create or modify microflow MyFirstModule.%s ($F: FeedbackModule.Feedback)
begin
  log info node 'X' 'start';
  change $F (Subject = 'x');
  log info node 'X' 'done';
end;`

// dec1694Shapes are decisions of the shape `alter … insert` produces, as the
// fragment of the insert.
var dec1694Shapes = []struct{ name, fragment string }{
	{"D1694IfElse", "if $F/Subject = empty then\n      log info node 'X' 'empty';\n    else\n      log info node 'X' 'set';\n    end if;"},
	{"D1694IfThen", "if $F/Subject = empty then\n      log info node 'X' 'empty';\n    end if;"},
	{"D1694TwoActs", "if $F/Subject = empty then\n      log info node 'X' 'one';\n      log info node 'X' 'two';\n    else\n      change $F (Description = 'd');\n    end if;"},
	{"D1694ElseOnly", "if $F/Subject = empty then\n    else\n      log info node 'X' 'set';\n    end if;"},
}

func dec1694Insert(name, fragment string) string {
	return "mdl 1;\nalter microflow MyFirstModule." + name + " {\n  insert after log node 'X' 'start' begin\n    " +
		fragment + "\n  end;\n};"
}

// sameIDs reports the $IDs in want that got does not have, and the ones got
// has that want does not.
func sameIDs(want, got map[model.ID]model.Point) (missing, extra int) {
	return idChurn(want, got)
}

func TestSpliceDecision1694_ReRunTakesTheInsertedDecisionOut(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	for _, sh := range dec1694Shapes {
		t.Run(sh.name, func(t *testing.T) {
			original := fmt.Sprintf(dec1694Flow, sh.name)
			if err := agreeExec(t, exec, original); err != nil {
				t.Fatalf("setup: %v\n%s", err, out.String())
			}
			before := flowObjectIDs(loopEditFlowOf(t, exec, sh.name).ObjectCollection)
			if err := agreeExec(t, exec, dec1694Insert(sh.name, sh.fragment)); err != nil {
				t.Fatalf("the insert: %v", err)
			}
			if d := describeFlow(t, exec, sh.name); !strings.Contains(d, "if ") {
				t.Fatalf("the insert wrote no decision:\n%s", d)
			}

			// Re-running the original script takes the decision out: check
			// and exec agree, and only the decision's elements go.
			out.Reset()
			agreeVerdict(t, exec, original, "")
			if !strings.Contains(out.String(), "Modified microflow: MyFirstModule."+sh.name+" (spliced: 1 dropped)") {
				t.Errorf("want the decision reported as one drop, got:\n%s", out.String())
			}
			d := describeFlow(t, exec, sh.name)
			if strings.Contains(d, "if ") || strings.Contains(d, "'empty'") || strings.Contains(d, "'set'") {
				t.Errorf("the decision is still there:\n%s", d)
			}
			bodyOrderTop(t, d, "'start'", "change $F", "'done'")
			after := flowObjectIDs(loopEditFlowOf(t, exec, sh.name).ObjectCollection)
			if missing, extra := sameIDs(before, after); missing != 0 || extra != 0 {
				t.Errorf("want the flow's own elements back, every $ID kept: %d missing, %d extra", missing, extra)
			}

			// A second re-run is Unchanged and writes nothing.
			stored := unitBytesOf(t, exec, sh.name)
			out.Reset()
			if err := agreeExec(t, exec, original); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "Unchanged") || !bytes.Equal(stored, unitBytesOf(t, exec, sh.name)) {
				t.Errorf("the second re-run wrote:\n%s", out.String())
			}
		})
	}

	// The control for the $ID assertion: an activity the script replaces by
	// another kind shows up as one $ID gone and one added, so the comparison
	// sees churn where there is some. (A rebuild is no control here: a flow
	// dropped and created again in one session gets its stored identities
	// back; nor is a changed log text, which edits the stored activity in
	// place and keeps its $ID, ako/mxcli#1761.)
	t.Run("control: a replaced activity is churn", func(t *testing.T) {
		const name = "D1694Control"
		original := fmt.Sprintf(dec1694Flow, name)
		if err := agreeExec(t, exec, original); err != nil {
			t.Fatal(err)
		}
		before := flowObjectIDs(loopEditFlowOf(t, exec, name).ObjectCollection)
		if err := agreeExec(t, exec, strings.Replace(original, "log info node 'X' 'start';", "commit $F;", 1)); err != nil {
			t.Fatal(err)
		}
		if missing, extra := sameIDs(before, flowObjectIDs(loopEditFlowOf(t, exec, name).ObjectCollection)); missing != 1 || extra != 1 {
			t.Errorf("a replaced activity: want 1 $ID gone and 1 added, got %d and %d", missing, extra)
		}
	})
}

// bodyOrderTop asserts the description holds the given texts in order.
func bodyOrderTop(t *testing.T, desc string, want ...string) {
	t.Helper()
	at := 0
	for _, w := range want {
		i := strings.Index(desc[at:], w)
		if i < 0 {
			t.Fatalf("the flow does not hold %q (in order):\n%s", w, desc)
		}
		at += i + len(w)
	}
}

func TestSpliceDecision1694_AlterDropAndReplace(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	setup := func(t *testing.T, name string) map[model.ID]model.Point {
		t.Helper()
		if err := agreeExec(t, exec, fmt.Sprintf(dec1694Flow, name)); err != nil {
			t.Fatalf("setup: %v\n%s", err, out.String())
		}
		before := flowObjectIDs(loopEditFlowOf(t, exec, name).ObjectCollection)
		if err := agreeExec(t, exec, dec1694Insert(name, dec1694Shapes[0].fragment)); err != nil {
			t.Fatalf("the insert: %v", err)
		}
		return before
	}

	t.Run("drop", func(t *testing.T) {
		const name = "D1694AlterDrop"
		before := setup(t, name)
		agreeVerdict(t, exec, "mdl 1;\nalter microflow MyFirstModule."+name+" {\n  drop '$F/Subject = empty';\n};", "")
		d := describeFlow(t, exec, name)
		if strings.Contains(d, "if ") {
			t.Errorf("the decision is still there:\n%s", d)
		}
		if missing, extra := sameIDs(before, flowObjectIDs(loopEditFlowOf(t, exec, name).ObjectCollection)); missing != 0 || extra != 0 {
			t.Errorf("want every other $ID kept: %d missing, %d extra", missing, extra)
		}
	})

	t.Run("replace", func(t *testing.T) {
		const name = "D1694AlterReplace"
		before := setup(t, name)
		agreeVerdict(t, exec, "mdl 1;\nalter microflow MyFirstModule."+name+" {\n"+
			"  replace '$F/Subject = empty' with begin log info node 'X' 'instead'; end;\n};", "")
		d := describeFlow(t, exec, name)
		if strings.Contains(d, "if ") {
			t.Errorf("the decision is still there:\n%s", d)
		}
		bodyOrderTop(t, d, "'start'", "'instead'", "change $F", "'done'")
		missing, extra := sameIDs(before, flowObjectIDs(loopEditFlowOf(t, exec, name).ObjectCollection))
		if missing != 0 || extra != 1 {
			t.Errorf("want every other $ID kept and one activity added: %d missing, %d extra", missing, extra)
		}
		// Its own description, executed back, is Unchanged.
		stored := unitBytesOf(t, exec, name)
		out.Reset()
		if err := agreeExec(t, exec, d); err != nil || !strings.Contains(out.String(), "Unchanged") ||
			!bytes.Equal(stored, unitBytesOf(t, exec, name)) {
			t.Errorf("describe -> exec is not Unchanged: %v\n%s", err, out.String())
		}
	})

	// create or modify replacing the decision by another statement in its
	// place: a replace of the decision.
	t.Run("create or modify replaces it", func(t *testing.T) {
		const name = "D1694ModifyReplace"
		before := setup(t, name)
		src := strings.Replace(fmt.Sprintf(dec1694Flow, name), "  log info node 'X' 'start';\n",
			"  log info node 'X' 'start';\n  log info node 'X' 'instead';\n", 1)
		out.Reset()
		agreeVerdict(t, exec, src, "")
		bodyOrderTop(t, describeFlow(t, exec, name), "'start'", "'instead'", "change $F", "'done'")
		missing, extra := sameIDs(before, flowObjectIDs(loopEditFlowOf(t, exec, name).ObjectCollection))
		if missing != 0 || extra != 1 {
			t.Errorf("want every other $ID kept and one activity added: %d missing, %d extra", missing, extra)
		}
	})
}

// Every other decision shape is still refused, by check and exec alike,
// writing nothing, with the part of the shape that is out of reach.
func TestSpliceDecision1694_OtherShapesRefused(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	cases := []struct {
		name, body, want string
	}{
		{"D1694Return", "if $F/Subject = empty then\n    log info node 'X' 'empty';\n    return;\n  end if;", "ends in a return"},
		{"D1694Nested", "if $F/Subject = empty then\n    if $F/Description = empty then\n      log info node 'X' 'both';\n    end if;\n  end if;", "holds a decision"},
		{"D1694Loop", "if $F/Subject = empty then\n    loop $It in $Items\n    begin\n      log info node 'X' 'it';\n    end loop;\n  end if;", "holds a loop"},
		{"D1694Handler", "if $F/Subject = empty then\n    commit $F on error begin log info node 'X' 'failed'; end error;\n  end if;", "has an error handler"},
		{"D1694Raise", "if $F/Subject = empty then\n    raise error;\n  end if;", "ends in a raise error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withDecision := "mdl 1;\ncreate or modify microflow MyFirstModule." + tc.name +
				" ($F: FeedbackModule.Feedback, $Items: List of FeedbackModule.Feedback)\nbegin\n  log info node 'X' 'start';\n  " +
				tc.body + "\n  change $F (Subject = 'x');\n  log info node 'X' 'done';\nend;"
			if err := agreeExec(t, exec, withDecision); err != nil {
				t.Fatalf("setup: %v\n%s", err, out.String())
			}
			without := "mdl 1;\ncreate or modify microflow MyFirstModule." + tc.name +
				" ($F: FeedbackModule.Feedback, $Items: List of FeedbackModule.Feedback)\nbegin\n  log info node 'X' 'start';\n" +
				"  change $F (Subject = 'x');\n  log info node 'X' 'done';\nend;"
			stored := unitBytesOf(t, exec, tc.name)
			agreeVerdict(t, exec, without, tc.want)
			agreeVerdict(t, exec, "mdl 1;\nalter microflow MyFirstModule."+tc.name+" {\n  drop '$F/Subject = empty';\n};", tc.want)
			agreeVerdict(t, exec, "mdl 1;\nalter microflow MyFirstModule."+tc.name+" {\n"+
				"  replace '$F/Subject = empty' with begin log info node 'X' 'instead'; end;\n};", tc.want)
			if !bytes.Equal(stored, unitBytesOf(t, exec, tc.name)) {
				t.Error("a refused statement wrote")
			}
		})
	}

	// A variable a branch declares and the flow reads after the decision is
	// still in use: taking the decision out is refused like dropping its
	// producer.
	t.Run("a branch output still read", func(t *testing.T) {
		const name = "D1694Used"
		if err := agreeExec(t, exec, "mdl 1;\ncreate or modify microflow MyFirstModule."+name+" ($F: FeedbackModule.Feedback)\nbegin\n"+
			"  log info node 'X' 'start';\n  if $F/Subject = empty then\n    declare $N Integer = 1;\n  end if;\n"+
			"  log info node 'X' 'n {1}' with ({1} = toString($N));\nend;"); err != nil {
			t.Fatalf("setup: %v", err)
		}
		stored := unitBytesOf(t, exec, name)
		agreeVerdict(t, exec, "mdl 1;\nalter microflow MyFirstModule."+name+" {\n  drop '$F/Subject = empty';\n};", "still used by")
		if !bytes.Equal(stored, unitBytesOf(t, exec, name)) {
			t.Error("the refused statement wrote")
		}
	})
}
