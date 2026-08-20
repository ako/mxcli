// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// accordionGetProperties is the getProperties body of the stock Accordion's
// compiled editorConfig.js (com.mendix.widget.web.Accordion 2.3.4, shipped with
// every Mendix 11 app), trimmed to the hide calls and kept minified — the
// extractor reads minified JS, so a prettified fixture would not exercise it.
//
// It is the case behind upstream #931: the widget hides its whole State group
// when `collapsible` is off, and hides `initiallyCollapsed` unless the GROUP's
// own `initialCollapsedState` is "dynamic".
const accordionGetProperties = `exports.getProperties=function(e,r,t){` +
	`return e.groups.forEach((function(n,o){` +
	`"text"===n.headerRenderMode?(p.hidePropertyIn(r,e,"groups",o,"headerContent"),` +
	`e.advancedMode||"web"!==t||p.hidePropertyIn(r,e,"groups",o,"headerHeading")):` +
	`(p.hidePropertyIn(r,e,"groups",o,"headerText"),p.hidePropertyIn(r,e,"groups",o,"headerHeading")),` +
	`"dynamic"!==n.initialCollapsedState&&p.hidePropertyIn(r,e,"groups",o,"initiallyCollapsed"),` +
	`(e.advancedMode||"web"!==t)&&e.collapsible||p.hideNestedPropertiesIn(r,e,"groups",o,` +
	`["collapsed","onToggleCollapsed","initialCollapsedState","initiallyCollapsed"])})),` +
	`e.collapsible||p.hidePropertiesIn(r,e,["expandBehavior","animate"]),r}`

func findNestedRule(rules []types.WidgetVisibilityRule, listKey, propertyKey, condKey string) *types.WidgetVisibilityRule {
	for i := range rules {
		r := rules[i]
		if r.ListPropertyKey == listKey && r.PropertyKey == propertyKey &&
			r.HiddenWhen != nil && r.HiddenWhen.PropertyKey == condKey {
			return &rules[i]
		}
	}
	return nil
}

// The nested hides are the ones #931 turns on: without them mxcli reported four
// harmless warnings (all on properties left at their defaults) and said nothing
// about the two values that actually fail the build.
func TestExtractNestedHideRules(t *testing.T) {
	rules, stats := extractVisibilityRulesFromJS(accordionGetProperties)

	for _, key := range []string{"initialCollapsedState", "initiallyCollapsed", "collapsed", "onToggleCollapsed"} {
		r := findNestedRule(rules, "groups", key, "collapsible")
		if r == nil {
			t.Fatalf("no rule hiding groups/%s when collapsible is falsy; got %+v", key, rules)
		}
		if r.HiddenWhen.Operator != "falsy" {
			t.Errorf("groups/%s: operator = %q, want falsy", key, r.HiddenWhen.Operator)
		}
		if r.HiddenWhen.Scope != "" {
			t.Errorf("groups/%s: scope = %q, want widget scope (collapsible is a widget property)", key, r.HiddenWhen.Scope)
		}
	}

	// The top-level hides must still be lifted — this is the pre-#931 behaviour.
	if r := findNestedRule(rules, "", "expandBehavior", "collapsible"); r == nil {
		t.Errorf("top-level expandBehavior rule lost; got %+v", rules)
	}

	if stats.Recognized == 0 {
		t.Error("no hide call recognized at all")
	}
}

// A nested condition can name the ITEM rather than the widget. Reading
// `item.initialCollapsedState` as a widget property looks up an absent key and
// reports nothing — which is exactly how the CE0463 for a non-default
// `initiallyCollapsed` stayed silent even once the nested rules existed.
func TestExtractNestedRuleItemScope(t *testing.T) {
	rules, _ := extractVisibilityRulesFromJS(accordionGetProperties)

	r := findNestedRule(rules, "groups", "initiallyCollapsed", "initialCollapsedState")
	if r == nil {
		t.Fatalf("no rule hiding groups/initiallyCollapsed on initialCollapsedState; got %+v", rules)
	}
	if r.HiddenWhen.Scope != types.ConditionScopeItem {
		t.Errorf("scope = %q, want %q — the condition reads the group, not the widget",
			r.HiddenWhen.Scope, types.ConditionScopeItem)
	}
	if r.HiddenWhen.Operator != "ne" || r.HiddenWhen.Value != "dynamic" {
		t.Errorf("condition = %s %q, want ne \"dynamic\"", r.HiddenWhen.Operator, r.HiddenWhen.Value)
	}
}

// A comparison guard obeys the connector's polarity. `"text" === mode ? hide(A) :
// hide(B)` hides A when the mode IS "text" and B when it is NOT; recording both
// as `eq` marks headerText hidden in the one configuration where it is used.
func TestExtractComparisonGuardPolarity(t *testing.T) {
	rules, _ := extractVisibilityRulesFromJS(accordionGetProperties)

	then := findNestedRule(rules, "groups", "headerContent", "headerRenderMode")
	if then == nil {
		t.Fatalf("no rule for groups/headerContent; got %+v", rules)
	}
	if then.HiddenWhen.Operator != "eq" {
		t.Errorf("headerContent: operator = %q, want eq (hidden in the THEN branch)", then.HiddenWhen.Operator)
	}

	els := findNestedRule(rules, "groups", "headerText", "headerRenderMode")
	if els == nil {
		t.Fatalf("no rule for groups/headerText; got %+v", rules)
	}
	if els.HiddenWhen.Operator != "ne" {
		t.Errorf("headerText: operator = %q, want ne (hidden in the ELSE branch)", els.HiddenWhen.Operator)
	}
}

// `X && Y || hide(...)` hides when `X && Y` is falsy, so "hide when Y falsy" is
// implied whatever X is. `X && Y && hide(...)` needs BOTH truthy, which one
// condition cannot express — emitting a rule there would over-fire.
func TestConjunctGuardOnlyLiftedForFalsyConnector(t *testing.T) {
	falsyForm := `f=function(e,r,t){return (e.a||"web"!==t)&&e.b||p.hidePropertiesIn(r,e,["k"]),r}`
	rules, _ := extractVisibilityRulesFromJS(falsyForm)
	r := findNestedRule(rules, "", "k", "b")
	if r == nil {
		t.Fatalf("`X && Y || hide` should yield 'hide when Y falsy'; got %+v", rules)
	}
	if r.HiddenWhen.Operator != "falsy" {
		t.Errorf("operator = %q, want falsy", r.HiddenWhen.Operator)
	}

	truthyForm := `f=function(e,r,t){return (e.a||"web"!==t)&&e.b&&p.hidePropertiesIn(r,e,["k"]),r}`
	rules, _ = extractVisibilityRulesFromJS(truthyForm)
	if r := findNestedRule(rules, "", "k", "b"); r != nil {
		t.Errorf("`X && Y && hide` must yield no rule (needs both operands); got %+v", *r.HiddenWhen)
	}
}
