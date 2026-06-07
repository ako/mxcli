// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mapPageWidget maps one executor page widget onto its pg_write_page (Pages$*)
// form, then attaches any conditional-visibility setting (VISIBLE IF) uniformly.
func (b *Backend) mapPageWidget(w pages.Widget) (map[string]any, error) {
	m, err := b.mapPageWidgetBody(w)
	if err != nil {
		return nil, err
	}
	if cv := conditionalVisibility(w); cv != nil {
		m["conditionalVisibilitySettings"] = cv
	}
	return m, nil
}

// conditionalVisibility builds a Pages$ConditionalVisibilitySettings from a
// widget's VISIBLE IF expression. The MDL `visible:` property only ever produces
// an expression (no module-role / attribute / source-variable conditions), so
// only the expression form is mapped; the rest stay at their pg defaults.
func conditionalVisibility(w pages.Widget) map[string]any {
	type baseWidgetGetter interface {
		GetBaseWidget() *pages.BaseWidget
	}
	bwg, ok := w.(baseWidgetGetter)
	if !ok {
		return nil
	}
	cv := bwg.GetBaseWidget().ConditionalVisibility
	if cv == nil || cv.Expression == "" {
		return nil
	}
	return map[string]any{
		"$Type":          "Pages$ConditionalVisibilitySettings",
		"expression":     cv.Expression,
		"conditions":     []any{},
		"moduleRoles":    []any{},
		"ignoreSecurity": false,
	}
}

// mapPageWidgetBody maps the widget's own Pages$* form (without conditional
// settings). Coverage grows one widget type at a time; unmapped widgets are
// rejected.
func (b *Backend) mapPageWidgetBody(w pages.Widget) (map[string]any, error) {
	switch wd := w.(type) {
	case *pages.Container:
		children, err := b.mapPageWidgets(wd.Widgets)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"$Type":      "Pages$DivContainer",
			"name":       wd.Name,
			"appearance": pageAppearance(wd.Class, wd.Style),
			"widgets":    children,
		}, nil
	case *pages.ActionButton:
		action, err := mapClientAction(wd.Action)
		if err != nil {
			return nil, fmt.Errorf("button %q: %w", wd.Name, err)
		}
		style := string(wd.ButtonStyle)
		if style == "" {
			style = "Default"
		}
		return map[string]any{
			"$Type":                         "Pages$ActionButton",
			"name":                          wd.Name,
			"appearance":                    pageAppearance(wd.Class, wd.Style),
			"conditionalVisibilitySettings": nil,
			"ct:caption":                    textValue(wd.Caption),
			"t:tooltip":                     textValue(wd.Tooltip),
			"icon":                          nil,
			"action":                        action,
			"tabIndex":                      wd.TabIndex,
			"renderType":                    "Button",
			"buttonStyle":                   style,
			"ariaRole":                      "Button",
		}, nil
	case *pages.DynamicText:
		content := ""
		if wd.Content != nil && wd.Content.Template != nil {
			content = textValue(wd.Content.Template)
		}
		if content == "" {
			content = wd.AttributePath
		}
		renderMode := string(wd.RenderMode)
		if renderMode == "" {
			renderMode = "Text"
		}
		return map[string]any{
			"$Type":           "Pages$DynamicText",
			"name":            wd.Name,
			"appearance":      pageAppearance(wd.Class, wd.Style),
			"ct:content":      content,
			"tabIndex":        wd.TabIndex,
			"renderMode":      renderMode,
			"nativeTextStyle": "Text",
		}, nil
	case *pages.DataView:
		src, err := mapDataViewSource(wd.DataSource)
		if err != nil {
			return nil, fmt.Errorf("data view %q: %w", wd.Name, err)
		}
		children, err := b.mapPageWidgets(wd.Widgets)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"$Type":      "Pages$DataView",
			"name":       wd.Name,
			"appearance": pageAppearance(wd.Class, wd.Style),
			"dataSource": src,
			"editable":   wd.Editable,
			"widgets":    children,
		}, nil
	case *pages.LayoutGrid:
		rows := make([]any, 0, len(wd.Rows))
		for _, r := range wd.Rows {
			cols := make([]any, 0, len(r.Columns))
			for _, c := range r.Columns {
				kids, err := b.mapPageWidgets(c.Widgets)
				if err != nil {
					return nil, err
				}
				weight := c.Weight
				if weight <= 0 {
					weight = 12
				}
				tablet := c.TabletWeight
				if tablet <= 0 {
					tablet = weight
				}
				phone := c.PhoneWeight
				if phone <= 0 {
					phone = 12
				}
				cols = append(cols, map[string]any{
					"$Type":             "Pages$LayoutGridColumn",
					"appearance":        pageAppearance("", ""),
					"weight":            weight,
					"tabletWeight":      tablet,
					"phoneWeight":       phone,
					"previewWidth":      -1,
					"verticalAlignment": "None",
					"widgets":           kids,
				})
			}
			rows = append(rows, map[string]any{
				"$Type":                 "Pages$LayoutGridRow",
				"appearance":            pageAppearance("", ""),
				"verticalAlignment":     "None",
				"horizontalAlignment":   "None",
				"spacingBetweenColumns": true,
				"columns":               cols,
			})
		}
		return map[string]any{
			"$Type":      "Pages$LayoutGrid",
			"name":       wd.Name,
			"appearance": pageAppearance(wd.Class, wd.Style),
			"tabIndex":   wd.TabIndex,
			"width":      "FullWidth",
			"rows":       rows,
		}, nil
	case *pages.ListView:
		src, err := mapDataViewSource(wd.DataSource)
		if err != nil {
			return nil, fmt.Errorf("list view %q: %w", wd.Name, err)
		}
		rowWidgets := wd.Widgets
		if len(rowWidgets) == 0 && len(wd.Templates) > 0 {
			rowWidgets = wd.Templates[0].Widgets
		}
		kids, err := b.mapPageWidgets(rowWidgets)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"$Type":      "Pages$ListView",
			"name":       wd.Name,
			"appearance": pageAppearance(wd.Class, wd.Style),
			"dataSource": src,
			"editable":   wd.Editable,
			"widgets":    kids,
		}, nil
	case *pages.TabContainer:
		tabs := make([]any, 0, len(wd.TabPages))
		for _, tp := range wd.TabPages {
			kids, err := b.mapPageWidgets(tp.Widgets)
			if err != nil {
				return nil, fmt.Errorf("tab page %q: %w", tp.Name, err)
			}
			caption := textValue(tp.Caption)
			if caption == "" {
				caption = tp.Name
			}
			tabs = append(tabs, map[string]any{
				"$Type":         "Pages$TabPage",
				"name":          tp.Name,
				"t:caption":     caption,
				"refreshOnShow": tp.RefreshOnShow,
				"widgets":       kids,
			})
		}
		return map[string]any{
			"$Type":      "Pages$TabContainer",
			"name":       wd.Name,
			"appearance": pageAppearance(wd.Class, wd.Style),
			"tabIndex":   wd.TabIndex,
			"tabPages":   tabs,
		}, nil
	case *pages.CustomWidget:
		return b.mapCustomWidget(wd)
	case *pages.DataGrid:
		return nil, fmt.Errorf("legacy DataGrid is not supported by the MCP backend — pg_write_page has no Pages$DataGrid type (use a ListView, or DataGrid 2 which is a pluggable widget)")
	case *pages.TextBox:
		return inputWidget("Pages$TextBox", wd.Name, wd.Label, wd.AttributePath, wd.Class, wd.Style), nil
	case *pages.CheckBox:
		return inputWidget("Pages$CheckBox", wd.Name, wd.Label, wd.AttributePath, wd.Class, wd.Style), nil
	case *pages.DatePicker:
		return inputWidget("Pages$DatePicker", wd.Name, wd.Label, wd.AttributePath, wd.Class, wd.Style), nil
	case *pages.TextArea:
		return inputWidget("Pages$TextArea", wd.Name, wd.Label, wd.AttributePath, wd.Class, wd.Style), nil
	case *pages.RadioButtons:
		return inputWidget("Pages$RadioButtonGroup", wd.Name, wd.Label, wd.AttributePath, wd.Class, wd.Style), nil
	default:
		return nil, fmt.Errorf("page widget type %s is not yet supported by the MCP backend", w.GetTypeName())
	}
}

// inputWidget builds a label+attribute input widget (TextBox/CheckBox/DatePicker).
// The executor already resolves AttributePath to a fully-qualified
// "Module.Entity.Attribute", which is exactly what pg's attributeRef wants.
func inputWidget(typ, name, label, attribute, class, style string) map[string]any {
	return map[string]any{
		"$Type":            typ,
		"name":             name,
		"appearance":       pageAppearance(class, style),
		"ct:labelTemplate": label,
		"attributeRef": map[string]any{
			"$Type":     "DomainModels$AttributeRef",
			"attribute": attribute,
		},
	}
}

// mapDataViewSource maps a data-view data source onto a Pages$DataViewSource.
// Supported: a page variable/parameter ($Var). Other source kinds (microflow,
// association, listen-to-widget, multi-step paths) are rejected for now.
func mapDataViewSource(ds pages.DataSource) (map[string]any, error) {
	switch s := ds.(type) {
	case *pages.DataViewSource:
		src := map[string]any{"$Type": "Pages$DataViewSource", "forceFullObjects": false}
		switch {
		case s.ParameterName != "":
			src["sourceVariable"] = map[string]any{
				"$Type":         "Pages$PageVariable",
				"pageParameter": s.ParameterName,
				"useAllPages":   false,
			}
		case s.EntityName != "":
			src["entityRef"] = map[string]any{
				"$Type":  "DomainModels$DirectEntityRef",
				"entity": s.EntityName,
			}
		default:
			return nil, fmt.Errorf("data view source has neither a page parameter nor an entity")
		}
		return src, nil
	case *pages.MicroflowSource:
		if s.Microflow == "" {
			return nil, fmt.Errorf("microflow data source has no microflow")
		}
		return map[string]any{
			"$Type":            "Pages$MicroflowSource",
			"forceFullObjects": false,
			"microflowSettings": map[string]any{
				"$Type":             "Pages$MicroflowSettings",
				"microflow":         s.Microflow,
				"parameterMappings": []any{},
				"outputMappings":    []any{},
				"progressBar":       "None",
				"asynchronous":      false,
				"formValidations":   "All",
			},
		}, nil
	case *pages.DatabaseSource:
		if s.XPathConstraint != "" || len(s.Sorting) > 0 {
			return nil, fmt.Errorf("database data source with an XPath constraint or sorting is not yet supported by the MCP backend")
		}
		if s.EntityName == "" {
			return nil, fmt.Errorf("database data source has no entity")
		}
		return map[string]any{
			"$Type":            "Pages$DataViewSource",
			"entityRef":        map[string]any{"$Type": "DomainModels$DirectEntityRef", "entity": s.EntityName},
			"forceFullObjects": false,
		}, nil
	case *pages.EntityPathSource:
		if strings.HasPrefix(s.EntityPath, "$") && !strings.Contains(s.EntityPath, "/") {
			return map[string]any{
				"$Type": "Pages$DataViewSource",
				"sourceVariable": map[string]any{
					"$Type":         "Pages$PageVariable",
					"pageParameter": strings.TrimPrefix(s.EntityPath, "$"),
					"useAllPages":   false,
				},
				"forceFullObjects": false,
			}, nil
		}
		return nil, fmt.Errorf("data view over path %q is not yet supported by the MCP backend (only a page variable $Var)", s.EntityPath)
	default:
		return nil, fmt.Errorf("data view source %T is not yet supported by the MCP backend", ds)
	}
}

func (b *Backend) mapPageWidgets(ws []pages.Widget) ([]any, error) {
	out := make([]any, 0, len(ws))
	for _, w := range ws {
		m, err := b.mapPageWidget(w)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// mapClientAction maps a widget client action onto its Pages$*ClientAction form.
func mapClientAction(a pages.ClientAction) (map[string]any, error) {
	switch act := a.(type) {
	case nil, *pages.NoClientAction:
		return map[string]any{"$Type": "Pages$NoClientAction"}, nil
	case *pages.MicroflowClientAction:
		return map[string]any{
			"$Type":     "Pages$MicroflowClientAction",
			"microflow": act.MicroflowName,
		}, nil
	case *pages.PageClientAction:
		return map[string]any{
			"$Type": "Pages$PageClientAction",
			"pageSettings": map[string]any{
				"$Type":             "Pages$PageSettings",
				"page":              act.PageName,
				"parameterMappings": []any{},
			},
		}, nil
	default:
		return nil, fmt.Errorf("client action %T is not yet supported by the MCP backend", a)
	}
}

// pageAppearance builds a Pages$Appearance from a widget's class/style.
func pageAppearance(class, style string) map[string]any {
	return map[string]any{
		"$Type":            "Pages$Appearance",
		"class":            class,
		"style":            style,
		"dynamicClasses":   "",
		"designProperties": map[string]any{},
	}
}

// textValue returns the en_US translation of a localized text (empty if nil).
func textValue(t *model.Text) string {
	if t == nil {
		return ""
	}
	return t.Translations["en_US"]
}
