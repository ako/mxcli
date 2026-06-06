// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mapPageWidget maps one executor page widget onto its pg_write_page (Pages$*)
// form. Coverage grows one widget type at a time; unmapped widgets are rejected.
func (b *Backend) mapPageWidget(w pages.Widget) (map[string]any, error) {
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
	case *pages.TextBox:
		return inputWidget("Pages$TextBox", wd.Name, wd.Label, wd.AttributePath, wd.Class, wd.Style), nil
	case *pages.CheckBox:
		return inputWidget("Pages$CheckBox", wd.Name, wd.Label, wd.AttributePath, wd.Class, wd.Style), nil
	case *pages.DatePicker:
		return inputWidget("Pages$DatePicker", wd.Name, wd.Label, wd.AttributePath, wd.Class, wd.Style), nil
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
