// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func init() {
	// A snippet always emits its Parameters and Variables arrays (empty = marker 3).
	codec.RegisterTypeDefaults("Forms$Snippet", codec.TypeDefaults{
		MandatoryLists: []string{"Parameters", "Variables"},
	})
}

// encodeSnippet builds and serializes a Forms$Snippet for a project of this
// version. Snippet.Variables shares the Forms$Page floor (10.17.0), and a
// snippet never populates the list — so the only thing that could reach a
// pre-10.17 project is the empty marker the defaults registry adds. Both
// CreateSnippet and UpdateSnippet go through here so the guard cannot be applied
// on one path and forgotten on the other.
func encodeSnippet(snippet *pages.Snippet, pv *types.ProjectVersion) ([]byte, error) {
	g, err := snippetToGen(snippet)
	if err != nil {
		return nil, err
	}
	g.SetID(element.ID(snippet.ID))
	return docEncoder("Forms$Snippet", pv).Encode(g)
}

// CreateSnippet inserts a new Forms$Snippet document — a reusable widget tree with
// its own parameters (entity-typed) and a flat Widgets list (no layout call).
func (b *Backend) CreateSnippet(snippet *pages.Snippet) error {
	// A pluggable widget's children are serialized while the executor builds the
	// page, before this call; drain any failure so an unsupported construct fails
	// the statement instead of silently landing as a widget with the piece missing.
	if err := takeChildSerializeErr(); err != nil {
		return fmt.Errorf("CreateSnippet: %w", err)
	}
	if snippet == nil {
		return fmt.Errorf("CreateSnippet: nil snippet")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateSnippet: not connected for writing")
	}
	if snippet.ID == "" {
		snippet.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := encodeSnippet(snippet, b.ProjectVersion())
	if err != nil {
		return fmt.Errorf("CreateSnippet: encode: %w", err)
	}
	if err := b.writer.InsertUnit(string(snippet.ID), string(snippet.ContainerID), "Documents", "Forms$Snippet", contents); err != nil {
		return fmt.Errorf("CreateSnippet: insert: %w", err)
	}
	return nil
}

// UpdateSnippet rewrites a snippet document (CREATE OR REPLACE).
func (b *Backend) UpdateSnippet(snippet *pages.Snippet) error {
	// A pluggable widget's children are serialized while the executor builds the
	// page, before this call; drain any failure so an unsupported construct fails
	// the statement instead of silently landing as a widget with the piece missing.
	if err := takeChildSerializeErr(); err != nil {
		return fmt.Errorf("UpdateSnippet: %w", err)
	}
	if snippet == nil {
		return fmt.Errorf("UpdateSnippet: nil snippet")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateSnippet: not connected for writing")
	}
	contents, err := encodeSnippet(snippet, b.ProjectVersion())
	if err != nil {
		return fmt.Errorf("UpdateSnippet: encode: %w", err)
	}
	if err := b.writer.UpdateRawUnit(string(snippet.ID), contents); err != nil {
		return fmt.Errorf("UpdateSnippet: update: %w", err)
	}
	return nil
}

// DeleteSnippet removes the snippet unit.
func (b *Backend) DeleteSnippet(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteSnippet: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// snippetToGen builds the gen Snippet (header + parameters + widget tree).
func snippetToGen(s *pages.Snippet) (*genPg.Snippet, error) {
	out := genPg.NewSnippet()
	out.SetName(s.Name)
	out.SetDocumentation(s.Documentation)
	// Carry the stored exclusion: hardcoding false silently un-excluded the
	// document on every rewrite (#914).
	out.SetExcluded(s.Excluded)
	out.SetExportLevel("Hidden")
	out.SetCanvasWidth(800)
	out.SetCanvasHeight(600)
	out.SetType("")

	for _, p := range s.Parameters {
		out.AddParameters(snippetParameterToGen(p))
	}
	for _, w := range s.Widgets {
		wg, err := widgetToGen(w)
		if err != nil {
			return nil, err
		}
		out.AddWidgets(wg)
	}
	return out, nil
}

// snippetParameterToGen builds a Forms$SnippetParameter. Its ParameterType is
// the same polymorphic DataTypes$DataType a page parameter carries, so it goes
// through the same builder: p.Type holds a primitive's BSON $Type when the
// parameter is primitive, and is empty for an entity parameter.
//
// It used to build a DataTypes$ObjectType unconditionally, so a primitive-typed
// parameter was written pointing at an entity named "" (mendixlabs/mxcli#1028).
func snippetParameterToGen(p *pages.SnippetParameter) *genPg.SnippetParameter {
	gp := genPg.NewSnippetParameter()
	if p.ID != "" {
		gp.SetID(element.ID(p.ID))
	}
	assignID(gp)
	gp.SetName(p.Name)
	gp.SetParameterType(paramTypeToGen(p.Type, p.EntityName))
	return gp
}
