// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genDT "github.com/mendixlabs/mxcli/modelsdk/gen/datatypes"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/modelsdk/element"
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
	out.SetAppearance(newAppearance("", ""))

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

// layoutCallToGen builds the Forms$LayoutCall (page → layout binding) with one
// Forms$FormCallArgument per placeholder. Widget-bearing arguments are refused.
func layoutCallToGen(lc *pages.LayoutCall) (*genPg.LayoutCall, error) {
	out := genPg.NewLayoutCall()
	assignID(out)
	out.SetLayoutQualifiedName(lc.LayoutName)
	for _, arg := range lc.Arguments {
		ga := genPg.NewLayoutCallArgument()
		// The gen mislabels this type; real BSON is Forms$FormCallArgument.
		ga.SetTypeName("Forms$FormCallArgument")
		assignID(ga)
		ga.SetParameterQualifiedName(string(arg.ParameterID))
		if arg.Widget != nil {
			wg, err := widgetToGen(arg.Widget)
			if err != nil {
				return nil, err
			}
			ga.AddWidgets(wg)
		}
		out.AddArguments(ga)
	}
	return out, nil
}

// pageParameterToGen converts a page parameter, including its ParameterType — an
// entity (DataTypes$ObjectType) or a primitive (DataTypes$StringType, …). Without
// the type the parameter can't resolve (CE5601/CE5606).
func pageParameterToGen(p *pages.PageParameter) *genPg.PageParameter {
	gp := genPg.NewPageParameter()
	if p.ID != "" {
		gp.SetID(element.ID(p.ID))
	}
	assignID(gp)
	gp.SetName(p.Name)
	gp.SetIsRequired(p.IsRequired)
	gp.SetDefaultValue(p.DefaultValue)
	gp.SetParameterType(pageParamTypeToGen(p))
	return gp
}

// pageParamTypeToGen builds a page parameter's type: a DataTypes$ObjectType for an
// entity parameter, or the named primitive DataTypes type. p.TypeName carries the
// primitive's BSON $Type (e.g. "DataTypes$StringType") when set.
func pageParamTypeToGen(p *pages.PageParameter) element.Element {
	if p.TypeName == "" {
		t := genDT.NewObjectType()
		assignID(t)
		t.SetEntityQualifiedName(p.EntityName)
		return t
	}
	var t element.Element
	switch p.TypeName {
	case "DataTypes$IntegerType":
		t = genDT.NewIntegerType()
	case "DataTypes$BooleanType":
		t = genDT.NewBooleanType()
	case "DataTypes$DecimalType":
		t = genDT.NewDecimalType()
	case "DataTypes$DateTimeType":
		t = genDT.NewDateTimeType()
	default:
		t = genDT.NewStringType()
	}
	assignID(t)
	return t
}
