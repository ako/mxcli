// SPDX-License-Identifier: Apache-2.0

package ast

// CreateAnnotationStmt represents:
//
//	CREATE [OR MODIFY] ANNOTATION IN Module ( Caption: '…', Position: (x, y), Width: n )
//
// A domain-model annotation is the note box Studio Pro draws on the canvas. It
// belongs to the module's domain model rather than being a document of its own,
// which is why the statement names a module and not a qualified name.
//
// Mendix stores four properties — Caption, ExportLevel, Location and Width — and
// no name, so the note is addressed by the first line of its caption. There is
// no colour: a "coloured section box" is this element in Studio Pro's styling,
// not stored state.
//
// There is also no HEIGHT (#1014). The note auto-sizes to its caption, so its
// height is a function of the text and the width rather than stored state.
// Measured four ways on 11.13.0, because the metamodel in this repo is an 11.6.0
// snapshot and cannot rule out a later property on its own: `mx dump-mpr` emits
// those four keys and no more; `mx convert -p`, which rewrites the model through
// Mendix's OWN object model and would materialise a defaulted property, adds
// nothing; and the published Model SDK's domainmodels.Annotation lists caption,
// exportLevel, location and width. Width is therefore the only height lever.
type CreateAnnotationStmt struct {
	Module  string
	Caption string
	// Position is the canvas location. Nil leaves an existing note where it is,
	// and puts a new one at Mendix's own default.
	Position *Position
	// Width in pixels. Zero leaves an existing note's width alone and gives a new
	// one the width Studio Pro uses.
	Width int
	// CreateOrModify is CREATE OR MODIFY / OR REPLACE: update the note whose
	// caption's first line matches, instead of refusing.
	CreateOrModify bool
}

func (s *CreateAnnotationStmt) isStatement() {}

// DropAnnotationStmt represents: DROP ANNOTATION 'title' IN Module
//
// The title is the first line of the note's caption — what SHOW ANNOTATIONS
// prints in its Title column.
type DropAnnotationStmt struct {
	// Title is the first line of the note's caption. Empty when the statement
	// used the AT form.
	Title string
	// Position addresses the note by where it sits — the form to use when a
	// script owns the layout, since it survives an edit to the wording.
	Position *Position
	Module   string
}

func (s *DropAnnotationStmt) isStatement() {}
