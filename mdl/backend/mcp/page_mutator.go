// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ALTER PAGE over MCP is a read-modify-write on pg's high-level content tree:
// pg_read_page loads the current widget tree, the mutator edits it in memory, and
// Save() writes it back with pg_write_page. This file implements the structural
// operations (INSERT / DROP / REPLACE widget, SET DataSource, SET Layout) plus the
// introspection the executor needs to build inserted widgets. Property-name
// translation (SET <widget> (Prop: value)) and the column/design/pluggable/
// variable operations are not yet mapped and return a clear error.

// OpenPageForMutation loads a page's pg content tree and returns a mutator.
func (b *Backend) OpenPageForMutation(unitID model.ID) (backend.PageMutator, error) {
	mod, page, err := b.resolvePageUnit(unitID)
	if err != nil {
		return nil, err
	}
	content, err := b.pgReadPage(mod, page)
	if err != nil {
		return nil, err
	}
	return &mcpPageMutator{backend: b, moduleName: mod, pageName: page, content: content}, nil
}

// resolvePageUnit maps a page unit ID to its module and page name (needed for the
// pg_* tools, which are name-addressed). Saved pages come from the local reader;
// pages created over MCP this session come from sessionPages.
func (b *Backend) resolvePageUnit(unitID model.ID) (moduleName, pageName string, err error) {
	for _, p := range b.sessionPages {
		if p.ID == unitID {
			mod, e := b.reader.GetModule(p.ContainerID)
			if e != nil {
				return "", "", e
			}
			return mod.Name, p.Name, nil
		}
	}
	p, e := b.reader.GetPage(unitID)
	if e != nil {
		return "", "", fmt.Errorf("resolve page %s for mutation: %w", unitID, e)
	}
	mod, e := b.reader.GetModule(p.ContainerID)
	if e != nil {
		return "", "", e
	}
	return mod.Name, p.Name, nil
}

type mcpPageMutator struct {
	backend    *Backend
	moduleName string
	pageName   string
	content    map[string]any
}

var _ backend.PageMutator = (*mcpPageMutator)(nil)

func (m *mcpPageMutator) ContainerType() backend.ContainerKind { return backend.ContainerPage }

// --- widget-tree navigation (generic over pg's container shapes) ---

// findWidget locates the widget with the given name anywhere in the tree and
// returns its containing map + key (so the slice can be rewritten) and index.
func findWidget(node any, name string) (parent map[string]any, key string, idx int, widget map[string]any, ok bool) {
	switch n := node.(type) {
	case map[string]any:
		// Stable key order so a tree with duplicate-named widgets (shouldn't
		// happen) resolves deterministically.
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if arr, isArr := n[k].([]any); isArr {
				for i, e := range arr {
					if em, isMap := e.(map[string]any); isMap && em["name"] == name {
						return n, k, i, em, true
					}
				}
			}
		}
		for _, k := range keys {
			if p, kk, ix, w, found := findWidget(n[k], name); found {
				return p, kk, ix, w, found
			}
		}
	case []any:
		for _, e := range n {
			if p, kk, ix, w, found := findWidget(e, name); found {
				return p, kk, ix, w, found
			}
		}
	}
	return nil, "", 0, nil, false
}

// walkWidgets calls fn for every widget map (a map carrying a "name") in the tree.
func walkWidgets(node any, fn func(map[string]any)) {
	switch n := node.(type) {
	case map[string]any:
		if _, hasName := n["name"]; hasName {
			fn(n)
		}
		for _, v := range n {
			walkWidgets(v, fn)
		}
	case []any:
		for _, e := range n {
			walkWidgets(e, fn)
		}
	}
}

func (m *mcpPageMutator) FindWidget(name string) bool {
	_, _, _, _, ok := findWidget(m.content, name)
	return ok
}

func (m *mcpPageMutator) WidgetScope() map[string]model.ID {
	scope := make(map[string]model.ID)
	walkWidgets(m.content, func(w map[string]any) {
		if name, _ := w["name"].(string); name != "" {
			scope[name] = model.ID("mcp~w~" + name)
		}
	})
	return scope
}

func (m *mcpPageMutator) ParamScope() (map[string]model.ID, map[string]string) {
	ids := make(map[string]model.ID)
	entities := make(map[string]string)
	if params, ok := m.content["parameters"].([]any); ok {
		for _, p := range params {
			pm, _ := p.(map[string]any)
			name, _ := pm["name"].(string)
			if name == "" {
				continue
			}
			ids[name] = model.ID("mcp~param~" + name)
			if ent, _ := pm["entity"].(string); ent != "" {
				entities[name] = ent
			}
		}
	}
	return ids, entities
}

// widgetEntity extracts the data-source entity bound to a widget (its own
// source), or "" if it has none / the source is non-database.
func widgetEntity(w map[string]any) string {
	if w == nil {
		return ""
	}
	// Pluggable widgets keep the source under object.datasource.
	if obj, ok := w["object"].(map[string]any); ok {
		if e := entityRefName(obj["datasource"]); e != "" {
			return e
		}
	}
	return entityRefName(w["dataSource"])
}

func entityRefName(ds any) string {
	dm, ok := ds.(map[string]any)
	if !ok {
		return ""
	}
	if ref, ok := dm["entityRef"].(map[string]any); ok {
		if e, _ := ref["entity"].(string); e != "" {
			return e
		}
	}
	return ""
}

// EnclosingEntityForChildren returns the widget's own source entity.
func (m *mcpPageMutator) EnclosingEntityForChildren(widgetRef string) string {
	_, _, _, w, ok := findWidget(m.content, widgetRef)
	if !ok {
		return ""
	}
	return widgetEntity(w)
}

// EnclosingEntity returns the entity context that surrounds a widget — the source
// entity of the nearest data-bearing ancestor.
func (m *mcpPageMutator) EnclosingEntity(widgetRef string) string {
	var search func(node any, current string) (string, bool)
	search = func(node any, current string) (string, bool) {
		switch n := node.(type) {
		case map[string]any:
			if n["name"] == widgetRef {
				return current, true
			}
			next := current
			if e := widgetEntity(n); e != "" {
				next = e
			}
			for _, v := range n {
				if r, found := search(v, next); found {
					return r, true
				}
			}
		case []any:
			for _, e := range n {
				if r, found := search(e, current); found {
					return r, true
				}
			}
		}
		return "", false
	}
	entity, _ := search(m.content, "")
	return entity
}

// --- structural mutations ---

func (m *mcpPageMutator) SetWidgetDataSource(widgetRef string, ds pages.DataSource) error {
	_, _, _, w, ok := findWidget(m.content, widgetRef)
	if !ok {
		return fmt.Errorf("widget %q not found", widgetRef)
	}
	if obj, isCustom := w["object"].(map[string]any); isCustom {
		src := customWidgetXPathSource(ds)
		if src == nil {
			return fmt.Errorf("data source %T is not supported for pluggable widget %q", ds, widgetRef)
		}
		obj["datasource"] = src
		return nil
	}
	var src map[string]any
	var err error
	switch w["$Type"] {
	case "Pages$ListView":
		src, err = mapListViewSource(ds)
	default:
		src, err = mapDataViewSource(ds)
	}
	if err != nil {
		return err
	}
	w["dataSource"] = src
	return nil
}

func (m *mcpPageMutator) InsertWidget(widgetRef string, columnRef string, position backend.InsertPosition, widgets []pages.Widget) error {
	if columnRef != "" {
		return fmt.Errorf("inserting into a column (%s.%s) is not yet supported by the MCP backend", widgetRef, columnRef)
	}
	parent, key, idx, _, ok := findWidget(m.content, widgetRef)
	if !ok {
		return fmt.Errorf("widget %q not found", widgetRef)
	}
	mapped, err := m.backend.mapPageWidgets(widgets)
	if err != nil {
		return err
	}
	arr, _ := parent[key].([]any)
	at := idx
	// The executor passes the AST token ("AFTER"/"BEFORE"), so compare case-insensitively.
	if strings.EqualFold(string(position), string(backend.InsertAfter)) {
		at = idx + 1
	}
	out := make([]any, 0, len(arr)+len(mapped))
	out = append(out, arr[:at]...)
	out = append(out, mapped...)
	out = append(out, arr[at:]...)
	parent[key] = out
	return nil
}

func (m *mcpPageMutator) DropWidget(refs []backend.WidgetRef) error {
	for _, ref := range refs {
		if ref.IsColumn() {
			return fmt.Errorf("dropping a column (%s) is not yet supported by the MCP backend", ref.Name())
		}
		parent, key, idx, _, ok := findWidget(m.content, ref.Widget)
		if !ok {
			return fmt.Errorf("widget %q not found", ref.Widget)
		}
		arr, _ := parent[key].([]any)
		parent[key] = append(arr[:idx], arr[idx+1:]...)
	}
	return nil
}

func (m *mcpPageMutator) ReplaceWidget(widgetRef string, columnRef string, widgets []pages.Widget) error {
	if columnRef != "" {
		return fmt.Errorf("replacing a column (%s.%s) is not yet supported by the MCP backend", widgetRef, columnRef)
	}
	parent, key, idx, _, ok := findWidget(m.content, widgetRef)
	if !ok {
		return fmt.Errorf("widget %q not found", widgetRef)
	}
	mapped, err := m.backend.mapPageWidgets(widgets)
	if err != nil {
		return err
	}
	arr, _ := parent[key].([]any)
	out := make([]any, 0, len(arr)+len(mapped)-1)
	out = append(out, arr[:idx]...)
	out = append(out, mapped...)
	out = append(out, arr[idx+1:]...)
	parent[key] = out
	return nil
}

func (m *mcpPageMutator) SetLayout(newLayout string, _ map[string]string) error {
	m.content["layout"] = newLayout
	return nil
}

func (m *mcpPageMutator) Save() error {
	return m.backend.pgWritePage(m.moduleName, m.pageName, m.content)
}

// --- not-yet-supported operations (clear errors, no silent drops) ---

// SetWidgetProperty maps an MDL widget property (SET <widget> (Prop: value)) onto
// its pg key. The MDL property names come from the executor (page builder); their
// pg equivalents are by convention: Class/Style live under the widget's
// appearance; Caption/Content/Label are `ct:`-prefixed client templates;
// ButtonStyle is normalized to pg's enum; Visible becomes a conditional-visibility
// expression. Values arrive unquoted and typed (string / number / bool).
func (m *mcpPageMutator) SetWidgetProperty(widgetRef, prop string, value any) error {
	_, _, _, w, ok := findWidget(m.content, widgetRef)
	if !ok {
		return fmt.Errorf("widget %q not found", widgetRef)
	}
	// Property names arrive verbatim from MDL (any case), so match case-insensitively.
	switch strings.ToLower(prop) {
	case "class", "style":
		ap, _ := w["appearance"].(map[string]any)
		if ap == nil {
			ap = pageAppearance("", "")
			w["appearance"] = ap
		}
		ap[strings.ToLower(prop)] = toStr(value) // "class" / "style"
	case "tabindex":
		w["tabIndex"] = toInt(value)
	case "caption":
		w["ct:caption"] = toStr(value)
	case "content":
		w["ct:content"] = toStr(value)
	case "label":
		w["ct:labelTemplate"] = toStr(value)
	case "rendermode":
		w["renderMode"] = toStr(value)
	case "buttonstyle":
		w["buttonStyle"] = buttonStyle(toStr(value))
	case "editable":
		w["editable"] = value
	case "name":
		w["name"] = toStr(value)
	case "visible":
		// Static visibility is expressed as a conditional-visibility expression
		// ("true"/"false"), since pg widgets carry no bare `visible` field.
		w["conditionalVisibilitySettings"] = map[string]any{
			"$Type":          "Pages$ConditionalVisibilitySettings",
			"expression":     visibleExpr(value),
			"conditions":     []any{},
			"moduleRoles":    []any{},
			"ignoreSecurity": false,
		}
	default:
		return fmt.Errorf("SET %s (%s: …): property %q is not yet supported by the MCP backend", widgetRef, prop, prop)
	}
	return nil
}

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func visibleExpr(v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "true"
		}
		return "false"
	case string:
		return x
	}
	return fmt.Sprint(v)
}

func (m *mcpPageMutator) SetColumnProperty(gridRef, columnRef, prop string, _ any) error {
	return fmt.Errorf("setting column property %s on %s.%s is not yet supported by the MCP backend", prop, gridRef, columnRef)
}

func (m *mcpPageMutator) InsertColumns(gridRef, afterColumnRef string, _ backend.InsertPosition, _ []*backend.DataGridColumnSpec) error {
	return fmt.Errorf("inserting columns into %s is not yet supported by the MCP backend", gridRef)
}

func (m *mcpPageMutator) ReplaceColumn(gridRef, columnRef string, _ []*backend.DataGridColumnSpec) error {
	return fmt.Errorf("replacing column %s.%s is not yet supported by the MCP backend", gridRef, columnRef)
}

func (m *mcpPageMutator) SetDesignProperty(widgetRef, key, _, _ string) error {
	return fmt.Errorf("setting design property %s on %s is not yet supported by the MCP backend", key, widgetRef)
}

func (m *mcpPageMutator) RemoveDesignProperty(widgetRef, key string) error {
	return fmt.Errorf("removing design property %s on %s is not yet supported by the MCP backend", key, widgetRef)
}

func (m *mcpPageMutator) ClearDesignProperties(widgetRef string) error {
	return fmt.Errorf("clearing design properties on %s is not yet supported by the MCP backend", widgetRef)
}

func (m *mcpPageMutator) SetPluggableProperty(widgetRef, propKey string, _ backend.PluggablePropertyOp, _ backend.PluggablePropertyContext) error {
	return fmt.Errorf("setting pluggable property %s on %s is not yet supported by the MCP backend", propKey, widgetRef)
}

func (m *mcpPageMutator) AddVariable(name, _, _ string) error {
	return fmt.Errorf("adding page variable %q is not yet supported by the MCP backend", name)
}

func (m *mcpPageMutator) DropVariable(name string) error {
	return fmt.Errorf("dropping page variable %q is not yet supported by the MCP backend", name)
}
