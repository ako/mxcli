// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// The guard vocabulary editorConfig actually uses. Each shape below was found
// unrecognised in a real marketplace widget, and each is stated with the
// polarity flip that a `||` connector or a ternary ELSE branch applies — a
// shape that lifts under `&&` and inverts wrongly under `||` produces a rule
// that hides a property in exactly the configuration the editor shows it.
func TestGuardShapes(t *testing.T) {
	tests := []struct {
		name  string
		guard string
		falsy bool
		key   string
		op    string
		value string
	}{
		// `null === x` is "the author picked no datasource / no action". It is
		// NOT falsy: "false" and "0" are values a real property can hold.
		{"null on the left", `null===t.optionsSourceAssociationDataSource`, false, "optionsSourceAssociationDataSource", "empty", ""},
		{"null on the right", `t.someDataSource===null`, false, "someDataSource", "empty", ""},
		{"null inverted", `null===t.someDataSource`, true, "someDataSource", "notempty", ""},
		{"not-null", `null!==t.someDataSource`, false, "someDataSource", "notempty", ""},

		// terser writes `false` as `!1` and `true` as `!0`. Mapped to eq/ne
		// against the literal, not to falsy/truthy: `===false` does not fire on
		// an unset property, and falsy would.
		{"minified false", `!1===t.showFooter`, false, "showFooter", "eq", "false"},
		{"minified true", `!0===t.showFooter`, false, "showFooter", "eq", "true"},
		{"minified false inverted", `!1===t.showFooter`, true, "showFooter", "ne", "false"},
		{"minified false on the right", `t.showFooter===!1`, false, "showFooter", "eq", "false"},

		// `0 === x.length` is the same "nothing picked" claim about a list or
		// string. The property is x, not x.length.
		{"empty length", `0===t.databaseAttributeString.length`, false, "databaseAttributeString", "empty", ""},
		{"empty length inverted", `0===t.databaseAttributeString.length`, true, "databaseAttributeString", "notempty", ""},
		{"non-empty length", `t.databaseAttributeString.length!==0`, false, "databaseAttributeString", "notempty", ""},

		// set membership over enum values
		{"includes", `["enumeration","boolean"].includes(t.optionsSourceType)`, false, "optionsSourceType", "in", "enumeration,boolean"},
		{"includes inverted", `["enumeration","boolean"].includes(t.optionsSourceType)`, true, "optionsSourceType", "notin", "enumeration,boolean"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, ok := guardToCondition(tc.guard, tc.falsy, map[string]string{}, "")
			if !ok {
				t.Fatalf("guard not recognised: %s", tc.guard)
			}
			if c.PropertyKey != tc.key || c.Operator != tc.op || c.Value != tc.value {
				t.Errorf("got %s %s %q, want %s %s %q",
					c.PropertyKey, c.Operator, c.Value, tc.key, tc.op, tc.value)
			}
		})
	}
}

// A shape outside the vocabulary must yield NO rule rather than a guessed one:
// a wrong rule hides a property the author is using, which is the failure this
// whole area keeps producing. This is the control for the table above — without
// it, a `guardToCondition` that returned a rule for everything would pass.
func TestGuardShapesRefuseWhatTheyCannotRead(t *testing.T) {
	for _, guard := range []string{
		`t.items.filter(function(i){return i.on}).length>2`, // computed
		`3===t.count`,                    // a number we do not model
		`null===someLocal`,               // unresolvable bare identifier
		`["a"].includes(t.x)&&t.y`,       // compound
		`0===t.a.length&&0===t.b.length`, // compound
	} {
		if c, ok := guardToCondition(guard, false, map[string]string{}, ""); ok {
			t.Errorf("guard %q produced a rule (%s %s %q); it should be refused",
				guard, c.PropertyKey, c.Operator, c.Value)
		}
	}
}

// A hide that is not the FIRST in a comma group is guarded by its sibling's
// condition: `cond ? (hide(a), hide(b))` hides b under cond just as much as a.
// Attributing only the first left the rest reading as always visible.
func TestCommaGroupSiblingsShareTheGuard(t *testing.T) {
	js := `function getProperties(e,r,t){` +
		`return"text"===e.headerType?(M.hidePropertyIn(r,e,"headerContent"),M.hidePropertyIn(r,e,"openNodeOn")):0,r}`

	rules, _ := extractVisibilityRulesFromJS(js)
	got := map[string]string{}
	for _, r := range rules {
		if r.HiddenWhen != nil {
			got[r.PropertyKey] = r.HiddenWhen.PropertyKey + " " + r.HiddenWhen.Operator + " " + r.HiddenWhen.Value
		}
	}
	for _, key := range []string{"headerContent", "openNodeOn"} {
		if got[key] != `headerType eq text` {
			t.Errorf("%s: got %q, want %q", key, got[key], "headerType eq text")
		}
	}
}

// The walk must not cross a comma that is not a group separator. In
// `A ? B : hide(x), hide(y)` the text before hide(y) is a ternary ELSE branch,
// and attributing y to A is simply wrong — Maps hides `advanced` in exactly
// that position, and the mis-read made it "hidden when geodecodeApiKey is not
// set", a condition with nothing to do with it.
func TestCommaWalkStopsOutsideAGroup(t *testing.T) {
	js := `function getProperties(B,F,A){` +
		`return B.geodecodeApiKey?u.hidePropertyIn(F,B,"geodecodeApiKeyExp"):u.hidePropertyIn(F,B,"geodecodeApiKey"),` +
		`u.hidePropertyIn(F,B,"advanced"),F}`

	rules, _ := extractVisibilityRulesFromJS(js)
	for _, r := range rules {
		if r.PropertyKey == "advanced" {
			t.Errorf("attributed `advanced` to a guard it does not have: %s %s %q",
				r.HiddenWhen.PropertyKey, r.HiddenWhen.Operator, r.HiddenWhen.Value)
		}
	}
	// Control: the two properties that DO have that guard still get it, so the
	// test is not passing because extraction stopped altogether.
	var seen int
	for _, r := range rules {
		if r.PropertyKey == "geodecodeApiKey" || r.PropertyKey == "geodecodeApiKeyExp" {
			seen++
		}
	}
	if seen != 2 {
		t.Errorf("expected the two guarded rules to survive, got %d", seen)
	}
}

// A guard inside `outer ? ( … inner && hide(x) … )` states only the INNER term.
// Storing that alone claims hidden wherever inner holds, outer or not — which
// is wrong whenever outer is false. The rule must carry both terms.
func TestConjunctionCarriesEveryTerm(t *testing.T) {
	js := `function getProperties(t,e){` +
		`return"context"===e.source?(x.hidePropertiesIn(t,e,["a"]),!1===e.showFooter&&x.hidePropertiesIn(t,e,["menuFooterContent"])):0,t}`

	rules, _ := extractVisibilityRulesFromJS(js)
	var got *types.WidgetVisibilityRule
	for i := range rules {
		if rules[i].PropertyKey == "menuFooterContent" {
			got = &rules[i]
		}
	}
	if got == nil {
		t.Fatal("no rule for menuFooterContent")
	}
	conds := got.Conditions()
	if len(conds) != 2 {
		t.Fatalf("want 2 conditions, got %d: %+v", len(conds), conds)
	}
	// It must not fire on the inner term alone.
	fires, determinable := got.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
		return map[string]string{"showFooter": "false", "source": "database"}[c.PropertyKey], true
	})
	if !determinable || fires {
		t.Error("fired with source=database: the outer term was dropped")
	}
	// And it must fire when both hold — the control, without which the test
	// would pass against a rule that never fires at all.
	fires, determinable = got.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
		return map[string]string{"showFooter": "false", "source": "context"}[c.PropertyKey], true
	})
	if !determinable || !fires {
		t.Error("did not fire with both terms satisfied")
	}
}

// A conjunction is only as decidable as its least decidable term. One unknown
// value makes the whole rule indeterminable, so callers keep asking for the
// binding rather than guessing.
func TestConjunctionIsIndeterminableIfAnyTermIs(t *testing.T) {
	c1 := types.WidgetVisibilityCondition{PropertyKey: "a", Operator: "eq", Value: "1"}
	c2 := types.WidgetVisibilityCondition{PropertyKey: "b", Operator: "eq", Value: "2"}
	r := types.WidgetVisibilityRule{PropertyKey: "x", HiddenWhen: &c1, And: []types.WidgetVisibilityCondition{c2}}

	_, determinable := r.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
		if c.PropertyKey == "a" {
			return "1", true
		}
		return "", false // b unknown
	})
	if determinable {
		t.Error("reported determinable with an unknown term")
	}
	// Control: known values make it decidable and firing.
	fires, determinable := r.Fires(func(c types.WidgetVisibilityCondition) (string, bool) {
		return map[string]string{"a": "1", "b": "2"}[c.PropertyKey], true
	})
	if !determinable || !fires {
		t.Error("did not fire when every term is known and satisfied")
	}
}

// The platform argument is not part of the widget's configuration: TreeNode
// hides its icon properties under `"web"===platform ? (e.advancedMode || …)`,
// and MDL only writes web pages. Folding that term away keeps the rule; keeping
// it would make the rule indeterminable and over-list three bindings.
func TestPlatformTermIsNotAConfigurationConjunct(t *testing.T) {
	js := `function getProperties(e,r,t){` +
		`return"web"===t?(M.transformGroupsIntoTabs(r),e.advancedMode||M.hidePropertiesIn(r,e,["showIcon","animate"])):M.hidePropertyIn(r,e,"advancedMode"),r}`

	rules, _ := extractVisibilityRulesFromJS(js)
	for _, want := range []string{"showIcon", "animate"} {
		var found bool
		for _, r := range rules {
			if r.PropertyKey == want && r.HiddenWhen != nil &&
				r.HiddenWhen.PropertyKey == "advancedMode" && r.HiddenWhen.Operator == "falsy" {
				found = true
				if len(r.And) != 0 {
					t.Errorf("%s carries a platform term: %+v", want, r.And)
				}
			}
		}
		if !found {
			t.Errorf("lost the rule for %s", want)
		}
	}
}

// The English rendering must show every term. Reading out only the innermost
// makes a narrow rule look like a broad one, which is how a reader concludes a
// property is hidden far more often than it is.
func TestRuleTextJoinsEveryTerm(t *testing.T) {
	r := types.WidgetVisibilityRule{
		PropertyKey: "x",
		HiddenWhen:  &types.WidgetVisibilityCondition{PropertyKey: "showFooter", Operator: "eq", Value: "false"},
		And: []types.WidgetVisibilityCondition{
			{PropertyKey: "source", Operator: "eq", Value: "context"},
		},
	}
	got := ruleText(r)
	for _, want := range []string{`showFooter = "false"`, " and ", `source = "context"`} {
		if !strings.Contains(got, want) {
			t.Errorf("rendering %q is missing %q", got, want)
		}
	}
}

// A conjunction the walk cannot read is a reason to WITHHOLD a rule this
// extractor did not previously produce — emitting one states a single conjunct
// and over-fires. It is not a reason to drop a rule the older, single-condition
// vocabulary already lifted: that rule's accuracy is unchanged by conjunction
// support, and removing it loses detection the previous release had.
//
// This is the guarantee that failed silently once. The sweep that claimed "zero
// rules lost" listed widgets from their .def.json files, and Combo box — the
// widget the work was justified by — has none, so it was never measured. Six of
// its rules had been dropped.
func TestConjunctionNeverDropsAPreviouslyLiftedRule(t *testing.T) {
	// Combo box's REAL editorConfig, not a synthetic stand-in. A hand-written
	// snippet of the same apparent shape did NOT reproduce this — it was never
	// flagged conjunctive, so the test passed with the fix reverted and proved
	// nothing. The nesting that triggers it is three levels deep and specific.
	js, err := mpk.ReadEditorConfig(
		"../../testdata/expr-checker/widgets/com.mendix.widget.web.Combobox.mpk",
		"com.mendix.widget.web.combobox.Combobox")
	if err != nil || js == "" {
		t.Fatalf("read Combo box editorConfig: %v", err)
	}

	rules, _ := extractVisibilityRulesFromJS(js)
	got := map[string]bool{}
	for _, r := range rules {
		got[r.PropertyKey] = true
	}

	// Each of these was lifted by the older, single-condition vocabulary and sits
	// inside a group whose guard is about the same object. Conjunction support
	// must not cost them: their accuracy is unchanged by this work, and dropping
	// them loses detection the previous release had.
	//
	// This guarantee failed once, silently. The sweep that claimed "zero rules
	// lost" enumerated widgets from their .def.json files, and Combo box has
	// none — it is described straight from the .mpk — so the widget the work was
	// justified by was never in the sample. Six rules had gone.
	for _, want := range []string{
		"databaseAttributeString",
		"optionsSourceAssociationCaptionExpression",
		"optionsSourceAssociationCustomContent",
		"optionsSourceDatabaseCaptionExpression",
		"optionsSourceDatabaseCustomContent",
		"optionsSourceDatabaseValueAttribute",
	} {
		if !got[want] {
			t.Errorf("dropped %s: a rule the single-condition vocabulary already lifted", want)
		}
	}
}

// The other half of that distinction: a NEWLY supported guard shape in the same
// position IS withheld, because emitting it would ADD an over-firing rule rather
// than preserve an existing one. Datagrid hides pagingPosition only when
// pagination is off AND the row count is false, and pagination defaults on.
func TestConjunctionWithholdsANewShapeItCannotFullyRead(t *testing.T) {
	js := `function getProperties(t,e){` +
		`return e.pagination?x.hidePropertiesIn(t,e,["showNumberOfRows"]):` +
		`(x.hidePropertiesIn(t,e,["showPagingButtons"]),!1===e.showNumberOfRows&&x.hidePropertiesIn(t,e,["pagingPosition"])),t}`

	rules, _ := extractVisibilityRulesFromJS(js)
	for _, r := range rules {
		if r.PropertyKey == "pagingPosition" && len(r.And) == 0 {
			t.Errorf("emitted pagingPosition with only one conjunct (%s %s %q): "+
				"it is hidden when pagination is OFF *and* the row count is false, "+
				"and pagination defaults on",
				r.HiddenWhen.PropertyKey, r.HiddenWhen.Operator, r.HiddenWhen.Value)
		}
	}
	// Control: the sibling hide in the same group, whose guard the walk CAN
	// attribute, is still lifted — so the test is not passing because extraction
	// stopped altogether.
	var sawSibling bool
	for _, r := range rules {
		if r.PropertyKey == "showPagingButtons" || r.PropertyKey == "showNumberOfRows" {
			sawSibling = true
		}
	}
	if !sawSibling {
		t.Error("no rule survived at all; the withholding is too broad")
	}
}

// A chained ternary is an else-if ladder: each branch's BODY is parenthesised,
// each branch's CONDITION is not. Combo box nests two of them:
//
//	"context"===t.source ? ( …,
//	      ["enumeration","boolean"].includes(t.optionsSourceType) ? ( … )
//	    : "association"===t.optionsSourceType && ( …hides… ) )
//	: "database"===t.source ? ( … )
//
// The `&&` group's own condition is the operand immediately left of the `&&`.
// Reading back to the nearest STATEMENT separator instead returns the whole
// `A ? (…) : B` expression, which is not a comparison — so the chain read as
// unreadable and these rules went unlifted.
//
// Real .mpk, not a reconstruction: the shape that triggers this is specific
// enough that a hand-written stand-in did not reproduce it last time.
func TestChainedTernaryBranchConditionIsLifted(t *testing.T) {
	js, err := mpk.ReadEditorConfig(
		"../../testdata/expr-checker/widgets/com.mendix.widget.web.Combobox.mpk",
		"com.mendix.widget.web.combobox.Combobox")
	if err != nil || js == "" {
		t.Fatalf("read Combo box editorConfig: %v", err)
	}
	rules, _ := extractVisibilityRulesFromJS(js)

	// Each of these sits inside the `"association"===optionsSourceType && (…)`
	// group, itself the else branch of a ternary inside the `"context"===source`
	// branch. The rule must carry all three terms — its own, the `&&` operand,
	// and the outer branch — or it claims hidden where the editor shows it.
	want := map[string][]string{
		"menuFooterContent":                   {"showFooter", "optionsSourceType", "source"},
		"selectAllButtonCaption":              {"selectAllButton", "optionsSourceType", "source"},
		"optionsSourceAssociationCaptionType": {"optionsSourceAssociationDataSource", "optionsSourceType", "source"},
	}
	for prop, terms := range want {
		var found bool
		for _, r := range rules {
			if r.PropertyKey != prop || r.HiddenWhen == nil || r.HiddenWhen.PropertyKey != terms[0] {
				continue
			}
			found = true
			got := map[string]bool{}
			for _, c := range r.Conditions() {
				got[c.PropertyKey] = true
			}
			for _, term := range terms {
				if !got[term] {
					t.Errorf("%s: conjunction is missing %q (has %v)", prop, term, got)
				}
			}
		}
		if !found {
			t.Errorf("no rule lifted for %s guarded by %s", prop, terms[0])
		}
	}
}
