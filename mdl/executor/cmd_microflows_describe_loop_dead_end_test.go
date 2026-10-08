// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A decision in a loop body where one arm simply STOPS — the activity has no
// outgoing flow, which in Studio Pro ends the iteration — while the other arms
// meet at a merge that goes on (here, to `break`).
//
// Evora: GenAICommons.Trace_GetModelSpanInput. The ToolSpan arm appends to
// $ModelInput and stops; the other two arms meet and break. DESCRIBE printed
// the arms, closed the split and printed the break after it, so every arm fell
// into the break: re-executed, the loop stops at the first ToolSpan instead of
// collecting them all. The arm has to say that the iteration ends there.
//
// MDL cannot draw a dead end — the builder writes `continue` as a
// ContinueEvent — so the fixtures are authored with `continue` and the event
// is then removed, leaving the arm's last activity with no way out, which is
// exactly the stored shape.

const loopDeadEndSplitTypeMDL = `create microflow M.LoopDeadEndType ($L: List of M.Span) returns String
begin
  declare $Out String = '';
  loop $S in $L
  begin
    split type $S
      when M.ToolSpan then
        set $Out = $Out + 'tool';
        continue;
      when M.Span then
    end split;
    break;
  end loop;
  return $Out;
end;`

const loopDeadEndIfMDL = `create microflow M.LoopDeadEndIf ($L: List of M.Span) returns String
begin
  declare $Out String = '';
  loop $S in $L
  begin
    if $S/Kind = 'tool' then
      set $Out = $Out + 'tool';
      continue;
    end if;
    log info node 'T' 'other';
    break;
  end loop;
  return $Out;
end;`

func TestDescribeLoopDeadEndArm_SaysTheIterationEnds(t *testing.T) {
	for name, src := range map[string]string{"split type": loopDeadEndSplitTypeMDL, "if": loopDeadEndIfMDL} {
		t.Run(name, func(t *testing.T) {
			oc := buildMicroflowFromMDL(t, src)
			if n := removeContinueEvents(oc); n != 1 {
				t.Fatalf("fixture: removed %d ContinueEvents, want 1", n)
			}
			mf := &microflows.Microflow{ObjectCollection: oc}
			e := newTestExecutor()
			out := strings.Join(formatMicroflowActivities(e.newExecContext(t.Context()), mf, nil, nil), "\n")
			assertDescriptionRebuildsGraph(t, out, oc)
		})
	}
}

// Control: an arm that stops where the join itself goes nowhere needs no
// `continue` — every arm ends the iteration either way — and must not gain one,
// or every such loop's description would change for nothing.
func TestDescribeLoopDeadEndArm_NoContinueWhenTheJoinEndsToo(t *testing.T) {
	oc := buildMicroflowFromMDL(t, `create microflow M.LoopDeadEndBoth ($L: List of M.Span) returns String
begin
  declare $Out String = '';
  loop $S in $L
  begin
    if $S/Kind = 'tool' then
      set $Out = $Out + 'tool';
      continue;
    end if;
    log info node 'T' 'other';
  end loop;
  return $Out;
end;`)
	removeContinueEvents(oc)
	mf := &microflows.Microflow{ObjectCollection: oc}
	e := newTestExecutor()
	out := strings.Join(formatMicroflowActivities(e.newExecContext(t.Context()), mf, nil, nil), "\n")
	assertDescriptionRebuildsGraph(t, out, oc)
	if strings.Contains(out, "continue;") {
		t.Errorf("an arm whose join also ends the iteration gained a continue:\n%s", out)
	}
}

// removeContinueEvents deletes every ContinueEvent in the loops of oc, with the
// flows into it (wherever the builder stored them), and returns how many it
// removed.
func removeContinueEvents(oc *microflows.MicroflowObjectCollection) int {
	gone := map[model.ID]bool{}
	var strip func(c *microflows.MicroflowObjectCollection)
	strip = func(c *microflows.MicroflowObjectCollection) {
		var kept []microflows.MicroflowObject
		for _, o := range c.Objects {
			if _, isContinue := o.(*microflows.ContinueEvent); isContinue {
				gone[o.GetID()] = true
				continue
			}
			kept = append(kept, o)
			if loop, ok := o.(*microflows.LoopedActivity); ok && loop.ObjectCollection != nil {
				strip(loop.ObjectCollection)
			}
		}
		c.Objects = kept
	}
	strip(oc)
	var drop func(c *microflows.MicroflowObjectCollection)
	drop = func(c *microflows.MicroflowObjectCollection) {
		var flows []*microflows.SequenceFlow
		for _, f := range c.Flows {
			if !gone[f.DestinationID] {
				flows = append(flows, f)
			}
		}
		c.Flows = flows
		for _, o := range c.Objects {
			if loop, ok := o.(*microflows.LoopedActivity); ok && loop.ObjectCollection != nil {
				drop(loop.ObjectCollection)
			}
		}
	}
	drop(oc)
	return len(gone)
}
