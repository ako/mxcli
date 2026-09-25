// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"

	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// Widget properties that hold ONE Mendix expression, written as-is:
//
//	dynamicclasses: if $currentObject/Featured then 'is-featured' else ''
//	dynamicclasses: 'is-featured'          -- the string: the class is-featured
//
// A quoted value is a Mendix string, as for the OData client's credentials, not
// the expression's text — the old spelling, a quoted string holding the
// expression with its own quotes doubled, is
// refused at check time (MDL-WIDGET33). This is the named-property slice of
// PROPOSAL_first_class_expressions.md §6.2 slice 2; pluggable properties whose
// schema kind is Expression follow in the schema-driven slice.
var widgetExpressionProps = map[string]bool{
	"dynamicclasses":   true, // any widget's Appearance.DynamicClasses
	"dynamiccellclass": true, // a datagrid column's columnClass
}

func isWidgetExpressionProp(name string) bool {
	return widgetExpressionProps[strings.ToLower(name)]
}

// widgetExpressionValue returns an expression property's value: the source text
// of whatever the grammar matched after the `:` or `=`, whitespace kept. Which
// alternative matched is not a signal — `$x/Cls + ' x'` may be claimed by the
// datasource or action alternatives, which also start with a variable — so the
// text is taken from the value node itself. A bracketed list is the one
// exception: it is returned as the list the other readers build, so MDL-WIDGET27
// (empty) and MDL-WIDGET32 (non-empty) still report it.
func widgetExpressionValue(valueNode antlr.ParserRuleContext) any {
	if pv, ok := valueNode.(*parser.PropertyValueV3Context); ok && (pv.LBRACKET() != nil || pv.ObjectEntryListV3() != nil) {
		return buildPropertyValueV3(pv)
	}
	return ruleSourceText(valueNode)
}

// lastRuleChild is the rule node after `name :` / `name =` — the value.
func lastRuleChild(ctx antlr.ParserRuleContext) antlr.ParserRuleContext {
	children := ctx.GetChildren()
	for i := len(children) - 1; i >= 0; i-- {
		if rc, ok := children[i].(antlr.ParserRuleContext); ok {
			return rc
		}
	}
	return nil
}

func widgetExpressionNotAllowed(name string, expr parser.IExpressionContext) error {
	return fmt.Errorf(
		"property %s takes a plain value, not an expression: %s — "+
			"only DynamicClasses and a column's DynamicCellClass take an expression",
		name, ruleSourceText(expr.(antlr.ParserRuleContext)))
}
