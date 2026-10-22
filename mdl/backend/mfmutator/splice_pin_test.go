// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A node whose position the script states is not moved to make room
// (mendixlabs/mxcli#1598, case 6). The splice moved every node past the gap
// along by what the fragment lacked, including ones the script draws at a
// stated @position — so the stored flow no longer said what the script said,
// a fragment placed at its own stated position was left behind on the wrong
// side of its decision, and re-running the same script moved the nodes back
// (or was refused). Such a shift is refused now, saying which node is in the
// way; the controls show the same splice still makes room when nothing past
// the gap is pinned.
func TestSplice_MakingRoomRefusesToMoveAPinnedNode(t *testing.T) {
	positions := func(m *Mutator) map[string]point {
		out := map[string]point{}
		for _, n := range m.graph().order {
			out[n.id] = n.pos
		}
		return out
	}
	// b at (420, 200) sits 50px right of a; a 120px activity needs 200.
	t.Run("an insert, the node after the gap pinned", func(t *testing.T) {
		m, _ := newMutator(t, unit(line(), lineFlows()))
		if err := m.Pin(model.Point{X: 420, Y: 200}); err != nil {
			t.Fatal(err)
		}
		err := m.InsertAfter(model.ID(uid("a")), oneActivity())
		if err == nil || !strings.Contains(err.Error(), "the ActionActivity at (420, 200)") ||
			!strings.Contains(err.Error(), "150px") {
			t.Fatalf("got %v, want a refusal naming the pinned activity at (420, 200) and the 150px lacking", err)
		}
		if p := positions(m)[uid("b")]; p.X != 420 {
			t.Errorf("b moved to x=%d before the refusal", p.X)
		}
	})
	t.Run("control: nothing pinned", func(t *testing.T) {
		m, _ := newMutator(t, unit(line(), lineFlows()))
		if err := m.InsertAfter(model.ID(uid("a")), oneActivity()); err != nil {
			t.Fatal(err)
		}
		if p := positions(m)[uid("b")]; p.X != 570 {
			t.Errorf("b is at x=%d, want it moved along by 150 to 570", p.X)
		}
	})
	t.Run("control: only nodes before the gap pinned", func(t *testing.T) {
		m, _ := newMutator(t, unit(line(), lineFlows()))
		for _, p := range []model.Point{{X: 100, Y: 200}, {X: 250, Y: 200}} {
			if err := m.Pin(p); err != nil {
				t.Fatal(err)
			}
		}
		if err := m.InsertAfter(model.ID(uid("a")), oneActivity()); err != nil {
			t.Fatalf("a pin before the gap refused the insert: %v", err)
		}
	})
	t.Run("a replace longer than what it replaces, the end pinned", func(t *testing.T) {
		m, _ := newMutator(t, unit(line(), lineFlows()))
		if err := m.Pin(model.Point{X: 600, Y: 200}); err != nil {
			t.Fatal(err)
		}
		err := m.Replace(model.ID(uid("a")), twoActivities())
		if err == nil || !strings.Contains(err.Error(), "the EndEvent at (600, 200)") {
			t.Fatalf("got %v, want a refusal naming the pinned end event", err)
		}
	})
	t.Run("an earlier fragment drawn at a stated point", func(t *testing.T) {
		// The script states where the first fragment goes, so it is pinned
		// like a stored node: a later insert cannot slide it along.
		m, _ := newMutator(t, unit(line(), lineFlows()))
		placed := oneActivity()
		placed.Placed = true
		placed.Objects[0].SetPosition(model.Point{X: 600, Y: 400})
		if err := m.Pin(model.Point{X: 600, Y: 400}); err != nil {
			t.Fatal(err)
		}
		if err := m.InsertAfter(model.ID(uid("b")), placed); err != nil {
			t.Fatal(err)
		}
		err := m.InsertAfter(model.ID(uid("a")), oneActivity())
		if err == nil || !strings.Contains(err.Error(), "(600, 400)") {
			t.Fatalf("got %v, want a refusal naming the placed activity at (600, 400)", err)
		}
	})
}

// twoActivities is a two-activity fragment in builder coordinates, 290px long.
func twoActivities() *backend.MicroflowFragment {
	act := func(x int) *microflows.ActionActivity {
		return &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    model.Point{X: x, Y: 200},
			Size:        model.Size{Width: 120, Height: 60},
		}}}
	}
	a, b := act(360), act(530)
	return &backend.MicroflowFragment{
		Objects: []microflows.MicroflowObject{a, b},
		Flows: []*microflows.SequenceFlow{{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			OriginID:    a.ID, DestinationID: b.ID,
		}},
		Entry: a.ID, Exit: b.ID,
	}
}
