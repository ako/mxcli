package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ValidateRegionToggleMode flags a scroll-container region whose ToggleMode is
// not one of the five members Mendix stores (MDL-WIDGET29).
//
// The property decides whether the region collapses, which is the whole
// difference between Atlas_Default's sidebar and Atlas_TopBar's, and it is the
// one region property whose wrong value leaves no trace anywhere: an
// unrecognised member is dropped when the document loads, so the layout execs
// clean, builds at 0 errors, and renders exactly as if nothing had been written
// — a fixed strip of chrome at every viewport. Studio Pro's own dropdown shows
// captions ("Shrink content (initially closed)"), so typing what the UI says is
// the likely mistake and produces precisely that silence.
//
// Runs with no project, because the fault is the value's shape and the parse
// settles it. It has its own statement switch rather than riding
// ValidateWidgetPropertiesForStatement, which does not cover CREATE LAYOUT —
// and a layout is where nearly every region lives.
func ValidateRegionToggleMode(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		forEachWidget(stmt, func(w *ast.WidgetV3, where string) {
			if !strings.EqualFold(w.Type, "region") {
				return
			}
			raw := w.GetStringProp("ToggleMode")
			if raw == "" {
				return
			}
			for _, mode := range pages.ScrollContainerToggleModes {
				if strings.EqualFold(raw, mode) {
					return
				}
			}
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET29",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: ToggleMode %q is not one of Mendix's members, so it is dropped when the "+
						"document loads — the region renders as if no toggle behaviour had been set "+
						"(the build stays clean, which is why this is worth stopping here)",
					where, raw),
				Suggestion: fmt.Sprintf(
					"use one of %s — these are the stored names, not Studio Pro's captions (its "+
						"\"Shrink content (initially closed)\" is ShrinkContentInitiallyClosed)",
					strings.Join(pages.ScrollContainerToggleModes, ", ")),
			})
		})
	}
	return out
}
