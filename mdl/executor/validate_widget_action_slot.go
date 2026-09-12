// SPDX-License-Identifier: Apache-2.0

// Check-time validation for a widget's action slot holding something that is
// not an action.
//
// `Action:`, `OnClick:` and `OnChange:` each have a dedicated grammar branch
// carrying actionExprV3. When the value does not match that rule — a real
// keyword short its argument (`Action: OPEN_LINK`), or a token that was never a
// keyword (`Action: TOTALLY_MADE_UP`) — ANTLR does not fail: it falls through to
// the generic `keyword COLON propertyValueV3` alternative at the end of
// widgetPropertyV3 and the slot ends up holding a plain string. WidgetV3's
// GetAction/GetOnChange type-assert to *ast.ActionV3, get nil, and the widget is
// written with Forms$NoAction.
//
// Nothing downstream notices. A no-action button is legal Mendix, so the build
// is clean — measured on 11.12.0: `mxcli check` passed, `exec` reported "Created
// page", `mx check` reported 0 errors, and `describe page` came back with no
// action on the widget at all. The author gets a button that renders, says
// "Unlink", and does nothing (mendixlabs/mxcli#1062).
//
// This is the third appearance of the class. SIGN_OUT and OPEN_LINK both used to
// reach Forms$NoAction through the *writer's* default branch (CapTrackV2
// FINDINGS §10, cited in sdk/mpr/writer_widgets_action.go); this is the same
// silent degrade one layer up, in the parser.
//
// The rule is only possible because `NOTHING` was promoted to a real
// actionExprV3 alternative in the same change. It is the documented spelling for
// a deliberately inert widget and it reached Forms$NoAction by exactly this
// fall-through, so before the promotion "scalar in an action slot" covered the
// working case and the two broken ones alike.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// actionSlotKeys are the property keys with a dedicated actionExprV3 branch.
//
// `Action:` and `OnClick:` are aliases that both land on Properties["Action"]
// once parsed (issue #603), but an *unparsed* `OnClick:` keeps its own key —
// the generic fall-through stores the property under the name as written — so
// both spellings have to be looked for here.
//
// A pluggable widget's NAMED action slot (`createFileAction: …`) degrades the
// same way and is deliberately not listed: for an arbitrary widget property a
// string is an ordinary value, so the fault is only decidable against a loaded
// widget definition, which `mxcli check` has no project to supply.
var actionSlotKeys = []string{"Action", "OnClick", "OnChange"}

// underSpecified maps an action keyword to what it is missing, so the message
// can name the one token the author left out rather than printing the whole
// grammar. Keyed lowercase; looked up case-insensitively.
var underSpecified = map[string]string{
	"open_link":     "a URL — `Action: OPEN_LINK 'https://example.com'`",
	"complete_task": "an outcome name — `Action: COMPLETE_TASK 'Approved'`",
	"show_page":     "a page — `Action: SHOW_PAGE Module.Page`",
	"create_object": "an entity — `Action: CREATE_OBJECT Module.Entity`",
	"microflow":     "a microflow — `Action: MICROFLOW Module.Flow`",
	"nanoflow":      "a nanoflow — `Action: NANOFLOW Module.Flow`",
}

// validateWidgetActionSlot reports (MDL-WIDGET28) an action slot whose value is
// not an action expression.
//
// An error, not a warning, unlike the neighbouring "silently dropped" rules
// (MDL-WIDGET20/21/23). Those fire on values that are *valid MDL* the writer
// happens not to route; this one fires on text that failed to parse as the thing
// the slot accepts, and after NOTHING was promoted there is no scalar spelling
// of an action left for it to catch by mistake. ALTER PAGE already refuses the
// same value ("Action value must be an action expression") — CREATE PAGE was the
// inconsistent one.
func validateWidgetActionSlot(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	var out []linter.Violation
	// Iterated over a fixed slice rather than over w.Properties: map order is
	// not stable, and a widget with two faulty slots must report them the same
	// way every run (CLAUDE.md, determinism).
	for _, key := range actionSlotKeys {
		raw, present := w.Properties[key]
		if !present {
			continue
		}
		if _, ok := raw.(*ast.ActionV3); ok {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-WIDGET28",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s` has `%s: %s`, which is not an action — the widget is written with "+
					"no action at all and renders as a dead control (Mendix accepts it, so the build stays clean)",
				locationPrefix, w.Name, key, renderActionSlotValue(raw)),
			Suggestion: actionSlotSuggestion(raw),
		})
	}
	return out
}

// renderActionSlotValue prints the offending value the way the author wrote it,
// so the message can be matched against the source line.
func renderActionSlotValue(raw any) string {
	if s, ok := raw.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", raw)
}

// actionSlotSuggestion names the missing token when the value is a real action
// keyword, and otherwise lists what the slot takes.
//
// The split matters: an author who wrote `OPEN_LINK` knows which action they
// want and needs one argument, while an author who wrote `TOTALLY_MADE_UP`
// needs the vocabulary. Telling the first one to "see `mxcli syntax
// page.action`" buries the answer they were one token away from.
func actionSlotSuggestion(raw any) string {
	if s, ok := raw.(string); ok {
		if missing, known := underSpecified[strings.ToLower(strings.TrimSpace(s))]; known {
			return fmt.Sprintf("`%s` is a real action but takes %s.", s, missing)
		}
	}
	return "Use an action expression — SAVE_CHANGES, CANCEL_CHANGES, CLOSE_PAGE, DELETE_OBJECT, " +
		"SIGN_OUT, SHOW_PAGE Module.Page, MICROFLOW Module.Flow, NANOFLOW Module.Flow, " +
		"OPEN_LINK 'url', COMPLETE_TASK 'Outcome', CREATE_OBJECT Module.Entity — " +
		"or `NOTHING` if the widget is meant to do nothing. See `mxcli syntax page.action`."
}
