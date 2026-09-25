// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func mfWithLoopAnnotations(ann *ast.ActivityAnnotations) *ast.CreateMicroflowStmt {
	return &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "MF"},
		Body: []ast.MicroflowStatement{
			&ast.LoopStmt{
				LoopVariable: "Item",
				ListVariable: "Items",
				Annotations:  ann,
				Body:         []ast.MicroflowStatement{},
			},
		},
	}
}

func loopHasMDL042(stmt *ast.CreateMicroflowStmt) bool {
	for _, v := range ValidateMicroflow(stmt) {
		if v.RuleID == "MDL042" {
			return true
		}
	}
	return false
}

// TestValidateMicroflow_CaptionOnLoopWarns guards MDL042: @caption on a loop is
// silently dropped (Mendix loops have no Caption property), so check must warn.
func TestValidateMicroflow_CaptionOnLoopWarns(t *testing.T) {
	if !loopHasMDL042(mfWithLoopAnnotations(&ast.ActivityAnnotations{Caption: "Process things"})) {
		t.Error("expected MDL042 warning for @caption on a loop")
	}
}

// @annotation (the supported way to label a loop) must NOT warn.
func TestValidateMicroflow_AnnotationOnLoopNoWarn(t *testing.T) {
	if loopHasMDL042(mfWithLoopAnnotations(&ast.ActivityAnnotations{Notes: []ast.MicroflowAnnotation{{Text: "Process things"}}})) {
		t.Error("MDL042 must not fire for @annotation on a loop")
	}
}

// A loop with no annotations must not warn.
func TestValidateMicroflow_PlainLoopNoWarn(t *testing.T) {
	if loopHasMDL042(mfWithLoopAnnotations(nil)) {
		t.Error("MDL042 must not fire for a plain loop")
	}
}

func mfWithWhileAnnotations(ann *ast.ActivityAnnotations) *ast.CreateMicroflowStmt {
	return &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Sample", Name: "MF"},
		Body: []ast.MicroflowStatement{
			&ast.WhileStmt{
				Condition:   &ast.LiteralExpr{Value: true, Kind: ast.LiteralBoolean},
				Annotations: ann,
				Body:        []ast.MicroflowStatement{},
			},
		},
	}
}

// A while loop builds the same LoopedActivity as a for-each loop, so its @caption
// is dropped the same way and must be reported the same way (mendixlabs/mxcli#1187).
// Before the fix a while loop's caption passed check silently and vanished on exec.
func TestValidateMicroflow_CaptionOnWhileWarns(t *testing.T) {
	if !loopHasMDL042(mfWithWhileAnnotations(&ast.ActivityAnnotations{Caption: "Months left?"})) {
		t.Error("expected MDL042 warning for @caption on a while loop")
	}
}

// @annotation is the supported label for a while loop too, and must not warn.
func TestValidateMicroflow_AnnotationOnWhileNoWarn(t *testing.T) {
	if loopHasMDL042(mfWithWhileAnnotations(&ast.ActivityAnnotations{Notes: []ast.MicroflowAnnotation{{Text: "Count the months"}}})) {
		t.Error("MDL042 must not fire for @annotation on a while loop")
	}
}
