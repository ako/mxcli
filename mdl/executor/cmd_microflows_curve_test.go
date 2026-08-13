// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// upstream #884. A hand-curved sequence flow did not survive a rewrite: mxcli
// rebuilt every flow and defaulted the line's control vectors to "0;0", so
// re-running unchanged MDL flattened a curve drawn in Studio Pro. Measured
// before the fix by patching "40;-90" into the stored BezierCurve and re-running
// the same script — it came back "0;0".
//
// Mendix stores no waypoints: a flow's shape is the two bezier control vectors
// on its Microflows$BezierCurve line, which is why @curve takes two (x, y)
// offsets rather than a polyline.

// The curve is recorded against the ACTIVITY and applied to that activity's
// outgoing flows in one pass, rather than threaded through the seven-odd sites
// that create a flow. This pins that the single pass reaches the right flow and
// leaves the others alone.
func TestApplyFlowCurvesStampsOnlyTheAnnotatedActivitysOutgoingFlows(t *testing.T) {
	a, b, c := model.ID("a"), model.ID("b"), model.ID("c")

	fb := &flowBuilder{
		flows: []*microflows.SequenceFlow{
			{OriginID: a, DestinationID: b},
			{OriginID: b, DestinationID: c},
		},
		curveByOrigin: map[model.ID]*ast.FlowCurve{
			a: {From: &ast.Position{X: 40, Y: -90}, To: &ast.Position{X: -40, Y: 90}},
		},
	}
	fb.applyFlowCurves()

	if got := fb.flows[0].OriginControlVector; got != "40;-90" {
		t.Errorf("a→b OriginControlVector = %q, want 40;-90", got)
	}
	if got := fb.flows[0].DestinationControlVector; got != "-40;90" {
		t.Errorf("a→b DestinationControlVector = %q, want -40;90", got)
	}
	if fb.flows[1].OriginControlVector != "" || fb.flows[1].DestinationControlVector != "" {
		t.Errorf("b→c was curved (%q/%q) but carries no @curve — the stamp must not leak to "+
			"other flows", fb.flows[1].OriginControlVector, fb.flows[1].DestinationControlVector)
	}
}

// Either end may be omitted, leaving that end straight.
func TestApplyFlowCurvesOneEndedCurve(t *testing.T) {
	a := model.ID("a")
	fb := &flowBuilder{
		flows:         []*microflows.SequenceFlow{{OriginID: a, DestinationID: model.ID("b")}},
		curveByOrigin: map[model.ID]*ast.FlowCurve{a: {From: &ast.Position{X: 10, Y: 20}}},
	}
	fb.applyFlowCurves()

	if got := fb.flows[0].OriginControlVector; got != "10;20" {
		t.Errorf("OriginControlVector = %q, want 10;20", got)
	}
	if got := fb.flows[0].DestinationControlVector; got != "" {
		t.Errorf("DestinationControlVector = %q, want empty — `to:` was not given, so that end "+
			"stays straight and the writer's own default applies", got)
	}
}

// DESCRIBE has to emit the curve, or the round trip still flattens it: that was
// the reporter's "DESCRIBE output is identical before and after manual
// adjustment". A straight edge must stay silent so untouched flows describe
// exactly as they did before.
func TestEmitCurveAnnotation(t *testing.T) {
	act := &microflows.ActionActivity{}
	act.ID = model.ID("a")

	for _, tc := range []struct {
		name     string
		origin   string
		dest     string
		want     string
		wantNone bool
	}{
		{name: "both ends", origin: "40;-90", dest: "-40;90", want: "@curve(from: (40, -90), to: (-40, 90))"},
		{name: "origin only", origin: "10;20", dest: "0;0", want: "@curve(from: (10, 20))"},
		{name: "destination only", origin: "0;0", dest: "-5;15", want: "@curve(to: (-5, 15))"},
		{name: "straight is silent", origin: "0;0", dest: "0;0", wantNone: true},
		{name: "unset is silent", origin: "", dest: "", wantNone: true},
		{name: "malformed is silent", origin: "not;a;vector", dest: "", wantNone: true},
	} {
		flows := map[model.ID][]*microflows.SequenceFlow{
			act.ID: {{OriginID: act.ID, OriginControlVector: tc.origin, DestinationControlVector: tc.dest}},
		}
		var lines []string
		emitCurveAnnotation(act, flows, nil, &lines, "")

		if tc.wantNone {
			if len(lines) != 0 {
				t.Errorf("%s: emitted %v, want nothing", tc.name, lines)
			}
			continue
		}
		if len(lines) != 1 {
			t.Errorf("%s: emitted %v, want one line", tc.name, lines)
			continue
		}
		if lines[0] != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, lines[0], tc.want)
		}
	}
}

// An activity with no outgoing flow (a return, the end of a branch) must not
// panic or emit.
func TestEmitCurveAnnotationNoOutgoingFlow(t *testing.T) {
	act := &microflows.ActionActivity{}
	act.ID = model.ID("a")
	var lines []string
	emitCurveAnnotation(act, map[model.ID][]*microflows.SequenceFlow{}, nil, &lines, "")
	if len(lines) != 0 {
		t.Errorf("emitted %v for an activity with no outgoing flow, want nothing", lines)
	}
}

// The describer's output has to parse back, or the round trip is broken in a way
// asserting on a string literal would not catch.
func TestCurveAnnotationRoundTripsThroughTheParser(t *testing.T) {
	act := &microflows.ActionActivity{}
	act.ID = model.ID("a")
	flows := map[model.ID][]*microflows.SequenceFlow{
		act.ID: {{OriginID: act.ID, OriginControlVector: "40;-90", DestinationControlVector: "-40;90"}},
	}
	var lines []string
	emitCurveAnnotation(act, flows, nil, &lines, "")
	if len(lines) != 1 {
		t.Fatalf("emitted %v, want one line", lines)
	}

	script := "create microflow M.F ()\nbegin\n  " + lines[0] +
		"\n  declare $A String = 'x';\n  return $A;\nend;\n"
	stmts, errs := visitor.Build(script)
	if len(errs) != 0 {
		t.Fatalf("DESCRIBE output does not parse: %v\n%s", errs, script)
	}
	mf, ok := stmts.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("statement is %T, want *ast.CreateMicroflowStmt", stmts.Statements[0])
	}
	ann := ast.StatementAnnotations(mf.Body[0])
	if ann == nil || ann.Curve == nil {
		t.Fatalf("re-parsed statement carries no curve: %+v", ann)
	}
	if ann.Curve.From == nil || *ann.Curve.From != (ast.Position{X: 40, Y: -90}) {
		t.Errorf("from = %+v, want {40 -90}", ann.Curve.From)
	}
	if ann.Curve.To == nil || *ann.Curve.To != (ast.Position{X: -40, Y: 90}) {
		t.Errorf("to = %+v, want {-40 90}", ann.Curve.To)
	}
	if len(ann.UnknownNames) != 0 {
		t.Errorf("@curve was reported as unknown (%v) — the validator's list and the visitor "+
			"have drifted", ann.UnknownNames)
	}
}

// The two tests above call applyFlowCurves and emitCurveAnnotation directly, so
// they pass even when the call SITES are removed — verified by deleting each in
// turn. These two cover the wiring instead: MDL text through the real builder,
// and an object through the real annotation emitter.
func TestCurveReachesTheFlowThroughTheBuilder(t *testing.T) {
	src := "create microflow M.F ($P: String)\nreturns String as $R\nbegin\n" +
		"  @position(200, 100)\n  @curve(from: (40, -90), to: (-40, 90))\n" +
		"  declare $A String = $P;\n  @position(520, 100)\n  declare $R String = $A;\n" +
		"  return $R;\nend;\n"
	prog, errs := visitor.Build(src)
	if len(errs) != 0 {
		t.Fatalf("parse: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)

	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
		measurer: &layoutMeasurer{},
	}
	fb.buildFlowGraph(mf.Body, mf.ReturnType)

	var curved int
	for _, f := range fb.flows {
		if f.OriginControlVector == "40;-90" && f.DestinationControlVector == "-40;90" {
			curved++
		}
	}
	if curved != 1 {
		t.Fatalf("%d flows carry the authored curve, want exactly 1 — the @curve reached the AST "+
			"but applyFlowCurves is not wired into buildFlowGraph", curved)
	}
}

func TestCurveIsEmittedByTheAnnotationEmitter(t *testing.T) {
	act := &microflows.ActionActivity{}
	act.ID = model.ID("a")
	act.Position = model.Point{X: 200, Y: 100}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{
		act.ID: {{OriginID: act.ID, OriginControlVector: "40;-90", DestinationControlVector: "-40;90"}},
	}
	var lines []string
	emitObjectAnnotations(act, &lines, "", nil, flowsByOrigin,
		map[model.ID][]*microflows.SequenceFlow{}, nil)

	var found bool
	for _, l := range lines {
		if l == "@curve(from: (40, -90), to: (-40, 90))" {
			found = true
		}
	}
	if !found {
		t.Errorf("emitObjectAnnotations produced %v — without the @curve line DESCRIBE is identical "+
			"before and after the edge is curved, which is what #884 reported", lines)
	}
}
