// SPDX-License-Identifier: Apache-2.0

// Tests for #965: an annotation inside a loop body emptied the whole loop.
//
// emitLoopBody picks the body's starting node as "the object with no incoming
// sequence flow, leftmost then topmost". A Microflows$Annotation IS an object in
// ObjectCollection.Objects, but it connects through AnnotationFlows and never
// through Flows — so its incoming-sequence-flow count is always 0 and it was
// always a start candidate. When it won, traversal began at the annotation,
// which has no outgoing sequence flows, and the body emitted ZERO lines:
//
//	loop $IteratorThing in $ThingList
//	begin
//	end loop;
//
// Silently, at exit code 0, and the truncated script passes `check`, so the
// documented describe -> exec round-trip deleted the loop body.
//
// The defect is POSITIONAL, not "any annotation": Studio Pro places annotations
// above/left by default, which loses the body, but an annotation to the RIGHT of
// the activity rendered correctly all along. A fixture that only covers the
// right-hand case passes against the broken code — hence the table below.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestEmitLoopBody_AnnotationNeverBecomesTheBodyStart(t *testing.T) {
	for _, tc := range []struct {
		name           string
		annotX, annotY int
	}{
		{"annotation left of the activity (Studio Pro default)", 20, 100},
		{"annotation above the activity, same X", 100, 20},
		{"annotation above-left of the activity", 20, 20},
		{"annotation right of the activity", 900, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loop := loopWithAnnotatedBody(tc.annotX, tc.annotY)

			var lines []string
			emitLoopBody(nil, loop, nil, nil, nil, nil, &lines, 0, nil, 0, nil)
			out := strings.Join(lines, "\n")

			if !strings.Contains(out, "in the loop") {
				t.Errorf("loop body was dropped — the activity is absent from describe output.\ngot:\n%s", out)
			}
			if !strings.Contains(out, "@annotation 'explains the step'") {
				t.Errorf("the annotation itself was dropped.\ngot:\n%s", out)
			}
		})
	}
}

// The annotation must not displace a real first activity even when it sorts
// ahead of every one of them: the body's statement ORDER has to survive.
func TestEmitLoopBody_AnnotationDoesNotReorderBody(t *testing.T) {
	first := loopBodyLog("body-1", 100, 100, "first statement")
	second := loopBodyLog("body-2", 300, 100, "second statement")
	annot := &microflows.Annotation{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: mkID("annot-1")},
			Position:    model.Point{X: 10, Y: 10},
		},
		Caption: "explains the step",
	}

	loop := &microflows.LoopedActivity{
		BaseMicroflowObject: mkObj("loop"),
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{annot, first, second},
			Flows:   []*microflows.SequenceFlow{mkFlow("body-1", "body-2")},
			AnnotationFlows: []*microflows.AnnotationFlow{
				{OriginID: mkID("annot-1"), DestinationID: mkID("body-2")},
			},
		},
	}

	var lines []string
	emitLoopBody(nil, loop, nil, nil, nil, nil, &lines, 0, nil, 0, nil)
	out := strings.Join(lines, "\n")

	firstAt := strings.Index(out, "first statement")
	secondAt := strings.Index(out, "second statement")
	if firstAt < 0 || secondAt < 0 {
		t.Fatalf("both statements must survive, got:\n%s", out)
	}
	if firstAt > secondAt {
		t.Errorf("body order inverted — traversal started at the wrong node:\n%s", out)
	}
}

// An annotation alone in a loop body has nothing to attach to and no flow to
// follow. That must emit an empty body rather than panicking or looping.
func TestEmitLoopBody_AnnotationOnlyBodyIsEmptyNotFatal(t *testing.T) {
	loop := &microflows.LoopedActivity{
		BaseMicroflowObject: mkObj("loop"),
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{
				&microflows.Annotation{
					BaseMicroflowObject: microflows.BaseMicroflowObject{
						BaseElement: model.BaseElement{ID: mkID("annot-1")},
					},
					Caption: "orphan",
				},
			},
		},
	}

	var lines []string
	emitLoopBody(nil, loop, nil, nil, nil, nil, &lines, 0, nil, 0, nil)
	if len(lines) != 0 {
		t.Errorf("an annotation-only body has no statements to emit, got:\n%s", strings.Join(lines, "\n"))
	}
}

// Build a loop with an annotated body through mxcli's OWN builder, then describe
// it. This is the guard that matters: it uses the positions mxcli really writes
// rather than hand-picked ones, and it closes the build -> describe round trip
// that #330's builder-side test and the existing emitLoopBody test each covered
// only half of.
//
// It also corrects the initial reading of #965 as a Studio-Pro-only defect. The
// builder places a loop-body annotation at the SAME X as the activity and ABOVE
// it (measured: activity (150, 80), annotation (150, -20)), which is precisely
// the losing shape — so mxcli's own describe -> exec round trip destroyed loop
// bodies it had authored itself.
func TestDescribeLoopBody_SurvivesAnnotationAtBuilderPositions(t *testing.T) {
	built := buildFlowGraphForTest(t, []ast.MicroflowStatement{
		&ast.LoopStmt{
			ListVariable: "Items", LoopVariable: "Item",
			Body: []ast.MicroflowStatement{
				&ast.LogStmt{
					Level:       ast.LogInfo,
					Message:     &ast.LiteralExpr{Kind: ast.LiteralString, Value: "has name"},
					Annotations: &ast.ActivityAnnotations{AnnotationText: "note on statement"},
				},
			},
		},
	}, map[string]string{"Items": "List of M.Item"})

	var loop *microflows.LoopedActivity
	for _, o := range built.Objects {
		if l, ok := o.(*microflows.LoopedActivity); ok {
			loop = l
			break
		}
	}
	if loop == nil {
		t.Fatal("builder produced no LoopedActivity")
	}

	// Sanity-check the premise: the annotation really does sort ahead of the
	// activity, so this fixture exercises the defect rather than dodging it.
	var actPos, annotPos model.Point
	for _, in := range loop.ObjectCollection.Objects {
		if _, isAnnot := in.(*microflows.Annotation); isAnnot {
			annotPos = in.GetPosition()
		} else {
			actPos = in.GetPosition()
		}
	}
	if !(annotPos.X < actPos.X || (annotPos.X == actPos.X && annotPos.Y < actPos.Y)) {
		t.Fatalf("fixture no longer exercises the defect: annotation %v does not sort ahead of activity %v", annotPos, actPos)
	}

	var lines []string
	emitLoopBody(nil, loop, nil, nil, nil, nil, &lines, 0, nil, 0, nil)
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "has name") {
		t.Errorf("round trip lost the loop body — describe emitted:\n%s", out)
	}
}

// --- fixtures --------------------------------------------------------------

func buildFlowGraphForTest(t *testing.T, body []ast.MicroflowStatement, vars map[string]string) *microflows.MicroflowObjectCollection {
	t.Helper()
	fb := &flowBuilder{
		posX:     100,
		posY:     100,
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
		varTypes: vars,
	}
	return fb.buildFlowGraph(body, nil)
}

func loopBodyLog(id string, x, y int, msg string) *microflows.ActionActivity {
	return &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: mkID(id)},
				Position:    model.Point{X: x, Y: y},
			},
		},
		Action: &microflows.LogMessageAction{
			LogLevel:        "Info",
			LogNodeName:     "'App'",
			MessageTemplate: &model.Text{Translations: map[string]string{"en_US": msg}},
		},
	}
}

// loopWithAnnotatedBody is one activity carrying one annotation, the annotation
// placed at (annotX, annotY) and the activity fixed at (100, 100).
func loopWithAnnotatedBody(annotX, annotY int) *microflows.LoopedActivity {
	body := loopBodyLog("body-1", 100, 100, "in the loop")
	annot := &microflows.Annotation{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: mkID("annot-1")},
			Position:    model.Point{X: annotX, Y: annotY},
		},
		Caption: "explains the step",
	}
	return &microflows.LoopedActivity{
		BaseMicroflowObject: mkObj("loop"),
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{annot, body},
			AnnotationFlows: []*microflows.AnnotationFlow{
				{OriginID: mkID("annot-1"), DestinationID: mkID("body-1")},
			},
		},
	}
}
