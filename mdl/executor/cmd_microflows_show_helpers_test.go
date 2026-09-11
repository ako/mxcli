// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// =============================================================================
// findBranchFlows
// =============================================================================

func TestFindBranchFlows_ExpressionCase(t *testing.T) {
	trueF := mkBranchFlow("s", "a", &microflows.ExpressionCase{Expression: "true"})
	falseF := mkBranchFlow("s", "b", &microflows.ExpressionCase{Expression: "false"})
	flows := []*microflows.SequenceFlow{trueF, falseF}

	gotTrue, gotFalse := findBranchFlows(flows)
	if gotTrue != trueF {
		t.Errorf("expected true flow to destination %s, got %v", trueF.DestinationID, gotTrue)
	}
	if gotFalse != falseF {
		t.Errorf("expected false flow to destination %s, got %v", falseF.DestinationID, gotFalse)
	}
}

func TestFindBranchFlows_EnumerationCase(t *testing.T) {
	trueF := mkBranchFlow("s", "a", microflows.EnumerationCase{Value: "true"})
	falseF := mkBranchFlow("s", "b", microflows.EnumerationCase{Value: "false"})
	flows := []*microflows.SequenceFlow{falseF, trueF} // reversed order

	gotTrue, gotFalse := findBranchFlows(flows)
	if gotTrue != trueF {
		t.Error("expected true flow via EnumerationCase")
	}
	if gotFalse != falseF {
		t.Error("expected false flow via EnumerationCase")
	}
}

func TestFindBranchFlows_BooleanCase(t *testing.T) {
	trueF := mkBranchFlow("s", "a", microflows.BooleanCase{Value: true})
	falseF := mkBranchFlow("s", "b", microflows.BooleanCase{Value: false})
	flows := []*microflows.SequenceFlow{trueF, falseF}

	gotTrue, gotFalse := findBranchFlows(flows)
	if gotTrue != trueF {
		t.Error("expected true flow via BooleanCase")
	}
	if gotFalse != falseF {
		t.Error("expected false flow via BooleanCase")
	}
}

func TestFindBranchFlows_NilCaseValue(t *testing.T) {
	flow := mkFlow("s", "a") // no CaseValue
	flows := []*microflows.SequenceFlow{flow}

	gotTrue, gotFalse := findBranchFlows(flows)
	if gotTrue != nil {
		t.Error("expected nil true flow for nil CaseValue")
	}
	if gotFalse != nil {
		t.Error("expected nil false flow for nil CaseValue")
	}
}

func TestFindBranchFlows_EmptyFlows(t *testing.T) {
	gotTrue, gotFalse := findBranchFlows(nil)
	if gotTrue != nil || gotFalse != nil {
		t.Error("expected nil flows for empty input")
	}
}

// =============================================================================
// findErrorHandlerFlow
// =============================================================================

func TestFindErrorHandlerFlow_Found(t *testing.T) {
	normal := mkFlow("a", "b")
	errFlow := mkErrorFlow("a", "c")
	flows := []*microflows.SequenceFlow{normal, errFlow}

	got := findErrorHandlerFlow(flows)
	if got != errFlow {
		t.Error("expected error handler flow")
	}
}

func TestFindErrorHandlerFlow_NotFound(t *testing.T) {
	flows := []*microflows.SequenceFlow{mkFlow("a", "b"), mkFlow("a", "c")}
	if got := findErrorHandlerFlow(flows); got != nil {
		t.Error("expected nil when no error handler flow")
	}
}

func TestFindErrorHandlerFlow_Nil(t *testing.T) {
	if got := findErrorHandlerFlow(nil); got != nil {
		t.Error("expected nil for nil input")
	}
}

// =============================================================================
// findNormalFlows
// =============================================================================

func TestFindNormalFlows_FiltersErrors(t *testing.T) {
	normal1 := mkFlow("a", "b")
	normal2 := mkFlow("a", "c")
	errFlow := mkErrorFlow("a", "d")
	flows := []*microflows.SequenceFlow{normal1, errFlow, normal2}

	got := findNormalFlows(flows)
	if len(got) != 2 {
		t.Fatalf("expected 2 normal flows, got %d", len(got))
	}
	if got[0] != normal1 || got[1] != normal2 {
		t.Error("expected normal1 and normal2")
	}
}

func TestFindNormalFlows_AllErrors(t *testing.T) {
	flows := []*microflows.SequenceFlow{mkErrorFlow("a", "b")}
	got := findNormalFlows(flows)
	if len(got) != 0 {
		t.Errorf("expected 0 normal flows, got %d", len(got))
	}
}

func TestEmitObjectAnnotations_EscapesMultilineText(t *testing.T) {
	obj := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: mkID("act")},
				Position:    model.Point{X: 100, Y: 200},
			},
			Caption:             "Caption\nLine",
			AutoGenerateCaption: false,
		},
	}

	annotationsByTarget := &annotationEmitter{
		byTarget: map[model.ID][]describedAnnotation{
			mkID("act"): {{Caption: "Note\nLine\tTabbed", Position: mustDefaultAnnotationPos(model.Point{X: 100, Y: 200}, 0)}},
		},
	}

	var lines []string
	// Pass nil flow maps — @anchor emission is intentionally suppressed here.
	emitObjectAnnotations(obj, &lines, "", annotationsByTarget, nil, nil, nil)

	got := strings.Join(lines, "\n")
	if !strings.Contains(got, "@caption 'Caption\\nLine'") {
		t.Fatalf("expected escaped caption, got:\n%s", got)
	}
	if !strings.Contains(got, "@annotation 'Note\\nLine\\tTabbed'") {
		t.Fatalf("expected escaped annotation, got:\n%s", got)
	}
}

func TestEmitObjectAnnotations_LoopCaption(t *testing.T) {
	obj := &microflows.LoopedActivity{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: mkID("loop")},
			Position:    model.Point{X: 100, Y: 200},
		},
		Caption: "Loop owner's\ncaption",
	}

	var lines []string
	// Pass nil flow maps — @anchor emission is intentionally suppressed here.
	emitObjectAnnotations(obj, &lines, "", nil, nil, nil, nil)

	got := strings.Join(lines, "\n")
	if !strings.Contains(got, "@caption 'Loop owner''s\\ncaption'") {
		t.Fatalf("expected escaped loop caption, got:\n%s", got)
	}
}

func TestPrependFreeAnnotationLines_ModelAnnotationsStayFree(t *testing.T) {
	oc := &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{
			&microflows.Annotation{
				BaseMicroflowObject: mkObj("free-note"),
				Caption:             "free synthetic note",
			},
			&microflows.Annotation{
				BaseMicroflowObject: mkObj("attached-note"),
				Caption:             "attached synthetic note",
			},
		},
		AnnotationFlows: []*microflows.AnnotationFlow{
			{
				BaseElement:   model.BaseElement{ID: mkID("annotation-flow")},
				OriginID:      mkID("attached-note"),
				DestinationID: mkID("activity"),
			},
		},
	}

	activityLines := []string{
		"@position(100, 200)",
		"@annotation 'attached synthetic note'",
		"log info 'Synthetic' 'message';",
	}

	gotLines := prependFreeAnnotationLines(oc, activityLines)
	got := strings.Join(gotLines, "\n")

	want := strings.Join([]string{
		// A free note is wired to no activity, so there is no activity position
		// for the writer to derive its place from — its own position is always
		// emitted. This fixture sets none, hence (0, 0); a real canvas note
		// carries the coordinates Studio Pro gave it (#1077).
		"@annotation(text: 'free synthetic note', position: (0, 0))",
		"@position(100, 200)",
		"@annotation 'attached synthetic note'",
		"log info 'Synthetic' 'message';",
	}, "\n")
	if got != want {
		t.Fatalf("free annotation describe output mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}
	if strings.Count(got, "attached synthetic note") != 1 {
		t.Fatalf("attached annotation was emitted as free too:\n%s", got)
	}
}

// =============================================================================
// formatErrorHandlingSuffix
// =============================================================================

// Rollback is the value stored when the author wrote no clause at all
// (convertErrorHandlingType(nil)) and the value the parser falls back to when
// the BSON key is absent. It therefore carries no information about what the
// author wrote, and read-back must not emit it — see issue #840 and
// TestFormatErrorHandlingSuffix_RollbackIsNotEmitted below.
func TestFormatErrorHandlingSuffix(t *testing.T) {
	tests := []struct {
		errType microflows.ErrorHandlingType
		want    string
	}{
		{microflows.ErrorHandlingTypeContinue, " on error continue"},
		{microflows.ErrorHandlingTypeRollback, ""},
		{microflows.ErrorHandlingTypeCustom, " on error"},
		{microflows.ErrorHandlingTypeCustomWithoutRollback, " on error without rollback"},
		{microflows.ErrorHandlingTypeAbort, ""},
		{"", ""},
		{"SomethingElse", ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.errType), func(t *testing.T) {
			got := formatErrorHandlingSuffix(tt.errType)
			if got != tt.want {
				t.Errorf("formatErrorHandlingSuffix(%q) = %q, want %q", tt.errType, got, tt.want)
			}
		})
	}
}

// =============================================================================
// hasCustomErrorHandler
// =============================================================================

func TestHasCustomErrorHandler(t *testing.T) {
	tests := []struct {
		errType microflows.ErrorHandlingType
		want    bool
	}{
		{microflows.ErrorHandlingTypeCustom, true},
		{microflows.ErrorHandlingTypeCustomWithoutRollback, true},
		{microflows.ErrorHandlingTypeContinue, false},
		{microflows.ErrorHandlingTypeRollback, false},
		{microflows.ErrorHandlingTypeAbort, false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.errType), func(t *testing.T) {
			got := hasCustomErrorHandler(tt.errType)
			if got != tt.want {
				t.Errorf("hasCustomErrorHandler(%q) = %v, want %v", tt.errType, got, tt.want)
			}
		})
	}
}

// =============================================================================
// getActionErrorHandlingType
// =============================================================================

func TestGetActionErrorHandlingType_NilActivity(t *testing.T) {
	got := getActionErrorHandlingType(nil)
	if got != "" {
		t.Errorf("expected empty for nil activity, got %q", got)
	}
}

func TestGetActionErrorHandlingType_NilAction(t *testing.T) {
	activity := &microflows.ActionActivity{}
	got := getActionErrorHandlingType(activity)
	if got != "" {
		t.Errorf("expected empty for nil action, got %q", got)
	}
}

func TestGetActionErrorHandlingType_MicroflowCall(t *testing.T) {
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			ErrorHandlingType: microflows.ErrorHandlingTypeAbort, // activity level
		},
	}
	activity.Action = &microflows.MicroflowCallAction{
		ErrorHandlingType: microflows.ErrorHandlingTypeContinue, // action level
	}
	got := getActionErrorHandlingType(activity)
	if got != microflows.ErrorHandlingTypeContinue {
		t.Errorf("expected action-level Continue, got %q", got)
	}
}

func TestGetActionErrorHandlingType_JavaActionCall(t *testing.T) {
	activity := &microflows.ActionActivity{}
	activity.Action = &microflows.JavaActionCallAction{
		ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
	}
	got := getActionErrorHandlingType(activity)
	if got != microflows.ErrorHandlingTypeRollback {
		t.Errorf("expected Rollback, got %q", got)
	}
}

func TestGetActionErrorHandlingType_RestCall(t *testing.T) {
	activity := &microflows.ActionActivity{}
	activity.Action = &microflows.RestCallAction{
		ErrorHandlingType: microflows.ErrorHandlingTypeCustom,
	}
	got := getActionErrorHandlingType(activity)
	if got != microflows.ErrorHandlingTypeCustom {
		t.Errorf("expected Custom, got %q", got)
	}
}

func TestGetActionErrorHandlingType_CommitObjects(t *testing.T) {
	activity := &microflows.ActionActivity{}
	activity.Action = &microflows.CommitObjectsAction{
		ErrorHandlingType: microflows.ErrorHandlingTypeCustomWithoutRollback,
	}
	got := getActionErrorHandlingType(activity)
	if got != microflows.ErrorHandlingTypeCustomWithoutRollback {
		t.Errorf("expected CustomWithoutRollBack, got %q", got)
	}
}

func TestGetActionErrorHandlingType_CallExternal(t *testing.T) {
	activity := &microflows.ActionActivity{}
	activity.Action = &microflows.CallExternalAction{
		ErrorHandlingType: microflows.ErrorHandlingTypeContinue,
	}
	got := getActionErrorHandlingType(activity)
	if got != microflows.ErrorHandlingTypeContinue {
		t.Errorf("expected Continue, got %q", got)
	}
}

func TestGetActionErrorHandlingType_FallbackToActivity(t *testing.T) {
	activity := &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
		},
	}
	// CreateObjectAction has no ErrorHandlingType field — falls back to activity
	activity.Action = &microflows.CreateObjectAction{}
	got := getActionErrorHandlingType(activity)
	if got != microflows.ErrorHandlingTypeRollback {
		t.Errorf("expected activity-level Rollback, got %q", got)
	}
}

// =============================================================================
// formatActivity
// =============================================================================

func TestFormatActivity_StartEvent(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.StartEvent{BaseMicroflowObject: mkObj("1")}
	got := e.formatActivity(obj, nil, nil)
	if got != "" {
		t.Errorf("expected empty for StartEvent, got %q", got)
	}
}

func TestFormatActivity_EndEvent_VoidOrUnknownContext(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.EndEvent{BaseMicroflowObject: mkObj("1")}
	got := e.formatActivity(obj, nil, nil)
	if got != "return;" {
		t.Errorf("expected bare return for EndEvent without return, got %q", got)
	}
}

func TestFormatActivity_EndEvent_NoReturnValueInReturningMicroflow(t *testing.T) {
	obj := &microflows.EndEvent{BaseMicroflowObject: mkObj("1")}
	got := formatActivity(&ExecContext{DescribingMicroflowHasReturnValue: true}, obj, nil, nil)
	if got != "" {
		t.Errorf("expected empty EndEvent to be skipped in value-returning microflow, got %q", got)
	}
}

func TestFormatActivity_EndEvent_WithReturn(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.EndEvent{
		BaseMicroflowObject: mkObj("1"),
		ReturnValue:         "Result",
	}
	got := e.formatActivity(obj, nil, nil)
	if got != "return $Result;" {
		t.Errorf("got %q, want %q", got, "return $Result;")
	}
}

func TestFormatActivity_EndEvent_WithDollarPrefix(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.EndEvent{
		BaseMicroflowObject: mkObj("1"),
		ReturnValue:         "$Result",
	}
	got := e.formatActivity(obj, nil, nil)
	if got != "return $Result;" {
		t.Errorf("got %q, want %q (should not double the $)", got, "return $Result;")
	}
}

func TestFormatActivity_ExclusiveSplit(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.ExclusiveSplit{
		BaseMicroflowObject: mkObj("1"),
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$Count > 5"},
	}
	got := e.formatActivity(obj, nil, nil)
	if got != "if $Count > 5 then" {
		t.Errorf("got %q, want %q", got, "if $Count > 5 then")
	}
}

func TestFormatActivity_ExclusiveSplit_NilCondition(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.ExclusiveSplit{BaseMicroflowObject: mkObj("1")}
	got := e.formatActivity(obj, nil, nil)
	if got != "if true then" {
		t.Errorf("got %q, want %q", got, "if true then")
	}
}

func TestFormatActivity_ExclusiveMerge(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.ExclusiveMerge{BaseMicroflowObject: mkObj("1")}
	got := e.formatActivity(obj, nil, nil)
	if got != "end if;" {
		t.Errorf("got %q, want %q", got, "end if;")
	}
}

func TestFormatActivity_LoopedActivity(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.LoopedActivity{
		BaseMicroflowObject: mkObj("1"),
		LoopSource: &microflows.IterableList{
			VariableName:     "Order",
			ListVariableName: "OrderList",
		},
	}
	got := e.formatActivity(obj, nil, nil)
	if got != "loop $Order in $OrderList" {
		t.Errorf("got %q, want %q", got, "loop $Order in $OrderList")
	}
}

func TestFormatActivity_LoopedActivity_Defaults(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.LoopedActivity{BaseMicroflowObject: mkObj("1")}
	got := e.formatActivity(obj, nil, nil)
	if got != "loop $Item in $List" {
		t.Errorf("got %q, want %q", got, "loop $Item in $List")
	}
}

func TestFormatActivity_BreakEvent(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.BreakEvent{BaseMicroflowObject: mkObj("1")}
	got := e.formatActivity(obj, nil, nil)
	if got != "break;" {
		t.Errorf("got %q, want %q", got, "break;")
	}
}

func TestFormatActivity_ContinueEvent(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.ContinueEvent{BaseMicroflowObject: mkObj("1")}
	got := e.formatActivity(obj, nil, nil)
	if got != "continue;" {
		t.Errorf("got %q, want %q", got, "continue;")
	}
}

func TestFormatActivity_ErrorEvent(t *testing.T) {
	e := newTestExecutor()
	obj := &microflows.ErrorEvent{BaseMicroflowObject: mkObj("1")}
	got := e.formatActivity(obj, nil, nil)
	if got != "raise error;" {
		t.Errorf("got %q, want %q", got, "raise error;")
	}
}

// TestFormatErrorHandlingSuffix_RollbackIsNotEmitted guards issue #840.
//
// DESCRIBE MICROFLOW invented an `on error rollback` clause on activities that
// were never written with one:
//
//	call microflow TFC.MF_Leaf();            -- authored
//	call microflow TFC.MF_Leaf() on error rollback;   -- described
//
// "Rollback" is what convertErrorHandlingType(nil) stores for an activity with
// no clause, and what the parser falls back to when ErrorHandlingType is absent
// from the BSON. It is structurally valid, so no checker can flag the invented
// clause — it just quietly grows the diff on every describe round-trip.
//
// Emitting nothing for Rollback is lossless: re-executing describe output
// without the clause stores Rollback again, so the model round-trips exactly.
// Emitting it is the lossy direction, because it adds a clause to the script
// that the author did not write.
func TestFormatErrorHandlingSuffix_RollbackIsNotEmitted(t *testing.T) {
	if got := formatErrorHandlingSuffix(microflows.ErrorHandlingTypeRollback); got != "" {
		t.Errorf("Rollback suffix = %q, want \"\" — Rollback is the stored default and cannot be distinguished from an authored clause", got)
	}
	// The explicit-only values must still round-trip: none of them is ever a
	// default, so their presence always means the author wrote them.
	explicit := map[microflows.ErrorHandlingType]string{
		microflows.ErrorHandlingTypeContinue:              " on error continue",
		microflows.ErrorHandlingTypeCustom:                " on error",
		microflows.ErrorHandlingTypeCustomWithoutRollback: " on error without rollback",
	}
	for errType, want := range explicit {
		if got := formatErrorHandlingSuffix(errType); got != want {
			t.Errorf("formatErrorHandlingSuffix(%q) = %q, want %q", errType, got, want)
		}
	}
}

// mustDefaultAnnotationPos is the position the writer would give an unplaced
// note, so this test exercises the SHORT emit form rather than accidentally
// asserting escaping on the parameterised one.
func mustDefaultAnnotationPos(activity model.Point, index int) model.Point {
	pos, _ := defaultAnnotationGeometry(activity, index)
	return pos
}
