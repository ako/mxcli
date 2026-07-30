// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#791: a microflow with a loop containing a split whose branch
// does `continue` wrote successfully but could not be opened in Studio Pro —
// "System.Collections.Generic.KeyNotFoundException: The given key '<guid>' was not
// present in the dictionary".
//
// microflowObjectToGen had no case for BreakEvent/ContinueEvent and its default
// branch returns nil, so the event object was dropped at serialization while the
// sequence flow pointing at it was written. The result is a dangling
// DestinationPointer: exactly the GUID Studio Pro cannot resolve. The legacy engine
// (sdk/mpr/writer_microflow.go) serializes both, so this was a modelsdk-engine gap.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestMicroflowObjectToGen_LoopEvents(t *testing.T) {
	tests := []struct {
		name     string
		obj      microflows.MicroflowObject
		wantType string
	}{
		{
			name: "break",
			obj: &microflows.BreakEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: "ev-break"},
				Position:    model.Point{X: 100, Y: 200},
				Size:        model.Size{Width: 20, Height: 20},
			}},
			wantType: "Microflows$BreakEvent",
		},
		{
			name: "continue",
			obj: &microflows.ContinueEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: "ev-continue"},
				Position:    model.Point{X: 300, Y: 400},
				Size:        model.Size{Width: 20, Height: 20},
			}},
			wantType: "Microflows$ContinueEvent",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := microflowObjectToGen(tc.obj)
			if g == nil {
				t.Fatalf("%s dropped at serialization — any flow pointing at it is left dangling (#791)", tc.wantType)
			}
			if g.TypeName() != tc.wantType {
				t.Errorf("TypeName = %q, want %q", g.TypeName(), tc.wantType)
			}
			if string(g.ID()) != string(tc.obj.GetID()) {
				t.Errorf("ID = %q, want %q — a changed ID leaves the flow pointing at the old one",
					g.ID(), tc.obj.GetID())
			}
		})
	}
}
