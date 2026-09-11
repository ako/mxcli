// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1077 — "roundtrip for annotation with multiple connections
// results in duplicate annotations".
//
// In Mendix a note is a NODE with edges: one Microflows$Annotation joined to any
// number of activities by Microflows$AnnotationFlow. MDL modelled it as one
// string per activity, and that lost BOTH directions of the relation:
//
//   A. one note → N activities came back as N notes (the report), and
//   B. N notes → one activity came back as ONE, the others deleted (found here,
//      and worse: A duplicates, B destroys).
//
// Both were measured end to end on a real 11.13.0 project with mxbuild at 0
// errors on every side, which is why neither had ever been noticed.
//
// A third loss rides along: an Annotation's own position and size were never
// emitted and were re-invented by the writer, so every round trip moved and
// resized every note.

// mfWithNotes builds the smallest microflow that can carry notes: two log
// activities between a start and an end.
func mfWithNotes(flows []*microflows.AnnotationFlow, notes ...*microflows.Annotation) *microflows.Microflow {
	objs := []microflows.MicroflowObject{
		&microflows.StartEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: "start"}, Position: model.Point{X: 0, Y: 100}}},
		&microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: "a1"}, Position: model.Point{X: 100, Y: 100}}},
			Action: &microflows.LogMessageAction{LogNodeName: "N", LogLevel: "Info", MessageTemplate: &model.Text{}}},
		&microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: "a2"}, Position: model.Point{X: 200, Y: 100}}},
			Action: &microflows.LogMessageAction{LogNodeName: "N", LogLevel: "Info", MessageTemplate: &model.Text{}}},
		&microflows.EndEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: "end"}, Position: model.Point{X: 300, Y: 100}}},
	}
	for _, n := range notes {
		objs = append(objs, n)
	}
	return &microflows.Microflow{ObjectCollection: &microflows.MicroflowObjectCollection{
		Objects: objs,
		Flows: []*microflows.SequenceFlow{
			{OriginID: "start", DestinationID: "a1"},
			{OriginID: "a1", DestinationID: "a2"},
			{OriginID: "a2", DestinationID: "end"},
		},
		AnnotationFlows: flows,
	}}
}

func note(id, caption string, pos model.Point, size model.Size) *microflows.Annotation {
	return &microflows.Annotation{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(id)}, Position: pos, Size: size},
		Caption: caption,
	}
}

func describeBody(t *testing.T, mf *microflows.Microflow) string {
	t.Helper()
	return strings.Join(formatMicroflowActivities(&ExecContext{}, mf, nil, nil), "\n")
}

// rebuildFromBody parses a described body and returns the notes the flow builder
// produced. The whole point is that the text is the contract: a helper that
// handed the AST straight to the builder would not exercise the describer.
func rebuildFromBody(t *testing.T, body string) (notes []*microflows.Annotation, flows []*microflows.AnnotationFlow, errs []string) {
	t.Helper()
	src := "create microflow M.F ()\nbegin\n" + body + "\nend;"
	prog, parseErrs := visitor.Build(src)
	if len(parseErrs) > 0 {
		t.Fatalf("DESCRIBE output does not parse: %v\n%s", parseErrs, src)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing,
		varTypes: map[string]string{}, declaredVars: map[string]string{}}
	fb.buildFlowGraph(mf.Body, nil)
	for _, o := range fb.objects {
		if a, ok := o.(*microflows.Annotation); ok {
			notes = append(notes, a)
		}
	}
	return notes, fb.annotationFlows, fb.GetErrors()
}

// ---------------------------------------------------------------------------
// A — the reported defect
// ---------------------------------------------------------------------------

// One note wired to two activities must survive as ONE note with two flows.
//
// Control: revert getAnnotationsByTarget to filing bare captions (drop the
// Shared flag) and the rebuild yields 2 notes — the reported symptom.
func TestSharedNote_SurvivesTheRoundTripAsOneNote(t *testing.T) {
	mf := mfWithNotes([]*microflows.AnnotationFlow{
		{BaseElement: model.BaseElement{ID: "af1"}, OriginID: "n", DestinationID: "a1"},
		{BaseElement: model.BaseElement{ID: "af2"}, OriginID: "n", DestinationID: "a2"},
	}, note("n", "shared note", model.Point{X: 175, Y: -40}, model.Size{Width: 260, Height: 70}))

	body := describeBody(t, mf)
	if strings.Count(body, "@annotation") != 2 {
		t.Fatalf("both activities should still be annotated:\n%s", body)
	}
	if !strings.Contains(body, "@annotation(id: n1, text: 'shared note', position: (175, -40), size: (260, 70))") {
		t.Errorf("the first mention must declare the note with its geometry:\n%s", body)
	}
	if !strings.Contains(body, "@annotation(id: n1)") {
		t.Errorf("the second mention must REFERENCE the note, not repeat it:\n%s", body)
	}

	notes, flows, errs := rebuildFromBody(t, body)
	if len(errs) > 0 {
		t.Fatalf("rebuild reported errors: %v", errs)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1 — the note was duplicated (#1077)", len(notes))
	}
	if len(flows) != 2 {
		t.Fatalf("got %d annotation flows, want 2 — the note lost a connection", len(flows))
	}
	if flows[0].OriginID != notes[0].ID || flows[1].OriginID != notes[0].ID {
		t.Errorf("both flows must originate from the one note, got %s and %s (note %s)",
			flows[0].OriginID, flows[1].OriginID, notes[0].ID)
	}
	if got := notes[0].Position; got != (model.Point{X: 175, Y: -40}) {
		t.Errorf("position = %v, want (175,-40)", got)
	}
	if got := notes[0].Size; got != (model.Size{Width: 260, Height: 70}) {
		t.Errorf("size = %v, want 260x70", got)
	}
}

// The round trip must be a FIXED POINT, not merely better once. Describing the
// rebuilt flow has to give back the same text — otherwise the model drifts a
// little further on every describe → exec cycle, which is how this defect
// stayed invisible in the first place.
func TestSharedNote_DescribeIsAFixedPoint(t *testing.T) {
	mf := mfWithNotes([]*microflows.AnnotationFlow{
		{BaseElement: model.BaseElement{ID: "af1"}, OriginID: "n", DestinationID: "a1"},
		{BaseElement: model.BaseElement{ID: "af2"}, OriginID: "n", DestinationID: "a2"},
	}, note("n", "shared note", model.Point{X: 175, Y: -40}, model.Size{Width: 260, Height: 70}))

	first := describeBody(t, mf)
	notes, flows, _ := rebuildFromBody(t, first)

	rebuilt := mfWithNotes([]*microflows.AnnotationFlow{
		{BaseElement: model.BaseElement{ID: "af1"}, OriginID: notes[0].ID, DestinationID: "a1"},
		{BaseElement: model.BaseElement{ID: "af2"}, OriginID: notes[0].ID, DestinationID: "a2"},
	}, notes[0])
	if len(flows) != 2 {
		t.Fatalf("precondition: want 2 flows, got %d", len(flows))
	}

	if second := describeBody(t, rebuilt); second != first {
		t.Errorf("describe is not a fixed point:\n--- first\n%s\n--- second\n%s", first, second)
	}
}

// ---------------------------------------------------------------------------
// B — the defect the report did not mention, which destroys rather than copies
// ---------------------------------------------------------------------------

// Control: put back `result.AnnotationText = text` (a single slot) in the
// visitor and only 'second note' survives.
func TestTwoNotesOnOneActivity_BothSurvive(t *testing.T) {
	mf := mfWithNotes([]*microflows.AnnotationFlow{
		{BaseElement: model.BaseElement{ID: "af1"}, OriginID: "n1", DestinationID: "a1"},
		{BaseElement: model.BaseElement{ID: "af2"}, OriginID: "n2", DestinationID: "a1"},
	},
		note("n1", "first note", model.Point{X: 100, Y: 0}, DefaultAnnotationSize),
		note("n2", "second note", model.Point{X: 100, Y: -60}, DefaultAnnotationSize),
	)

	body := describeBody(t, mf)
	notes, flows, errs := rebuildFromBody(t, body)
	if len(errs) > 0 {
		t.Fatalf("rebuild reported errors: %v", errs)
	}
	var captions []string
	for _, n := range notes {
		captions = append(captions, n.Caption)
	}
	if len(notes) != 2 {
		t.Fatalf("got %v, want both notes — one was silently deleted (#1077)", captions)
	}
	if captions[0] != "first note" || captions[1] != "second note" {
		t.Errorf("captions = %v, want [first note second note]", captions)
	}
	if len(flows) != 2 {
		t.Errorf("got %d flows, want 2", len(flows))
	}
	// Both notes point at the same activity, and neither landed on top of the
	// other: stacking is what makes fixing B safe to ship without geometry.
	if notes[0].Position == notes[1].Position {
		t.Errorf("both notes are at %v — they overlap on the canvas", notes[0].Position)
	}
}

// ---------------------------------------------------------------------------
// C — geometry, and the reason the everyday form did not get longer
// ---------------------------------------------------------------------------

// A note at the position and size the writer re-derives keeps the short form.
// This is what stops the fix churning every microflow that has a note in it,
// and it is only sound because both sides go through
// defaultAnnotationGeometry — see the test below.
func TestUnsharedNoteAtTheDefault_KeepsTheShortForm(t *testing.T) {
	pos, size := defaultAnnotationGeometry(model.Point{X: 100, Y: 100}, 0)
	mf := mfWithNotes([]*microflows.AnnotationFlow{
		{BaseElement: model.BaseElement{ID: "af1"}, OriginID: "n1", DestinationID: "a1"},
	}, note("n1", "plain note", pos, size))

	body := describeBody(t, mf)
	if !strings.Contains(body, "@annotation 'plain note'") {
		t.Errorf("a note at the default geometry must still emit the short form:\n%s", body)
	}
	if strings.Contains(body, "at:") || strings.Contains(body, "size:") {
		t.Errorf("no geometry should be emitted for a default-placed note:\n%s", body)
	}
}

// A note that was moved or resized on the canvas spells its geometry out, and
// the rebuild honours it exactly.
func TestMovedNote_CarriesItsGeometryThroughTheRoundTrip(t *testing.T) {
	mf := mfWithNotes([]*microflows.AnnotationFlow{
		{BaseElement: model.BaseElement{ID: "af1"}, OriginID: "n1", DestinationID: "a1"},
	}, note("n1", "moved note", model.Point{X: -40, Y: 900}, model.Size{Width: 420, Height: 30}))

	body := describeBody(t, mf)
	if !strings.Contains(body, "@annotation(text: 'moved note', position: (-40, 900), size: (420, 30))") {
		t.Fatalf("geometry not emitted:\n%s", body)
	}
	notes, _, errs := rebuildFromBody(t, body)
	if len(errs) > 0 {
		t.Fatalf("rebuild reported errors: %v", errs)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1", len(notes))
	}
	if notes[0].Position != (model.Point{X: -40, Y: 900}) || notes[0].Size != (model.Size{Width: 420, Height: 30}) {
		t.Errorf("geometry lost: pos=%v size=%v", notes[0].Position, notes[0].Size)
	}
}

// The describer decides whether to emit `at:`/`size:` by asking what the writer
// would re-derive. If the two ever disagree the omission becomes a silent drift
// — the note creeps further on every round trip — so this pins that there is
// exactly one formula and both sides use it.
//
// Control: change the -100 in defaultAnnotationGeometry and this still passes
// (both sides moved together), while hardcoding the old constant in either side
// alone fails TestUnsharedNoteAtTheDefault_KeepsTheShortForm.
func TestAnnotationGeometryDefaultIsSharedByBothSides(t *testing.T) {
	activity := model.Point{X: 640, Y: 320}
	for index := 0; index < 3; index++ {
		wantPos, wantSize := defaultAnnotationGeometry(activity, index)

		fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing,
			varTypes: map[string]string{}, declaredVars: map[string]string{}}
		fb.objects = append(fb.objects, &microflows.ActionActivity{
			BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: "act"}, Position: activity}}})
		fb.attachAnnotation(ast.MicroflowAnnotation{Text: "x"}, "act", index)

		var got *microflows.Annotation
		for _, o := range fb.objects {
			if a, ok := o.(*microflows.Annotation); ok {
				got = a
			}
		}
		if got == nil {
			t.Fatalf("index %d: no annotation created", index)
		}
		if got.Position != wantPos || got.Size != wantSize {
			t.Errorf("index %d: writer placed the note at %v/%v, describer assumes %v/%v",
				index, got.Position, got.Size, wantPos, wantSize)
		}
	}
}

// ---------------------------------------------------------------------------
// Authoring the long form directly, and the refusals
// ---------------------------------------------------------------------------

func TestAuthorSharedNote_OneNoteTwoFlows(t *testing.T) {
	notes, flows, errs := rebuildFromBody(t, `
  @annotation(id: shared, text: 'watch out')
  log info node 'N' 'one';
  @annotation(id: shared)
  log info node 'N' 'two';
  return;`)
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(notes) != 1 || len(flows) != 2 {
		t.Fatalf("got %d notes / %d flows, want 1 / 2", len(notes), len(flows))
	}
	if notes[0].Caption != "watch out" {
		t.Errorf("caption = %q", notes[0].Caption)
	}
}

// Control for the test above: WITHOUT the id, the same two lines must produce
// two separate notes. Otherwise the test above would also pass for an
// implementation that deduplicated on text, which would be a different and
// wrong behaviour — two notes with the same words are still two notes.
func TestAuthorTwoNotesWithTheSameText_StayTwoNotes(t *testing.T) {
	notes, flows, errs := rebuildFromBody(t, `
  @annotation 'watch out'
  log info node 'N' 'one';
  @annotation 'watch out'
  log info node 'N' 'two';
  return;`)
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(notes) != 2 || len(flows) != 2 {
		t.Fatalf("got %d notes / %d flows, want 2 / 2 — identical text must not be merged",
			len(notes), len(flows))
	}
}

func TestAuthorNote_UndeclaredIDIsRefused(t *testing.T) {
	_, _, errs := rebuildFromBody(t, `
  @annotation(id: ghost)
  log info node 'N' 'one';
  return;`)
	if len(errs) == 0 {
		t.Fatal("a reference to a note that was never given text must be refused")
	}
	if !strings.Contains(strings.Join(errs, "\n"), "ghost") {
		t.Errorf("the error should name the id: %v", errs)
	}
}

func TestAuthorNote_SameIDWithDifferentTextIsRefused(t *testing.T) {
	_, _, errs := rebuildFromBody(t, `
  @annotation(id: n, text: 'one thing')
  log info node 'N' 'one';
  @annotation(id: n, text: 'another thing')
  log info node 'N' 'two';
  return;`)
	if len(errs) == 0 {
		t.Fatal("one id naming two different notes must be refused")
	}
}

// MDL079 catches at `check` time what the builder refuses at exec time. It has
// to be a separate check because a label is declared on one statement and used
// on another, and because the builder's errors do not escape a loop body.
func TestMDL079_RefusesAnUndeclaredNoteID(t *testing.T) {
	if ids := validateMicroflowSource(t, `
  @annotation(id: ghost)
  log info node 'N' 'one';
  return;`); !reportsRule(ids, "MDL079") {
		t.Errorf("check should report MDL079, got %v", ids)
	}
}

// The control: the same script WITH the declaration must be clean, or MDL079
// would be firing on everything and proving nothing.
func TestMDL079_AcceptsADeclaredNoteID(t *testing.T) {
	if ids := validateMicroflowSource(t, `
  @annotation(id: ok, text: 'declared here')
  log info node 'N' 'one';
  @annotation(id: ok)
  log info node 'N' 'two';
  return;`); reportsRule(ids, "MDL079") {
		t.Errorf("a declared id must not be reported: %v", ids)
	}
}

func TestMDL079_RefusesAnUnknownParameter(t *testing.T) {
	if ids := validateMicroflowSource(t, `
  @annotation(txt: 'typo in the key')
  log info node 'N' 'one';
  return;`); !reportsRule(ids, "MDL079") {
		t.Errorf("an unknown @annotation parameter must be reported, got %v", ids)
	}
}

func validateMicroflowSource(t *testing.T, body string) []string {
	t.Helper()
	prog, errs := visitor.Build("create microflow M.F ()\nbegin\n" + body + "\nend;")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	var ids []string
	for _, v := range ValidateMicroflow(mf) {
		ids = append(ids, v.RuleID)
	}
	return ids
}

func reportsRule(ids []string, ruleID string) bool {
	for _, id := range ids {
		if id == ruleID {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Adjacent gaps found while fixing the above
// ---------------------------------------------------------------------------

// A note written inside an `on error { … }` body was dropped: the handler's
// sub-builder collected annotation flows into its own slice, and the merge back
// into the parent copied objects and sequence flows but not annotation flows.
// The Annotation object arrived, the edge did not — so the note came back as a
// free-floating one, detached from the activity it documented.
func TestNoteInsideAnErrorHandlerKeepsItsConnection(t *testing.T) {
	notes, flows, errs := rebuildFromBody(t, `
  $Car = create M.Car () on error {
    @annotation 'why this failed'
    log error node 'N' 'boom';
  };
  return;`)
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1", len(notes))
	}
	if len(flows) != 1 {
		t.Fatalf("got %d annotation flows, want 1 — the note lost its connection", len(flows))
	}
	if flows[0].OriginID != notes[0].ID {
		t.Errorf("flow originates from %s, want the note %s", flows[0].OriginID, notes[0].ID)
	}
}

// The read half of the same gap. collectErrorHandlerStatements is a second,
// smaller describer — the main traversal never steps inside an `on error { … }`
// block — and it emitted no annotations at all. Fixing only the write half
// would have left the round trip losing the note exactly as before, with the
// model now merely differently wrong.
func TestNoteInsideAnErrorHandlerIsDescribed(t *testing.T) {
	notes, flows, errs := rebuildFromBody(t, `
  $Car = create M.Car () on error {
    @annotation 'why this failed'
    log error node 'N' 'boom';
  };
  return;`)
	if len(errs) > 0 || len(notes) != 1 || len(flows) != 1 {
		t.Fatalf("precondition: notes=%d flows=%d errs=%v", len(notes), len(flows), errs)
	}

	// Rebuild the described microflow from the model the builder produced, and
	// describe THAT — the note has to come back out of the handler body.
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing,
		varTypes: map[string]string{}, declaredVars: map[string]string{}}
	prog, _ := visitor.Build(`create microflow M.F ()
begin
  $Car = create M.Car () on error {
    @annotation 'why this failed'
    log error node 'N' 'boom';
  };
  return;
end;`)
	oc := fb.buildFlowGraph(prog.Statements[0].(*ast.CreateMicroflowStmt).Body, nil)
	body := describeBody(t, &microflows.Microflow{ObjectCollection: oc})
	if !strings.Contains(body, "why this failed") {
		t.Errorf("the handler-body note was dropped by DESCRIBE:\n%s", body)
	}
}

// A refusal raised inside a loop body used to be swallowed: the loop's
// sub-builder collected errors into its own slice and nothing merged them back,
// so exec reported success and wrote a flow with the note missing. `check`
// (MDL079) catches this shape too, but the two must agree.
func TestNoteRefusalInsideALoopBodyIsNotSwallowed(t *testing.T) {
	_, _, errs := rebuildFromBody(t, `
  declare $Items list of M.Car = empty;
  loop $Item in $Items begin
    @annotation(id: ghost)
    log info node 'N' 'inside';
  end loop;
  return;`)
	if len(errs) == 0 {
		t.Fatal("a bad note reference inside a loop body must reach the caller")
	}
}

// A note declared outside a loop and attached to an activity inside it. The
// describer emits this shape (its label state spans the loop overlay), so the
// builder has to accept it or exec would refuse mxcli's own DESCRIBE output.
//
// Measured at 0 errors on mxbuild 11.13 before this was allowed — see the
// comment on the loop sub-builder.
func TestNoteSharedAcrossALoopBoundary(t *testing.T) {
	notes, flows, errs := rebuildFromBody(t, `
  @annotation(id: n, text: 'spans the loop')
  log info node 'N' 'outer';
  loop $Item in $Items begin
    @annotation(id: n)
    log info node 'N' 'inner';
  end loop;
  return;`)
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1", len(notes))
	}
	if len(flows) != 2 {
		t.Fatalf("got %d annotation flows, want 2", len(flows))
	}
}

// `check` and `exec` must refuse the SAME scripts. A reference written above
// its declaration is one the builder cannot satisfy — it walks statements in
// order — so MDL079 refuses it too rather than passing a script exec will then
// reject. DESCRIBE never emits this shape.
func TestMDL079_AgreesWithTheBuilderOnDeclarationOrder(t *testing.T) {
	body := `
  @annotation(id: n)
  log info node 'N' 'one';
  @annotation(id: n, text: 'declared too late')
  log info node 'N' 'two';
  return;`

	if ids := validateMicroflowSource(t, body); !reportsRule(ids, "MDL079") {
		t.Errorf("check accepted a reference above its declaration, got %v", ids)
	}
	if _, _, errs := rebuildFromBody(t, body); len(errs) == 0 {
		t.Error("exec accepted a reference above its declaration")
	}
}
