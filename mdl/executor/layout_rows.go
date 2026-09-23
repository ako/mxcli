// SPDX-License-Identifier: Apache-2.0

// Package executor - wrapping a long main line onto several rows.
//
// The builder lays the happy path left to right and never stops: every top-level
// statement advances posX by one HorizontalSpacing, so a flow is exactly as wide as
// it is long. Measured on a generated app of 41 microflows, with no `@position`
// anywhere, nothing overlapped — and the widest flow was 6930x160 px, 43:1, four
// screens of horizontal scrolling for something 160 px tall. A reviewer cannot see
// such a flow; Studio Pro draws what is stored and does not re-arrange.
//
// Wrapping breaks that line into rows of bounded width. The next row starts below
// everything the current row occupies — branch lanes and loop boxes included, which
// is what a fixed vertical step gets wrong — so a row can be as deep as it needs and
// the one after it still clears it.
//
// It applies to the main flow only, and only past MaxRowWidth: a flow that already
// fits on a screen keeps the coordinates it has today, which is what keeps the
// existing layout tests (and every round trip through DESCRIBE) unchanged.
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

const (
	// MaxRowWidth is how far the main line may run before the next statement starts
	// a new row, measured from the row's first element. Studio Pro's canvas on a
	// 1920px screen is about 1500px wide once the toolbox and properties pane are
	// open; two of those is what a reader will scroll sideways without losing the
	// thread, and it keeps a pair of loops — or a flow of eighteen activities — on
	// one line, where one screen's worth stacked them into rows joined by a long
	// diagonal. Past that, the flow wraps.
	MaxRowWidth = 2880

	// RowGap is the empty space between the bottom of one row and the top of the
	// next, edge to edge. Studio Pro prints an activity's output variable and its type
	// under the box, about 35px of text, and the line that joins two rows has to cross
	// the whole diagram in this band: at BranchGap (40) the text of one row touched the
	// boxes of the next and the joining line ran through both.
	RowGap = VerticalSpacing

	// RowOverhang is how far past MaxRowWidth a row may run when that finishes the
	// flow. Without it a flow a little over the limit wrapped its last two activities
	// and the end event onto a row of their own — a second row three elements long
	// under one of sixteen, reached by a line back across the whole diagram.
	RowOverhang = 2 * HorizontalSpacing
)

// rowTracker remembers where the current row began, so wrapRow can put the next row
// under the full extent of this one rather than a fixed step below its centre line.
type rowTracker struct {
	// startX is the x every row begins at: the first statement's x, not the start
	// event's, so a flow whose first statement carries an @position wraps back to
	// that column instead of to the canvas origin.
	startX int
	// firstObject is the index into fb.objects of the first object of this row.
	// Everything from there on contributes to the row's depth.
	firstObject int
	// pendingWrapEdge marks that the next flow created on the main line crosses
	// from one row to the next, so it needs the anchors that keep it out of the
	// row above (takeWrapAnchors).
	pendingWrapEdge bool
	// wrapNoteAbove says the element starting the new row has a note over it, which
	// is where the joining line would otherwise arrive.
	wrapNoteAbove bool
	// starts is the index into fb.objects where each row begins, in order. The
	// final pass walks it to push rows apart where the estimate fell short.
	starts []int
}

// rowBottom returns the lowest edge occupied by the objects placed in the current
// row. An object's Position is its centre (RelativeMiddlePoint), so its bottom is
// the centre plus half its height; a loop box contributes its own full height,
// which is how a row containing a loop pushes the next row far enough down.
func (fb *flowBuilder) rowBottom() int {
	bottom := fb.posY + ActivityHeight/2
	if fb.row.firstObject > len(fb.objects) {
		return bottom
	}
	for _, o := range fb.objects[fb.row.firstObject:] {
		if o == nil {
			continue
		}
		p := o.GetPosition()
		height := ActivityHeight
		if withSize, ok := o.(interface{ GetSize() model.Size }); ok {
			if h := withSize.GetSize().Height; h > 0 {
				height = h
			}
		}
		if b := p.Y + height/2; b > bottom {
			bottom = b
		}
	}
	return bottom
}

// shouldWrap reports whether the next top-level statement would take the main line
// past MaxRowWidth. It is asked before the statement is placed, so posX is where
// that statement's centre would go.
//
// rest is the statement and everything after it on the main line.
//
// A statement that draws nothing — a RETURN, which the builder turns into the end
// event already on the happy path — never starts a row: wrapping there stranded a
// lone end event on a row of its own, with a line running back across the diagram
// to reach it.
func (fb *flowBuilder) shouldWrap(stmt ast.MicroflowStatement, rest []ast.MicroflowStatement) bool {
	if !fb.allowWrap {
		return false
	}
	width := ActivityWidth
	if fb.measurer != nil {
		w := fb.measurer.measureStatement(stmt).Width
		if w == 0 {
			return false
		}
		width = w
	}
	// The whole element has to fit, not just its starting column: a loop box 810px
	// wide begun at 1290 past the row start ran the row to 2100, well past a screen.
	used := fb.posX - fb.row.startX
	if used+width <= MaxRowWidth {
		return false
	}
	// Past the limit — unless everything that is left fits in the overhang, in which
	// case finishing on this row reads better than a stub of a row underneath.
	if fb.measurer != nil && used+fb.measurer.measureStatements(rest).Width <= MaxRowWidth+RowOverhang {
		return false
	}
	return true
}

// startsWithNote reports whether a statement carries a note, which is drawn above it.
func startsWithNote(stmt ast.MicroflowStatement) bool {
	ann := getStatementAnnotations(stmt)
	return ann != nil && (len(ann.Notes) > 0 || len(ann.FreeNotes) > 0)
}

// wrapRow moves the cursor to the start of a new row, below everything the current
// row occupies. noteAbove says the element starting the new row carries a note, which
// decides where the joining line arrives (takeWrapAnchors).
//
// The new centre line is placed as if an activity started the row. That is only a
// first position: a loop box hangs half its height above its centre line, a note sits
// above that, and neither size is known until the element is built. separateRows
// measures the finished rows and moves them apart, so no estimate is made here.
func (fb *flowBuilder) wrapRow(noteAbove bool) {
	next := fb.rowBottom() + RowGap + ActivityHeight/2
	fb.posX = fb.row.startX
	fb.posY = next
	fb.baseY = next
	fb.row.firstObject = len(fb.objects)
	fb.row.pendingWrapEdge = true
	fb.row.wrapNoteAbove = noteAbove
	fb.row.starts = append(fb.row.starts, fb.row.firstObject)
}

// takeWrapAnchors is asked once per flow on the main line and answers only for the
// one that crosses from the last element of a row to the first of the next — the one
// flow that travels backwards. It leaves the BOTTOM of its origin and arrives on TOP
// of its destination, so it runs through the empty band between the rows.
//
// Anchored right-to-left, as every other flow is, it is drawn straight back across
// the row it just left. Bottom-to-left cleared that row but still arrived from the
// right, over the first elements of the new row, to reach the far side of the first
// one. Only an element with a note above it keeps the left side: the note stands
// exactly where the line would come down.
func (fb *flowBuilder) takeWrapAnchors() (origin, destination int, ok bool) {
	if !fb.row.pendingWrapEdge {
		return 0, 0, false
	}
	fb.row.pendingWrapEdge = false
	if fb.row.wrapNoteAbove {
		return AnchorBottom, AnchorLeft, true
	}
	return AnchorBottom, AnchorTop, true
}

// noteRowStart records where the main line begins, once the first top-level
// statement's column is known.
func (fb *flowBuilder) noteRowStart() {
	fb.row.startX = fb.posX
	fb.row.firstObject = len(fb.objects)
	fb.row.starts = []int{fb.row.firstObject}
}

// separateRows pushes each row down until it clears the one above it by RowGap,
// measuring what was actually built.
//
// A row's depth cannot be known when the row is begun: a loop box is sized from its
// body AFTER the body is laid out (fitContainerSize) — the measurer's guess for a
// one-branch body is 250 against 310 fitted — and a note adds its own height on top.
// Rather than estimate, wrapRow starts each row at an activity's depth and this pass
// measures the finished geometry once and shifts whole rows. Notes are objects too,
// so they are part of what is measured.
func (fb *flowBuilder) separateRows() {
	if !fb.allowWrap || len(fb.row.starts) < 2 {
		return
	}
	bounds := func(from, to int) (top, bottom int, ok bool) {
		for i := from; i < to && i < len(fb.objects); i++ {
			o := fb.objects[i]
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
		return
	}
	shift := func(from int, dy int) {
		for i := from; i < len(fb.objects); i++ {
			o := fb.objects[i]
			if o == nil {
				continue
			}
			p := o.GetPosition()
			o.SetPosition(model.Point{X: p.X, Y: p.Y + dy})
		}
	}
	for r := 1; r < len(fb.row.starts); r++ {
		end := len(fb.objects)
		if r+1 < len(fb.row.starts) {
			end = fb.row.starts[r+1]
		}
		_, prevBottom, okPrev := bounds(fb.row.starts[r-1], fb.row.starts[r])
		top, _, okRow := bounds(fb.row.starts[r], end)
		if !okPrev || !okRow {
			continue
		}
		if gap := top - prevBottom; gap < RowGap {
			shift(fb.row.starts[r], RowGap-gap)
		}
	}
}

// ownPositionX returns the x of a statement that places itself with @position. Such
// a statement is never moved: its coordinates round-trip through DESCRIBE, so
// wrapping it would make a describe→exec cycle rewrite the model it just read. The
// row re-anchors to it instead, so the statements after it wrap to its column.
func ownPositionX(stmt ast.MicroflowStatement) (int, bool) {
	ann := getStatementAnnotations(stmt)
	if ann == nil || ann.Position == nil {
		return 0, false
	}
	return ann.Position.X, true
}
