// SPDX-License-Identifier: Apache-2.0

// Pluggable widget .def.json extraction. Shared between the `mxcli widget
// init` CLI command and in-executor entry points (refresh catalog, etc.).
//
// A widget definition file (.def.json) tells the pluggable widget engine
// which MDL keywords route into which widget property keys. The file is
// derived from the widget's .mpk package (the React widget bundle that
// Mendix Studio Pro and the runtime use). Whenever mxcli is upgraded and
// learns to emit new fields (e.g. `objectLists` for engine-routed widgets
// like Accordion / Maps / PopupMenu), existing on-disk definitions can
// become stale. RefreshWidgetDefinitions handles that transparently:
// generate fresh content, compare byte-by-byte, overwrite when drifted.
package executor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// WidgetDefRefreshStats reports the outcome of a RefreshWidgetDefinitions
// call.
type WidgetDefRefreshStats struct {
	Extracted int // newly written defs (no prior file)
	Refreshed int // content drifted — overwrote stale def
	UpToDate  int // content matched — no write
	Skipped   int // skipped (built-in or unparseable mpk)
}

// RefreshWidgetDefinitions scans projectDir/widgets/ for .mpk files and
// (re)generates projectDir/.mxcli/widgets/<name>.def.json for each.
// Auto-refreshes definitions whose generated content has drifted (the case
// that triggers "unsupported widget type: group" after upgrading mxcli).
//
// projectPath is the path to the .mpr (or any file in its directory).
// force=true rewrites every .def.json unconditionally.
// If output is non-nil, per-widget changes are written with `+` (new) /
// `~` (refreshed) markers.
func RefreshWidgetDefinitions(projectPath string, force bool, output io.Writer) (WidgetDefRefreshStats, error) {
	projectDir := filepath.Dir(projectPath)
	widgetsDir := filepath.Join(projectDir, "widgets")
	outputDir := filepath.Join(projectDir, ".mxcli", "widgets")

	var stats WidgetDefRefreshStats

	matches, err := filepath.Glob(filepath.Join(widgetsDir, "*.mpk"))
	if err != nil {
		return stats, fmt.Errorf("failed to scan widgets directory: %w", err)
	}
	if len(matches) == 0 {
		return stats, nil
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return stats, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Built-in registry — definitions hand-crafted in sdk/widgets/definitions/
	// (COMBOBOX, GALLERY, DATAGRID, filters, …). Skip those when extracting
	// from the project; the built-in def overrides any .mpk-derived one.
	builtinRegistry, _ := NewWidgetRegistry()

	for _, mpkPath := range matches {
		mpkDef, err := mpk.ParseMPK(mpkPath)
		if err != nil {
			log.Printf("warning: skipping %s: %v", filepath.Base(mpkPath), err)
			stats.Skipped++
			continue
		}

		mdlName := DeriveMDLName(mpkDef.ID)
		filename := strings.ToLower(mdlName) + ".def.json"
		outPath := filepath.Join(outputDir, filename)

		if builtinRegistry != nil {
			if _, ok := builtinRegistry.GetByWidgetID(mpkDef.ID); ok {
				stats.Skipped++
				continue
			}
		}

		defJSON := GenerateDefJSON(mpkDef, mdlName)
		freshData, err := json.MarshalIndent(defJSON, "", "  ")
		if err != nil {
			log.Printf("warning: skipping %s: %v", mpkDef.ID, err)
			stats.Skipped++
			continue
		}
		freshData = append(freshData, '\n')

		existingData, existsErr := os.ReadFile(outPath)
		switch {
		case existsErr != nil:
			stats.Extracted++
		case bytes.Equal(existingData, freshData):
			if force {
				stats.Refreshed++
			} else {
				stats.UpToDate++
				continue
			}
		default:
			stats.Refreshed++
		}

		if err := os.WriteFile(outPath, freshData, 0644); err != nil {
			return stats, fmt.Errorf("failed to write %s: %w", outPath, err)
		}
		if output != nil {
			kind := "custom"
			if mpkDef.IsPluggable {
				kind = "pluggable"
			}
			marker := "+"
			if existsErr == nil {
				marker = "~"
			}
			fmt.Fprintf(output, "  %s %-12s %-20s %s\n", marker, kind, mdlName, mpkDef.ID)
		}
	}

	return stats, nil
}

// DeriveMDLName derives the uppercase MDL keyword name from a widget ID
// (e.g. "com.mendix.widget.web.accordion.Accordion" → "ACCORDION").
func DeriveMDLName(widgetID string) string {
	parts := strings.Split(widgetID, ".")
	name := parts[len(parts)-1]
	return strings.ToUpper(name)
}

// GenerateDefJSON creates a skeleton WidgetDefinition from an mpk.WidgetDefinition.
// Properties are handled explicitly from MDL via the engine's explicit property pass,
// so no propertyMappings or childSlots are generated for unknown types.
func GenerateDefJSON(mpkDef *mpk.WidgetDefinition, mdlName string) *WidgetDefinition {
	widgetKind := "custom"
	if mpkDef.IsPluggable {
		widgetKind = "pluggable"
	}
	def := &WidgetDefinition{
		WidgetID:        mpkDef.ID,
		MDLName:         mdlName,
		WidgetKind:      widgetKind,
		TemplateFile:    strings.ToLower(mdlName) + ".json",
		DefaultEditable: "Always",
	}

	// Generate property mappings and child slots from MPK property definitions.
	// Two passes: datasource first (association depends on entityContext set by datasource).
	var assocMappings []PropertyMapping
	for _, p := range mpkDef.Properties {
		// Object-list properties (e.g. Accordion `groups`, DataGrid `columns`)
		// are emitted as ObjectListMapping entries.
		if p.Type == "object" && p.IsList {
			def.ObjectLists = append(def.ObjectLists, makeObjectListMapping(p))
			continue
		}
		switch p.Type {
		case "widgets":
			container := strings.ToUpper(p.Key)
			if p.Key == "content" {
				container = "TEMPLATE"
			}
			def.ChildSlots = append(def.ChildSlots, ChildSlotMapping{
				PropertyKey:  p.Key,
				MDLContainer: container,
				Operation:    "widgets",
			})
		case "datasource":
			def.PropertyMappings = append(def.PropertyMappings, PropertyMapping{
				PropertyKey: p.Key,
				Source:      "DataSource",
				Operation:   "datasource",
			})
		case "attribute":
			def.PropertyMappings = append(def.PropertyMappings, PropertyMapping{
				PropertyKey: p.Key,
				Source:      "Attribute",
				Operation:   "attribute",
			})
		case "association":
			assocMappings = append(assocMappings, PropertyMapping{
				PropertyKey: p.Key,
				Source:      "Association",
				Operation:   "association",
			})
		case "selection":
			def.PropertyMappings = append(def.PropertyMappings, PropertyMapping{
				PropertyKey: p.Key,
				Source:      "Selection",
				Operation:   "selection",
				Default:     p.DefaultValue,
			})
		case "boolean", "integer", "decimal", "string", "enumeration":
			m := PropertyMapping{
				PropertyKey: p.Key,
				Operation:   "primitive",
			}
			if p.DefaultValue != "" {
				m.Value = p.DefaultValue
			}
			def.PropertyMappings = append(def.PropertyMappings, m)
		}
	}
	def.PropertyMappings = append(def.PropertyMappings, assocMappings...)

	return def
}

// makeObjectListMapping converts an MPK object-list PropertyDef (e.g. Accordion
// `groups`) into an ObjectListMapping. The MDL keyword is the singular form of
// the property key (groups → GROUP, basicItems → ITEM, series → SERIES,
// markers → MARKER).
func makeObjectListMapping(p mpk.PropertyDef) ObjectListMapping {
	mapping := ObjectListMapping{
		PropertyKey:  p.Key,
		MDLContainer: deriveObjectListKeyword(p.Key),
	}
	for _, child := range p.Children {
		if child.Type == "widgets" {
			mapping.ItemSlots = append(mapping.ItemSlots, ItemSlotMapping{
				PropertyKey:  child.Key,
				MDLContainer: strings.ToUpper(child.Key),
				Operation:    "widgets",
			})
			continue
		}
		op := operationForType(child.Type)
		if op == "" {
			continue
		}
		item := ItemPropertyMapping{
			PropertyKey: child.Key,
			Operation:   op,
		}
		switch op {
		case "attribute":
			item.Source = "Attribute"
		case "datasource":
			item.Source = "DataSource"
		case "association":
			item.Source = "Association"
		case "primitive":
			if child.DefaultValue != "" {
				item.Value = child.DefaultValue
			}
		}
		mapping.ItemProperties = append(mapping.ItemProperties, item)
	}
	return mapping
}

// deriveObjectListKeyword turns a property key like "groups" / "basicItems" /
// "series" / "markers" into an uppercase MDL keyword in the singular form.
func deriveObjectListKeyword(propertyKey string) string {
	overrides := map[string]string{
		"basicItems":     "ITEM",
		"customItems":    "CUSTOMITEM",
		"dynamicMarkers": "DYNAMICMARKER",
		"attributesList": "ATTR",
		"filterOptions":  "OPTION",
		"series":         "SERIES", // Latin singular == plural
	}
	if k, ok := overrides[propertyKey]; ok {
		return k
	}
	lower := strings.ToLower(propertyKey)
	singular := strings.TrimSuffix(lower, "s")
	return strings.ToUpper(singular)
}

// operationForType maps an MPK property type to the engine's operation name.
// Returns "" for unsupported types (which are skipped in object-list extraction).
func operationForType(t string) string {
	switch t {
	case "attribute":
		return "attribute"
	case "association":
		return "association"
	case "datasource":
		return "datasource"
	case "textTemplate":
		return "texttemplate"
	case "expression":
		return "expression"
	case "action":
		return "action"
	case "boolean", "integer", "decimal", "string", "enumeration":
		return "primitive"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Skill markdown generation
// ---------------------------------------------------------------------------

// RegenerateWidgetDocs scans projectDir/widgets/ for .mpk files and writes a
// per-widget .md skill file under .claude/skills/widgets/ (or
// .ai-context/skills/widgets/ when that directory exists). The docs combine
// human-readable info from the .mpk (descriptions, defaults) with the MDL
// keyword routing from the matching .def.json (object lists, child slots,
// MDL container keywords). Returns the number of files written.
func RegenerateWidgetDocs(projectPath string) (int, error) {
	projectDir := filepath.Dir(projectPath)
	widgetsDir := filepath.Join(projectDir, "widgets")
	defsDir := filepath.Join(projectDir, ".mxcli", "widgets")
	docsDir := filepath.Join(projectDir, ".claude", "skills", "widgets")
	if _, err := os.Stat(filepath.Join(projectDir, ".ai-context")); err == nil {
		docsDir = filepath.Join(projectDir, ".ai-context", "skills", "widgets")
	}

	matches, err := filepath.Glob(filepath.Join(widgetsDir, "*.mpk"))
	if err != nil {
		return 0, fmt.Errorf("failed to scan widgets directory: %w", err)
	}
	if len(matches) == 0 {
		return 0, nil
	}

	if err := os.MkdirAll(docsDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create docs directory: %w", err)
	}

	var generated int
	var indexEntries []string

	for _, mpkPath := range matches {
		mpkDef, err := mpk.ParseMPK(mpkPath)
		if err != nil {
			continue
		}

		mdlName := DeriveMDLName(mpkDef.ID)
		filename := strings.ToLower(mdlName) + ".md"
		outPath := filepath.Join(docsDir, filename)

		// Load the matching .def.json (may not exist for built-in widgets like
		// COMBOBOX / GALLERY — those have hand-crafted definitions in
		// sdk/widgets/definitions/ that we don't extract per-project).
		var def *WidgetDefinition
		defPath := filepath.Join(defsDir, strings.ToLower(mdlName)+".def.json")
		if data, err := os.ReadFile(defPath); err == nil {
			def = &WidgetDefinition{}
			if jsonErr := json.Unmarshal(data, def); jsonErr != nil {
				def = nil
			}
		}

		doc := widgetDocMarkdown(mpkDef, def, mdlName)
		if err := os.WriteFile(outPath, []byte(doc), 0644); err != nil {
			log.Printf("warning: failed to write %s: %v", filename, err)
			continue
		}

		kind := "CUSTOMWIDGET"
		if mpkDef.IsPluggable {
			kind = "PLUGGABLEWIDGET"
		}
		indexEntries = append(indexEntries, fmt.Sprintf("| `%s` | %s | `%s` | %s | %d |",
			kind, mdlName, mpkDef.ID, mpkDef.Name, len(mpkDef.Properties)))
		generated++
	}

	var indexBuf strings.Builder
	indexBuf.WriteString("# Available Widgets\n\n")
	indexBuf.WriteString("Auto-generated. See individual files for property details, child slots, and object lists.\n\n")
	indexBuf.WriteString("| Prefix | Name | Widget ID | Display Name | Props |\n")
	indexBuf.WriteString("|--------|------|-----------|--------------|-------|\n")
	for _, entry := range indexEntries {
		indexBuf.WriteString(entry)
		indexBuf.WriteString("\n")
	}
	indexBuf.WriteString("\n**Usage in MDL:**\n```sql\n")
	indexBuf.WriteString("-- React pluggable widgets\n")
	indexBuf.WriteString("PLUGGABLEWIDGET 'com.mendix.widget.custom.badge.Badge' badge1\n\n")
	indexBuf.WriteString("-- Legacy custom widgets\n")
	indexBuf.WriteString("CUSTOMWIDGET 'com.company.OldWidget' legacy1\n")
	indexBuf.WriteString("```\n")

	if err := os.WriteFile(filepath.Join(docsDir, "_index.md"), []byte(indexBuf.String()), 0644); err != nil {
		return generated, fmt.Errorf("failed to write index: %w", err)
	}

	return generated, nil
}

// widgetDocMarkdown produces the per-widget skill markdown. Combines mpkDef
// (for human descriptions, defaults, version) with def (for MDL keyword
// routing — object lists, child slots, property bindings). def may be nil for
// widgets without an extracted .def.json (e.g., hand-crafted built-ins).
func widgetDocMarkdown(mpkDef *mpk.WidgetDefinition, def *WidgetDefinition, mdlName string) string {
	var buf strings.Builder

	prefix := "CUSTOMWIDGET"
	if mpkDef.IsPluggable {
		prefix = "PLUGGABLEWIDGET"
	}

	buf.WriteString(fmt.Sprintf("# %s\n\n", mpkDef.Name))
	buf.WriteString(fmt.Sprintf("- **Widget ID:** `%s`\n", mpkDef.ID))
	buf.WriteString(fmt.Sprintf("- **Type:** %s\n", prefix))
	buf.WriteString(fmt.Sprintf("- **Version:** %s\n\n", mpkDef.Version))

	buf.WriteString("## MDL Example\n\n```sql\n")
	buf.WriteString(fmt.Sprintf("%s '%s' widget1", prefix, mpkDef.ID))
	if def != nil && (len(def.ChildSlots) > 0 || len(def.ObjectLists) > 0) {
		buf.WriteString(" {\n")
		for _, slot := range def.ChildSlots {
			buf.WriteString(fmt.Sprintf("  %s {\n    -- widgets for `%s`\n  }\n", strings.ToLower(slot.MDLContainer), slot.PropertyKey))
		}
		for _, ol := range def.ObjectLists {
			itemKw := strings.ToLower(ol.MDLContainer)
			buf.WriteString(fmt.Sprintf("  %s item1   -- one entry of `%s`\n", itemKw, ol.PropertyKey))
		}
		buf.WriteString("}\n")
	} else {
		buf.WriteString("\n")
	}
	buf.WriteString("```\n\n")

	if len(mpkDef.Properties) > 0 {
		buf.WriteString("## Properties\n\n")
		buf.WriteString("| Property | Type | Required | Default | Description |\n")
		buf.WriteString("|----------|------|----------|---------|-------------|\n")
		for _, prop := range mpkDef.Properties {
			if prop.IsSystem {
				continue
			}
			req := ""
			if prop.Required {
				req = "Yes"
			}
			desc := prop.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			buf.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s |\n",
				prop.Key, prop.Type, req, prop.DefaultValue, desc))
		}
		buf.WriteString("\n")
	}

	if def != nil && len(def.ChildSlots) > 0 {
		buf.WriteString("## Child Slots (curly-brace blocks)\n\n")
		buf.WriteString("| MDL keyword | Widget property |\n|-------------|----------------|\n")
		for _, s := range def.ChildSlots {
			buf.WriteString(fmt.Sprintf("| `%s` | `%s` |\n", strings.ToLower(s.MDLContainer), s.PropertyKey))
		}
		buf.WriteString("\n")
	}

	if def != nil && len(def.ObjectLists) > 0 {
		buf.WriteString("## Object Lists (repeating child entries)\n\n")
		for _, ol := range def.ObjectLists {
			buf.WriteString(fmt.Sprintf("### `%s` → property `%s`\n\n", strings.ToLower(ol.MDLContainer), ol.PropertyKey))
			if len(ol.ItemProperties) > 0 {
				buf.WriteString("Item properties:\n\n")
				buf.WriteString("| Property | Operation |\n|----------|-----------|\n")
				for _, ip := range ol.ItemProperties {
					buf.WriteString(fmt.Sprintf("| `%s` | %s |\n", ip.PropertyKey, ip.Operation))
				}
				buf.WriteString("\n")
			}
			if len(ol.ItemSlots) > 0 {
				buf.WriteString("Item child slots:\n\n")
				buf.WriteString("| MDL keyword | Widget property |\n|-------------|----------------|\n")
				for _, s := range ol.ItemSlots {
					buf.WriteString(fmt.Sprintf("| `%s` | `%s` |\n", strings.ToLower(s.MDLContainer), s.PropertyKey))
				}
				buf.WriteString("\n")
			}
		}
	}

	return buf.String()
}
