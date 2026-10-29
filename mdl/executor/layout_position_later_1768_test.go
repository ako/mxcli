// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// A statement placed with @position starts the row for what follows it, and the
// start event sits one spacing left of the first statement when THAT one is
// placed. The builder found "the first statement with an @position" instead of
// looking at the first statement, so a position written on a later statement
// moved the start event to it — and every unplaced statement before it along
// with the start, onto the placed one (ako/mxcli#1768 case 2, mendixlabs/mxcli#1672).
// Reported: `describe … with handles` printed @position(840, 200) for both
// `declare $A` and `declare $B`, and MPR008 reported the overlap.
func TestStatementsBeforeAPlacedOneKeepTheirDerivedPlaces(t *testing.T) {
	oc := buildLayout(t, `create microflow MyModule.SUB_PositionedLater ($Url: String) returns String as $B
begin
  declare $A String = $Url;
  @position(840, 200)
  declare $B String = $A;
  return $B;
end;`)
	if overlaps := canvasOverlaps(oc.Objects); len(overlaps) > 0 {
		t.Errorf("objects share a place:\n  %s", strings.Join(overlaps, "\n  "))
	}
	got := map[string]bool{}
	for _, o := range oc.Objects {
		got[canvasLabel(o)] = true
	}
	for _, want := range []string{
		"StartEvent 200;200",     // where a flow with no placed first statement starts
		"ActionActivity 360;200", // declare $A: one spacing after the start
		"ActionActivity 840;200", // declare $B: where the script put it
		"EndEvent 1000;200",      // one spacing after $B
	} {
		if !got[want] {
			t.Errorf("missing %s", want)
		}
	}
}

// Control for the rule above: a placed FIRST statement still carries the start
// event with it, one spacing to its left on its centre line.
func TestPlacedFirstStatementStillCarriesTheStartEvent(t *testing.T) {
	oc := buildLayout(t, `create microflow MyModule.SUB_PositionedFirst ($Url: String) returns String as $B
begin
  @position(840, 300)
  declare $A String = $Url;
  declare $B String = $A;
  return $B;
end;`)
	got := map[string]bool{}
	for _, o := range oc.Objects {
		got[canvasLabel(o)] = true
	}
	for _, want := range []string{
		"StartEvent 680;300",
		"ActionActivity 840;300",
		"ActionActivity 1000;300",
	} {
		if !got[want] {
			t.Errorf("missing %s", want)
		}
	}
}
