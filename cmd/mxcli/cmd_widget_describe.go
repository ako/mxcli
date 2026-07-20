// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/types"
	mwidgets "github.com/mendixlabs/mxcli/modelsdk/widgets"
	mmpk "github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
	"github.com/spf13/cobra"
)

var widgetDescribeCmd = &cobra.Command{
	Use:   "describe <widget>",
	Short: "Inspect a pluggable widget's discovered format",
	Long: `Show the format mxcli has discovered for a pluggable widget: its expected
properties (key, type, caption, category, required, default, enum options) and the
dynamic property rules (which properties the widget's editor hides under which
configuration) lifted from the widget package's editorConfig.

The widget can be named by its MDL keyword (e.g. COMBOBOX, DATAGRID2) or its full
widget id (e.g. com.mendix.widget.web.combobox.Combobox).

With -p, the properties and dynamic rules come from the widget package actually
installed in the project (widgets/*.mpk) — the version-accurate "discovered" format,
including marketplace widgets mxcli has no built-in knowledge of. Without -p, they
come from mxcli's embedded template for that widget.`,
	Example: `  mxcli widget describe COMBOBOX
  mxcli widget describe DATAGRID2 -p app.mpr
  mxcli widget describe com.mendix.widget.web.combobox.Combobox -p app.mpr --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runWidgetDescribe,
}

func init() {
	widgetDescribeCmd.Flags().StringP("project", "p", "", "Path to .mpr project file (use the project's installed widget version)")
	widgetDescribeCmd.Flags().String("format", "text", "Output format: text or json")
	widgetCmd.AddCommand(widgetDescribeCmd)
}

// describedProperty is one property of a widget's discovered format.
type describedProperty struct {
	Key      string              `json:"key"`
	Type     string              `json:"type"`
	Caption  string              `json:"caption,omitempty"`
	Category string              `json:"category,omitempty"`
	Required bool                `json:"required"`
	Default  string              `json:"default,omitempty"`
	System   bool                `json:"system,omitempty"`
	Enum     []string            `json:"enum,omitempty"`
	Children []describedProperty `json:"children,omitempty"`
}

// describedRule is one dynamic (visibility) rule of a widget's discovered format.
type describedRule struct {
	Property   string `json:"property"`
	HiddenWhen string `json:"hiddenWhen"`
}

// widgetDescription is the full inspection result (also the JSON shape).
type widgetDescription struct {
	WidgetID     string              `json:"widgetId"`
	MDLName      string              `json:"mdlName,omitempty"`
	Name         string              `json:"name,omitempty"`
	Version      string              `json:"version,omitempty"`
	Source       string              `json:"source"` // "project .mpk" | "embedded template"
	Kind         string              `json:"kind,omitempty"`
	Properties   []describedProperty `json:"properties"`
	Rules        []describedRule     `json:"dynamicRules"`
	RuleCoverage string              `json:"ruleCoverage,omitempty"`
}

func runWidgetDescribe(cmd *cobra.Command, args []string) error {
	arg := args[0]
	projectPath, _ := cmd.Flags().GetString("project")
	format, _ := cmd.Flags().GetString("format")

	registry, err := executor.NewWidgetRegistry()
	if err != nil {
		return fmt.Errorf("failed to create widget registry: %w", err)
	}
	if projectPath != "" {
		_ = registry.LoadUserDefinitions(projectPath)
	}

	// Resolve the target widget id + optional built-in definition.
	widgetID, def := resolveWidgetTarget(registry, arg)
	if widgetID == "" {
		return widgetNotFoundError(registry, arg)
	}

	desc := widgetDescription{WidgetID: widgetID}
	if def != nil {
		desc.MDLName = def.MDLName
		desc.Kind = def.WidgetKind
	}
	if desc.Kind == "" {
		desc.Kind = "pluggable"
	}

	// Properties + version: prefer the project's installed .mpk (version-accurate,
	// includes marketplace widgets); else fall back to mxcli's embedded template.
	if projectPath != "" {
		if dir := projectDirOf(projectPath); dir != "" {
			if mpkPath, ferr := mmpk.FindMPK(dir, widgetID); ferr == nil && mpkPath != "" {
				if wd, perr := mmpk.ParseMPKForWidget(mpkPath, widgetID); perr == nil && wd != nil {
					desc.Name = wd.Name
					desc.Version = wd.Version
					desc.Source = "project .mpk"
					desc.Properties = propsFromMPK(wd)
					desc.Rules, desc.RuleCoverage = rulesFromProject(mpkPath, widgetID)
				}
			}
		}
	}
	if desc.Source == "" {
		// Embedded template fallback.
		tmpl, terr := mwidgets.GetTemplate(widgetID)
		if terr != nil || tmpl == nil {
			return fmt.Errorf("no installed .mpk and no embedded template for %q — try -p <project> to inspect a project widget", arg)
		}
		desc.Name = tmpl.Name
		desc.Version = tmpl.Version
		desc.Source = "embedded template"
		desc.Properties = propsFromTemplate(tmpl.Type)
		if def != nil {
			desc.Rules = rulesFromDef(def.PropertyVisibility)
		}
	}

	if strings.EqualFold(format, "json") {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(desc)
	}
	printWidgetDescription(cmd, desc)
	return nil
}

// resolveWidgetTarget maps a CLI argument (MDL keyword or widget id) to a widget id
// and, when known, the built-in WidgetDefinition. A dotted argument is treated as a
// widget id directly.
func resolveWidgetTarget(registry *executor.WidgetRegistry, arg string) (string, *executor.WidgetDefinition) {
	if strings.Contains(arg, ".") {
		if def, ok := registry.GetByWidgetID(arg); ok {
			return arg, def
		}
		return arg, nil // unknown to the registry, but a valid id to look up in the project
	}
	upper := strings.ToUpper(arg)
	if def, ok := registry.Get(upper); ok {
		return def.WidgetID, def
	}
	// Well-known widgets that are special-cased in the executor (no .def.json in the
	// registry) but that users still name by keyword.
	if id, ok := builtinWidgetAliases[upper]; ok {
		def, _ := registry.GetByWidgetID(id)
		return id, def
	}
	return "", nil
}

// builtinWidgetAliases maps MDL keywords for executor-special-cased widgets (which
// have no .def.json registry entry) to their widget ids, so `widget describe` can
// resolve them by the same friendly names users write in MDL.
var builtinWidgetAliases = map[string]string{
	"DATAGRID":  "com.mendix.widget.web.datagrid.Datagrid",
	"DATAGRID2": "com.mendix.widget.web.datagrid.Datagrid",
}

// widgetNotFoundError builds a helpful error listing the known MDL names.
func widgetNotFoundError(registry *executor.WidgetRegistry, arg string) error {
	var names []string
	for _, d := range registry.All() {
		if d.MDLName != "" {
			names = append(names, d.MDLName)
		}
	}
	for alias := range builtinWidgetAliases {
		names = append(names, strings.ToLower(alias))
	}
	sort.Strings(names)
	return fmt.Errorf("unknown widget %q — use an MDL keyword (%s) or a full widget id (com.mendix.widget…). Run `mxcli widget list` to see all",
		arg, strings.Join(names, ", "))
}

// projectDirOf returns the directory containing widgets/ for a project path
// (accepts either the .mpr file or its directory).
func projectDirOf(projectPath string) string {
	if strings.EqualFold(filepath.Ext(projectPath), ".mpr") {
		return filepath.Dir(projectPath)
	}
	return projectPath
}

// propsFromMPK builds described properties from a parsed .mpk definition, in the
// widget's declared order (regular + system interleaved).
func propsFromMPK(wd *mmpk.WidgetDefinition) []describedProperty {
	order := wd.AllTopLevel
	if len(order) == 0 {
		order = wd.Properties
	}
	out := make([]describedProperty, 0, len(order))
	for _, p := range order {
		out = append(out, describedPropFromMPK(p))
	}
	return out
}

func describedPropFromMPK(p mmpk.PropertyDef) describedProperty {
	dp := describedProperty{
		Key:      p.Key,
		Type:     p.Type,
		Caption:  p.Caption,
		Category: p.Category,
		Required: p.Required,
		Default:  p.DefaultValue,
		System:   p.IsSystem,
	}
	if dp.System && dp.Type == "" {
		dp.Type = "system"
	}
	for _, ev := range p.EnumValues {
		dp.Enum = append(dp.Enum, ev.Key)
	}
	for _, c := range p.Children {
		dp.Children = append(dp.Children, describedPropFromMPK(c))
	}
	return dp
}

// propsFromTemplate walks an embedded template's Type map (ObjectType.PropertyTypes)
// to build described properties. Used when no project .mpk is available.
func propsFromTemplate(typ map[string]any) []describedProperty {
	objType, _ := typ["ObjectType"].(map[string]any)
	pts, _ := objType["PropertyTypes"].([]any)
	var out []describedProperty
	for _, pt := range pts {
		m, ok := pt.(map[string]any)
		if !ok {
			continue // leading array marker
		}
		out = append(out, describedPropFromTemplate(m))
	}
	return out
}

func describedPropFromTemplate(m map[string]any) describedProperty {
	dp := describedProperty{
		Key:      asString(m["PropertyKey"]),
		Caption:  asString(m["Caption"]),
		Category: asString(m["Category"]),
	}
	vt, _ := m["ValueType"].(map[string]any)
	if vt != nil {
		dp.Type = asString(vt["Type"])
		dp.Default = asString(vt["DefaultValue"])
		if r, ok := vt["Required"].(bool); ok {
			dp.Required = r
		}
		if evs, ok := vt["EnumerationValues"].([]any); ok {
			for _, ev := range evs {
				if em, ok := ev.(map[string]any); ok {
					if k := asString(em["_Key"]); k != "" {
						dp.Enum = append(dp.Enum, k)
					}
				}
			}
		}
		if nested, ok := vt["ObjectType"].(map[string]any); ok {
			if npts, ok := nested["PropertyTypes"].([]any); ok {
				for _, npt := range npts {
					if nm, ok := npt.(map[string]any); ok {
						dp.Children = append(dp.Children, describedPropFromTemplate(nm))
					}
				}
			}
		}
	}
	dp.System = isSystemPropKey(dp.Key)
	return dp
}

func isSystemPropKey(key string) bool {
	switch key {
	case "Label", "Visibility", "Editability", "Name", "TabIndex":
		return true
	}
	return false
}

// rulesFromProject extracts dynamic rules from the project's installed .mpk editor
// config, returning the rules and a coverage note (recognized / total hide-calls).
func rulesFromProject(mpkPath, widgetID string) ([]describedRule, string) {
	rules, recognized, total := executor.ExtractWidgetVisibilityStats(mpkPath, widgetID)
	coverage := ""
	if total > 0 {
		coverage = fmt.Sprintf("%d of %d editor hide-rules recognized", recognized, total)
	}
	return rulesToDescribed(rules), coverage
}

func rulesFromDef(rules []types.WidgetVisibilityRule) []describedRule {
	return rulesToDescribed(rules)
}

func rulesToDescribed(rules []types.WidgetVisibilityRule) []describedRule {
	out := make([]describedRule, 0, len(rules))
	for _, r := range rules {
		if r.HiddenWhen == nil {
			continue
		}
		out = append(out, describedRule{Property: r.PropertyKey, HiddenWhen: conditionText(r.HiddenWhen)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Property < out[j].Property })
	return out
}

// conditionText renders a visibility condition as readable English.
func conditionText(c *types.WidgetVisibilityCondition) string {
	switch c.Operator {
	case "eq":
		return fmt.Sprintf("%s = %q", c.PropertyKey, c.Value)
	case "ne":
		return fmt.Sprintf("%s ≠ %q", c.PropertyKey, c.Value)
	case "truthy":
		return fmt.Sprintf("%s is set", c.PropertyKey)
	case "falsy":
		return fmt.Sprintf("%s is not set", c.PropertyKey)
	default:
		return fmt.Sprintf("%s %s %q", c.PropertyKey, c.Operator, c.Value)
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func printWidgetDescription(cmd *cobra.Command, d widgetDescription) {
	out := cmd.OutOrStdout()
	title := d.Name
	if title == "" {
		title = d.WidgetID
	}
	fmt.Fprintf(out, "Widget: %s", title)
	if d.MDLName != "" {
		fmt.Fprintf(out, " (%s)", d.MDLName)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  ID:      %s\n", d.WidgetID)
	if d.Version != "" {
		fmt.Fprintf(out, "  Version: %s\n", d.Version)
	}
	fmt.Fprintf(out, "  Kind:    %s\n", d.Kind)
	fmt.Fprintf(out, "  Source:  %s\n", d.Source)

	fmt.Fprintf(out, "\nProperties (%d):\n", countProps(d.Properties))
	printProps(out, d.Properties, 0)

	fmt.Fprintf(out, "\nDynamic property rules (%d):\n", len(d.Rules))
	if len(d.Rules) == 0 {
		fmt.Fprintln(out, "  (none discovered)")
	}
	for _, r := range d.Rules {
		fmt.Fprintf(out, "  %-40s hidden when %s\n", r.Property, r.HiddenWhen)
	}
	if d.RuleCoverage != "" {
		fmt.Fprintf(out, "  — %s\n", d.RuleCoverage)
	}
}

func countProps(props []describedProperty) int {
	n := 0
	for _, p := range props {
		n++
		n += countProps(p.Children)
	}
	return n
}

func printProps(out interface{ Write([]byte) (int, error) }, props []describedProperty, depth int) {
	indent := strings.Repeat("  ", depth+1)
	for _, p := range props {
		req := ""
		if p.Required {
			req = " required"
		}
		sys := ""
		if p.System {
			sys = " [system]"
		}
		line := fmt.Sprintf("%s%-34s %-13s", indent, p.Key, p.Type)
		extra := strings.TrimRight(req+sys, " ")
		if p.Default != "" {
			extra = strings.TrimSpace(extra + " default=" + p.Default)
		}
		if len(p.Enum) > 0 {
			extra = strings.TrimSpace(extra + " {" + strings.Join(p.Enum, "|") + "}")
		}
		if p.Category != "" {
			extra = strings.TrimSpace(extra + "  (" + p.Category + ")")
		}
		fmt.Fprintf(out, "%s %s\n", strings.TrimRight(line, " "), extra)
		if len(p.Children) > 0 {
			printProps(out, p.Children, depth+1)
		}
	}
}
