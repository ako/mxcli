// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateObjectEntryProperties (MDL-WIDGET27) rejects a repeatable widget
// property written as a property VALUE:
//
//	htmlelement frame ( attributes: [(attributeName: 'data-x')] )
//
// # Why this is an error rather than a fix-up
//
// mendixlabs/mxcli#999. The shape had two failure modes and the dangerous one
// looked like success:
//
//	[(configMode: simple)]            parsed as a list of EXPRESSIONS, flattened
//	                                  to a []string no writer claimed — check
//	                                  clean, exec successful, property gone from
//	                                  storage
//	[(configMode: simple, x: y)]      died as `missing ')' at ','`, which names a
//	                                  paren and leaves the author to guess
//
// The reporter's framing was "check/exec-pass-then-discard". On FileUploader the
// dropped property was `allowedFileFormats`, which restricts what a user may
// upload — so the silent drop is a correctness problem and a mild security one
// in the built app.
//
// MDL already has a spelling for these, and it works: the entries are CONTAINER
// BLOCKS in the widget body, which slices 2-3 of the def-driven widget work made
// authorable for every widget with a definition.
//
//	htmlelement frame ( tagName: 'div' ) {
//	  attribute a1 (attributeName: 'data-x', attributeValueType: 'expression')
//	}
//
// So this is NOT wired to the object-list builder as a second route. Two
// spellings for one construct is the anti-pattern the syntax design guide names:
// it doubles what a reader has to learn, doubles what DESCRIBE must choose
// between, and the two would drift. The property form is reported, and the
// message names the container keyword so the error carries its own remedy.
//
// # An error, not a warning
//
// A warning would leave `exec` free to write the page and discard the entries,
// which is the bug. `exec` refuses only on errors.
//
// # It needs no project
//
// The SHAPE is wrong regardless of what the widget declares, so the rule fires
// without `-p` — which is how `make check-mdl` runs, and how a rule that needed
// a definition would be inert in CI. A definition, when there is one, only makes
// the message better: it supplies the container keyword.
func validateObjectEntryProperties(w *ast.WidgetV3, registry *WidgetRegistry, locationPrefix string) []linter.Violation {
	if w == nil || len(w.Properties) == 0 {
		return nil
	}

	// Sorted, because ranging a map emits violations in a random order and two
	// runs of the same binary then disagree — the noise floor that had to be
	// fixed before the slice 2-3 corpus diff could be read at all.
	keys := make([]string, 0, len(w.Properties))
	for k := range w.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []linter.Violation
	for _, key := range keys {
		entries, ok := w.Properties[key].(*ast.ObjectEntryListV3)
		if !ok || entries == nil {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET27",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` property `%s` is a repeated entry written as a property value — "+
					"MDL writes these as %s in the widget body, and a value here is discarded on write",
				locationPrefix, w.Name, key, objectEntryRemedy(w, registry, key)),
			Suggestion: objectEntryExample(w, registry, key, entries),
		})
	}
	return out
}

// objectEntryRemedy names the container keyword when the widget's definition
// declares one, and describes the shape when it does not — with no project there
// is no definition to consult, and naming a keyword that might be wrong is worse
// than describing the form.
func objectEntryRemedy(w *ast.WidgetV3, registry *WidgetRegistry, propertyKey string) string {
	if kw := objectEntryKeyword(w, registry, propertyKey); kw != "" {
		return fmt.Sprintf("`%s <name> (…)` blocks", kw)
	}
	return "`<container> <name> (…)` blocks"
}

// objectEntryKeyword resolves the property key to the MDL container keyword the
// widget declares for it, or "" when it cannot be known.
func objectEntryKeyword(w *ast.WidgetV3, registry *WidgetRegistry, propertyKey string) string {
	def := lookupWidgetDef(w, registry)
	if def == nil {
		return ""
	}
	for _, ol := range def.ObjectLists {
		if strings.EqualFold(ol.PropertyKey, propertyKey) {
			return strings.ToLower(ol.MDLContainer)
		}
	}
	return ""
}

// objectEntryExample rewrites what the author wrote into the form that works, so
// the fix is a copy rather than a translation exercise. Falls back to naming the
// discovery command when the keyword is unknown.
func objectEntryExample(w *ast.WidgetV3, registry *WidgetRegistry, propertyKey string, entries *ast.ObjectEntryListV3) string {
	kw := objectEntryKeyword(w, registry, propertyKey)
	if kw == "" {
		return "move the entries into the widget body as container blocks; " +
			"`mxcli widget describe <widget> -p <project.mpr>` lists the container keywords"
	}
	if len(entries.Entries) == 0 {
		return fmt.Sprintf("write `%s %s1 (…)` inside the widget body", kw, kw)
	}
	var parts []string
	for k, v := range entries.Entries[0] {
		parts = append(parts, fmt.Sprintf("%s: %s", k, formatEntryValue(v)))
	}
	sort.Strings(parts)
	return fmt.Sprintf("write `%s %s1 (%s)` inside the widget body", kw, kw, strings.Join(parts, ", "))
}

func formatEntryValue(v any) string {
	switch t := v.(type) {
	case string:
		return "'" + t + "'"
	case nil:
		return "''"
	default:
		return fmt.Sprintf("%v", t)
	}
}
