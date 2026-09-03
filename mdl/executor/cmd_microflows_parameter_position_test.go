// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The reported symptom (#993): a parameter is a stored node with real geometry,
// and no annotation reached it — so a generated flow's parameter block landed
// wherever the writer put it, and a hand-aligned one was moved back there by any
// rewrite. Measured before the fix on a real project: a nanoflow parameter at
// -77;0 came back at 200;53 from a describe → exec of mxcli's own output.
//
// DerivedParameterPosition is the arithmetic both writers used inline. It is
// pinned here because the whole design rests on a reader being able to recognise
// it: a parameter sitting exactly there is mxcli's own layout handed back and
// carries no intent, so it is re-derived rather than pinned.
func TestDerivedParameterPositionMatchesTheWriters(t *testing.T) {
	for idx, want := range []model.Point{{X: 200, Y: 53}, {X: 300, Y: 53}, {X: 400, Y: 53}} {
		if got := microflows.DerivedParameterPosition(idx); got != want {
			t.Errorf("DerivedParameterPosition(%d) = %v, want %v", idx, got, want)
		}
	}
}

// A stored position that is the derived one carries no intent and must not be
// carried over. Carrying it UNCONDITIONALLY is the #951 mistake one node family
// over: inserting a parameter would leave the existing ones on the old grid
// while the new one lands on top of them.
func TestAuthoredParameterPositionIgnoresTheDerivedSpot(t *testing.T) {
	if p := microflows.AuthoredParameterPosition(model.Point{X: 200, Y: 53}, 0); p != nil {
		t.Errorf("index 0 at the derived spot: got %v, want nil", *p)
	}
	if p := microflows.AuthoredParameterPosition(model.Point{X: 300, Y: 53}, 1); p != nil {
		t.Errorf("index 1 at the derived spot: got %v, want nil", *p)
	}
	// The same point at a DIFFERENT index is not the derived one, so it is intent.
	got := microflows.AuthoredParameterPosition(model.Point{X: 200, Y: 53}, 1)
	if got == nil || *got != (model.Point{X: 200, Y: 53}) {
		t.Errorf("index 1 at index 0's spot: got %v, want 200;53 kept", got)
	}
}

// 0;0 is a position a person can choose — two flows in the reference project use
// it — so "unset" cannot be spelled as the zero value. This is why Position is a
// pointer, and the test exists because a bool-free `if p.Position != (Point{})`
// would pass every other case here.
func TestAuthoredParameterPositionKeepsOrigin(t *testing.T) {
	got := microflows.AuthoredParameterPosition(model.Point{X: 0, Y: 0}, 0)
	if got == nil {
		t.Fatal("0;0 was dropped as if unset; it is a position a person can choose")
	}
	if *got != (model.Point{}) {
		t.Errorf("got %v, want 0;0", *got)
	}
}

// DESCRIBE emits the annotation only for an authored position. Emitting the
// derived one would restate mxcli's own arithmetic and pin every parameter of
// every rewritten flow — the round-trip failure startAnnotationLines documents.
func TestParameterPositionAnnotation(t *testing.T) {
	derived := &microflows.MicroflowParameter{Position: &model.Point{X: 200, Y: 53}}
	if got := parameterPositionAnnotation(derived, 0, "  "); got != "" {
		t.Errorf("derived position emitted %q, want no line", got)
	}
	authored := &microflows.MicroflowParameter{Position: &model.Point{X: -77, Y: 0}}
	if got := parameterPositionAnnotation(authored, 0, "  "); got != "  @position(-77, 0)" {
		t.Errorf("got %q, want \"  @position(-77, 0)\"", got)
	}
	if got := parameterPositionAnnotation(&microflows.MicroflowParameter{}, 0, "  "); got != "" {
		t.Errorf("unset position emitted %q, want no line", got)
	}
}

// The control for the fix: with Position dropped on the way in — which is
// exactly what both readers did before #993 — the describer emits nothing and
// the writer has only the index to go on, so the annotation cannot round-trip.
// Without this the suite would pass against a build that never had the fix.
func TestDescribeMicroflowParametersCarriesAuthoredPositionOnly(t *testing.T) {
	fmtType := func(*microflows.MicroflowParameter) string { return "Integer" }
	params := []*microflows.MicroflowParameter{
		{Name: "A", Type: &microflows.IntegerType{}, Position: &model.Point{X: 300, Y: 100}},
		{Name: "B", Type: &microflows.IntegerType{}},
	}
	got := describeMicroflowParameters(params, fmtType)
	want := []string{"  @position(300, 100)", "  $A: Integer,", "  $B: Integer"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %q, want %d %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}

	// Control: strip the positions, as the pre-fix readers did.
	for _, p := range params {
		p.Position = nil
	}
	if got := describeMicroflowParameters(params, fmtType); len(got) != 2 {
		t.Errorf("with positions dropped, got %q — the annotation must not appear", got)
	}
}

// An annotation a parameter does not take is refused, not ignored. A typo of
// @position is the case that matters: it parses, does nothing, and discards the
// placement the author was trying to state.
func TestValidateFlowParameterAnnotations(t *testing.T) {
	params := []ast.MicroflowParam{
		{Name: "A", UnknownAnnotations: []string{"postion"}},
		{Name: "B", Position: &ast.Position{X: 1, Y: 2}},
	}
	got := ValidateFlowParameterAnnotations("nanoflow 'M.NF'", params)
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	if got[0].RuleID != "MDL059" {
		t.Errorf("rule = %s, want MDL059", got[0].RuleID)
	}
	// Control: a parameter with a valid @position and no unknown names is clean.
	if v := ValidateFlowParameterAnnotations("x", params[1:]); len(v) != 0 {
		t.Errorf("valid @position flagged: %+v", v)
	}
}
