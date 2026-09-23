// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// logStatements returns n log activities, which the builder lays out as a straight
// horizontal run of ActivityWidth boxes one HorizontalSpacing apart.
func logStatements(n int) []ast.MicroflowStatement {
	stmts := make([]ast.MicroflowStatement, 0, n)
	for i := 0; i < n; i++ {
		stmts = append(stmts, &ast.LogStmt{Level: ast.LogInfo, Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"}})
	}
	return stmts
}

// buildRows lays out statements through a builder configured the way a whole
// microflow is built, and returns the activities in the order they were placed.
func buildRows(t *testing.T, stmts []ast.MicroflowStatement) []microflows.MicroflowObject {
	t.Helper()
	fb := &flowBuilder{
		posX:         200,
		posY:         200,
		baseY:        200,
		spacing:      HorizontalSpacing,
		allowWrap:    true,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{varTypes: map[string]string{}},
	}
	fb.buildFlowGraph(stmts, nil)
	return fb.objects
}

func activityPositions(objects []microflows.MicroflowObject) []model.Point {
	var out []model.Point
	for _, o := range objects {
		if _, ok := o.(*microflows.ActionActivity); ok {
			out = append(out, o.GetPosition())
		}
	}
	return out
}

func byIDPosition(objects []microflows.MicroflowObject, id model.ID) (model.Point, bool) {
	for _, o := range objects {
		if o.GetID() == id {
			return o.GetPosition(), true
		}
	}
	return model.Point{}, false
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
