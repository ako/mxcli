// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A decision whose caption was cleared in Studio Pro must keep an empty caption
// through describe -> exec (mendixlabs/mxcli#1254). Describe used to leave an
// empty caption out, and a decision with no @caption is built with its condition
// as the caption — so the round trip added "@caption 'true'" to the diagram.
func TestSplitEmptyCaptionRoundTrips_Issue1254(t *testing.T) {
	split := &microflows.ExclusiveSplit{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: mkID("split")},
			Position:    model.Point{X: 200, Y: 100},
		},
		Caption:        "",
		SplitCondition: &microflows.ExpressionSplitCondition{Expression: "true"},
	}

	var lines []string
	emitObjectAnnotations(split, &lines, "", nil, nil, nil, nil)

	script := "create microflow M.F ()\nreturns Boolean\nbegin\n  " + strings.Join(lines, "\n  ") +
		"\n  if true then\n    return true;\n  else\n    return false;\n  end if;\nend;\n"
	prog, errs := visitor.Build(script)
	if len(errs) != 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, script)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)

	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
		measurer: &layoutMeasurer{},
	}
	fb.buildFlowGraph(mf.Body, mf.ReturnType)

	for _, obj := range fb.objects {
		if sp, ok := obj.(*microflows.ExclusiveSplit); ok {
			if sp.Caption != "" {
				t.Fatalf("round-tripped split caption = %q, want empty; describe emitted:\n%s",
					sp.Caption, strings.Join(lines, "\n"))
			}
			return
		}
	}
	t.Fatal("no ExclusiveSplit built")
}

// An enum split builds its caption from the variable the same way; an explicit
// empty @caption must win there too.
func TestEnumSplitExplicitEmptyCaption_Issue1254(t *testing.T) {
	src := "create microflow M.F ($E: enum M.Color)\nreturns Boolean\nbegin\n  @caption ''\n  case $E\n" +
		"    when Red then\n      return true;\n    when (empty) then\n      return false;\n  end case;\nend;\n"
	prog, errs := visitor.Build(src)
	if len(errs) != 0 {
		t.Fatalf("parse: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	ann := ast.StatementAnnotations(mf.Body[0])
	if ann == nil || !ann.HasCaption() || ann.Caption != "" {
		t.Fatalf("@caption '' not carried on the statement: %+v", ann)
	}

	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing,
		varTypes: map[string]string{"E": "Enumeration(M.Color)"}, declaredVars: map[string]string{"E": "Enumeration(M.Color)"},
		measurer: &layoutMeasurer{},
	}
	fb.buildFlowGraph(mf.Body, nil)
	for _, obj := range fb.objects {
		if sp, ok := obj.(*microflows.ExclusiveSplit); ok {
			if sp.Caption != "" {
				t.Fatalf("enum split caption = %q, want empty", sp.Caption)
			}
			return
		}
	}
	t.Fatal("no ExclusiveSplit built")
}
