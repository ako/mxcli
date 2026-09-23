// SPDX-License-Identifier: Apache-2.0

// Check-time validation for `contentparams:` on a pluggable widget.
//
// A pluggable widget's text-template property takes `{1}`-style placeholders
// backed by `contentparams:`, or mxcli's `{AttrName}` convenience spelling which
// is resolved against the entity context. Parameters with no numeric placeholder
// to fill have nothing to attach to and are dropped on write — the same silent
// class as the bug #928 was filed for, and the residue left by fixing it.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validatePluggableContentParams reports (MDL-WIDGET21) `contentparams:` on a
// pluggable widget where no property text carries a `{N}` placeholder to consume
// them.
//
// A warning, not an error: the widget's property vocabulary comes from its own
// definition, and a text-template property whose value arrives by some route
// this check cannot see would make a hard reject a false positive.
func validatePluggableContentParams(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || len(w.GetContentParams()) == 0 {
		return nil
	}
	for _, v := range w.Properties {
		if s, ok := v.(string); ok && numericTemplatePlaceholderRe.MatchString(s) {
			return nil // something can consume them
		}
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET21",
		Severity: linter.SeverityWarning,
		Message: fmt.Sprintf(
			"%s: widget `%s` (%s) has `contentparams` but no property text contains a `{1}`-style "+
				"placeholder to use them, so they are dropped on write",
			locationPrefix, w.Name, w.Type,
		),
		Suggestion: "Put a numbered placeholder in the text property (e.g. `imageUrl: '{1}'`), or drop " +
			"the contentparams — a single attribute can also be written inline as `'{AttrName}'`",
	}}
}

// validatePluggableTemplateParams reports (MDL-WIDGET21) a `<Name>Params`
// companion whose own text property carries no `{N}` placeholder to consume it.
//
// The companion binds ONE text-template property (#575), so unlike the
// widget-wide `contentparams:` above there is a specific property to look at:
// `headerCaptionParams` is consumed by `headerCaption` and by nothing else. A
// companion written beside a literal caption is the shape the issue was filed
// for, one step short of the fix — the parameters are dropped and the literal
// still renders on every row.
func validatePluggableTemplateParams(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	var out []linter.Violation
	for _, key := range sortedPropertyKeys(w) {
		base, ok := strings.CutSuffix(key, "Params")
		if !ok || base == "" {
			continue
		}
		// ContentParams / CaptionParams are the widget-wide spelling, judged
		// against every property by the check above.
		switch strings.ToLower(key) {
		case "contentparams", "captionparams":
			continue
		}
		raw, ok := lookupProperty(w.Properties, key)
		if !ok {
			continue
		}
		if params, isParams := raw.([]ast.ParamAssignmentV3); !isParams || len(params) == 0 {
			continue
		}
		text, _ := lookupProperty(w.Properties, base)
		if s, isStr := text.(string); isStr && numericTemplatePlaceholderRe.MatchString(s) {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET21",
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf(
				"%s: widget `%s` (%s) has `%s` but `%s` contains no `{1}`-style placeholder to use "+
					"it, so the binding is dropped on write and the text renders literally",
				locationPrefix, w.Name, w.Type, key, base,
			),
			Suggestion: fmt.Sprintf(
				"Write the text as a template, e.g. `%s: '{1}', %s: [{1} = <attr>]`", base, key),
		})
	}
	return out
}
