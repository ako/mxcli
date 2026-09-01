// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// #993, codec engine side. The two engines derive the same grid, so a fix
// applied to one and not the other would make a parameter's placement depend on
// which engine happened to run.
func TestMicroflowParameterToGenKeepsAuthoredPosition(t *testing.T) {
	authored := &microflows.MicroflowParameter{
		Name:     "Feedback",
		Position: &model.Point{X: -77, Y: 0},
	}
	g := microflowParameterToGen(authored, 0, 11).(*genMf.MicroflowParameter)
	if got := g.RelativeMiddlePoint(); got != "-77;0" {
		t.Errorf("authored position = %q, want -77;0", got)
	}

	// Control: unannotated parameters still derive from the index.
	derived := &microflows.MicroflowParameter{Name: "Feedback"}
	g = microflowParameterToGen(derived, 0, 11).(*genMf.MicroflowParameter)
	if got := g.RelativeMiddlePoint(); got != "200;53" {
		t.Errorf("derived position at index 0 = %q, want 200;53", got)
	}
	g = microflowParameterToGen(derived, 1, 11).(*genMf.MicroflowParameter)
	if got := g.RelativeMiddlePoint(); got != "300;53" {
		t.Errorf("derived position at index 1 = %q, want 300;53", got)
	}
}
