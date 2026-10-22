// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// A parameter is not on the line a fragment is spliced into, so making room
// does not move it (ako/mxcli#1547). shift moved every top-level node past the
// cut, parameters included: one inserted if/else moved a nanoflow's `$Flag`
// from its derived (200, 53) to (539, 53), and from then on describe printed a
// @position the script never stated, the row drifting right with every insert.
// Studio Pro authors draw a parameter above the start event — 63 of 71 in
// PedApp and JTSBootLogboek sit at the start's x, none within 60px of the line
// — and an insert in Studio Pro moves nothing. The control is the activity
// after the gap, which does move.
func TestSplice_MakingRoomLeavesTheParameters(t *testing.T) {
	positions := func(m *Mutator) map[string]point {
		out := map[string]point{}
		for _, n := range m.graph().order {
			out[n.id] = n.pos
		}
		return out
	}
	objs := append(line(), obj("param", "Microflows$MicroflowParameter", 500, 53))
	for _, c := range []struct {
		name   string
		splice func(m *Mutator) error
	}{
		{"an insert", func(m *Mutator) error { return m.InsertAfter(model.ID(uid("a")), oneActivity()) }},
		{"a replace longer than what it replaces", func(m *Mutator) error { return m.Replace(model.ID(uid("a")), twoActivities()) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, _ := newMutator(t, unit(objs, lineFlows()))
			if err := c.splice(m); err != nil {
				t.Fatal(err)
			}
			got := positions(m)
			if p := got[uid("b")]; p.X <= 420 {
				t.Fatalf("control: b is at x=%d, want it moved along past 420 to make room", p.X)
			}
			if p := got[uid("param")]; p != (point{500, 53}) {
				t.Errorf("the parameter moved to (%d, %d), want it left at (500, 53)", p.X, p.Y)
			}
		})
	}
}
