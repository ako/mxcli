// SPDX-License-Identifier: Apache-2.0

// A hide guarded by the THEN-branch of a ternary, with a second condition:
//
//	e.showTooltip ? "value" === e.tooltipType && hidePropertyIn(t, e, "tooltip")
//	              : hidePropertiesIn(t, e, ["tooltip", "tooltipType"])
//
// parseGuard walks back from the call, strips the `&&`, and is left with
// `e.showTooltip?"value"===e.tooltipType` — not a comparison it can parse — so
// the whole call was skipped and only the ELSE branch produced a rule.
//
// The property is then recorded as hidden only when showTooltip is falsy. With
// the Slider's defaults (showTooltip true, tooltipType "value") it IS hidden,
// the rule disagrees, and ApplyVisibilityRules takes its *visible* branch and
// fills an empty ClientTemplate where Studio Pro stores null — CE0463 (#238).
package executor

import "testing"

func rulesByKey(t *testing.T, js string) map[string][]string {
	t.Helper()
	rules, _ := extractVisibilityRulesFromJS(js)
	out := map[string][]string{}
	for _, r := range rules {
		if r.HiddenWhen == nil {
			continue
		}
		out[r.PropertyKey] = append(out[r.PropertyKey],
			r.HiddenWhen.PropertyKey+" "+r.HiddenWhen.Operator+" "+r.HiddenWhen.Value)
	}
	return out
}

// Verbatim shape from Slider 3.x. Both branches hide `tooltip`, so the two rules
// are a correct disjunction: hidden ⟺ ¬showTooltip ∨ tooltipType=="value".
func TestExtractVisibility_TernaryThenBranchWithSecondCondition(t *testing.T) {
	js := `exports.getProperties=function(e,t){return e.showTooltip?"value"===e.tooltipType&&a.hidePropertyIn(t,e,"tooltip"):a.hidePropertiesIn(t,e,["tooltip","tooltipType"]),t}`

	got := rulesByKey(t, js)

	if !hasCond(got["tooltip"], "showTooltip falsy ") {
		t.Errorf("the else-branch rule was lost: %v", got["tooltip"])
	}
	if !hasCond(got["tooltip"], "tooltipType eq value") {
		t.Errorf("the then-branch's own condition was not lifted, so a tooltip hidden by the widget's DEFAULTS reads as visible: %v", got["tooltip"])
	}
}

// The same shape WITHOUT an else-branch hide of that property. Here
// hidden ⟺ A ∧ B, which one condition cannot express — recording B alone would
// hide the property whenever B holds, including when A does not. Skipping is
// safe (it degrades to "not hidden"); guessing is not.
func TestExtractVisibility_TernaryThenBranchWithoutElseCoverIsSkipped(t *testing.T) {
	js := `exports.getProperties=function(e,t){return e.advanced?"x"===e.mode&&a.hidePropertyIn(t,e,"lonely"):null,t}`

	got := rulesByKey(t, js)

	if conds, ok := got["lonely"]; ok {
		t.Errorf("a conjunction was recorded as a single condition %v — the property would be pruned "+
			"whenever mode==\"x\", even with advanced off, which is CE0463's mirror image", conds)
	}
}

func hasCond(got []string, want string) bool {
	for _, g := range got {
		if g == want {
			return true
		}
	}
	return false
}
