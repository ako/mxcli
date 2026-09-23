// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// pageArgContext is what a widget-tree walk knows about the object Mendix would
// pass to a page opened by a widget's SHOW_PAGE action.
//
// mxcli stores a widget's show-page action with an EMPTY ParameterMappings array
// and lets Mendix infer the argument from the enclosing widget's context object.
// That is deliberate and twice-confirmed: an explicit Forms$PageParameterMapping
// whose Argument is "$currentObject" makes Studio Pro report CE0115 "parameters do
// not match", because a widget's current-row object is an inferred WidgetValue and
// not an Argument expression (issue #296, re-confirmed against mxbuild 11.12.1 for
// mxcli-formula1 §56). See formSettingsToGen in mdl/backend/modelsdk/widget_write.go.
//
// So the argument is honoured only when it names the context object, either as
// $currentObject or by the name of the variable the enclosing data widget is bound
// to. Anything else is refused by the caller rather than silently re-pointed.
//
// Three states, not two, and the third is what mendixlabs/mxcli#1029 reported:
//
//   - known=false — the pass cannot say what encloses the widget. ALTER PAGE's
//     SET/INSERT build an action against a stored page they never traverse, so an
//     empty varName means nothing about the argument. The guard stands down: it
//     only ever refuses what it can prove is discarded.
//   - known=true, present=true — a data-bound widget encloses this one, so there
//     IS a context object. The argument is honoured when it names that object.
//   - known=true, present=false — the walk started at the root of a page, layout
//     or snippet and never entered a data widget. There is NO context object:
//     $currentObject is unbound and the empty mapping mxcli writes is not an
//     inferred one, it is a missing one. EVERY argument is discarded here,
//     whatever its form, and mxbuild reports CE1571 per parameter of the target
//     page.
type pageArgContext struct {
	known   bool
	present bool
	// varName is the name the data source gives the context object, without the
	// "$" (e.g. "Car" for `dataview dv (DataSource: $Car)`). Empty when the
	// context object has no name of its own — a database, association, microflow
	// or selection source supplies a row object addressable only as
	// $currentObject.
	varName string
	// entity is the qualified entity of that context object, for the message.
	entity string
}

// enteringDataWidget is the context below a data-bound widget bound to ds and
// yielding entity.
func enteringDataWidget(ds *ast.DataSourceV3, entity string) pageArgContext {
	return pageArgContext{known: true, present: true, varName: contextVarFor(ds), entity: entity}
}

// atDocumentRoot is the context at the top of a page, snippet or layout the pass
// walks in full: knowable, and empty.
func atDocumentRoot() pageArgContext {
	return pageArgContext{known: true}
}

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

// contextFreeContainers are the widget kinds that hold other widgets and bind no
// data of their own, so their children see exactly the context they do.
//
// The list is an allow-list on purpose. Concluding "there is no context object
// here" is only safe when every widget between the page root and this one is
// known to bind nothing, and a widget's data source is not always readable from
// the AST: `datagrid dg (DataSource: Mod.Entity)` — the bare-entity shorthand —
// leaves a plain string rather than a parsed *ast.DataSourceV3, and a pluggable
// widget names its source under its own key. Anything not listed here therefore
// degrades the context to UNKNOWN rather than to ABSENT, so a row-scoped button
// is never refused (mdl-examples/bug-tests/295-showpage-null-variable.mdl is that
// case, and it is exactly what the first cut of #1029 broke).
var contextFreeContainers = map[string]bool{
	"container": true, "customcontainer": true, "slot": true,
	"layoutgrid": true, "row": true, "column": true,
	"tabcontainer": true, "tabpage": true,
	"groupbox": true, "scrollcontainer": true, "region": true,
	"header": true, "footer": true, "placeholder": true,
}

// argContextForChildren is the context the children of w see.
func argContextForChildren(w *ast.WidgetV3, parent pageArgContext) pageArgContext {
	if ds := w.GetDataSource(); ds != nil {
		// The entity is only used to word the refusal; the executor's builder
		// overwrites this with one that carries it.
		return enteringDataWidget(ds, "")
	}
	if parent.known && !parent.present && !contextFreeContainers[strings.ToLower(w.Type)] {
		return pageArgContext{}
	}
	return parent
}

// argContextForOwnAction is the context a widget's OWN action is judged in.
//
// A button's action runs in the context its parent supplies. A list widget's does
// not: `onClick` on a data grid, list view or gallery fires PER ROW, and the row
// it renders is the context object — so a widget that binds a source of its own
// supplies the context for its own action. Judging it in the parent's context
// refused `datagrid dg (DataSource: DATABASE M.E, onClick: SHOW_PAGE M.Edit(E:
// $currentObject))` against an mxbuild that accepts it at 0 errors, and on a list
// view the refusal contradicted its own wording (ako/mxcli#552).
//
// `Action:` and `onClick:` are aliases that both land on Properties["Action"], so
// this covers every spelling of a widget's own action.
func argContextForOwnAction(w *ast.WidgetV3, parent pageArgContext) pageArgContext {
	if w == nil {
		return parent
	}
	if ds := w.GetDataSource(); ds != nil {
		// The entity is only used to word a refusal; the executor's builder
		// overwrites this with one that carries it.
		return enteringDataWidget(ds, "")
	}
	if bindsDataInAnUnreadableShape(w) {
		// The widget plainly binds data, so a context object EXISTS — but this
		// pass cannot say what it is called. Unknown, not absent, and the guard
		// stands down exactly as it does for ALTER PAGE. The bare-entity
		// shorthand `datagrid dg (DataSource: M.E)` is this case.
		return pageArgContext{}
	}
	return parent
}

// bindsDataInAnUnreadableShape reports whether w names a data source this pass
// cannot parse into a *ast.DataSourceV3 — the bare-entity shorthand, or a
// pluggable widget naming its source under its own key.
func bindsDataInAnUnreadableShape(w *ast.WidgetV3) bool {
	for name, v := range w.Properties {
		if !strings.EqualFold(name, "DataSource") {
			continue
		}
		if _, parsed := v.(*ast.DataSourceV3); !parsed && v != nil {
			return true
		}
	}
	return false
}

// argContextForSubtreeOf is argContextForChildren for the executor's builder,
// which builds a widget AND its children in one call. A widget with no children
// carries nothing but its own action, and that action is judged in the context
// its parent supplies — degrading there would stand the guard down on exactly the
// page-level button #1029 is about. A data-bound widget is the exception, and the
// reason is argContextForOwnAction's: its own action is row-scoped, so it needs
// the context it creates whether or not it has children to give it to.
func argContextForSubtreeOf(w *ast.WidgetV3, parent pageArgContext) pageArgContext {
	if own := argContextForOwnAction(w, parent); own != parent {
		return own
	}
	if len(w.Children) == 0 {
		return parent
	}
	return argContextForChildren(w, parent)
}

// binds reports whether the argument `value`, written on a SHOW_PAGE widget
// action, denotes the object Mendix will actually pass to the target page.
//
// Arguments that are not a $-reference (a literal or an expression) are left
// alone WHERE A CONTEXT OBJECT EXISTS: they cannot be checked against it, and
// refusing them would be guesswork. Where none exists there is nothing to guess
// about — the mapping is empty either way — so they are refused too.
func (c pageArgContext) binds(value string) bool {
	if !c.known {
		return true
	}
	if !c.present {
		return false
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
	return c.varName != "" && strings.EqualFold(name, c.varName)
}

// describeContextObject names the object Mendix will actually pass, for the
// refusal message.
func (c pageArgContext) describeContextObject() string {
	if c.varName != "" {
		return "$" + c.varName
	}
	if c.entity != "" {
		return "the row object of the enclosing widget (" + c.entity + ")"
	}
	return "the enclosing context object"
}

// contextVarAlternative offers the context variable by name when it has one, so
// the message names a spelling that works rather than only one that does not.
func (c pageArgContext) contextVarAlternative() string {
	if c.varName == "" {
		return ""
	}
	return " (or $" + c.varName + ")"
}

// refuseShowPageArgument is the single wording `mxcli check` and `exec` share.
// Two copies in two currencies is how a resolver drifts, and this one already
// had to be told about a third context state.
//
// The rule ID is NOT in the text: `mxcli check` renders it from the violation,
// so embedding it here prints it twice. The exec path, which has no violation to
// render, appends it itself.
func refuseShowPageArgument(widgetName, target, argName, argValue string, c pageArgContext) string {
	where := "widget `" + widgetName + "`"
	if widgetName == "" {
		where = "this widget"
	}
	if !c.present {
		return fmt.Sprintf(
			"show_page %s: argument %s: %s cannot be stored — %s is not inside a data view, list view or grid row, "+
				"so there is no context object at all, and a widget's page argument is always that object. "+
				"The page would be opened with no argument, which mxbuild reports as CE1571 \"No argument has been "+
				"selected for parameter '%s'\". Put the button inside a data widget bound to the object, or call a "+
				"microflow that shows the page with it",
			target, argName, argValue, where, argName)
	}
	return fmt.Sprintf(
		"show_page %s: argument %s: %s cannot be stored — a widget's page argument is always the enclosing context "+
			"object, which mxcli records by leaving the mapping empty (an explicit one is rejected as CE0115). "+
			"Writing %s here would silently open the page with %s instead. Use $currentObject%s, or call a microflow "+
			"that shows the page with the object you want",
		target, argName, argValue, argValue, c.describeContextObject(), c.contextVarAlternative())
}

// validateShowPageArguments is the check-time mirror of the executor guard, so
// `mxcli check` reports the ignored argument without needing a project — the same
// pairing as MDL-WIDGET09.
func validateShowPageArguments(w *ast.WidgetV3, c pageArgContext, locationPrefix string) []linter.Violation {
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
		if !ok || c.binds(strVal) {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL-PAGEARG01",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf("%s: %s",
				locationPrefix, refuseShowPageArgument(w.Name, action.Target, arg.Name, strVal, c)),
		})
	}
	return out
}
