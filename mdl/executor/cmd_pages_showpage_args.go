// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// contextVarFor returns the name a data source gives its context object, without
// the "$". Only a parameter/variable source names it; a database, association,
// microflow or selection source yields a row object addressable only as
// $currentObject, so the name is empty.
func contextVarFor(ds *ast.DataSourceV3) string {
	if ds == nil || ds.Type != "parameter" {
		return ""
	}
	return strings.TrimPrefix(ds.Reference, "$")
}

// pageArgumentBindsContextObject reports whether the argument `value`, written on
// a SHOW_PAGE widget action, denotes the object Mendix will actually pass to the
// target page.
//
// mxcli stores a widget's show-page action with an EMPTY ParameterMappings array
// and lets Mendix infer the argument from the enclosing widget's context object.
// That is deliberate and twice-confirmed: an explicit Forms$PageParameterMapping
// whose Argument is "$currentObject" makes Studio Pro report CE0115 "parameters do
// not match", because a widget's current-row object is an inferred WidgetValue and
// not an Argument expression (issue #296, re-confirmed against mxbuild 11.12.1 for
// mxcli-formula1 §56). See the comment on *pages.PageClientAction in
// sdk/mpr/writer_widgets_action.go.
//
// The half that was missing is what happens when the author names something that
// is NOT the context object. `SHOW_PAGE Detail(Car: $Other)` inside a data view
// bound to $Car stored the same empty array, so the button opened Detail with
// $Car. The argument was not rejected, not warned about, and not visible
// afterwards: DESCRIBE prints the mapping Mendix infers, and `mx check` reports 0
// errors, because an inferred mapping is perfectly valid. The model is a valid
// model of a different app than the one the author wrote — the exact trap §39's
// reporter spent three cycles in while distrusting a button that was correct.
//
// So the argument is honoured only when it names the context object, either as
// $currentObject or by the name of the variable the enclosing data widget is bound
// to. Anything else is refused by the caller rather than silently re-pointed.
//
// Arguments that are not a $-reference (a literal or an expression) are left
// alone: they cannot be checked this way, and refusing them would be guesswork.
// validateShowPageArguments is the check-time mirror of the executor guard, so
// `mxcli check` reports the ignored argument without needing a project — the same
// pairing as MDL-WIDGET09. contextVar is the variable the nearest enclosing data
// widget is bound to, "" when the context object has no name of its own.
func validateShowPageArguments(w *ast.WidgetV3, contextVar string, contextKnown bool, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	action := w.GetAction()
	if action == nil || action.Type != "showPage" {
		return nil
	}
	var out []linter.Violation
	for _, arg := range action.Args {
		strVal, ok := arg.Value.(string)
		if !ok || pageArgumentBindsContextObject(strVal, contextVar, contextKnown) {
			continue
		}
		bound := "the enclosing widget's context object"
		if contextVar != "" {
			bound = "$" + contextVar
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-PAGEARG01",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf(
				"%s: widget `%s`: show_page %s argument `%s: %s` cannot be stored — a widget's page argument is always the enclosing context object, so the page would open with %s instead. Use $currentObject, or call a microflow that shows the page with the object you want",
				locationPrefix, w.Name, action.Target, arg.Name, strVal, bound,
			),
		})
	}
	return out
}

// describeContextObject names the object Mendix will actually pass, for the
// refusal message.
func (pb *pageBuilder) describeContextObject() string {
	if pb.contextVarName != "" {
		return "$" + pb.contextVarName
	}
	if pb.entityContext != "" {
		return "the row object of the enclosing widget (" + pb.entityContext + ")"
	}
	return "the enclosing context object"
}

// contextVarAlternative offers the context variable by name when it has one, so
// the message names a spelling that works rather than only one that does not.
func (pb *pageBuilder) contextVarAlternative() string {
	if pb.contextVarName == "" {
		return ""
	}
	return " (or $" + pb.contextVarName + ")"
}

// contextKnown is false when the caller did not walk in through a data widget and
// so cannot say what the context object is — ALTER PAGE's SET/INSERT build an
// action against a stored page this pass never traverses. The guard then allows
// the argument: it only ever refuses what it can prove is discarded, and an
// unprovable case must behave exactly as it did before the guard existed.
func pageArgumentBindsContextObject(value, contextVar string, contextKnown bool) bool {
	if !contextKnown {
		return true
	}
	if !strings.HasPrefix(value, "$") {
		return true
	}
	name := strings.TrimPrefix(value, "$")
	// A path expression ($obj/Module.Assoc) is not a plain variable reference.
	if strings.ContainsAny(name, "/.") {
		return true
	}
	if strings.EqualFold(name, "currentObject") {
		return true
	}
	return contextVar != "" && strings.EqualFold(name, contextVar)
}
