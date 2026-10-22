// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"
	"sync"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/modelsdk/widgets"
)

// What a check without a project knows about Data grid 2.
//
// A widget's properties come from its package (.mpk), which only a project
// has. The widgets mxcli embeds a definition for (combobox, gallery, the grid
// filters, image) are known without one. DATAGRID is not among them: its
// definition is generated from the project's own package, and embedding one
// would put it ahead of that package for every write
// (RefreshWidgetDefinitions skips an id the registry already holds).
//
// So without a project the widget had no definition at all, and the check fell
// back on the vocabulary of the built-in widgets — a union of what their
// builders read and what describe prints. Every Data grid 2 setting outside it
// was reported as "not recognized and will be silently dropped on write"
// (MDL-WIDGET07) although exec writes it, and an Expression-typed one was an
// ERROR (MDL-WIDGET42, "a plain-value property cannot hold the expression").
// `mxcli check` of a script `exec` takes, and `describe` prints, failed.
//
// What mxcli does have is the template it embeds for the widget, which declares
// every property. That is a baseline (the oldest supported Mendix version, see
// widget-engine.md), so it is evidence of what a key IS, never of what it is
// not: a key it declares is accepted, a key it does not is reported as one that
// cannot be verified without a project.

// embeddedWidgetSchema is the property list of a widget template mxcli embeds.
type embeddedWidgetSchema struct {
	// Title and Version are the template's: "Data grid 2", "11.6.3".
	Title   string
	Version string
	// Names are the declared property keys, in declaration order.
	Names []string
	// types maps a lower-cased key to its declared type ("Expression",
	// "TextTemplate", "Boolean", …).
	types map[string]string
	// returns maps a lower-cased Expression key to its declared return type
	// ("String", "Boolean", …); absent when the template declares none.
	returns map[string]string
	// slotAccepts are the widget types an item's widgets slot routes, by slot
	// key (a column's `filter` → textfilter, …); empty for a widget's own schema.
	slotAccepts map[string][]string
}

// widgetSlots lists the schema's widgets-typed properties — the slots a child
// widget goes in — with the widget types each one routes.
func (s *embeddedWidgetSchema) widgetSlots() []itemSlotHint {
	if s == nil {
		return nil
	}
	var out []itemSlotHint
	for _, n := range s.Names {
		if s.types[strings.ToLower(n)] == "Widgets" {
			out = append(out, itemSlotHint{Key: n, Accepts: s.slotAccepts[n]})
		}
	}
	return out
}

// declares reports whether the template declares key, case-insensitively — or
// the `<key>Params` companion of a text-template property, which carries its
// `{N}` parameters.
func (s *embeddedWidgetSchema) declares(key string) bool {
	if s == nil {
		return false
	}
	lower := strings.ToLower(key)
	if _, ok := s.types[lower]; ok {
		return true
	}
	if base, found := strings.CutSuffix(lower, "params"); found {
		return s.types[base] == "TextTemplate"
	}
	return false
}

// operation is the engine operation that writes key ("expression",
// "texttemplate", "attribute", "primitive"), or "" for a kind with none.
func (s *embeddedWidgetSchema) operation(key string) (string, bool) {
	if s == nil {
		return "", false
	}
	t, ok := s.types[strings.ToLower(key)]
	if !ok {
		return "", false
	}
	return operationForValueType(t), true
}

// pluggableWidgetIDOfKeyword is the id of the pluggable widget an MDL keyword
// names, whatever the Mendix version (DATAGRID → Data grid 2), or "".
func pluggableWidgetIDOfKeyword(keyword string) string {
	upper := strings.ToUpper(keyword)
	for _, m := range keywordDispatchTable {
		if m.Keyword != upper {
			continue
		}
		for _, b := range m.Bindings {
			if b.Kind == bindingKindPluggable && b.WidgetID != "" {
				return b.WidgetID
			}
		}
	}
	return ""
}

var embeddedSchemas sync.Map // widgetID + "/" + listKey -> *embeddedWidgetSchema (nil when there is none)

// embeddedSchemaFor returns the embedded schema for the keyword widget w —
// the grid's own properties for `datagrid`, a column's for a `column` inside
// one — or nil when w is not a keyword widget the registry lacks a definition
// for (everything else is judged exactly as before).
func embeddedSchemaFor(w, parent *ast.WidgetV3) *embeddedWidgetSchema {
	if w == nil || w.TypeIsGeneric || explicitWidgetID(w) != "" {
		return nil
	}
	if id := pluggableWidgetIDOfKeyword(w.Type); id != "" {
		return loadEmbeddedSchema(id, "")
	}
	if strings.EqualFold(w.Type, "column") && parent != nil && !parent.TypeIsGeneric && explicitWidgetID(parent) == "" {
		if id := pluggableWidgetIDOfKeyword(parent.Type); id != "" {
			return loadEmbeddedSchema(id, "columns")
		}
	}
	return nil
}

// loadEmbeddedSchema reads the property list of the embedded template for
// widgetID — or, with listKey, the item type of that object-list property.
func loadEmbeddedSchema(widgetID, listKey string) *embeddedWidgetSchema {
	cacheKey := widgetID + "/" + listKey
	if v, ok := embeddedSchemas.Load(cacheKey); ok {
		s, _ := v.(*embeddedWidgetSchema)
		return s
	}
	var schema *embeddedWidgetSchema
	if tmpl, err := widgets.GetTemplate(widgetID); err == nil && tmpl != nil {
		if propTypes := embeddedTemplatePropertyTypes(tmpl.Type, listKey); len(propTypes) > 0 {
			schema = &embeddedWidgetSchema{Title: tmpl.Name, Version: tmpl.Version, types: map[string]string{}, returns: map[string]string{}}
			if listKey != "" {
				schema.Title += " column"
				schema.slotAccepts = itemSlotAcceptedChildTypes[widgetID][listKey]
			}
			for _, pt := range propTypes {
				key, _ := pt["PropertyKey"].(string)
				if key == "" {
					continue
				}
				kind := ""
				if vt, ok := pt["ValueType"].(map[string]any); ok {
					kind, _ = vt["Type"].(string)
					if rt, ok := vt["ReturnType"].(map[string]any); ok && kind == "Expression" {
						if ret, _ := rt["Type"].(string); ret != "" {
							schema.returns[strings.ToLower(key)] = ret
						}
					}
				}
				schema.Names = append(schema.Names, key)
				schema.types[strings.ToLower(key)] = kind
			}
			sort.Strings(schema.Names)
		}
	}
	embeddedSchemas.Store(cacheKey, schema)
	return schema
}

// embeddedTemplatePropertyTypes lists the property types of a template's widget type —
// or of the item type of its object-list property listKey.
func embeddedTemplatePropertyTypes(widgetType map[string]any, listKey string) []map[string]any {
	objType, _ := widgetType["ObjectType"].(map[string]any)
	list := embeddedPropertyTypeMaps(objType)
	if listKey == "" {
		return list
	}
	for _, pt := range list {
		if key, _ := pt["PropertyKey"].(string); key != listKey {
			continue
		}
		vt, _ := pt["ValueType"].(map[string]any)
		itemType, _ := vt["ObjectType"].(map[string]any)
		return embeddedPropertyTypeMaps(itemType)
	}
	return nil
}

// embeddedPropertyTypeMaps is an object type's PropertyTypes as maps; the typed-array
// marker that opens the array is not one.
func embeddedPropertyTypeMaps(objType map[string]any) []map[string]any {
	var out []map[string]any
	if objType == nil {
		return nil
	}
	arr, _ := objType["PropertyTypes"].([]any)
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
