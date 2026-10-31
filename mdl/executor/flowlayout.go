// SPDX-License-Identifier: Apache-2.0

// Package executor - the layout style a flow is built with.
//
// The builder places every activity while it creates it (layout.go,
// layout_lanes.go, layout_rows.go). Teams do not agree on how a flow should look
// (ako/mxcli#1832, docs/11-proposals/PROPOSAL_configurable_flow_layout.md), so
// the decisions a style changes are read from a flowLayout instead of being
// fixed in the builder.
//
// The zero value is the default style, happy-line, and reproduces the layout
// mxcli has always drawn: a builder or measurer with no flowLayout lays out
// exactly as before. That is what keeps every existing layout test, and every
// DESCRIBE round trip, unchanged.
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// IfElseStyle is how an IF with an ELSE is drawn.
type IfElseStyle int

const (
	// IfElseLanes: the TRUE body runs on the centre line, the ELSE body in a lane
	// under it, left to right. The default.
	IfElseLanes IfElseStyle = iota
	// IfElseColumn: the TRUE body runs on the centre line, the ELSE body is stacked
	// top to bottom in a column under the split, and rejoins the merge from below.
	IfElseColumn
)

// GuardStyle is how a guard — `if X then …; return; end if`, no ELSE — is drawn.
type GuardStyle int

const (
	// GuardReturnBelow: the TRUE branch with its early return goes in the lane
	// under the split; the happy path carries straight on along the FALSE exit.
	// The default.
	GuardReturnBelow GuardStyle = iota
	// GuardTrueRight: the TRUE branch carries straight on to the right, on the
	// centre line; the happy path leaves the split's bottom and continues on a
	// new line below, starting in the split's column.
	GuardTrueRight
)

// flowLayout holds the layout decisions a style changes. Its zero value is the
// default style.
type flowLayout struct {
	IfElse IfElseStyle
	Guard  GuardStyle
}

// layoutStyle returns the builder's layout style; a builder without one uses the
// default.
func (fb *flowBuilder) layoutStyle() flowLayout {
	if fb == nil || fb.layout == nil {
		return flowLayout{}
	}
	return *fb.layout
}

// layoutStyle returns the measurer's layout style; a measurer without one uses the
// default.
func (m *layoutMeasurer) layoutStyle() flowLayout {
	if m == nil || m.layout == nil {
		return flowLayout{}
	}
	return *m.layout
}

// elseColumnCursor places the statements of an ELSE body one under the other in
// the split's column (IfElseColumn). A nil cursor does nothing, which is how the
// default style's ELSE lane goes through the same loop untouched.
//
// Each statement is built where the cursor says and then moved, as a whole,
// straight down until its top edge is RowGap under the bottom of the one before.
// Measuring what was built, rather than estimating it, is what separateRows does
// for wrapped rows (layout_rows.go): a loop box is sized only after its body, and
// a note or an error handler's row adds its own height.
type elseColumnCursor struct {
	fb *flowBuilder
	x  int
	// top is where the next statement's top edge goes.
	top int
	// first is the index into fb.objects of the statement being built.
	first int
	// pinned says the statement places itself with @position; it is not moved,
	// and the column carries on under it.
	pinned bool
}

func (fb *flowBuilder) startElseColumn(x, top int) *elseColumnCursor {
	return &elseColumnCursor{fb: fb, x: x, top: top}
}

func (c *elseColumnCursor) begin(stmt ast.MicroflowStatement) {
	if c == nil {
		return
	}
	c.first = len(c.fb.objects)
	_, c.pinned = ownPositionX(stmt)
	c.fb.posX = c.x
	c.fb.posY = c.top + ActivityHeight/2
}

func (c *elseColumnCursor) end() {
	if c == nil {
		return
	}
	top, bottom, ok := objectsExtent(c.fb.objects[c.first:])
	if !ok {
		return
	}
	if !c.pinned && top != c.top {
		for _, o := range c.fb.objects[c.first:] {
			if o == nil {
				continue
			}
			p := o.GetPosition()
			o.SetPosition(model.Point{X: p.X, Y: p.Y + c.top - top})
		}
		bottom += c.top - top
	}
	c.top = bottom + RowGap
	c.fb.posX = c.x
	c.fb.posY = c.top + ActivityHeight/2
}

// objectsExtent returns the top and bottom edges of the objects' boxes. An
// object's Position is its centre.
func objectsExtent(objects []microflows.MicroflowObject) (top, bottom int, ok bool) {
	for _, o := range objects {
		if o == nil {
			continue
		}
		p := o.GetPosition()
		h := ActivityHeight
		if ws, okSize := o.(interface{ GetSize() model.Size }); okSize {
			if hh := ws.GetSize().Height; hh > 0 {
				h = hh
			}
		}
		t, b := p.Y-h/2, p.Y+h/2
		if !ok {
			top, bottom, ok = t, b, true
			continue
		}
		top, bottom = min(top, t), max(bottom, b)
	}
	return top, bottom, ok
}

// measureColumn measures an ELSE body stacked as a column: as wide as its widest
// statement, as tall as all of them with RowGap between. A RETURN draws an end
// event at the bottom.
func (m *layoutMeasurer) measureColumn(stmts []ast.MicroflowStatement) Bounds {
	width, height, n := 0, 0, 0
	for _, stmt := range stmts {
		b := m.measureStatement(stmt)
		w, h := b.Width, max(b.Height, ActivityHeight)
		if w == 0 {
			if _, isReturn := stmt.(*ast.ReturnStmt); !isReturn {
				continue
			}
			w, h = EventSize, EventSize
		}
		if row := m.handlerRowWidth(stmt); row > 0 {
			w = max(w, row)
			h += ErrorHandlerPitch
		}
		width = max(width, w)
		height += h
		n++
	}
	if n > 1 {
		height += (n - 1) * RowGap
	}
	return Bounds{Width: width, Height: height}
}

// elseColumnBranchWidth is the branch width addIfStatement places the merge
// after, for an ELSE column. The merge goes past the TRUE body as before, and
// past the column too: the column is centred under the split, so it begins half
// an activity left of the split's centre, where the TRUE body begins
// SplitWidth + HorizontalSpacing/2 to the right of it.
func elseColumnBranchWidth(thenBounds, column Bounds) int {
	return max(thenBounds.Width, column.Width-(SplitWidth+HorizontalSpacing/2))
}

// withDefaultSides returns anchors whose unset sides are from and to. A side the
// script states is kept: an @anchor always wins over the style.
func withDefaultSides(a *ast.FlowAnchors, from, to int) *ast.FlowAnchors {
	out := ast.FlowAnchors{From: ast.AnchorSide(from), To: ast.AnchorSide(to)}
	if a != nil {
		if a.From != ast.AnchorSideUnset {
			out.From = a.From
		}
		if a.To != ast.AnchorSideUnset {
			out.To = a.To
		}
	}
	return &out
}

// measureGuardTrueRight measures a statement list from a GuardTrueRight guard
// on: the guard's branch on this line, everything after it on a new line below,
// starting in the split's column. left is where the guard starts, lineHeight the
// height of what the line held before it.
//
// The height is reported as if centred on the line, as every Bounds is: the
// callers place things under a body at centre + Height/2, and the drop makes this
// body reach far further below its line than above it. Twice the depth below is
// what keeps them clear; the cost is room above the line that goes unused.
func (m *layoutMeasurer) measureGuardTrueRight(guard *ast.IfStmt, rest []ast.MicroflowStatement, left, extent, lineHeight int) Bounds {
	// guardBranchInset is measured from the split's centre; what follows a body
	// (a merge) is placed from its left edge, so the split's left half is added.
	// Without it the merge after a TRUE-right guard sat on the guard's end event.
	branch := m.measureBranch(guard.ThenBody)
	extent = max(extent, left+SplitWidth/2+guardBranchInset+branch.Width)
	line := max(lineHeight, branch.Height, ActivityHeight)
	after := m.measureStatements(rest)
	if after.Width == 0 {
		return Bounds{Width: extent, Height: line}
	}
	// The new line starts with its first element centred under the split: half an
	// activity left of the split's left edge, at most.
	extent = max(extent, left+after.Width)
	below := line/2 + RowGap + max(after.Height, ActivityHeight)
	return Bounds{Width: extent, Height: 2 * below}
}
