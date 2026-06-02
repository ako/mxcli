// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ============================================================================
// SHOW DESIGN PROPERTIES
// ============================================================================

func execShowDesignProperties(ctx *ExecContext, s *ast.ShowDesignPropertiesStmt) error {

	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if ctx.MprPath == "" {
		return mdlerrors.NewValidationf("project path unavailable — connected via mock backend without MprPath")
	}

	projectDir := filepath.Dir(ctx.MprPath)
	registry, err := loadThemeRegistry(projectDir)
	if err != nil {
		return mdlerrors.NewBackend("load theme registry", err)
	}

	if len(registry.WidgetProperties) == 0 {
		fmt.Fprintln(ctx.Output, "No design properties found. Check that themesource/*/web/design-properties.json exists in the project directory.")
		return nil
	}

	if s.WidgetType != "" {
		// Show properties for a specific widget type
		dpKey := resolveDesignPropsKey(s.WidgetType)
		props := registry.GetPropertiesForWidget(dpKey)
		if len(props) == 0 {
			fmt.Fprintf(ctx.Output, "No design properties found for widget type %s (%s)\n", s.WidgetType, dpKey)
			return nil
		}
		fmt.Fprintf(ctx.Output, "Design Properties for %s:\n\n", s.WidgetType)
		printDesignProperties(ctx, registry, dpKey)
	} else {
		// Show all widget types and their properties
		keys := make([]string, 0, len(registry.WidgetProperties))
		for k := range registry.WidgetProperties {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			props := registry.WidgetProperties[key]
			if len(props) == 0 {
				continue
			}
			fmt.Fprintf(ctx.Output, "=== %s ===\n", key)
			for _, p := range props {
				printOneProperty(ctx, p)
			}
			fmt.Fprintln(ctx.Output)
		}
	}

	return nil
}

// printDesignProperties prints properties for a widget type, showing inherited "Widget" props separately.
func printDesignProperties(ctx *ExecContext, registry *ThemeRegistry, dpKey string) {
	// Print inherited Widget properties
	if widgetProps, ok := registry.WidgetProperties["Widget"]; ok && len(widgetProps) > 0 {
		fmt.Fprintf(ctx.Output, "From: Widget (inherited)\n")
		for _, p := range widgetProps {
			printOneProperty(ctx, p)
		}
	}

	// Print type-specific properties
	if dpKey != "Widget" {
		if typeProps, ok := registry.WidgetProperties[dpKey]; ok && len(typeProps) > 0 {
			fmt.Fprintf(ctx.Output, "From: %s\n", dpKey)
			for _, p := range typeProps {
				printOneProperty(ctx, p)
			}
		}
	}
}

// printOneProperty prints a single design property in a readable format.
func printOneProperty(ctx *ExecContext, p ThemeProperty) {
	switch p.Type {
	case "Toggle":
		fmt.Fprintf(ctx.Output, "  %-24s Toggle      class: %s\n", p.Name, p.Class)
	case "Dropdown", "ColorPicker", "ToggleButtonGroup":
		options := make([]string, 0, len(p.Options))
		for _, o := range p.Options {
			options = append(options, o.Name)
		}
		fmt.Fprintf(ctx.Output, "  %-24s %-11s [%s]\n", p.Name, p.Type, strings.Join(options, ", "))
	default:
		fmt.Fprintf(ctx.Output, "  %-24s %s\n", p.Name, p.Type)
	}
}

// ============================================================================
// DESCRIBE STYLING
// ============================================================================

func execDescribeStyling(ctx *ExecContext, s *ast.DescribeStylingStmt) error {

	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	var rawWidgets []rawWidget

	// ContainerType is stored uppercase ("PAGE"/"SNIPPET") by the visitor;
	// normalize so comparisons are case-insensitive.
	containerType := strings.ToLower(s.ContainerType)

	if containerType == "page" {
		// Find page
		allPages, err := ctx.Backend.ListPages()
		if err != nil {
			return mdlerrors.NewBackend("list pages", err)
		}

		var foundPage *pages.Page
		for _, p := range allPages {
			modID := h.FindModuleID(p.ContainerID)
			modName := h.GetModuleName(modID)
			if p.Name == s.ContainerName.Name && (s.ContainerName.Module == "" || modName == s.ContainerName.Module) {
				foundPage = p
				break
			}
		}
		if foundPage == nil {
			return mdlerrors.NewNotFound("page", s.ContainerName.String())
		}
		rawWidgets = getPageWidgetsFromRaw(ctx, foundPage.ID)
	} else if containerType == "snippet" {
		// Find snippet
		allSnippets, err := ctx.Backend.ListSnippets()
		if err != nil {
			return mdlerrors.NewBackend("list snippets", err)
		}

		var foundSnippet *pages.Snippet
		for _, sn := range allSnippets {
			modID := h.FindModuleID(sn.ContainerID)
			modName := h.GetModuleName(modID)
			if sn.Name == s.ContainerName.Name && (s.ContainerName.Module == "" || modName == s.ContainerName.Module) {
				foundSnippet = sn
				break
			}
		}
		if foundSnippet == nil {
			return mdlerrors.NewNotFound("snippet", s.ContainerName.String())
		}
		rawWidgets = getSnippetWidgetsFromRaw(ctx, foundSnippet.ID)
	}

	if len(rawWidgets) == 0 {
		fmt.Fprintf(ctx.Output, "No widgets found in %s %s\n", containerType, s.ContainerName.String())
		return nil
	}

	// Collect styled widgets
	styledWidgets := collectStyledWidgets(rawWidgets, s.WidgetName)

	if len(styledWidgets) == 0 {
		if s.WidgetName != "" {
			return mdlerrors.NewNotFoundMsg("widget", s.WidgetName, fmt.Sprintf("widget %q not found in %s %s", s.WidgetName, containerType, s.ContainerName.String()))
		}
		fmt.Fprintf(ctx.Output, "No styled widgets found in %s %s\n", containerType, s.ContainerName.String())
		return nil
	}

	// Output
	for i, w := range styledWidgets {
		if i > 0 {
			fmt.Fprintln(ctx.Output)
		}
		displayName := getWidgetDisplayName(w.Type)
		fmt.Fprintf(ctx.Output, "widget %s (%s)\n", w.Name, displayName)
		if w.Class != "" {
			fmt.Fprintf(ctx.Output, "  Class: '%s'\n", w.Class)
		}
		if w.Style != "" {
			fmt.Fprintf(ctx.Output, "  Style: '%s'\n", w.Style)
		}
		if len(w.DesignProperties) > 0 {
			fmt.Fprintf(ctx.Output, "  DesignProperties: [")
			for j, dp := range w.DesignProperties {
				if j > 0 {
					fmt.Fprint(ctx.Output, ", ")
				}
				if dp.ValueType == "toggle" {
					fmt.Fprintf(ctx.Output, "'%s': on", dp.Key)
				} else {
					fmt.Fprintf(ctx.Output, "'%s': '%s'", dp.Key, dp.Option)
				}
			}
			fmt.Fprintln(ctx.Output, "]")
		}
	}

	return nil
}

// collectStyledWidgets walks rawWidget tree and collects widgets that have styling.
// If widgetName is set, only returns the widget matching that name.
func collectStyledWidgets(widgets []rawWidget, widgetName string) []rawWidget {
	var result []rawWidget
	var walk func(ws []rawWidget)
	walk = func(ws []rawWidget) {
		for _, w := range ws {
			if widgetName != "" {
				// Looking for specific widget
				if w.Name == widgetName {
					result = append(result, w)
					return // Found it
				}
			} else {
				// Collect all widgets with any styling
				if w.Class != "" || w.Style != "" || len(w.DesignProperties) > 0 {
					result = append(result, w)
				}
			}
			// Walk children
			walk(w.Children)
			// Walk rows (for LayoutGrid)
			for _, row := range w.Rows {
				for _, col := range row.Columns {
					walk(col.Widgets)
				}
			}
			// Walk filter/controlbar widgets
			walk(w.FilterWidgets)
			walk(w.ControlBar)
		}
	}
	walk(widgets)
	return result
}

// ============================================================================
// ALTER STYLING
// ============================================================================

func execAlterStyling(ctx *ExecContext, s *ast.AlterStylingStmt) error {

	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// ContainerType is stored uppercase ("PAGE"/"SNIPPET") by the visitor;
	// normalize so comparisons are case-insensitive.
	containerType := strings.ToLower(s.ContainerType)

	var unitID model.ID
	switch containerType {
	case "page":
		page, err := findPageByName(ctx, s.ContainerName, h)
		if err != nil {
			return err
		}
		unitID = page.ID
	case "snippet":
		snippet, _, err := findSnippetByName(ctx, s.ContainerName, h)
		if err != nil {
			return err
		}
		unitID = snippet.ID
	default:
		return mdlerrors.NewUnsupported("unsupported container type: " + s.ContainerType)
	}

	// Open the unit for mutation via the backend (mutator pattern). This works
	// uniformly on pages/snippets created in Studio Pro and by the MDL builder.
	mutator, err := ctx.Backend.OpenPageForMutation(unitID)
	if err != nil {
		return mdlerrors.NewBackend("open "+containerType+" for mutation", err)
	}

	if !mutator.FindWidget(s.WidgetName) {
		return mdlerrors.NewNotFoundMsg("widget", s.WidgetName,
			fmt.Sprintf("widget %q not found in %s %s", s.WidgetName, containerType, s.ContainerName.String()))
	}

	if err := applyStylingMutator(mutator, s); err != nil {
		return mdlerrors.NewBackend("alter styling", err)
	}

	if err := mutator.Save(); err != nil {
		return mdlerrors.NewBackend("save "+containerType, err)
	}

	fmt.Fprintf(ctx.Output, "Updated styling on widget %q in %s %s\n", s.WidgetName, containerType, s.ContainerName.String())
	return nil
}

// applyStylingMutator applies the ALTER STYLING assignments through the page
// mutator. CLEAR DESIGN PROPERTIES is applied first, then each assignment in order.
func applyStylingMutator(mutator backend.PageMutator, s *ast.AlterStylingStmt) error {
	if s.ClearDesignProps {
		if err := mutator.ClearDesignProperties(s.WidgetName); err != nil {
			return err
		}
	}

	for _, a := range s.Assignments {
		switch a.Property {
		case "Class", "Style":
			if err := mutator.SetWidgetProperty(s.WidgetName, a.Property, a.Value); err != nil {
				return err
			}
		default:
			// Design property assignment.
			switch {
			case a.IsToggle && a.ToggleOn:
				if err := mutator.SetDesignProperty(s.WidgetName, a.Property, "toggle", ""); err != nil {
					return err
				}
			case a.IsToggle && !a.ToggleOn:
				if err := mutator.RemoveDesignProperty(s.WidgetName, a.Property); err != nil {
					return err
				}
			default:
				if err := mutator.SetDesignProperty(s.WidgetName, a.Property, "option", a.Value); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// findPageByName looks up a page by qualified name.
func findPageByName(ctx *ExecContext, name ast.QualifiedName, h *ContainerHierarchy) (*pages.Page, error) {

	allPages, err := ctx.Backend.ListPages()
	if err != nil {
		return nil, mdlerrors.NewBackend("list pages", err)
	}
	for _, p := range allPages {
		modID := h.FindModuleID(p.ContainerID)
		modName := h.GetModuleName(modID)
		if p.Name == name.Name && (name.Module == "" || modName == name.Module) {
			return p, nil
		}
	}
	return nil, mdlerrors.NewNotFound("page", name.String())
}

// findSnippetByName looks up a snippet by qualified name.
func findSnippetByName(ctx *ExecContext, name ast.QualifiedName, h *ContainerHierarchy) (*pages.Snippet, model.ID, error) {

	allSnippets, err := ctx.Backend.ListSnippets()
	if err != nil {
		return nil, "", mdlerrors.NewBackend("list snippets", err)
	}
	for _, s := range allSnippets {
		modID := h.FindModuleID(s.ContainerID)
		modName := h.GetModuleName(modID)
		if s.Name == name.Name && (name.Module == "" || modName == name.Module) {
			return s, modID, nil
		}
	}
	return nil, "", mdlerrors.NewNotFound("snippet", name.String())
}
