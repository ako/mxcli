// SPDX-License-Identifier: Apache-2.0

// cmd_annotations.go — CREATE/SHOW/DROP ANNOTATION, the notes on a domain model.
//
// A domain-model annotation is the box Studio Pro draws on the canvas to explain
// the diagram — every blank Mendix app ships with one. It is a child of the
// module's domain model rather than a document of its own, so the statements
// name a module.
//
// Mendix stores four properties: Caption, ExportLevel, Location and Width. There
// is **no colour**. A domain model holds exactly four child collections
// (Annotations, Associations, CrossAssociations, Entities), and nothing in
// DomainModels$Annotation describes styling — so the "coloured section box" a
// modeller sees is this element drawn in Studio Pro's own palette, and mxcli
// cannot author a colour because the model has nowhere to keep one.
//
// The element has no name either, which is the awkward part: a note is addressed
// by the FIRST LINE of its caption, the way an unnamed DataGrid2 column is
// addressed by its caption. Two notes whose first lines match are ambiguous and
// refused rather than resolved by guessing.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// annotationTitle is how a note is addressed: the first non-empty line of its
// caption, trimmed.
//
// A caption is prose and often several lines — the blank app's is a paragraph
// with a URL in it — so the whole thing is unusable as a key. The first line is
// what a person calls the note, and what SHOW prints.
func annotationTitle(caption string) string {
	for line := range strings.SplitSeq(strings.ReplaceAll(caption, "\r\n", "\n"), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// annotationDomainModel resolves the module's domain model, or reports why not.
func annotationDomainModel(ctx *ExecContext, moduleName string) (*domainmodel.DomainModel, error) {
	module, err := findModule(ctx, moduleName)
	if err != nil {
		return nil, mdlerrors.NewNotFound("module", moduleName)
	}
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil || dm == nil {
		return nil, mdlerrors.NewBackend("get domain model for "+moduleName, err)
	}
	return dm, nil
}

// findAnnotationAt locates the note at an exact canvas position.
//
// Position is a note's identity whenever the script gives one, because it is the
// one stored property a script controls that does NOT change when the wording
// does. Matching on the caption alone meant editing a note's first line created
// a second note instead of updating the first — the exact thing a re-runnable
// layout script does every time it is edited.
//
// Two notes cannot overlap exactly in any sane diagram, so an exact hit is
// unambiguous; if somehow both are stacked, the first is taken, and the title
// path below is what would have refused anyway.
func findAnnotationAt(dm *domainmodel.DomainModel, pos model.Point) *domainmodel.Annotation {
	for _, a := range dm.Annotations {
		if a.Location == pos {
			return a
		}
	}
	return nil
}

// findAnnotationByTitle locates the single note whose title matches.
//
// Ambiguity is refused rather than resolved: with no name to fall back on,
// picking one of two would silently edit or delete the wrong note, and the
// author has no way to tell which.
func findAnnotationByTitle(dm *domainmodel.DomainModel, moduleName, title string) (*domainmodel.Annotation, error) {
	var found *domainmodel.Annotation
	matches := 0
	for _, a := range dm.Annotations {
		if strings.EqualFold(annotationTitle(a.Caption), title) {
			found = a
			matches++
		}
	}
	switch {
	case matches == 0:
		return nil, nil
	case matches > 1:
		return nil, mdlerrors.NewValidationf(
			"%d annotations in this domain model start with %q, so it does not identify one. "+
				"An annotation has no name — it is addressed by the first line of its caption. "+
				"Address it by where it sits instead (drop annotation at (x, y) in %s; "+
				"show annotations prints each one's position), or give them distinct first lines",
			matches, title, moduleName)
	}
	return found, nil
}

// execCreateAnnotation handles CREATE [OR MODIFY] ANNOTATION IN Module (…).
func execCreateAnnotation(ctx *ExecContext, s *ast.CreateAnnotationStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	if strings.TrimSpace(s.Caption) == "" {
		return mdlerrors.NewValidation(
			"an annotation needs a Caption: it is the note's whole content, and it is " +
				"also how the note is addressed (by its first line)")
	}

	dm, err := annotationDomainModel(ctx, s.Module)
	if err != nil {
		return err
	}

	title := annotationTitle(s.Caption)

	// When a Position is given it IS the note's identity, and the title is not
	// consulted at all: a script that places its notes can edit their wording and
	// re-run without accumulating duplicates, and two notes may legitimately share
	// a first line as long as they sit in different places.
	//
	// With no Position there is nothing stable to hold on to, so the title is all
	// that is left — and a collision there really is ambiguous.
	var existing *domainmodel.Annotation
	if s.Position != nil {
		existing = findAnnotationAt(dm, model.Point{X: s.Position.X, Y: s.Position.Y})
	} else {
		var err error
		if existing, err = findAnnotationByTitle(dm, s.Module, title); err != nil {
			return err
		}
	}
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExistsMsg("annotation", title,
			fmt.Sprintf("an annotation starting %q already exists in %s "+
				"(use create or modify to update it)", title, s.Module))
	}

	target := existing
	if target == nil {
		target = &domainmodel.Annotation{ContainerID: dm.ID}
		target.ID = model.ID(types.GenerateID())
		// Studio Pro's own defaults, so a note authored here looks like one drawn
		// on the canvas rather than a 0x0 box in the top-left corner.
		target.Location = model.Point{X: 60, Y: 240}
		target.Width = 440
		dm.Annotations = append(dm.Annotations, target)
	}
	target.Caption = s.Caption
	// An omitted property leaves what is stored alone: re-running a script to
	// change wording must not drag every note back to a default position.
	if s.Position != nil {
		target.Location = model.Point{X: s.Position.X, Y: s.Position.Y}
	}
	if s.Width > 0 {
		target.Width = s.Width
	}

	if err := ctx.Backend.SetDomainModelAnnotations(dm.ID, dm.Annotations); err != nil {
		return mdlerrors.NewBackend("write annotations", err)
	}
	invalidateDomainModelsCache(ctx)

	verb := "Created"
	if existing != nil {
		verb = "Modified"
	}
	ctx.ReportMutation(verb, "annotation in %s: %s", s.Module, title)
	return nil
}

// execDropAnnotation handles DROP ANNOTATION 'title' IN Module.
func execDropAnnotation(ctx *ExecContext, s *ast.DropAnnotationStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	dm, err := annotationDomainModel(ctx, s.Module)
	if err != nil {
		return err
	}
	var target *domainmodel.Annotation
	label := s.Title
	if s.Position != nil {
		target = findAnnotationAt(dm, model.Point{X: s.Position.X, Y: s.Position.Y})
		label = fmt.Sprintf("at (%d, %d)", s.Position.X, s.Position.Y)
	} else if target, err = findAnnotationByTitle(dm, s.Module, s.Title); err != nil {
		return err
	}
	if target == nil {
		return mdlerrors.NewNotFound("annotation", label+" in "+s.Module)
	}

	kept := make([]*domainmodel.Annotation, 0, len(dm.Annotations))
	for _, a := range dm.Annotations {
		if a != target {
			kept = append(kept, a)
		}
	}
	if err := ctx.Backend.SetDomainModelAnnotations(dm.ID, kept); err != nil {
		return mdlerrors.NewBackend("write annotations", err)
	}
	invalidateDomainModelsCache(ctx)

	ctx.ReportMutation("Dropped", "annotation in %s: %s", s.Module, label)
	return nil
}

// listAnnotations handles SHOW ANNOTATIONS [IN Module].
//
// The Title column is the addressable key, so what DROP and CREATE OR MODIFY
// want is visible rather than something the author has to derive.
func listAnnotations(ctx *ExecContext, inModule string) error {
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return mdlerrors.NewBackend("list domain models", err)
	}

	type row struct {
		module, title      string
		x, y, width, lines int
	}
	var rows []row
	for _, dm := range dms {
		moduleName := h.GetModuleName(dm.ContainerID)
		if moduleName == "" || !moduleMatches(moduleName, inModule) {
			continue
		}
		for _, a := range dm.Annotations {
			caption := strings.ReplaceAll(a.Caption, "\r\n", "\n")
			rows = append(rows, row{
				module: moduleName,
				title:  annotationTitle(a.Caption),
				x:      a.Location.X,
				y:      a.Location.Y,
				width:  a.Width,
				lines:  len(strings.Split(caption, "\n")),
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].module != rows[j].module {
			return rows[i].module < rows[j].module
		}
		return rows[i].title < rows[j].title
	})

	result := &TableResult{
		Columns: []string{"Module", "Title", "Position", "Width", "Lines"},
		Summary: fmt.Sprintf("(%d annotation(s))", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{
			r.module, r.title, fmt.Sprintf("(%d, %d)", r.x, r.y), r.width, r.lines,
		})
	}
	return writeResult(ctx, result)
}
