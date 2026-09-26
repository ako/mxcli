// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
)

// designPropRename is what a stored design-property key that the theme has
// RENAMED became. Themes keep a key's earlier spellings as `oldNames` so Studio
// Pro can offer to update pages authored against an older version; until that
// happens the page keeps the old key, and mxbuild reports CE6087 "Design
// properties have been renamed in your theme and need to be updated" on it —
// unless the page is excluded, which is why a project full of them can still
// check clean (Feedback v4.0.2's *_Logo example pages on Atlas Core 4.1.3).
type designPropRename struct {
	// NewKey is the current property name.
	NewKey string
	// Replacement is the current MDL spelling of the stored key AND value, or
	// "" when the value has no current equivalent the theme declares.
	Replacement string
	// OldValues lists the values the old key took, for when Replacement is "".
	OldValues []string
}

// findRenamedThemeProp reports whether key is an old name of one of props, and
// what to write instead. It reads the three places a theme records one:
//
//   - a property's own oldNames ("Align content" → "Align content (deprecated)"),
//     with the value mapped through the options' oldNames as well;
//   - a Spacing property's per-side oldNames, "<old key>::<old value>"
//     ("Spacing bottom::Outer medium" → Spacing, margin-bottom, M);
//   - a multi-select option's oldNames, which are the separate toggles the
//     option replaced ("Hide on phone" → Hide on: Phone).
//
// An option's oldNames on an ordinary property are old VALUES, not keys, so they
// are only consulted to map a value.
func findRenamedThemeProp(props []ThemeProperty, key, value string) *designPropRename {
	for i := range props {
		p := &props[i]
		if containsString(p.OldNames, key) {
			r := &designPropRename{NewKey: p.Name}
			switch {
			case strings.EqualFold(value, "on") || strings.EqualFold(value, "off"):
				r.Replacement = fmt.Sprintf("'%s': %s", p.Name, strings.ToLower(value))
			case len(p.Options) == 0:
				r.Replacement = fmt.Sprintf("'%s': '%s'", p.Name, value)
			default:
				if opt := currentOptionName(p.Options, value); opt != "" {
					r.Replacement = fmt.Sprintf("'%s': '%s'", p.Name, opt)
				}
				for _, o := range p.Options {
					r.OldValues = append(r.OldValues, o.Name)
				}
			}
			return r
		}
		if p.Type == "Spacing" {
			if r := spacingRename(p, key, value); r != nil {
				return r
			}
		}
		if p.MultiSelect {
			for _, o := range p.Options {
				if containsString(o.OldNames, key) {
					return &designPropRename{
						NewKey:      p.Name,
						Replacement: fmt.Sprintf("'%s': ['%s': on]", p.Name, o.Name),
					}
				}
			}
		}
	}
	return nil
}

// spacingRename maps an old per-side spacing key ("Spacing bottom") to the
// Spacing compound. The side and the step both come from the matching
// "<old key>::<old value>" entry, so the old value decides the step: "Outer
// medium" is margin M, "Inner large" padding L.
func spacingRename(p *ThemeProperty, key, value string) *designPropRename {
	var r *designPropRename
	seen := map[string]bool{}
	for _, group := range []struct {
		kind  string
		steps []ThemeSpacingStep
	}{{"margin", p.Margin}, {"padding", p.Padding}} {
		for _, step := range group.steps {
			for _, side := range []struct {
				name string
				s    *ThemeSpacingSide
			}{{"top", step.Top}, {"right", step.Right}, {"bottom", step.Bottom}, {"left", step.Left}} {
				if side.s == nil {
					continue
				}
				for _, old := range side.s.OldNames {
					oldKey, oldValue, ok := strings.Cut(old, "::")
					if !ok || oldKey != key {
						continue
					}
					if r == nil {
						r = &designPropRename{NewKey: p.Name}
					}
					if oldValue == value && r.Replacement == "" {
						r.Replacement = fmt.Sprintf("'%s': ['%s-%s': '%s']", p.Name, group.kind, side.name, step.Name)
					}
					if !seen[oldValue] {
						seen[oldValue] = true
						r.OldValues = append(r.OldValues, oldValue)
					}
				}
			}
		}
	}
	return r
}

// currentOptionName returns the current name of value — itself when it is a
// declared option, the option it was renamed to when it is an old one, "" when
// neither.
func currentOptionName(options []ThemeOption, value string) string {
	for _, o := range options {
		if o.Name == value {
			return o.Name
		}
	}
	for _, o := range options {
		if containsString(o.OldNames, value) {
			return o.Name
		}
	}
	return ""
}

// renamedDesignPropSuggestion is the fix for a renamed key: its current
// spelling, or — when the stored value has none — the old values the theme
// maps, so the author can see which one was meant.
func renamedDesignPropSuggestion(r *designPropRename, value string) string {
	if r.Replacement != "" {
		return fmt.Sprintf("Write it as %s.", r.Replacement)
	}
	return fmt.Sprintf("Write it under %q. %q has no current equivalent in the theme; the values it maps are: %s",
		r.NewKey, value, strings.Join(r.OldValues, ", "))
}
