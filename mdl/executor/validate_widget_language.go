// SPDX-License-Identifier: Apache-2.0

// The widget refusals ako/mxcli#842 adds, gated on the language version.
//
// Writing a Gallery's row click is the fix of a silent wrong write and applies
// under both versions. The two refusals that come with it are new rejections,
// so per ADR-0011 they apply only under `mdl 1;`; a headerless script keeps the
// old outcome and is told, with the change's MDL-V1 code, what the header
// would change:
//
//   - an `onClick:`/`OnChange:` on a widget with no slot for it is dropped, as
//     before, with a warning instead of in silence (MDL-V1-ACTIONSLOT);
//   - a gallery row click on a single-click selection is written, as the
//     script says, and `mx check` then reports the widget's own "The item
//     click action is ambiguous" (MDL-V1-GALLERYCLICK) — the same choice
//     MDL-V1-REMOTETYPE makes for a write mx check will refuse.
//
// Validators emit these violations under the change's code as errors;
// GateWidgetViolations turns them into the rule's own ID under mdl 1 and into
// warnings under mdl 0. Every caller of the widget validators goes through it.
package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

var actionSlotRefused = langver.Change{
	Code:  "MDL-V1-ACTIONSLOT",
	Since: langver.V1,
	Old:   "An `onClick:` or `OnChange:` on a widget with no slot for it is dropped on write",
	New:   "a refusal (MDL-WIDGET37), with nothing written",
}

var galleryClickRefused = langver.Change{
	Code:  "MDL-V1-GALLERYCLICK",
	Since: langver.V1,
	Old: "A gallery row click on a single-click selection is written, and `mx check` fails the page " +
		"(\"The item click action is ambiguous\")",
	New: "a refusal (MDL-WIDGET36), with nothing written",
}

// widgetLanguageChanges maps each gated violation code to its change and the
// rule ID it carries once the change applies.
var widgetLanguageChanges = map[string]struct {
	change  langver.Change
	errorID string
}{
	actionSlotRefused.Code:   {actionSlotRefused, "MDL-WIDGET37"},
	galleryClickRefused.Code: {galleryClickRefused, "MDL-WIDGET36"},
	tabPagePropDropped.Code:  {tabPagePropDropped, "MDL-WIDGET44"},
}

// GateWidgetViolations applies the script's language version to the widget
// violations that are new rejections: an error under the rule's own ID where
// the change applies, a warning naming the change where it does not.
func GateWidgetViolations(vs []linter.Violation, v langver.Version) []linter.Violation {
	for i := range vs {
		g, ok := widgetLanguageChanges[vs[i].RuleID]
		if !ok {
			continue
		}
		if g.change.Applies(v) {
			vs[i].RuleID = g.errorID
			vs[i].Severity = linter.SeverityError
			continue
		}
		vs[i].Severity = linter.SeverityWarning
		vs[i].Message = strings.TrimSuffix(vs[i].Message, ".") + ". " + g.change.Warning(v)
	}
	return vs
}

// actionKeywordSource maps an AST property key holding one of MDL's two
// widget-action keywords to the engine source that reads it. `onClick:` and
// `Action:` are both stored under "Action" by the visitor (#603).
func actionKeywordSource(key string) (string, bool) {
	switch {
	case strings.EqualFold(key, "Action"), strings.EqualFold(key, "OnClick"):
		return "OnClick", true
	case strings.EqualFold(key, "OnChange"):
		return "OnChange", true
	}
	return "", false
}

// actionKeywordSpelling is the MDL spelling of an action source, for messages.
func actionKeywordSpelling(src string) string {
	if src == "OnClick" {
		return "onClick"
	}
	return src
}

// defRoutesActionSource reports whether any of a definition's mappings — top
// level or in any mode — writes an action read from the given source. It is
// the question the engine answers when it resolves a mapping (resolveMapping's
// `case "OnClick"` / `case "OnChange"`); a definition without such a mapping
// never reads the keyword at all (#842).
func defRoutesActionSource(def *WidgetDefinition, src string) bool {
	if mappingsRouteActionSource(def.PropertyMappings, src) {
		return true
	}
	for _, mode := range def.Modes {
		if mappingsRouteActionSource(mode.PropertyMappings, src) {
			return true
		}
	}
	return false
}

func mappingsRouteActionSource(ms []PropertyMapping, src string) bool {
	for _, m := range ms {
		if m.Operation == "action" && m.Source == src {
			return true
		}
	}
	return false
}
