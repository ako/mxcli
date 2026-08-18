// SPDX-License-Identifier: Apache-2.0

// Package executor - Microflow layout algorithm
//
// Layout principles:
// 1. Left-to-right flow: Happy path goes straight horizontally
// 2. False/alternate paths below: ELSE branches go down, then merge back
// 3. Auto-sized containers: Loop boxes sized to fit content + padding
// 4. Horizontal connections: Connection lines are straight where possible
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Layout constants
const (
	// Activity dimensions
	ActivityWidth  = 120
	ActivityHeight = 60

	// Split/merge dimensions
	SplitWidth  = 90
	SplitHeight = 60
	MergeSize   = 40

	// Start/End event dimensions
	EventSize = 20

	// Spacing
	HorizontalSpacing = 160 // Space between activities horizontally (edge-to-edge ~40px)
	VerticalSpacing   = 90  // Space between branches for error-handler flows
	BranchGap         = 40  // Minimum edge-to-edge gap between parallel branches
	LoopPadding       = 50  // Padding inside loop boxes
	MinLoopWidth      = 200
	MinLoopHeight     = 100
)

// Bounds represents the bounding box of a layout element
type Bounds struct {
	Width  int
	Height int
}

// layoutMeasurer calculates dimensions of microflow statements
type layoutMeasurer struct {
	varTypes map[string]string
}

// measureStatements calculates the total bounds for a list of statements.
// Spacing is only added between pairs of non-zero-width elements so that
// zero-width statements (e.g. ReturnStmt, which produces no visual box)
// don't artificially inflate the measured width.
func (m *layoutMeasurer) measureStatements(stmts []ast.MicroflowStatement) Bounds {
	if len(stmts) == 0 {
		return Bounds{Width: 0, Height: 0}
	}

	totalWidth := 0
	maxHeight := ActivityHeight

	for _, stmt := range stmts {
		bounds := m.measureStatement(stmt)
		maxHeight = max(maxHeight, bounds.Height)
		if bounds.Width == 0 {
			continue
		}
		if totalWidth > 0 {
			totalWidth += HorizontalSpacing
		}
		totalWidth += bounds.Width
	}

	return Bounds{Width: totalWidth, Height: maxHeight}
}

// measureStatementsSpan returns the horizontal extent a statement run actually
// occupies once laid out, for runs where that can be derived exactly.
//
// measureStatements sums every element's full width and adds HorizontalSpacing
// between them. HorizontalSpacing is a centre-to-centre pitch — the builder does
// `posX += spacing` and centres each activity on posX — so counting it *on top of*
// each width over-measures a run of n simple activities by (n-1)*ActivityWidth.
// Sizing a loop box from that left it far wider than its contents: 880px around
// 440px of activities for a three-statement body (mendixlabs/mxcli#790).
//
// The correction applies only when every element is a simple activity, whose pitch
// is exactly HorizontalSpacing. A compound element (IF/split, nested loop) advances
// posX by geometry this function cannot reproduce without duplicating the builder —
// guessing there under-sizes the box and pushes activities outside it, which is
// worse than a box that is too wide. Such runs fall back to measureStatements.
func (m *layoutMeasurer) measureStatementsSpan(stmts []ast.MicroflowStatement) Bounds {
	count := 0
	maxHeight := ActivityHeight
	for _, stmt := range stmts {
		b := m.measureStatement(stmt)
		maxHeight = max(maxHeight, b.Height)
		if b.Width == 0 {
			continue
		}
		if b.Width != ActivityWidth {
			return m.measureStatements(stmts) // compound element — cannot place exactly
		}
		count++
	}
	if count == 0 {
		return Bounds{Width: 0, Height: maxHeight}
	}
	// n activities centred HorizontalSpacing apart span from the first centre minus
	// half a width to the last centre plus half a width.
	return Bounds{Width: (count-1)*HorizontalSpacing + ActivityWidth, Height: maxHeight}
}

// measureStatement calculates the bounds for a single statement
func (m *layoutMeasurer) measureStatement(stmt ast.MicroflowStatement) Bounds {
	switch s := stmt.(type) {
	case *ast.IfStmt:
		return m.measureIfStatement(s)
	case *ast.EnumSplitStmt:
		return m.measureEnumSplitStatement(s)
	case *ast.InheritanceSplitStmt:
		return m.measureInheritanceSplitStatement(s)
	case *ast.LoopStmt:
		return m.measureLoopStatement(s)
	case *ast.WhileStmt:
		return m.measureWhileStatement(s)
	case *ast.ReturnStmt:
		// Return doesn't add visual element (handled by EndEvent)
		return Bounds{Width: 0, Height: 0}
	default:
		// Simple activities have fixed size
		return Bounds{Width: ActivityWidth, Height: ActivityHeight}
	}
}

func (m *layoutMeasurer) measureEnumSplitStatement(s *ast.EnumSplitStmt) Bounds {
	maxBranchWidth := 0
	var branchHeights []int
	for _, c := range s.Cases {
		bounds := m.measureStatements(c.Body)
		maxBranchWidth = max(maxBranchWidth, bounds.Width)
		branchHeights = append(branchHeights, max(bounds.Height, ActivityHeight))
	}
	if len(s.ElseBody) > 0 {
		bounds := m.measureStatements(s.ElseBody)
		maxBranchWidth = max(maxBranchWidth, bounds.Width)
		branchHeights = append(branchHeights, max(bounds.Height, ActivityHeight))
	}
	if maxBranchWidth == 0 {
		maxBranchWidth = HorizontalSpacing / 2
	}
	if len(branchHeights) == 0 {
		branchHeights = []int{ActivityHeight}
	}

	totalHeight := 0
	for _, h := range branchHeights {
		totalHeight += h
	}
	totalHeight += (len(branchHeights) - 1) * BranchGap

	width := SplitWidth + HorizontalSpacing/2 + maxBranchWidth + HorizontalSpacing/2 + MergeSize
	return Bounds{Width: width, Height: totalHeight}
}

func (m *layoutMeasurer) measureInheritanceSplitStatement(s *ast.InheritanceSplitStmt) Bounds {
	maxBranchWidth := 0
	branchCount := len(s.Cases)
	for _, c := range s.Cases {
		bounds := m.measureStatements(c.Body)
		maxBranchWidth = max(maxBranchWidth, bounds.Width)
	}
	if len(s.ElseBody) > 0 {
		bounds := m.measureStatements(s.ElseBody)
		maxBranchWidth = max(maxBranchWidth, bounds.Width)
		branchCount++
	}
	if maxBranchWidth == 0 {
		maxBranchWidth = HorizontalSpacing / 2
	}
	if branchCount == 0 {
		branchCount = 1
	}

	width := ActivityWidth + HorizontalSpacing/2 + maxBranchWidth + HorizontalSpacing/2 + MergeSize
	height := ActivityHeight + (branchCount-1)*VerticalSpacing
	return Bounds{Width: width, Height: height}
}

// measureIfStatement calculates bounds for IF/ELSE
// Layout strategy matches addIfStatement:
// - IF with ELSE: TRUE path horizontal, FALSE path below
// - IF without ELSE: FALSE path horizontal, TRUE path below
func (m *layoutMeasurer) measureIfStatement(s *ast.IfStmt) Bounds {
	// Measure THEN branch
	thenBounds := m.measureStatements(s.ThenBody)

	// Measure ELSE branch
	elseBounds := m.measureStatements(s.ElseBody)

	// Width: split + max(then, else) + merge + spacing
	branchWidth := max(thenBounds.Width, elseBounds.Width)
	// If branches are empty, we still need some width for the flow lines
	if branchWidth == 0 {
		branchWidth = HorizontalSpacing / 2
	}

	totalWidth := SplitWidth + HorizontalSpacing/2 + branchWidth + HorizontalSpacing/2 + MergeSize

	// Height depends on layout strategy
	var totalHeight int
	if len(s.ElseBody) > 0 {
		// IF WITH ELSE: TRUE path horizontal (main line), FALSE path below
		// Height = THEN branch height + gap + ELSE branch height
		thenHeight := max(thenBounds.Height, ActivityHeight)
		elseHeight := max(elseBounds.Height, ActivityHeight)
		totalHeight = thenHeight + BranchGap + elseHeight
	} else {
		// IF WITHOUT ELSE: FALSE path horizontal (main line), TRUE path below
		// Height = main activity height + gap + THEN branch height
		thenHeight := max(thenBounds.Height, ActivityHeight)
		totalHeight = ActivityHeight + BranchGap + thenHeight
	}

	return Bounds{Width: totalWidth, Height: totalHeight}
}

// measureLoopStatement calculates bounds for LOOP
func (m *layoutMeasurer) measureLoopStatement(s *ast.LoopStmt) Bounds {
	// Measure loop body
	bodyBounds := m.measureStatements(s.Body)

	// Loop box size: body + padding on all sides
	width := max(bodyBounds.Width+2*LoopPadding, MinLoopWidth)
	height := max(bodyBounds.Height+2*LoopPadding, MinLoopHeight)

	return Bounds{Width: width, Height: height}
}

// measureWhileStatement calculates bounds for WHILE
func (m *layoutMeasurer) measureWhileStatement(s *ast.WhileStmt) Bounds {
	bodyBounds := m.measureStatements(s.Body)
	width := max(bodyBounds.Width+2*LoopPadding, MinLoopWidth)
	height := max(bodyBounds.Height+2*LoopPadding, MinLoopHeight)
	return Bounds{Width: width, Height: height}
}

// LayoutContext holds the current position during layout
type LayoutContext struct {
	X        int // Current X position
	Y        int // Current Y position (baseline for THEN path)
	BaseY    int // Original Y for returning after ELSE branch
	VarTypes map[string]string
}

// NewLayoutContext creates a new layout context
func NewLayoutContext(startX, startY int) *LayoutContext {
	return &LayoutContext{
		X:        startX,
		Y:        startY,
		BaseY:    startY,
		VarTypes: make(map[string]string),
	}
}

// Advance moves X position forward by given amount
func (ctx *LayoutContext) Advance(dx int) {
	ctx.X += dx
}

// AdvanceToNext moves to the next activity position
func (ctx *LayoutContext) AdvanceToNext() {
	ctx.X += HorizontalSpacing
}

// Note: Position in Mendix is stored as RelativeMiddlePoint, which IS the center
// of the element. No offset calculations needed - just use the center coordinates directly.

// Connection anchor indices for SequenceFlow
// These determine which side of an element the connection attaches to
const (
	AnchorTop    = 0
	AnchorRight  = 1
	AnchorBottom = 2
	AnchorLeft   = 3
)

// containerBounds returns the bounding box actually occupied by a container's
// children, in the container's own coordinate space. Each object contributes
// Position ± Size/2, since a Mendix position is the element's centre.
func containerBounds(objects []microflows.MicroflowObject) (minX, minY, maxX, maxY int, n int) {
	for _, o := range objects {
		if o == nil {
			continue
		}
		p := o.GetPosition()
		var sz model.Size
		if withSize, ok := o.(interface{ GetSize() model.Size }); ok {
			sz = withSize.GetSize()
		}
		l, t := p.X-sz.Width/2, p.Y-sz.Height/2
		r, b := p.X+sz.Width/2, p.Y+sz.Height/2
		if n == 0 {
			minX, minY, maxX, maxY = l, t, r, b
		} else {
			minX, minY = min(minX, l), min(minY, t)
			maxX, maxY = max(maxX, r), max(maxY, b)
		}
		n++
	}
	return
}

// fitContainerSize sizes a container box to the children it actually holds
// (mendixlabs/mxcli#884 problem 1).
//
// The box used to come from measureStatementsSpan — a pre-pass over the AST run
// BEFORE the body was built — so it was a function of statement COUNT alone.
// Varying only the children's positions left it unchanged: two activities at
// x=150/310, at x=1500/2000 and at x=160/170 all produced 480;160, and in the
// second case both children sat entirely outside their own container. The model
// carries no geometry rules, so `mx check` reports nothing.
//
// Sizing happens AFTER the body is built, which is what makes an explicit
// `@position` on a child effective. Nesting falls out of the recursion: an inner
// loop is sized when its own addLoopStatement returns, so by the time the outer
// container measures it, its Size is already correct.
//
// The children are NOT translated. Their positions round-trip through DESCRIBE
// as `@position`, so moving them would make a describe→exec cycle produce
// different coordinates than it read — and under ADR-0008 that turns an
// otherwise-quiet re-run into a write. The box grows to fit the contents
// instead, which is also what a hand-laid-out loop needs.
func fitContainerSize(objects []microflows.MicroflowObject, leftInset, minWidth, minHeight int) (width, height int) {
	_, _, maxX, maxY, n := containerBounds(objects)
	if n == 0 {
		return minWidth, minHeight
	}
	width = max(maxX+LoopPadding, minWidth)
	height = max(maxY+LoopPadding, minHeight)
	_ = leftInset
	return width, height
}
