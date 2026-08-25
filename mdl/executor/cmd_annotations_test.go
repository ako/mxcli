// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A domain-model annotation has no name — Mendix stores Caption, ExportLevel,
// Location and Width — so a statement has to address it by something else.
//
// Position is the identity whenever a script gives one, because it is the only
// stored property a script controls that does NOT change when the wording does.
// Keying on the caption alone meant editing a note's first line created a SECOND
// note rather than updating the first, which is what a re-runnable layout script
// does every time someone rephrases a sentence.

func annotationAt(x, y int, caption string) *domainmodel.Annotation {
	a := &domainmodel.Annotation{
		Caption:  caption,
		Location: model.Point{X: x, Y: y},
		Width:    440,
	}
	a.ID = nextID("annot")
	return a
}

// annotationCtx wires a module whose domain model holds the given notes, and
// captures what the executor writes back.
func annotationCtx(t *testing.T, stored ...*domainmodel.Annotation) (*ExecContext, *[]*domainmodel.Annotation) {
	t.Helper()
	mod := mkModule("Notes")
	dm := &domainmodel.DomainModel{ContainerID: mod.ID, Annotations: stored}
	dm.ID = nextID("dm")

	var written []*domainmodel.Annotation
	h := mkHierarchy(mod)
	withContainer(h, dm.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		SetDomainModelAnnotationsFunc: func(_ model.ID, a []*domainmodel.Annotation) error {
			written = a
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, &written
}

// Editing the wording of a note that keeps its position updates it in place.
func TestCreateAnnotationModifiesTheNoteAtThatPosition(t *testing.T) {
	stored := annotationAt(60, 40, "Orders\nEverything about an order.")
	ctx, written := annotationCtx(t, stored)

	err := execCreateAnnotation(ctx, &ast.CreateAnnotationStmt{
		Module:         "Notes",
		Caption:        "Orders and invoices\nEverything about an order.",
		Position:       &ast.Position{X: 60, Y: 40},
		CreateOrModify: true,
	})
	assertNoError(t, err)

	got := *written
	if len(got) != 1 {
		t.Fatalf("got %d annotations, want 1 — a reworded note became a second note", len(got))
	}
	if !strings.HasPrefix(got[0].Caption, "Orders and invoices") {
		t.Errorf("Caption = %q, want the new wording", got[0].Caption)
	}
	if got[0].ID != stored.ID {
		t.Errorf("ID = %q, want the stored %q — a fresh one makes an unchanged model differ (ADR-0008)",
			got[0].ID, stored.ID)
	}
	if got[0].Width != 440 {
		t.Errorf("Width = %d, want the stored 440 — an omitted property must not reset it", got[0].Width)
	}
}

// The control: two notes may share a first line as long as they sit apart,
// because position is what identifies them. Without this, "position is the
// identity" could be implemented as "the title still blocks you".
func TestCreateAnnotationAllowsSameTitleAtDifferentPositions(t *testing.T) {
	ctx, written := annotationCtx(t, annotationAt(10, 10, "Same title"))

	err := execCreateAnnotation(ctx, &ast.CreateAnnotationStmt{
		Module:   "Notes",
		Caption:  "Same title",
		Position: &ast.Position{X: 20, Y: 20},
	})
	assertNoError(t, err)

	if got := *written; len(got) != 2 {
		t.Fatalf("got %d annotations, want 2 — a distinct position is a distinct note", len(got))
	}
}

// With no position there is nothing but the title, so a collision is refused
// rather than silently updating or duplicating.
func TestCreateAnnotationWithoutPositionRefusesADuplicateTitle(t *testing.T) {
	ctx, _ := annotationCtx(t, annotationAt(10, 10, "Same title"))

	err := execCreateAnnotation(ctx, &ast.CreateAnnotationStmt{
		Module:  "Notes",
		Caption: "Same title",
	})
	if err == nil {
		t.Fatal("a second note with the same title and no position was accepted")
	}
	if !strings.Contains(err.Error(), "create or modify") {
		t.Errorf("error = %q, want it to name the alternative", err)
	}
}

// A new note gets Studio Pro's own defaults rather than a 0x0 box at the origin.
func TestCreateAnnotationUsesStudioProDefaults(t *testing.T) {
	ctx, written := annotationCtx(t)

	assertNoError(t, execCreateAnnotation(ctx, &ast.CreateAnnotationStmt{
		Module:  "Notes",
		Caption: "Reference data",
	}))

	got := *written
	if len(got) != 1 {
		t.Fatalf("got %d annotations, want 1", len(got))
	}
	if got[0].Width == 0 || got[0].Location == (model.Point{}) {
		t.Errorf("new note has Width %d at %+v, want Studio Pro's defaults",
			got[0].Width, got[0].Location)
	}
}

// A caption is required: it is the note's whole content AND its fallback
// address, so an empty one leaves a blank box nobody can name.
func TestCreateAnnotationRequiresACaption(t *testing.T) {
	ctx, _ := annotationCtx(t)
	if err := execCreateAnnotation(ctx, &ast.CreateAnnotationStmt{Module: "Notes"}); err == nil {
		t.Fatal("an annotation with no caption was accepted")
	}
}

func TestDropAnnotationByTitleAndByPosition(t *testing.T) {
	t.Run("by title", func(t *testing.T) {
		ctx, written := annotationCtx(t,
			annotationAt(10, 10, "Orders"), annotationAt(20, 20, "Reference data"))

		assertNoError(t, execDropAnnotation(ctx, &ast.DropAnnotationStmt{Module: "Notes", Title: "Orders"}))

		got := *written
		if len(got) != 1 || annotationTitle(got[0].Caption) != "Reference data" {
			t.Fatalf("remaining = %d notes, want just Reference data", len(got))
		}
	})

	t.Run("by position", func(t *testing.T) {
		ctx, written := annotationCtx(t,
			annotationAt(10, 10, "Orders"), annotationAt(20, 20, "Reference data"))

		assertNoError(t, execDropAnnotation(ctx, &ast.DropAnnotationStmt{
			Module: "Notes", Position: &ast.Position{X: 20, Y: 20},
		}))

		got := *written
		if len(got) != 1 || annotationTitle(got[0].Caption) != "Orders" {
			t.Fatalf("remaining = %d notes, want just Orders", len(got))
		}
	})
}

// Two notes sharing a first line cannot be told apart by title, so the
// title-addressed form refuses instead of deleting whichever came first — and
// the message names the form that CAN tell them apart.
func TestDropAnnotationRefusesAnAmbiguousTitle(t *testing.T) {
	ctx, _ := annotationCtx(t,
		annotationAt(10, 10, "Same title"), annotationAt(20, 20, "Same title"))

	err := execDropAnnotation(ctx, &ast.DropAnnotationStmt{Module: "Notes", Title: "Same title"})
	if err == nil {
		t.Fatal("an ambiguous title deleted a note; it must refuse")
	}
	if !strings.Contains(err.Error(), "drop annotation at") {
		t.Errorf("error = %q, want it to name the position-addressed form", err)
	}
}

func TestDropAnnotationNotFoundIsAnError(t *testing.T) {
	ctx, _ := annotationCtx(t, annotationAt(10, 10, "Orders"))
	if err := execDropAnnotation(ctx, &ast.DropAnnotationStmt{Module: "Notes", Title: "Nope"}); err == nil {
		t.Fatal("dropping a note that does not exist reported success")
	}
}

// The title is the FIRST non-empty line, so a note whose caption opens with a
// blank line is still addressable, and a multi-line note is not keyed on its
// entire prose.
func TestAnnotationTitleIsTheFirstNonEmptyLine(t *testing.T) {
	for _, tc := range []struct{ caption, want string }{
		{"Orders", "Orders"},
		{"Orders\nmore text", "Orders"},
		{"\n\n  Orders  \nmore", "Orders"},
		{"Orders\r\nwindows line ending", "Orders"},
		{"", ""},
	} {
		if got := annotationTitle(tc.caption); got != tc.want {
			t.Errorf("annotationTitle(%q) = %q, want %q", tc.caption, got, tc.want)
		}
	}
}
