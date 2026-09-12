// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// An end event accepts exactly ONE incoming sequence flow — joining two paths
// is what a merge is for — and an empty `on error … { }` handler inside a branch
// whose sibling also returns gave one two, which mxbuild rejects with CE0709
// "Sequence flow is not accepted by origin or destination".
//
// Only mxbuild caught it. The document is otherwise well formed, so `mxcli
// check` reported success, `exec` wrote it, and the project still opened —
// which is how it survived a describe → exec round trip of every microflow in
// ako/TestApp with every other check green (1 of 41 microflows,
// FeedbackModule.SUB_Feedback_SendToServer, whose stored Studio Pro version
// routes the error flow through a chain of merges that DESCRIBE cannot spell
// and emits as an empty handler block).

// endEventInDegree counts inbound sequence flows per end event, at the top level
// and inside any loop body.
func endEventInDegree(col *microflows.MicroflowObjectCollection) map[model.ID]int {
	ends := map[model.ID]bool{}
	var findEnds func(objs []microflows.MicroflowObject)
	findEnds = func(objs []microflows.MicroflowObject) {
		for _, o := range objs {
			switch v := o.(type) {
			case *microflows.EndEvent:
				ends[v.ID] = true
			case *microflows.LoopedActivity:
				findEnds(v.ObjectCollection.Objects)
			}
		}
	}
	findEnds(col.Objects)

	in := map[model.ID]int{}
	var count func(flows []*microflows.SequenceFlow)
	count = func(flows []*microflows.SequenceFlow) {
		for _, f := range flows {
			if f != nil && ends[f.DestinationID] {
				in[f.DestinationID]++
			}
		}
	}
	count(col.Flows)
	var walk func(objs []microflows.MicroflowObject)
	walk = func(objs []microflows.MicroflowObject) {
		for _, o := range objs {
			if loop, ok := o.(*microflows.LoopedActivity); ok {
				count(loop.ObjectCollection.Flows)
				walk(loop.ObjectCollection.Objects)
			}
		}
	}
	walk(col.Objects)
	return in
}

func assertNoOverConnectedEndEvent(t *testing.T, col *microflows.MicroflowObjectCollection) {
	t.Helper()
	seen := false
	for id, n := range endEventInDegree(col) {
		seen = true
		if n > 1 {
			t.Errorf("end event %s has %d incoming sequence flows, want 1 — "+
				"two paths must join at a merge (CE0709)", id, n)
		}
	}
	if !seen {
		t.Fatal("no end event in the graph; the assertion would be vacuous")
	}
}

// buildMicroflowFromMDL parses real MDL and builds its flow graph, so the test
// exercises the AST the visitor actually produces. Hand-building the equivalent
// AST does NOT reproduce this bug — the two paths end up at separate end events
// and never collide — which is itself the reason to go through the parser.
func buildMicroflowFromMDL(t *testing.T, src string) *microflows.MicroflowObjectCollection {
	t.Helper()
	prog := parseMDL(t, src)
	var create *ast.CreateMicroflowStmt
	for _, stmt := range prog.Statements {
		if c, ok := stmt.(*ast.CreateMicroflowStmt); ok {
			create = c
			break
		}
	}
	if create == nil {
		t.Fatal("no CREATE MICROFLOW in the parsed program")
	}
	fb := &flowBuilder{
		posX:     100,
		posY:     100,
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
		varTypes: map[string]string{},
	}
	return fb.buildFlowGraph(create.Body, create.ReturnType)
}

// The shape that failed: a call with an EMPTY custom error handler in a branch
// that returns, beside an else branch that also returns.
const emptyHandlerMDL = `create microflow M.EmptyHandler () returns String
begin
  if 1 = 1 then
    $r = call microflow M.Sub() on error without rollback { };
    return $r;
  else
    return 'no';
  end if;
end;`

const filledHandlerMDL = `create microflow M.FilledHandler () returns String
begin
  if 1 = 1 then
    $r = call microflow M.Sub() on error without rollback { return 'err'; };
    return $r;
  else
    return 'no';
  end if;
end;`

const plainIfMDL = `create microflow M.PlainIf () returns String
begin
  if 1 = 1 then
    return 'yes';
  else
    return 'no';
  end if;
end;`

func TestBuilder_EmptyErrorHandlerDoesNotOverConnectEndEvent(t *testing.T) {
	col := buildMicroflowFromMDL(t, emptyHandlerMDL)
	assertNoOverConnectedEndEvent(t, col)

	// The join has to be a merge, not a second end event: the microflow has one
	// return value and MDL never said what a second one would return.
	merges := 0
	for _, o := range col.Objects {
		if _, ok := o.(*microflows.ExclusiveMerge); ok {
			merges++
		}
	}
	if merges == 0 {
		t.Error("no ExclusiveMerge was created to join the two paths")
	}
}

// The control. Without it this would pass just as well against a builder that
// emitted a merge in front of every end event unconditionally, and against one
// that dropped the error flow entirely.
func TestBuilder_FilledErrorHandlerIsUnchanged(t *testing.T) {
	col := buildMicroflowFromMDL(t, filledHandlerMDL)
	assertNoOverConnectedEndEvent(t, col)

	// A handler with a body routes its own path and never needed the post-pass;
	// this pins that it did not gain a merge it does not use.
	errFlows := 0
	for _, f := range col.Flows {
		if f != nil && f.IsErrorHandler {
			errFlows++
		}
	}
	if errFlows != 1 {
		t.Errorf("error-handler flows = %d, want 1", errFlows)
	}
}

// A microflow with no error handling at all must be untouched by the post-pass —
// the narrowest statement that it does not rewrite ordinary graphs.
func TestBuilder_PlainIfGainsNoMerge(t *testing.T) {
	col := buildMicroflowFromMDL(t, plainIfMDL)
	assertNoOverConnectedEndEvent(t, col)
	for _, o := range col.Objects {
		if _, ok := o.(*microflows.ExclusiveMerge); ok {
			t.Error("a plain if/else with no error handling gained an ExclusiveMerge")
		}
	}
}
