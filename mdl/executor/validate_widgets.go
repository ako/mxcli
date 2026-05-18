// SPDX-License-Identifier: Apache-2.0

// Widget property validation for `mxcli check`. Walks CREATE PAGE / CREATE
// SNIPPET / ALTER PAGE statements, finds pluggable widget AST nodes, and
// verifies that each property key the user wrote matches a known property
// in the widget's .def.json. Catches the most common authoring mistake:
// typos like `expanBehavior` instead of `expandBehavior`.
//
// Requires either built-in registry alone (no project context) or the
// project's .mxcli/widgets/*.def.json files (richer coverage) loaded via
// LoadUserDefinitions.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// universalWidgetProperties are AST keys that the visitor sets on every
// widget regardless of type. Any widget definition implicitly accepts them
// even though they don't appear in propertyMappings.
var universalWidgetProperties = map[string]bool{
	"widgettype":            true, // set by the pluggablewidget / customwidget grammar
	"class":                 true, // Forms$Appearance.Class
	"style":                 true, // Forms$Appearance.Style
	"designproperties":      true, // Forms$Appearance.DesignProperties
	"visible":               true,
	"editable":              true,
	"conditionalvisibility": true,
	"conditionaleditability": true,
}

// ValidateWidgetProperties walks pluggable widget AST nodes in the program
// and flags property keys that the widget definition doesn't recognize.
// projectPath, if non-empty, loads project-level .def.json files so
// project-installed widgets get validated too; otherwise only built-in
// definitions are consulted.
func ValidateWidgetProperties(prog *ast.Program, projectPath string) []linter.Violation {
	registry := LoadWidgetRegistry(projectPath)
	if registry == nil {
		return nil
	}
	var violations []linter.Violation
	for _, stmt := range prog.Statements {
		violations = append(violations, ValidateWidgetPropertiesForStatement(stmt, registry)...)
	}
	return violations
}

// LoadWidgetRegistry returns a widget registry loaded with both the built-in
// definitions and (when projectPath is non-empty) project-level .def.json
// files. Returns nil if registry initialization fails. The LSP uses this to
// load the registry once per session rather than per validation pass.
func LoadWidgetRegistry(projectPath string) *WidgetRegistry {
	registry, err := NewWidgetRegistry()
	if err != nil || registry == nil {
		return nil
	}
	if projectPath != "" {
		_ = registry.LoadUserDefinitions(projectPath)
	}
	return registry
}

// ValidateWidgetPropertiesForStatement runs widget property validation on a
// single statement using a pre-loaded registry. Returns no violations for
// statements that don't carry pluggable widgets (everything except
// CreatePageStmtV3, CreateSnippetStmtV3, AlterPageStmt).
func ValidateWidgetPropertiesForStatement(stmt ast.Statement, registry *WidgetRegistry) []linter.Violation {
	if registry == nil {
		return nil
	}
	switch s := stmt.(type) {
	case *ast.CreatePageStmtV3:
		return validateWidgetTree(s.Widgets, registry, "page "+s.Name.String())
	case *ast.CreateSnippetStmtV3:
		return validateWidgetTree(s.Widgets, registry, "snippet "+s.Name.String())
	case *ast.AlterPageStmt:
		var out []linter.Violation
		for _, op := range s.Operations {
			switch o := op.(type) {
			case *ast.InsertWidgetOp:
				out = append(out, validateWidgetTree(o.Widgets, registry, "alter "+s.PageName.String())...)
			case *ast.ReplaceWidgetOp:
				out = append(out, validateWidgetTree(o.NewWidgets, registry, "alter "+s.PageName.String())...)
			}
		}
		return out
	}
	return nil
}

// validateWidgetTree recursively walks the AST widget tree and validates
// pluggable widgets it encounters.
func validateWidgetTree(widgets []*ast.WidgetV3, registry *WidgetRegistry, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, w := range widgets {
		if w == nil {
			continue
		}
		out = append(out, validatePluggableWidgetProperties(w, registry, locationPrefix)...)
		if len(w.Children) > 0 {
			out = append(out, validateWidgetTree(w.Children, registry, locationPrefix)...)
		}
	}
	return out
}

// validatePluggableWidgetProperties checks every AST property key on a
// pluggable widget against the widget's def.json. Non-pluggable widgets are
// skipped (those go through the static builder which already validates props).
func validatePluggableWidgetProperties(w *ast.WidgetV3, registry *WidgetRegistry, locationPrefix string) []linter.Violation {
	def := lookupWidgetDef(w, registry)
	if def == nil {
		return nil
	}
	allowed, knownKeys := allowedWidgetProperties(def)

	var out []linter.Violation
	for key := range w.Properties {
		lower := strings.ToLower(key)
		if allowed[lower] {
			continue
		}
		suggestion := nearestKey(key, knownKeys)
		hint := ""
		if suggestion != "" {
			hint = fmt.Sprintf(" — did you mean `%s`?", suggestion)
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET01",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` (%s) has no property `%s`%s",
				locationPrefix, w.Name, def.MDLName, key, hint,
			),
		})
	}
	return out
}

// lookupWidgetDef finds the WidgetDefinition for an AST widget node.
// Tries `Properties["WidgetType"]` (set when MDL uses the explicit
// `pluggablewidget 'widget.id'` form) and then the registry's MDL-name
// lookup (when MDL uses a keyword like `accordion` or `combobox`).
// Returns nil for static built-in keywords (textbox, dataview, etc.)
// that don't go through the pluggable engine.
func lookupWidgetDef(w *ast.WidgetV3, registry *WidgetRegistry) *WidgetDefinition {
	if w == nil {
		return nil
	}
	if id, ok := w.Properties["WidgetType"].(string); ok && id != "" {
		if def, ok := registry.GetByWidgetID(id); ok {
			return def
		}
	}
	if w.Type == "" {
		return nil
	}
	if def, ok := registry.Get(strings.ToUpper(w.Type)); ok {
		return def
	}
	return nil
}

// allowedWidgetProperties returns the set of property keys (lowercased) that
// the widget definition recognizes — propertyMappings + childSlots +
// objectLists + mode-specific mappings + universal infrastructure keys.
// Also returns the same set as a slice in original case (for "did you mean"
// suggestions).
func allowedWidgetProperties(def *WidgetDefinition) (map[string]bool, []string) {
	allowed := make(map[string]bool, 32)
	var keys []string
	add := func(k string) {
		if k == "" {
			return
		}
		l := strings.ToLower(k)
		if !allowed[l] {
			allowed[l] = true
			keys = append(keys, k)
		}
	}

	for k := range universalWidgetProperties {
		allowed[k] = true
	}

	for _, m := range def.PropertyMappings {
		add(m.PropertyKey)
		add(m.Source)
	}
	for _, m := range def.ChildSlots {
		add(m.PropertyKey)
	}
	for _, m := range def.ObjectLists {
		add(m.PropertyKey)
	}
	for _, mode := range def.Modes {
		for _, m := range mode.PropertyMappings {
			add(m.PropertyKey)
			add(m.Source)
		}
		for _, m := range mode.ChildSlots {
			add(m.PropertyKey)
		}
	}

	sort.Strings(keys)
	return allowed, keys
}

// nearestKey returns the candidate key nearest to `input` by Levenshtein
// distance, when the distance is small enough (≤ 3 or ≤ ceil(len/3)).
// Returns "" when no candidate is close enough.
func nearestKey(input string, candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	lower := strings.ToLower(input)
	best := ""
	bestDist := -1
	limit := 3
	if len(input)/3 > limit {
		limit = len(input) / 3
	}
	for _, c := range candidates {
		d := levenshtein(lower, strings.ToLower(c))
		if d > limit {
			continue
		}
		if bestDist == -1 || d < bestDist {
			best = c
			bestDist = d
		}
	}
	return best
}

// levenshtein returns the edit distance between two strings.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
