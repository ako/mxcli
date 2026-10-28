// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#886 (acceptance rehearsal 2, classes R1 and D1): `create or
// modify` of a flow treated an equivalent respelling of a stored statement as
// a change — `AND` where the stored expression is `and`, a create/change
// member list laid out over several lines where the stored one is on one
// line, `Long` where a flow stores the one Integer/Long type. The built
// comparison could not take over wherever the stored flow holds a merge that
// joins nothing (older builds and Studio Pro leave them), so the statement
// diff decided: under mdl 1 a refusal on every run inside a loop body, and
// outside one a re-splice on every run, under both dialects, that moved every
// node after it further right each time (D1: +1440 px per run in formula1).
//
// Each case stores its flow from the one-line, lower-case spelling, then runs
// the respelled script twice: neither run may write. The control is a real
// change in the same flow: written once, every node that did not change stays
// where it is drawn, and the run after it writes nothing.
var respellingCases = []struct {
	name, flow, setup, script string
	// change edits script into a real change outside any loop body, for
	// the control; "" when the flow has none to make.
	from, to string
}{
	{
		name: "upper-case AND, stray merge in a loop (repros2/uppercase-and-stray-merge)",
		flow: "Repro_StrayMerge",
		setup: `create or modify microflow MyFirstModule.Repro_StrayMerge ($Items: List of System.User)
returns String as $Out
begin
  declare $Out String = '';
  loop $U in $Items
  begin
    set $Out = $Out + ',';
    @merge(400, 130)
    if $U/Name != empty and $U/Name != 'x' then
      set $Out = $Out + $U/Name;
    end if;
  end loop;
  return $Out;
end;
`,
		script: `create or modify microflow MyFirstModule.Repro_StrayMerge ($Items: List of System.User)
returns String as $Out
begin
  declare $Out String = '';
  loop $U in $Items
  begin
    set $Out = $Out + ',';
    if $U/Name != empty AND $U/Name != 'x' then
      set $Out = $Out + $U/Name;
    end if;
  end loop;
  return $Out;
end;
`,
		from: "declare $Out String = '';", to: "declare $Out String = '[';",
	},
	{
		name: "member list over several lines in a loop (repros2/multiline-members-in-loop)",
		flow: "Repro_StrayMerge2",
		setup: `create or modify microflow MyFirstModule.Repro_StrayMerge2 ($Items: List of System.User)
returns Integer as $N
begin
  declare $N Integer = 0;
  loop $U in $Items
  begin
    set $N = $N + 1;
    @merge(400, 130)
    if $U/Name != empty then
      change $U (Name = $U/Name, WebServiceUser = false);
    end if;
  end loop;
  return $N;
end;
`,
		script: `create or modify microflow MyFirstModule.Repro_StrayMerge2 ($Items: List of System.User)
returns Integer as $N
begin
  declare $N Integer = 0;
  loop $U in $Items
  begin
    set $N = $N + 1;
    if $U/Name != empty then
      change $U (
        Name = $U/Name,
        WebServiceUser = false
      );
    end if;
  end loop;
  return $N;
end;
`,
		from: "declare $N Integer = 0;", to: "declare $N Integer = 1;",
	},
	{
		// The D1 shape (formula1 DSJ_CircuitOutline): the loop and the `if`
		// after it both looked changed, so the run of the two was spliced
		// as one replace of the loop and a drop of the `if` — every node in
		// the loop rebuilt, and every node after it moved right, on every
		// run, under both dialects.
		name: "AND in a loop, Long and a multi-line expression after it (D1)",
		flow: "Repro_Drift",
		setup: `create or modify microflow MyFirstModule.Repro_Drift ($Items: List of System.User)
returns String as $Out
begin
  declare $Out String = '';
  declare $Last Integer = 0;
  loop $U in $Items
  begin
    @merge(400, 130)
    if $U/Name != empty and $U/Name != 'x' then
      set $Out = $Out + $U/Name;
    end if;
  end loop;
  if $Out != '' then
    set $Out = $Out + ',' + toString($Last);
  end if;
  set $Out = $Out + ']';
  return $Out;
end;
`,
		script: `create or modify microflow MyFirstModule.Repro_Drift ($Items: List of System.User)
returns String as $Out
begin
  declare $Out String = '';
  declare $Last Long = 0;
  loop $U in $Items
  begin
    if $U/Name != empty AND $U/Name != 'x' then
      set $Out = $Out + $U/Name;
    end if;
  end loop;
  if $Out != '' then
    set $Out = $Out
      + ',' + toString($Last);
  end if;
  set $Out = $Out + ']';
  return $Out;
end;
`,
		from: "+ ']';", to: "+ '}';",
	},
}

func TestSpliceRerun_EquivalentRespelling(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	for _, c := range respellingCases {
		for _, header := range []string{"mdl 1;\n", ""} {
			label := fmt.Sprintf("%s (header %q)", c.name, strings.TrimSpace(header))
			t.Run(label, func(t *testing.T) { respelledTwice(t, h, label, header, c.flow, c.setup, c.script, c.from, c.to) })
		}
	}
}

// respelledTwice is one case of TestSpliceRerun_EquivalentRespelling.
func respelledTwice(t *testing.T, h *harness, label, header, flow, setup, script, from, to string) {
	h.restore()
	if err := h.exec("mdl 1;\n" + setup); err != nil {
		t.Fatalf("%s: setup: %v", label, err)
	}
	if !strings.Contains(h.describeUnder("mdl 1;", "microflow MyFirstModule."+flow), "@merge") {
		t.Fatalf("%s: the stored flow has no merge that joins nothing; the case would not reach the statement diff", label)
	}
	for run := 1; run <= 2; run++ {
		before := h.snapshot()
		if err := h.exec(header + script); err != nil {
			t.Fatalf("%s: run %d: %v\n%s", label, run, err, h.out.String())
		}
		if changed := before.diff(h.snapshot()); len(changed) != 0 {
			t.Errorf("%s: run %d of the respelled script wrote: %v\n%s", label, run, changed, h.out.String())
		}
		if out := h.out.String(); strings.Contains(out, "spliced") || !strings.Contains(out, "Unchanged microflow") {
			t.Errorf("%s: run %d must be Unchanged:\n%s", label, run, out)
		}
	}

	// Control: a real change outside any loop body is written, and
	// moves nothing it does not change. The changed statement is the
	// same kind of activity, so it is edited in place and keeps its $ID
	// too (ako/mxcli#1761).
	edited := strings.Replace(script, from, to, 1)
	if edited == script {
		t.Fatalf("%s: the script has no %q", label, from)
	}
	stored := drawnObjects(t, h.flowUnit(t, flow))
	before := h.snapshot()
	if err := h.exec(header + edited); err != nil {
		t.Fatalf("%s: control: %v\n%s", label, err, h.out.String())
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 edited in place)") {
		t.Errorf("%s: control: want one statement edited in place:\n%s", label, h.out.String())
	}
	if len(before.diff(h.snapshot())) == 0 {
		t.Fatalf("%s: control: the real change wrote nothing", label)
	}
	after := drawnObjects(t, h.flowUnit(t, flow))
	kept := 0
	for id, p := range stored {
		q, ok := after[id]
		if !ok {
			continue
		}
		kept++
		if q != p {
			t.Errorf("%s: control: the unchanged %s moved from %s to %s", label, id, p, q)
		}
	}
	if kept != len(stored) || len(after) != len(stored) {
		t.Errorf("%s: control: %d of %d stored objects kept, %d after; want every one, the edited activity too",
			label, kept, len(stored), len(after))
	}
	if !samePositions(stored, after) {
		t.Errorf("%s: control: the edited activity is not drawn where it was", label)
	}
	// The twice-exec rule for the change.
	second := h.snapshot()
	if err := h.exec(header + edited); err != nil {
		t.Fatalf("%s: control run 2: %v", label, err)
	}
	if changed := second.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("%s: control run 2 wrote: %v\n%s", label, changed, h.out.String())
	}
}

// drawnObjects maps every flow object of a microflow unit, at any depth
// (a loop's body too), by its $ID to where it is drawn.
func drawnObjects(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode unit: %v", err)
	}
	out := map[string]string{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			var id, pos string
			for _, e := range x {
				switch e.Key {
				case "$ID":
					id = fmt.Sprint(e.Value)
				case "RelativeMiddlePoint":
					pos = fmt.Sprint(e.Value)
				}
			}
			if pos != "" {
				out[id] = pos
			}
			for _, e := range x {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}

// samePositions reports whether two flows draw objects at the same set of
// places, whatever their IDs.
func samePositions(a, b map[string]string) bool {
	count := map[string]int{}
	for _, p := range a {
		count[p]++
	}
	for _, p := range b {
		count[p]--
	}
	for _, n := range count {
		if n != 0 {
			return false
		}
	}
	return true
}
