// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"

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
	default:
		return nil, fmt.Errorf("page widget type %s is not yet supported by the MCP backend", w.GetTypeName())
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
