// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// #927 bug 2: `@expect count($Brands) = 999` on a body that only retrieves the
// list reported PASS against an empty table, for any value of 999.
//
// count() is not a Mendix expression function — counting a list is an Aggregate
// list activity — so the condition could never be compiled into the decision
// that evaluates the assertion. The annotation was dropped, and a test with no
// assertions passes as long as its body does not throw.
//
// The aggregate is now lifted into the activity the author would have written by
// hand, so the assertion is really evaluated.
func TestParseExpectCountIsHoistedToAnAggregate(t *testing.T) {
	exp, err := ParseExpect("count($Brands) = 5")
	if err != nil {
		t.Fatalf("ParseExpect: %v", err)
	}
	if len(exp.Aggregates) != 1 {
		t.Fatalf("got %d aggregate(s), want 1: %+v", len(exp.Aggregates), exp)
	}
	agg := exp.Aggregates[0]
	if agg.Op != "COUNT" || agg.List != "$Brands" {
		t.Errorf("aggregate = %+v, want COUNT over $Brands", agg)
	}
	if exp.Condition != agg.Var+" = 5" {
		t.Errorf("Condition = %q, want the aggregate variable compared with 5", exp.Condition)
	}
	if exp.Raw != "count($Brands) = 5" {
		t.Errorf("Raw = %q, want the annotation as written (it is the failure message)", exp.Raw)
	}
	if exp.Actual != "toString("+agg.Var+")" {
		t.Errorf("Actual = %q, want the observed count rendered for the failure message", exp.Actual)
	}
}

// Two assertions over the same list share one variable, so the list is counted
// once — and two different lists get two variables.
func TestParseExpectCountVariablesAreStableAndDistinct(t *testing.T) {
	both, err := ParseExpect("count($Brands) > 0 and count($Types) = 4")
	if err != nil {
		t.Fatalf("ParseExpect: %v", err)
	}
	if len(both.Aggregates) != 2 {
		t.Fatalf("got %d aggregate(s), want 2: %+v", len(both.Aggregates), both.Aggregates)
	}
	if both.Aggregates[0].Var == both.Aggregates[1].Var {
		t.Errorf("two lists share a variable: %+v", both.Aggregates)
	}

	same, err := ParseExpect("count($Brands) > 0 and count($Brands) < 10")
	if err != nil {
		t.Fatalf("ParseExpect: %v", err)
	}
	if len(same.Aggregates) != 1 {
		t.Errorf("counting one list twice produced %d aggregates, want 1", len(same.Aggregates))
	}
}

// The remaining aggregates need an attribute to aggregate over, which an
// assertion cannot supply. They are refused — and the message has to name the
// way out, because the old one ("not a Mendix expression function") described
// the implementation rather than the fix.
func TestParseExpectOtherAggregatesAreRefusedWithGuidance(t *testing.T) {
	for _, name := range []string{"sum", "average", "minimum", "maximum"} {
		_, err := ParseExpect(name + "($Orders) = 10")
		if err == nil {
			t.Errorf("%s(): want an error, got none", name)
			continue
		}
		if !strings.Contains(err.Error(), "microflow") {
			t.Errorf("%s(): error %q does not point at the helper-microflow workaround",
				name, err)
		}
	}
}

// count() over anything but a list variable cannot be turned into the activity,
// so it is an error rather than a silently dropped assertion.
func TestParseExpectCountRejectsNonListArguments(t *testing.T) {
	for _, raw := range []string{
		"count($order/Lines) = 2",
		"count('literal') = 1",
		"count() = 0",
	} {
		if _, err := ParseExpect(raw); err == nil {
			t.Errorf("%q: want an error, got none", raw)
		}
	}
}

// A bare count is a value, not a pass/fail condition — the same rule that
// rejects `@expect $result` without a comparison.
func TestParseExpectBareCountIsNotACondition(t *testing.T) {
	if _, err := ParseExpect("count($Brands)"); err == nil {
		t.Error("want an error: a count is a value, not a condition")
	}
}

// The end of the chain: the generated microflow must compute the count before
// the decision that reads it, and it must parse.
func TestGenerateTestFlowsEmitsCountAggregate(t *testing.T) {
	suite := &TestSuite{
		Name: "counts",
		Tests: []TestCase{{
			ID:      "test_1",
			Name:    "exactly 5 brands are seeded",
			MDL:     "retrieve $Brands from eShop.CatalogBrand;",
			Expects: []Expect{expectOf("count($Brands) = 5")},
			Cleanup: CleanupNone,
		}},
	}

	mdl := GenerateTestFlows(suite)
	agg := suite.Tests[0].Expects[0].Aggregates[0]
	activity := agg.Var + " = COUNT($Brands);"
	if !strings.Contains(mdl, activity) {
		t.Fatalf("generated flow is missing %q:\n%s", activity, mdl)
	}
	if strings.Index(mdl, activity) > strings.Index(mdl, "IF "+agg.Var) {
		t.Errorf("the count is computed after the decision that reads it:\n%s", mdl)
	}
	if !strings.Contains(mdl, "retrieve $Brands from eShop.CatalogBrand;") {
		t.Errorf("the body was dropped:\n%s", mdl)
	}
	if _, errs := visitor.Build(mdl); len(errs) > 0 {
		t.Fatalf("generated flow should parse, got %v\n%s", errs[0], mdl)
	}
}

// The monolithic runner compiles every test into one microflow, so a generated
// aggregate variable has to be renamed per test exactly like the body's own —
// otherwise two tests counting a list of the same name declare it twice.
func TestGenerateTestRunnerRenamesAggregateVariables(t *testing.T) {
	one := expectOf("count($List) = 1")
	two := expectOf("count($List) = 2")
	suite := &TestSuite{
		Name: "counts",
		Tests: []TestCase{
			{ID: "test_1", Name: "first", MDL: "$List = CREATE LIST OF MfTest.Product;", Expects: []Expect{one}},
			{ID: "test_2", Name: "second", MDL: "$List = CREATE LIST OF MfTest.Product;", Expects: []Expect{two}},
		},
	}

	mdl := GenerateTestRunner(suite)
	for _, want := range []string{
		"$mxtest_count_List_1 = COUNT($List_1);",
		"$mxtest_count_List_2 = COUNT($List_2);",
	} {
		if !strings.Contains(mdl, want) {
			t.Errorf("missing %q:\n%s", want, mdl)
		}
	}
	if strings.Contains(mdl, "$mxtest_count_List =") {
		t.Errorf("an un-suffixed aggregate variable collides across tests:\n%s", mdl)
	}
	if _, errs := visitor.Build(mdl); len(errs) > 0 {
		t.Fatalf("generated runner should parse, got %v\n%s", errs[0], mdl)
	}
}

// Fail-closed control, from the file down: an aggregate this package cannot
// compile must make the test an ERROR and must not produce a microflow. That is
// what stopped bug 2's false PASS — a dropped assertion is indistinguishable
// from a passing one — and it has to keep holding for the aggregates count()
// support does not cover.
func TestUncompilableAggregateExpectIsAnErrorNotAPass(t *testing.T) {
	tests, err := parseMDLTests(`/**
 * @test asserts something impossible
 * @expect sum($Orders) = 1
 */
retrieve $Orders from eShop.Order;
/
`, "bogus.test.mdl")
	if err != nil {
		t.Fatalf("parseMDLTests: %v", err)
	}
	if len(tests) != 1 {
		t.Fatalf("got %d test(s), want 1", len(tests))
	}
	tc := tests[0]
	if len(tc.AssertionErrors) == 0 {
		t.Fatal("the assertion was dropped silently — the test would report PASS")
	}
	if tc.AssertionCount() != 0 {
		t.Errorf("AssertionCount = %d, want 0: nothing here can be evaluated", tc.AssertionCount())
	}
	res, ok := assertionErrorResult(tc)
	if !ok || res.Status != StatusError {
		t.Errorf("result = %+v (ok=%v), want an ERROR", res, ok)
	}
	if mdl := GenerateTestFlows(&TestSuite{Name: "s", Tests: []TestCase{tc}}); strings.Contains(mdl, "Test_test_1") {
		t.Errorf("a test that cannot assert got a microflow:\n%s", mdl)
	}
}
