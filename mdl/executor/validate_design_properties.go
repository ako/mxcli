// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// LoadThemeRegistryForProject loads the design-property registry for a project
// given the path to its .mpr file (the theme lives in <projectDir>/themesource).
// Returns nil when there is no usable metadata (no project, no themesource, or an
// empty registry) so callers can skip validation cleanly. The LSP loads this once
// per session to keep file I/O out of the per-keystroke path.
func LoadThemeRegistryForProject(mprPath string) *ThemeRegistry {
	if mprPath == "" {
		return nil
	}
	reg, err := loadThemeRegistry(filepath.Dir(mprPath))
	if err != nil || reg == nil || len(reg.WidgetProperties) == 0 {
		return nil
	}
	return reg
}

// ValidateDesignProperties validates the design properties authored on page /
// snippet / alter-page widgets against the project's theme registry
// (themesource/*/web/design-properties.json). It warns (never blocks — a newer
// theme may legitimately add keys/values the snapshot lacks) on:
//
//   - MDL-WIDGET11: a design-property key that is not defined for the widget type.
//   - MDL-WIDGET12: an option/toggle-group value that is not one of the property's
//     allowed values — the message lists the allowed values (design-property
//     values are case-sensitive, so this catches the common casing typo).
//
// It only runs with --project AND a themesource that actually defines design
// properties; otherwise there is no metadata to validate against and it is a
// no-op. Compound (nested) entries are skipped — the registry does not model
// their sub-properties, so validating them would produce false positives.
func ValidateDesignProperties(prog *ast.Program, projectPath string) []linter.Violation {
	reg := LoadThemeRegistryForProject(projectPath)
	if reg == nil {
		return nil
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		out = append(out, ValidateDesignPropertiesForStatement(stmt, reg)...)
	}
	return out
}

// ValidateDesignPropertiesForStatement validates one statement's widget design
// properties against a pre-loaded registry (LSP entry point).
func ValidateDesignPropertiesForStatement(stmt ast.Statement, reg *ThemeRegistry) []linter.Violation {
	if reg == nil {
		return nil
	}
	switch s := stmt.(type) {
	case *ast.CreatePageStmtV3:
		return validateDesignPropsTree(s.Widgets, reg, "page "+s.Name.String())
	case *ast.CreateSnippetStmtV3:
		return validateDesignPropsTree(s.Widgets, reg, "snippet "+s.Name.String())
	case *ast.AlterPageStmt:
		var out []linter.Violation
		for _, op := range s.Operations {
			switch o := op.(type) {
			case *ast.InsertWidgetOp:
				out = append(out, validateDesignPropsTree(o.Widgets, reg, "alter "+s.PageName.String())...)
			case *ast.ReplaceWidgetOp:
				out = append(out, validateDesignPropsTree(o.NewWidgets, reg, "alter "+s.PageName.String())...)
			}
		}
		return out
	}
	return nil
}

func validateDesignPropsTree(widgets []*ast.WidgetV3, reg *ThemeRegistry, locationPrefix string) []linter.Violation {
	return validateDesignPropsSubtree(nil, widgets, reg, locationPrefix)
}

func validateDesignPropsSubtree(parent *ast.WidgetV3, widgets []*ast.WidgetV3, reg *ThemeRegistry, locationPrefix string) []linter.Violation {
	var out []linter.Violation
	for _, w := range widgets {
		if w == nil {
			continue
		}
		switch designPropsSlotOf(parent, w) {
		case slotDropped:
			out = append(out, droppedSlotDesignProps(parent, w, locationPrefix)...)
		case slotOfPluggable:
			// Built by the pluggable engine from the parent's object lists; the
			// keyword names no native widget here, so there is nothing to resolve.
		default:
			out = append(out, validateWidgetDesignProps(w, reg, locationPrefix)...)
		}
		if len(w.Children) > 0 {
			out = append(out, validateDesignPropsSubtree(w, w.Children, reg, locationPrefix)...)
		}
	}
	return out
}

type designPropsSlot int

const (
	notASlot designPropsSlot = iota
	// slotDropped: a structural part of its native parent that the builder
	// assembles itself, never through buildWidgetV3, so applyWidgetAppearance
	// never sees its design properties.
	slotDropped
	// slotOfPluggable: a keyword that is a native widget elsewhere, used as a
	// child of a pluggable widget — one of its object-list entries or slots.
	slotOfPluggable
)

// slotKeywords are the keywords that build a Forms$DivContainer on their own but
// name a part of their parent when nested in one: a layout grid's row, a row's
// column, a dataview's footer, a data grid's column or a gallery's template.
var slotKeywords = map[string]bool{
	"row": true, "column": true, "header": true, "footer": true,
	"controlbar": true, "template": true, "filter": true,
}

// designPropsSlotOf reports whether child is a slot of parent rather than a
// widget. mdlKeywordStorageType maps `row` / `column` / `footer` to the
// Forms$DivContainer they build at the top level; that is only true where
// buildWidgetV3 builds them.
func designPropsSlotOf(parent, child *ast.WidgetV3) designPropsSlot {
	if parent == nil || child == nil {
		return notASlot
	}
	p, c := strings.ToLower(parent.Type), strings.ToLower(child.Type)
	switch {
	case p == "layoutgrid" && c == "row", // buildLayoutGridRowV3
		p == "row" && c == "column",      // buildLayoutGridColumnV3
		p == "dataview" && c == "footer": // children moved into FooterWidgets
		return slotDropped
	}
	if !slotKeywords[c] {
		return notASlot
	}
	if _, native := mdlKeywordStorageType[p]; !native {
		return slotOfPluggable
	}
	return notASlot
}

// droppedSlotDesignProps reports design properties written on a slot the builder
// drops them from. Silence here read as approval of a value that never reaches
// the model — the "check passes, exec succeeds, the value is gone" shape.
func droppedSlotDesignProps(parent, w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if len(w.GetDesignProperties()) == 0 {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET07",
		Severity: linter.SeverityWarning,
		Message: fmt.Sprintf("%s: %s %q inside %s %q sets DesignProperties, but it is part of its parent, "+
			"not a widget — mxcli does not write design properties there, so they are silently dropped",
			locationPrefix, strings.ToLower(w.Type), w.Name, strings.ToLower(parent.Type), parent.Name),
		Location:   linter.Location{DocumentType: "page", DocumentName: locationPrefix},
		Suggestion: fmt.Sprintf("Put them on a container inside the %s instead.", strings.ToLower(w.Type)),
	}}
}

func validateWidgetDesignProps(w *ast.WidgetV3, reg *ThemeRegistry, locationPrefix string) []linter.Violation {
	astProps := w.GetDesignProperties()
	if len(astProps) == 0 {
		return nil
	}
	key := resolveDesignPropsKey(w.Type)
	// Only validate widgets we have type-specific metadata for. For an unknown
	// widget type (e.g. a pluggable widget not in the theme registry) we don't
	// know the full property set, so we must not flag its keys as unknown.
	if _, ok := reg.WidgetProperties[key]; !ok {
		return nil
	}
	props := reg.GetPropertiesForWidget(key)

	var out []linter.Violation
	for _, p := range astProps {
		if len(p.Nested) > 0 {
			continue // compound — registry doesn't model sub-properties
		}
		tp := findThemeProp(props, p.Key)
		if tp != nil && tp.MultiSelect && len(p.Nested) == 0 && p.Value != "" {
			// A multi-select property is stored as a compound, one entry per
			// selected option. A flat value names a declared option, so nothing
			// else here objects — and mxbuild then refuses the document with
			// CE6084 (ako/mxcli#511). exec refuses it too; saying so at check
			// time is what keeps the two in step.
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET12",
				Severity: linter.SeverityWarning,
				Message: fmt.Sprintf("%s: widget %q (%s) sets design property %q to a single value, "+
					"but it takes a SET of options — mxbuild refuses that with CE6084",
					locationPrefix, w.Name, w.Type, p.Key),
				Location: linter.Location{DocumentType: "page", DocumentName: locationPrefix},
				Suggestion: fmt.Sprintf("Write it as `'%s': ['%s': on]`, with one `'<option>': on` "+
					"per selection.", p.Key, firstOptionName(tp, p.Value)),
			})
			continue
		}
		if tp == nil {
			// An OLD name of a current property is not a typo: it is what a page
			// authored against an earlier theme version stores. It is still
			// flagged — mxbuild refuses it on a live page with CE6087 — but as a
			// rename with its current spelling, which "not defined" plus a list
			// of every key could not give.
			if r := findRenamedThemeProp(props, p.Key, p.Value); r != nil {
				out = append(out, linter.Violation{
					RuleID:   "MDL-WIDGET11",
					Severity: linter.SeverityWarning,
					Message: fmt.Sprintf("%s: widget %q (%s) sets design property %q, which the theme has renamed to %q"+
						" — mxbuild reports CE6087 \"Design properties have been renamed in your theme\" unless the page is excluded",
						locationPrefix, w.Name, w.Type, p.Key, r.NewKey),
					Location:   linter.Location{DocumentType: "page", DocumentName: locationPrefix},
					Suggestion: renamedDesignPropSuggestion(r, p.Value),
				})
				continue
			}
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET11",
				Severity: linter.SeverityWarning,
				Message: fmt.Sprintf("%s: widget %q (%s) sets design property %q, which is not defined for this widget type",
					locationPrefix, w.Name, w.Type, p.Key),
				Location:   linter.Location{DocumentType: "page", DocumentName: locationPrefix},
				Suggestion: designPropKeySuggestion(props, p.Key),
			})
			continue
		}
		// Value validation for enumerated types. Toggle values are on/off (handled
		// by the writer); options/pickers must be one of the declared values.
		if len(tp.Options) > 0 && !strings.EqualFold(p.Value, "on") && !strings.EqualFold(p.Value, "off") {
			// A ColorPicker also takes a custom colour, which the builder writes as
			// Forms$CustomDesignPropertyValue and mxbuild accepts — whether it
			// reaches the page is the theme's call (colorPickerValueProblem).
			if tp.Type == "ColorPicker" {
				if msg, suggestion, ok := colorPickerValueProblem(tp, p.Value); !ok {
					out = append(out, linter.Violation{
						RuleID:   "MDL-WIDGET12",
						Severity: linter.SeverityWarning,
						Message: fmt.Sprintf("%s: widget %q (%s) design property %q has value %q, which %s",
							locationPrefix, w.Name, w.Type, p.Key, p.Value, msg),
						Location:   linter.Location{DocumentType: "page", DocumentName: locationPrefix},
						Suggestion: suggestion,
					})
				}
				continue
			}
			if !themeOptionAllowed(tp.Options, p.Value) {
				out = append(out, linter.Violation{
					RuleID:   "MDL-WIDGET12",
					Severity: linter.SeverityWarning,
					Message: fmt.Sprintf("%s: widget %q (%s) design property %q has value %q, which is not an allowed value",
						locationPrefix, w.Name, w.Type, p.Key, p.Value),
					Location:   linter.Location{DocumentType: "page", DocumentName: locationPrefix},
					Suggestion: fmt.Sprintf("Allowed values (case-sensitive): %s", themeOptionNames(tp.Options)),
				})
			}
		}
	}
	return out
}

// findThemeProp returns the property whose Name matches key exactly.
func findThemeProp(props []ThemeProperty, key string) *ThemeProperty {
	for i := range props {
		if props[i].Name == key {
			return &props[i]
		}
	}
	return nil
}

// designPropKeySuggestion offers a case-insensitive near match (design-property
// keys are case-sensitive, so a casing typo is the most common cause), otherwise
// lists the defined keys for the widget.
func designPropKeySuggestion(props []ThemeProperty, key string) string {
	for i := range props {
		if strings.EqualFold(props[i].Name, key) {
			return fmt.Sprintf("Design-property keys are case-sensitive — did you mean %q?", props[i].Name)
		}
	}
	var names []string
	for i := range props {
		names = append(names, props[i].Name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "Run 'mxcli show design properties' to list the valid design properties for this widget."
	}
	return "Valid design properties for this widget: " + strings.Join(names, ", ")
}

func themeOptionAllowed(options []ThemeOption, value string) bool {
	for _, o := range options {
		if o.Name == value {
			return true
		}
	}
	return false
}

func themeOptionNames(options []ThemeOption) string {
	names := make([]string, 0, len(options))
	for _, o := range options {
		names = append(names, o.Name)
	}
	return strings.Join(names, ", ")
}
