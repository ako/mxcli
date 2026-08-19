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
