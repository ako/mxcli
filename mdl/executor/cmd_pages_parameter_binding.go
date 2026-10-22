// SPDX-License-Identifier: Apache-2.0

// A binding that names its object: `$Param.Attr`.
//
// A widget placed in a page or snippet outside every data container can still
// show or edit an attribute — of a page or snippet PARAMETER. Studio Pro stores
// the binding's AttributeRef (or a visibility setting's Attribute) beside a
// Forms$PageVariable that names the parameter in its PageParameter or
// SnippetParameter slot, with no Widget. Measured on TestApp's WorkflowCommons
// snippets: the combo boxes of Snip_TaskDashboard_Header /
// Snip_WorkflowDashboard_Header (attributeEnumeration and attributeAssociation
// alike), the image visibility of Snip_UserTask_NameColumnWithIcon, and the
// date picker, text areas and radio buttons of Snip_WorkflowUserTaskView_Details.
//
// MDL spells it `$Param.Attr` — the form a text template parameter already uses
// for a parameter, and the one ako/mxcli#826 gave an enclosing data view
// (`$dataView1.Attr`). Before, describe printed the attribute bare: exec then
// either refused it (there is no enclosing object) or, where it did not check,
// wrote a binding with no SourceVariable — a different document.
package executor

import (
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// namedObjectBinding splits `$name.rest` into name (without the "$") and rest.
// ok is false for anything else — a bare or qualified attribute, an
// association path, `$currentObject/…`.
func namedObjectBinding(s string) (name, rest string, ok bool) {
	after, isVar := strings.CutPrefix(s, "$")
	if !isVar {
		return "", "", false
	}
	name, rest, ok = strings.Cut(after, ".")
	if !ok || name == "" || rest == "" || strings.ContainsAny(name, "/ ") || strings.Contains(rest, "/") {
		return "", "", false
	}
	return name, rest, true
}

// parameterAttributeValue is the `$Param.Attr` text of a pluggable widget
// property authored by its own key. Unlike `Attribute:`, a generic key's value
// goes through the expression grammar, so `eventDataAttribute: $Ctx.EventData`
// arrives as an *ast.WidgetExpressionV3 — which every attribute property used
// to refuse as MDL-WIDGET42 (mendixlabs/mxcli#1591). Anything other than that
// one shape is still an expression.
func parameterAttributeValue(raw any) (string, bool) {
	e, ok := raw.(*ast.WidgetExpressionV3)
	if !ok || !isParameterAttributeText(e.Text) {
		return "", false
	}
	return e.Text, true
}

// namedObjectBindingKinds are the widgets whose `Attribute: $name.Attr` may
// name an enclosing data view as well as a parameter (resolveInputBinding).
// Every other widget that takes the form — the combo box — reads a parameter
// only: the data-view pair is measured on the built-in inputs alone.
var namedObjectBindingKinds = map[string]bool{
	"textbox": true, "textarea": true, "checkbox": true,
	"datepicker": true, "radiobuttons": true, "dropdown": true,
}

func dataViewClause(kind string) string {
	if namedObjectBindingKinds[kind] {
		return " nor a data view enclosing the widget"
	}
	return ""
}

// parameterSourceName is the parameter a stored SourceVariable reads, when it
// is exactly a page or snippet parameter: a Forms$PageVariable with the name in
// PageParameter or SnippetParameter and no Widget or LocalVariable. "" for
// anything else — null (the enclosing object), a data view pair (#826), a page
// variable.
func parameterSourceName(sv any) string {
	m, ok := sv.(map[string]any)
	if !ok || m == nil {
		return ""
	}
	if extractString(m["Widget"]) != "" || extractString(m["LocalVariable"]) != "" {
		return ""
	}
	if name := extractString(m["SnippetParameter"]); name != "" {
		return name
	}
	return extractString(m["PageParameter"])
}

// customWidgetPropertyParameter is the parameter a CustomWidget property's
// WidgetValue reads (its SourceVariable), or "".
func customWidgetPropertyParameter(w map[string]any, propertyKey string) string {
	obj, ok := w["Object"].(map[string]any)
	if !ok {
		return ""
	}
	propTypeKeyMap := buildPropertyTypeKeyMap(w, false)
	for _, prop := range getBsonArrayElements(obj["Properties"]) {
		propMap, ok := prop.(map[string]any)
		if !ok || propTypeKeyMap[extractBinaryID(propMap["TypePointer"])] != propertyKey {
			continue
		}
		value, _ := propMap["Value"].(map[string]any)
		return parameterSourceName(value["SourceVariable"])
	}
	return ""
}

// parameterAttributeMDL spells an attribute read from parameter name:
// `$Param.Attr`, the attribute's own name quoted when it is a keyword.
func parameterAttributeMDL(name, attrQN string) string {
	return "$" + name + "." + mdlIdent(shortAttributeName(attrQN))
}

// parameterAttributeTextRe is `$Param.Attr` and nothing more: one variable,
// one attribute name (quoted when it is a keyword). namedObjectBinding alone
// takes `$Ctx.Name + 'x'` too, which is an expression.
var parameterAttributeTextRe = regexp.MustCompile(`^\$[A-Za-z_][A-Za-z0-9_]*\.(?:[A-Za-z_][A-Za-z0-9_]*|"[A-Za-z_][A-Za-z0-9_]*")$`)

// isParameterAttributeText reports whether text is the `$Param.Attr` shape.
func isParameterAttributeText(text string) bool {
	return parameterAttributeTextRe.MatchString(text)
}
