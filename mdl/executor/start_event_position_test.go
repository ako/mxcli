// SPDX-License-Identifier: Apache-2.0

// A microflow's StartEvent position is not expressible in MDL and is not
// captured by DESCRIBE, so a describe→exec round-trip used to move it: a
// Studio-Pro-authored flow whose start sat at 145;200 came back at 100;200,
// because the builder derives the start as (first annotated activity X −
// spacing) and 145 is not derivable from 260.
//
// Every other coordinate in that flow round-trips exactly, so this was the one
// piece of a hand-laid-out microflow mxcli could not preserve. It is preserved
// the way the folder and the allowed module roles already are on CREATE OR
// MODIFY: read off the stored document and carried over.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func startEventOf(objects []microflows.MicroflowObject) *microflows.StartEvent {
	for _, o := range objects {
		if se, ok := o.(*microflows.StartEvent); ok {
			return se
		}
	}
	return nil
}

func buildFlowWithStart(t *testing.T, start *model.Point) *microflows.StartEvent {
	t.Helper()
	fb := &flowBuilder{
		posX: 100, posY: 200, baseY: 200, spacing: HorizontalSpacing,
		varTypes:      map[string]string{},
		declaredVars:  map[string]string{},
		measurer:      &layoutMeasurer{},
		startPosition: start,
	}
	stmt := &ast.LogStmt{
		Message:     &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"},
		Annotations: &ast.ActivityAnnotations{Position: &ast.Position{X: 260, Y: 200}},
	}
	fb.buildFlowGraph([]ast.MicroflowStatement{stmt}, nil)
	se := startEventOf(fb.objects)
	if se == nil {
		t.Fatal("no StartEvent in the built flow")
	}
	return se
}

// The stored position wins over the derived one.
func TestStartEvent_PreservesStoredPosition(t *testing.T) {
	se := buildFlowWithStart(t, &model.Point{X: 145, Y: 200})
	if se.Position.X != 145 || se.Position.Y != 200 {
		t.Errorf("StartEvent at %d;%d, want 145;200 — a hand-laid-out start must survive a rebuild",
			se.Position.X, se.Position.Y)
	}
}

// With nothing stored (a fresh CREATE) the derived placement is unchanged:
// one spacing unit left of the first annotated activity.
func TestStartEvent_DerivesWhenNothingStored(t *testing.T) {
	se := buildFlowWithStart(t, nil)
	if want := 260 - HorizontalSpacing; se.Position.X != want {
		t.Errorf("StartEvent at %d, want %d (derived)", se.Position.X, want)
	}
}
