// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#884 problem 1: a LoopedActivity's Size was computed from a
// pre-pass over the AST, before the body was built, so it was a function of
// statement COUNT alone. Child positions — including an explicit @position —
// had no effect on the box meant to contain them.
//
// Measured before the fix, on the same body with only the children's positions
// varied: 2 activities at x=150/310, at x=1500/2000 and at x=160/170 all
// produced Size 480;160. In the second case both children sat entirely outside
// their own container, and `mx check` reported nothing — the model has no
// geometry rules, so only the eye catches it.
//
// The invariant these tests pin is containment, not specific pixels: every
// child of a LoopedActivity must lie inside its parent's box.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// loopChildBounds returns the bounding box of a LoopedActivity's own children,
// in the container's coordinate space.
func loopChildBounds(loop *microflows.LoopedActivity) (minX, minY, maxX, maxY int, n int) {
	if loop.ObjectCollection == nil {
		return 0, 0, 0, 0, 0
	}
	first := true
	for _, o := range loop.ObjectCollection.Objects {
		p := o.GetPosition()
		sz := model.Size{}
		if withSize, ok := o.(interface{ GetSize() model.Size }); ok {
			sz = withSize.GetSize()
		}
		l, t := p.X-sz.Width/2, p.Y-sz.Height/2
		r, bt := p.X+sz.Width/2, p.Y+sz.Height/2
		if first {
			minX, minY, maxX, maxY = l, t, r, bt
			first = false
			n++
			continue
		}
		minX = min(minX, l)
		minY = min(minY, t)
		maxX = max(maxX, r)
		maxY = max(maxY, bt)
		n++
	}
	return
}

// assertLoopContainsItsChildren is the invariant. Pixel values are deliberately
// not asserted — the box must fit the contents, whatever the layout engine
// chooses to do with them.
func assertLoopContainsItsChildren(t *testing.T, objects []microflows.MicroflowObject) {
	t.Helper()
	if got := checkLoops(t, objects); got == 0 {
		t.Fatal("no LoopedActivity in the built flow — fixture is wrong")
	}
}

// checkLoops walks every LoopedActivity at any depth and returns how many it
// checked, so nested loops are covered without demanding one at every level.
func checkLoops(t *testing.T, objects []microflows.MicroflowObject) int {
	t.Helper()
	seen := 0
	for _, o := range objects {
		loop, ok := o.(*microflows.LoopedActivity)
		if !ok {
			continue
		}
		seen++
		minX, minY, maxX, maxY, n := loopChildBounds(loop)
		if n > 0 {
			w, h := loop.Size.Width, loop.Size.Height
			if minX < 0 || minY < 0 || maxX > w || maxY > h {
				t.Errorf("loop children escape the box: children span x[%d,%d] y[%d,%d], box is %dx%d",
					minX, maxX, minY, maxY, w, h)
			}
		}
		if loop.ObjectCollection != nil {
			seen += checkLoops(t, loop.ObjectCollection.Objects)
		}
	}
	return seen
}

func logStmt(msg string) *ast.LogStmt {
	return &ast.LogStmt{Message: &ast.LiteralExpr{Kind: ast.LiteralString, Value: msg}}
}

// positioned sets an explicit @position. It assigns the field directly rather
// than going through ast.StatementAnnotations, which returns nil when the field
// is nil and would make this a silent no-op — the test would then pass without
// testing anything.
func positioned(s *ast.LogStmt, x, y int) ast.MicroflowStatement {
	s.Annotations = &ast.ActivityAnnotations{Position: &ast.Position{X: x, Y: y}}
	return s
}

func buildLoopFlow(t *testing.T, body []ast.MicroflowStatement) []microflows.MicroflowObject {
	t.Helper()
	fb := &flowBuilder{
		posX:         100,
		posY:         200,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{"Items": "List of Test.Item", "Inner": "List of Test.Item"},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{varTypes: map[string]string{"Items": "List of Test.Item"}},
	}
	fb.addLoopStatement(&ast.LoopStmt{
		ListVariable: "Items",
		LoopVariable: "Item",
		Body:         body,
	})
	return fb.objects
}

// The default case: the box must fit its contents rather than a count.
func TestLoopBox_ContainsDefaultLaidOutChildren(t *testing.T) {
	for _, n := range []int{1, 2, 4, 7} {
		body := make([]ast.MicroflowStatement, 0, n)
		for i := 0; i < n; i++ {
			body = append(body, logStmt("x"))
		}
		assertLoopContainsItsChildren(t, buildLoopFlow(t, body))
	}
}

// The reported case: an explicit @position must not be able to put a child
// outside its own container.
func TestLoopBox_GrowsToContainExplicitlyPositionedChildren(t *testing.T) {
	body := []ast.MicroflowStatement{
		positioned(logStmt("one"), 1500, 60),
		positioned(logStmt("two"), 2000, 60),
	}
	assertLoopContainsItsChildren(t, buildLoopFlow(t, body))
}

// Loops inside loops: the inner box must be sized before the outer measures it,
// or the outer fits a stale inner size.
func TestLoopBox_NestedLoopsEachContainTheirChildren(t *testing.T) {
	inner := &ast.LoopStmt{
		ListVariable: "Inner",
		LoopVariable: "I2",
		Body:         []ast.MicroflowStatement{logStmt("a"), logStmt("b"), logStmt("c")},
	}
	assertLoopContainsItsChildren(t, buildLoopFlow(t, []ast.MicroflowStatement{
		logStmt("before"), inner, logStmt("after"),
	}))
}

// buildWhileFlow is the same fixture for a WHILE loop, which has its own builder
// (addWhileStatement) and therefore its own layout code.
func buildWhileFlow(t *testing.T, body []ast.MicroflowStatement) []microflows.MicroflowObject {
	t.Helper()
	fb := &flowBuilder{
		posX:         100,
		posY:         200,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{"N": "Integer"},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{varTypes: map[string]string{"N": "Integer"}},
	}
	fb.addWhileStatement(&ast.WhileStmt{
		Condition: &ast.LiteralExpr{Kind: ast.LiteralBoolean, Value: true},
		Body:      body,
	})
	return fb.objects
}

// A WHILE loop's first activity escaped its own box, on every while loop mxcli
// wrote — reported from a real project as "MPR011 on every `while` loop … first
// activity at (50,80) lies outside the loop box … even single-level loops"
// (ako/mxcli#645).
//
// The cause is one missing term. addWhileStatement's comment says its "layout
// matches addLoopStatement but without iterator icon space", and it dropped the
// iterator space correctly — but took ActivityWidth/2 with it:
//
//	foreach: innerStartX = LoopPadding + iteratorSpace + ActivityWidth/2  = 210
//	while:   innerStartX = LoopPadding                                    =  50
//
// A child's Position is its CENTRE ("RelativeMiddlePoint in Mendix"), so a
// centre at x=50 with ActivityWidth=120 puts the left edge at -10. The half-width
// is not iterator space; it is what converts a centre to a left edge. The very
// next line proves the omission was accidental rather than a choice — it adds
// ActivityHeight/2 for exactly this reason on the Y axis.
//
// This survived because the containment invariant WAS tested, on the foreach
// builder only. An invariant is worth what its coverage is: two builders, one
// tested, and the untested one shipped the violation to every project.
func TestWhileLoopBox_ContainsDefaultLaidOutChildren(t *testing.T) {
	for _, n := range []int{1, 2, 4, 7} {
		body := make([]ast.MicroflowStatement, 0, n)
		for i := 0; i < n; i++ {
			body = append(body, logStmt("x"))
		}
		assertLoopContainsItsChildren(t, buildWhileFlow(t, body))
	}
}

// The X axis specifically, so a regression cannot hide behind a box that grew
// tall enough. A box that merely got wider on the right does not fix a child
// hanging off the left edge.
func TestWhileLoopFirstChildLeftEdgeIsInsideTheBox(t *testing.T) {
	objects := buildWhileFlow(t, []ast.MicroflowStatement{logStmt("only")})

	for _, o := range objects {
		loop, ok := o.(*microflows.LoopedActivity)
		if !ok {
			continue
		}
		minX, _, _, _, n := loopChildBounds(loop)
		if n == 0 {
			t.Fatal("the while loop has no children — fixture is wrong")
		}
		if minX < 0 {
			t.Errorf("the first activity's left edge is at x=%d, outside its own box: "+
				"a centre at LoopPadding is half an activity too far left", minX)
		}
		return
	}
	t.Fatal("no LoopedActivity in the built flow — fixture is wrong")
}
