// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	genTexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func init() {
	// Conditional-visibility / native-accessibility slots are null when unset, on
	// every widget that has them (verified against real page BSON).
	for _, t := range []string{"Forms$DivContainer", "Forms$DynamicText"} {
		codec.RegisterTypeDefaults(t, codec.TypeDefaults{
			NullFields: []string{"ConditionalVisibilitySettings", "NativeAccessibilitySettings"},
		})
		// Widgets nested in a Widgets list use the typed-array marker 2 when present.
		codec.RegisterListMarker(t, 2)
	}
	// A ClientTemplate's Parameters list is always emitted with marker 2, even empty
	// (unusual — most empty lists are marker 3).
	codec.RegisterTypeDefaults("Forms$ClientTemplate", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Parameters": 2},
	})
	// LayoutGrid and its rows carry a null ConditionalVisibilitySettings; the grid,
	// rows, and columns all use the typed-array marker 2 in their parent lists.
	for _, t := range []string{"Forms$LayoutGrid", "Forms$LayoutGridRow"} {
		codec.RegisterTypeDefaults(t, codec.TypeDefaults{
			NullFields: []string{"ConditionalVisibilitySettings"},
		})
	}
	codec.RegisterListMarker("Forms$LayoutGrid", 2)
	codec.RegisterListMarker("Forms$LayoutGridRow", 2)
	codec.RegisterListMarker("Forms$LayoutGridColumn", 2)
	// ActionButton: null Icon/visibility/accessibility slots; marker 2 as a widget.
	codec.RegisterTypeDefaults("Forms$ActionButton", codec.TypeDefaults{
		NullFields: []string{"Icon", "ConditionalVisibilitySettings", "NativeAccessibilitySettings"},
	})
	codec.RegisterListMarker("Forms$ActionButton", 2)
	// Title: null visibility/accessibility slots; marker 2 as a widget.
	codec.RegisterTypeDefaults("Forms$Title", codec.TypeDefaults{
		NullFields: []string{"ConditionalVisibilitySettings", "NativeAccessibilitySettings"},
	})
	codec.RegisterListMarker("Forms$Title", 2)
	// A caption parameter's AttributeRef/SourceVariable are null for the literal-
	// expression form; populated Parameters lists use marker 2.
	codec.RegisterTypeDefaults("Forms$ClientTemplateParameter", codec.TypeDefaults{
		NullFields: []string{"AttributeRef", "SourceVariable"},
	})
	codec.RegisterListMarker("Forms$ClientTemplateParameter", 2)
	// DataView: null visibility/editability settings; Widgets/FooterWidgets always
	// emitted with marker 2 (even empty). A page-context DataViewSource has null
	// EntityRef/SourceVariable when unbound.
	codec.RegisterTypeDefaults("Forms$DataView", codec.TypeDefaults{
		NullFields:           []string{"ConditionalVisibilitySettings", "ConditionalEditabilitySettings"},
		MandatoryListMarkers: map[string]int32{"Widgets": 2, "FooterWidgets": 2},
	})
	codec.RegisterListMarker("Forms$DataView", 2)
	codec.RegisterTypeDefaults("Forms$DataViewSource", codec.TypeDefaults{
		NullFields: []string{"EntityRef", "SourceVariable"},
	})
	// TextBox: many null slots when unbound (attribute ref, screen-reader label,
	// source variable, label template, visibility/editability/native settings).
	codec.RegisterTypeDefaults("Forms$TextBox", codec.TypeDefaults{
		NullFields: []string{
			"AttributeRef", "ScreenReaderLabel", "SourceVariable", "LabelTemplate",
			"ConditionalVisibilitySettings", "ConditionalEditabilitySettings",
			"NativeAccessibilitySettings",
		},
	})
	codec.RegisterListMarker("Forms$TextBox", 2)
}

// widgetToGen converts a model widget to its gen element, recursing into
// containers. Unsupported widget types are refused loudly (ADR-0005) so a page
// is never written with a silently-dropped widget.
func widgetToGen(w pages.Widget) (element.Element, error) {
	switch x := w.(type) {
	case *pages.Container:
		g := genPg.NewDivContainer()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetRenderMode(orDefaultStr(string(x.RenderMode), "Div"))
		g.SetScreenReaderHidden(false)
		g.SetOnClickAction(noActionGen())
		for _, c := range x.Widgets {
			cg, err := widgetToGen(c)
			if err != nil {
				return nil, err
			}
			g.AddWidgets(cg)
		}
		return g, nil

	case *pages.DynamicText:
		g := genPg.NewDynamicText()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetRenderMode(orDefaultStr(string(x.RenderMode), "Text"))
		g.SetNativeTextStyle("Text")
		g.SetContent(clientTemplateToGen(x.Content))
		return g, nil

	case *pages.LayoutGrid:
		g := genPg.NewLayoutGrid()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetWidth("FullWidth")
		for _, row := range x.Rows {
			rg, err := layoutGridRowToGen(row)
			if err != nil {
				return nil, err
			}
			g.AddRows(rg)
		}
		return g, nil

	case *pages.DataView:
		g := genPg.NewDataView()
		applyWidgetBase(g, &x.BaseWidget)
		ds, err := dataViewSourceToGen(x.DataSource)
		if err != nil {
			return nil, err
		}
		g.SetDataSource(ds)
		g.SetEditability(editability(x.ReadOnly))
		g.SetReadOnlyStyle("Control")
		g.SetShowFooter(x.ShowFooter)
		if x.LabelWidth != nil {
			g.SetLabelWidth(int32(*x.LabelWidth))
		}
		g.SetNoEntityMessage(captionToGen(x.NoEntityMessage))
		for _, c := range x.Widgets {
			cg, err := widgetToGen(c)
			if err != nil {
				return nil, err
			}
			g.AddWidgets(cg)
		}
		for _, c := range x.FooterWidgets {
			cg, err := widgetToGen(c)
			if err != nil {
				return nil, err
			}
			g.AddFooterWidgets(cg)
		}
		return g, nil

	case *pages.Title:
		g := genPg.NewTitle()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetCaption(captionToGen(x.Caption))
		return g, nil

	case *pages.TextBox:
		g := genPg.NewTextBox()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetAriaRequired(false)
		g.SetAutoFocus(false)
		g.SetAutocomplete(true)
		g.SetAutocompletePurpose("On")
		if ref := attributeRefToGen(x.AttributePath); ref != nil {
			g.SetAttributeRef(ref)
		}
		g.SetEditable("Always")
		g.SetFormattingInfo(newFormattingInfo())
		g.SetInputMask("")
		g.SetIsPasswordBox(x.IsPassword)
		g.SetKeyboardType("Default")
		if x.Label != "" {
			g.SetLabelTemplate(textAsClientTemplate(textFromString(x.Label)))
		}
		g.SetMaxLengthCode(-1)
		onChange, err := clientActionToGen(x.OnChangeAction)
		if err != nil {
			return nil, err
		}
		onEnter, err := clientActionToGen(x.OnEnterAction)
		if err != nil {
			return nil, err
		}
		g.SetOnChangeAction(onChange)
		g.SetOnEnterAction(onEnter)
		g.SetOnEnterKeyPressAction(noActionGen())
		g.SetOnLeaveAction(noActionGen())
		g.SetPlaceholderTemplate(textAsClientTemplate(x.Placeholder))
		g.SetReadOnlyStyle("Inherit")
		g.SetSubmitBehaviour("OnEndEditing")
		g.SetSubmitOnInputDelay(300)
		g.SetValidation(widgetValidationToGen())
		return g, nil

	case *pages.ActionButton:
		g := genPg.NewActionButton()
		applyWidgetBase(g, &x.BaseWidget)
		g.SetAriaRole("Button")
		g.SetRenderType("Button")
		g.SetButtonStyle(orDefaultStr(string(x.ButtonStyle), "Default"))
		if x.CaptionTemplate != nil {
			g.SetCaption(clientTemplateToGen(x.CaptionTemplate))
		} else {
			g.SetCaption(textAsClientTemplate(x.Caption))
		}
		g.SetTooltip(textAsClientTemplate(x.Tooltip))
		act, err := clientActionToGen(x.Action)
		if err != nil {
			return nil, err
		}
		g.SetAction(act)
		return g, nil

	default:
		return nil, fmt.Errorf("CreatePage: widget %T not yet supported by the modelsdk engine — rerun with MXCLI_ENGINE=legacy", w)
	}
}

// layoutGridRowToGen converts a LayoutGridRow (alignment defaults match the
// legacy serializer; not a full widget, so no name/tabindex).
func layoutGridRowToGen(row *pages.LayoutGridRow) (element.Element, error) {
	g := genPg.NewLayoutGridRow()
	if row.ID != "" {
		g.SetID(element.ID(row.ID))
	}
	assignID(g)
	g.SetAppearance(newAppearance("", ""))
	g.SetHorizontalAlignment("None")
	g.SetSpacingBetweenColumns(true)
	g.SetVerticalAlignment("None")
	for _, col := range row.Columns {
		cg, err := layoutGridColumnToGen(col)
		if err != nil {
			return nil, err
		}
		g.AddColumns(cg)
	}
	return g, nil
}

// layoutGridColumnToGen converts a LayoutGridColumn. Weights default to -1 (auto,
// via columnWeight); PreviewWidth is always -1 — matching the legacy serializer.
func layoutGridColumnToGen(col *pages.LayoutGridColumn) (element.Element, error) {
	g := genPg.NewLayoutGridColumn()
	if col.ID != "" {
		g.SetID(element.ID(col.ID))
	}
	assignID(g)
	g.SetAppearance(newAppearance("", ""))
	g.SetWeight(int32(columnWeight(col.Weight)))
	g.SetTabletWeight(int32(columnWeight(col.TabletWeight)))
	g.SetPhoneWeight(int32(columnWeight(col.PhoneWeight)))
	g.SetPreviewWidth(-1)
	g.SetVerticalAlignment("None")
	for _, w := range col.Widgets {
		wg, err := widgetToGen(w)
		if err != nil {
			return nil, err
		}
		g.AddWidgets(wg)
	}
	return g, nil
}

// columnWeight maps an unset weight (0) to -1 (auto-fill), matching the legacy
// serializer's columnWeight.
func columnWeight(w int) int32 {
	if w == 0 {
		return -1
	}
	return int32(w)
}

// widgetBaseGen is the shared setter surface of a gen widget element.
type widgetBaseGen interface {
	element.Element
	SetID(element.ID)
	SetName(string)
	SetAppearance(element.Element)
	SetTabIndex(int32)
}

// applyWidgetBase sets the fields every widget shares: identity, name, appearance
// (carrying class/style), and tab index. ConditionalVisibility/native
// accessibility are emitted null via the registered defaults.
func applyWidgetBase(g widgetBaseGen, b *pages.BaseWidget) {
	if b.ID != "" {
		g.SetID(element.ID(b.ID))
	}
	assignID(g)
	g.SetName(b.Name)
	g.SetAppearance(newAppearance(b.Class, b.Style))
	g.SetTabIndex(int32(b.TabIndex))
}

// newAppearance builds a Forms$Appearance with the given class/style (empty
// DesignProperties / dynamic classes).
func newAppearance(class, style string) *genPg.Appearance {
	a := genPg.NewAppearance()
	assignID(a)
	a.SetClass(class)
	a.SetStyle(style)
	a.SetDynamicClasses("")
	return a
}

// noActionGen builds the default Forms$NoAction (DisabledDuringExecution=true)
// used by widget OnClick slots that have no action.
func noActionGen() element.Element {
	a := genPg.NewNoClientAction() // emits $Type Forms$NoAction
	assignID(a)
	a.SetDisabledDuringExecution(true)
	return a
}

// clientTemplateToGen builds the Forms$ClientTemplate that backs a dynamic text
// or button caption (Template + Fallback are Texts$Text; Parameters supply the
// {1}/{2}… placeholder values).
func clientTemplateToGen(ct *pages.ClientTemplate) element.Element {
	g := genPg.NewClientTemplate()
	assignID(g)
	if ct == nil {
		g.SetTemplate(genTexts.NewText())
		g.SetFallback(genTexts.NewText())
		return g
	}
	g.SetTemplate(captionToGen(ct.Template))
	g.SetFallback(captionToGen(ct.Fallback))
	for _, p := range ct.Parameters {
		g.AddParameters(clientTemplateParameterToGen(p))
	}
	return g
}

// textAsClientTemplate wraps a plain caption/tooltip Text in a ClientTemplate.
func textAsClientTemplate(t *model.Text) element.Element {
	return clientTemplateToGen(&pages.ClientTemplate{Template: t})
}

// clientTemplateParameterToGen converts a caption parameter ({n} value). Only the
// literal-expression form is supported; AttributeRef/SourceVariable stay null
// (registered defaults). FormattingInfo carries the standard defaults.
func clientTemplateParameterToGen(p *pages.ClientTemplateParameter) element.Element {
	g := genPg.NewClientTemplateParameter()
	if p.ID != "" {
		g.SetID(element.ID(p.ID))
	}
	assignID(g)
	g.SetExpression(p.Expression)
	if p.AttributeRef != "" {
		g.SetAttributePath(p.AttributeRef)
	}
	g.SetFormattingInfo(newFormattingInfo())
	return g
}

// newFormattingInfo builds the default Forms$FormattingInfo (matches the legacy
// serializer; TimeFormat is intentionally omitted — it triggers CE0463).
func newFormattingInfo() element.Element {
	f := genPg.NewFormattingInfo()
	assignID(f)
	f.SetCustomDateFormat("")
	f.SetDateFormat("Date")
	f.SetDecimalPrecision(2)
	f.SetEnumFormat("Text")
	f.SetGroupDigits(false)
	return f
}

// textFromString wraps a non-empty string as a single-translation model.Text.
func textFromString(s string) *model.Text {
	if s == "" {
		return nil
	}
	return &model.Text{Translations: map[string]string{"en_US": s}}
}

// attributeRefToGen builds a DomainModels$AttributeRef for a fully-qualified
// attribute path (Module.Entity.Attribute); returns nil otherwise (the slot is
// then emitted null via the registered default), matching the legacy serializer.
func attributeRefToGen(path string) element.Element {
	if strings.Count(path, ".") < 2 {
		return nil
	}
	r := genDm.NewAttributeRef()
	assignID(r)
	r.SetAttributeQualifiedName(path)
	return r
}

// widgetValidationToGen builds the default empty Forms$WidgetValidation.
func widgetValidationToGen() element.Element {
	v := genPg.NewWidgetValidation()
	assignID(v)
	v.SetExpression("")
	v.SetMessage(genTexts.NewText())
	return v
}

// editability maps a read-only flag to the Forms editability enum.
func editability(readOnly bool) string {
	if readOnly {
		return "Never"
	}
	return "Always"
}

// dataViewSourceToGen builds a DataView's data source. The page-context source
// (Forms$DataViewSource: entity ref + page/snippet parameter) is supported; flow
// and database sources (which carry settings sub-objects) are refused for now.
func dataViewSourceToGen(ds pages.DataSource) (element.Element, error) {
	src := genPg.NewDataViewSource()
	src.SetForceFullObjects(false)
	switch d := ds.(type) {
	case nil:
		// empty DataViewSource — DataView requires a non-null source object
	case *pages.DataViewSource:
		if d.ID != "" {
			src.SetID(element.ID(d.ID))
		}
		if d.EntityName != "" {
			ref := genDm.NewDirectEntityRef()
			assignID(ref)
			ref.SetEntityQualifiedName(d.EntityName)
			src.SetEntityRef(ref)
		}
		if d.ParameterName != "" {
			pv := genPg.NewPageVariable()
			assignID(pv)
			if d.IsSnippetParameter {
				pv.SetSnippetParameterQualifiedName(d.ParameterName)
			} else {
				pv.SetPageParameterQualifiedName(d.ParameterName)
			}
			src.SetSourceVariable(pv)
		}
	default:
		return nil, fmt.Errorf("CreatePage: DataView source %T not yet supported by the modelsdk engine — rerun with MXCLI_ENGINE=legacy", ds)
	}
	assignID(src)
	return src, nil
}

// clientActionToGen converts a widget client action. Simple actions are supported;
// the page/microflow/nanoflow/create-object actions (which carry settings sub-
// objects) are refused loudly for now.
func clientActionToGen(a pages.ClientAction) (element.Element, error) {
	switch x := a.(type) {
	case nil, *pages.NoClientAction:
		return noActionGen(), nil
	case *pages.SaveChangesClientAction:
		g := genPg.NewSaveChangesClientAction()
		assignID(g)
		g.SetClosePage(x.ClosePage)
		g.SetSyncAutomatically(true)
		return g, nil
	case *pages.CancelChangesClientAction:
		g := genPg.NewCancelChangesClientAction()
		assignID(g)
		g.SetClosePage(x.ClosePage)
		return g, nil
	case *pages.ClosePageClientAction:
		g := genPg.NewClosePageClientAction()
		assignID(g)
		return g, nil
	case *pages.DeleteClientAction:
		g := genPg.NewDeleteClientAction()
		assignID(g)
		g.SetClosePage(x.ClosePage)
		return g, nil
	default:
		return nil, fmt.Errorf("CreatePage: client action %T not yet supported by the modelsdk engine — rerun with MXCLI_ENGINE=legacy", a)
	}
}

// orDefaultStr returns s, or def when s is empty.
func orDefaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
