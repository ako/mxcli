// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func rmp(t *testing.T, doc bson.D) string {
	t.Helper()
	v, _ := doc.Map()["RelativeMiddlePoint"].(string)
	return v
}

// #993: a hand-placed parameter must be written where it was placed. Before the
// fix the legacy serializer computed the position from the index and ignored
// anything stored, so a describe → exec of mxcli's own output moved a real
// parameter from -77;0 to 200;53.
func TestSerializeMicroflowParameterKeepsAuthoredPosition(t *testing.T) {
	authored := &microflows.MicroflowParameter{
		Name:     "Feedback",
		Position: &model.Point{X: -77, Y: 0},
	}
	if got := rmp(t, serializeMicroflowParameter(authored, 0, 11)); got != "-77;0" {
		t.Errorf("authored position = %q, want -77;0", got)
	}

	// Control: with no authored position the parameter goes where the layout
	// puts it — the behaviour every unannotated flow still relies on. Without
	// this the test would pass against a writer that had simply stopped
	// deriving.
	derived := &microflows.MicroflowParameter{Name: "Feedback"}
	if got := rmp(t, serializeMicroflowParameter(derived, 0, 11)); got != "200;53" {
		t.Errorf("derived position at index 0 = %q, want 200;53", got)
	}
	if got := rmp(t, serializeMicroflowParameter(derived, 2, 11)); got != "400;53" {
		t.Errorf("derived position at index 2 = %q, want 400;53", got)
	}
}

// The reader is where the derived/authored arbitration happens, so that
// everything downstream can treat a non-nil Position as intent. A parameter
// stored on the derived grid must come back unset — carrying it over would pin
// it, and inserting a parameter would then strand the others (#951's shape).
func TestParseMicroflowParameterNormalizesDerivedPosition(t *testing.T) {
	raw := func(pos string) map[string]any {
		return map[string]any{"Name": "A", "RelativeMiddlePoint": pos}
	}
	if p := parseMicroflowParameter(raw("200;53"), 0); p.Position != nil {
		t.Errorf("derived position came back as %v, want nil", *p.Position)
	}
	if p := parseMicroflowParameter(raw("300;53"), 1); p.Position != nil {
		t.Errorf("derived position at index 1 came back as %v, want nil", *p.Position)
	}
	p := parseMicroflowParameter(raw("-77;0"), 0)
	if p.Position == nil {
		t.Fatal("authored position was dropped — this is the #993 read-side loss")
	}
	if *p.Position != (model.Point{X: -77, Y: 0}) {
		t.Errorf("position = %v, want -77;0", *p.Position)
	}
}
