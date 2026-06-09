// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func init() {
	// A page always emits its Parameters and Variables arrays (empty = marker 3).
	codec.RegisterTypeDefaults("Forms$Page", codec.TypeDefaults{
		MandatoryLists: []string{"Parameters", "Variables"},
	})
	// FormCall arguments use the typed-array marker 2 when populated (the codec
	// emits 3 for empty lists automatically).
	codec.RegisterListMarker("Forms$FormCallArgument", 2)
}

// CreatePage inserts a new Forms$Page document unit. The page header (appearance,
// layout call, title, parameters) is built here; the widget tree is not yet
// ported, so a page that places any widget is refused loudly rather than written
// half-formed (ADR-0005 guard-don't-drop).
func (b *Backend) CreatePage(page *pages.Page) error {
	if page == nil {
		return fmt.Errorf("CreatePage: nil page")
	}
	if b.writer == nil {
		return fmt.Errorf("CreatePage: not connected for writing")
	}
	if page.ID == "" {
		page.ID = model.ID(mmpr.GenerateID())
	}
	g, err := pageToGen(page)
	if err != nil {
		return err
	}
	g.SetID(element.ID(page.ID))
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("CreatePage: encode: %w", err)
	}
	if err := b.writer.InsertUnit(string(page.ID), string(page.ContainerID), "Documents", "Forms$Page", contents); err != nil {
		return fmt.Errorf("CreatePage: insert: %w", err)
	}
	return nil
}

// pageToGen builds the gen Page header. Returns an error if the page contains a
// widget (widget-tree conversion is a later phase).
func pageToGen(page *pages.Page) (*genPg.Page, error) {
	out := genPg.NewPage()
	out.SetName(page.Name)
	out.SetDocumentation(page.Documentation)
	out.SetExcluded(page.Excluded)
	out.SetExportLevel("Hidden")
	out.SetAutofocus("DesktopOnly")
	out.SetCanvasWidth(1200)
	out.SetCanvasHeight(600)
	out.SetMarkAsUsed(page.MarkAsUsed)
	out.SetUrl(page.URL)
	out.SetPopupCloseAction("")
	out.SetPopupWidth(int32(page.PopupWidth))
	out.SetPopupHeight(int32(page.PopupHeight))
	out.SetPopupResizable(page.PopupResizable)
	out.SetAllowedRolesQualifiedNames(moduleRoleNames(page.AllowedRoles))
	out.SetTitle(captionToGen(page.Title))
	out.SetAppearance(newPageAppearance())

	if page.LayoutCall != nil {
		lc, err := layoutCallToGen(page.LayoutCall)
		if err != nil {
			return nil, err
		}
		out.SetLayoutCall(lc)
	}

	for _, p := range page.Parameters {
		out.AddParameters(pageParameterToGen(p))
	}
	return out, nil
}

// newPageAppearance builds the empty Forms$Appearance every page carries.
func newPageAppearance() *genPg.Appearance {
	a := genPg.NewAppearance()
	assignID(a)
	a.SetClass("")
	a.SetStyle("")
	a.SetDynamicClasses("")
	return a
}

// layoutCallToGen builds the Forms$LayoutCall (page → layout binding) with one
// Forms$FormCallArgument per placeholder. Widget-bearing arguments are refused.
func layoutCallToGen(lc *pages.LayoutCall) (*genPg.LayoutCall, error) {
	out := genPg.NewLayoutCall()
	assignID(out)
	out.SetLayoutQualifiedName(lc.LayoutName)
	for _, arg := range lc.Arguments {
		if arg.Widget != nil {
			return nil, fmt.Errorf("CreatePage: widget %T in placeholder %q not yet supported by the modelsdk engine — rerun with MXCLI_ENGINE=legacy", arg.Widget, arg.ParameterID)
		}
		ga := genPg.NewLayoutCallArgument()
		// The gen mislabels this type; real BSON is Forms$FormCallArgument.
		ga.SetTypeName("Forms$FormCallArgument")
		assignID(ga)
		ga.SetParameterQualifiedName(string(arg.ParameterID))
		out.AddArguments(ga)
	}
	return out, nil
}

// pageParameterToGen converts a page parameter (entity or primitive typed).
func pageParameterToGen(p *pages.PageParameter) *genPg.PageParameter {
	gp := genPg.NewPageParameter()
	if p.ID != "" {
		gp.SetID(element.ID(p.ID))
	}
	assignID(gp)
	gp.SetName(p.Name)
	return gp
}
