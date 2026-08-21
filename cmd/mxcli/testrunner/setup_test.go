// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// @setup was parsed into the test case and read by nothing: no generator, no
// runner, no reporter. `@setup Mod.Seed` did no setup and said nothing about it.
// It now names a microflow called before the test's own statements.
func TestSetupIsParsedPerTestAndRepeatable(t *testing.T) {
	tests, err := parseMDLTests(`/**
 * @test two fixtures, in order
 * @setup eShop.ACT_SeedBrands
 * @setup eShop.ACT_SeedTypes
 */
retrieve $Brands from eShop.CatalogBrand;
/
`, "setup.test.mdl")
	if err != nil {
		t.Fatalf("parseMDLTests: %v", err)
	}
	if len(tests) != 1 {
		t.Fatalf("got %d test(s), want 1", len(tests))
	}
	want := []string{"eShop.ACT_SeedBrands", "eShop.ACT_SeedTypes"}
	if got := tests[0].Setups; !equalStrings(got, want) {
		t.Errorf("Setups = %v, want %v — repeating the annotation composes fixtures", got, want)
	}
}

// A file header's @setup applies to every test below it. This is the whole
// reason the annotation beats writing `call microflow X;` as the body's first
// line: one declaration for the file instead of copy-paste.
func TestSetupInFileHeaderAppliesToEveryTest(t *testing.T) {
	tests, err := parseMDLTests(`/**
 * Seeds every test in this file.
 * @setup eShop.ACT_SeedCatalog
 */

/**
 * @test uses the file fixture
 */
retrieve $Brands from eShop.CatalogBrand;
/

/**
 * @test adds one of its own
 * @setup eShop.ACT_SeedOneBrand
 */
retrieve $Brands from eShop.CatalogBrand;
/
`, "header.test.mdl")
	if err != nil {
		t.Fatalf("parseMDLTests: %v", err)
	}
	if len(tests) != 2 {
		t.Fatalf("got %d test(s), want 2", len(tests))
	}
	if got := tests[0].Setups; !equalStrings(got, []string{"eShop.ACT_SeedCatalog"}) {
		t.Errorf("tests[0].Setups = %v, want the file's fixture", got)
	}
	// The file's fixture runs first: it is the broader precondition, and a test's
	// own setup may depend on it.
	want := []string{"eShop.ACT_SeedCatalog", "eShop.ACT_SeedOneBrand"}
	if got := tests[1].Setups; !equalStrings(got, want) {
		t.Errorf("tests[1].Setups = %v, want %v", got, want)
	}
}

// A header that is really a test's own doc comment must not donate anything: the
// first javadoc in the file is only a header when it carries no @test.
func TestSetupOnFirstTestIsNotFileLevel(t *testing.T) {
	tests, err := parseMDLTests(`/**
 * @test the first test has its own setup
 * @setup eShop.ACT_SeedOne
 */
retrieve $Brands from eShop.CatalogBrand;
/

/**
 * @test the second test has none
 */
retrieve $Brands from eShop.CatalogBrand;
/
`, "not-header.test.mdl")
	if err != nil {
		t.Fatalf("parseMDLTests: %v", err)
	}
	if len(tests) != 2 {
		t.Fatalf("got %d test(s), want 2", len(tests))
	}
	if got := tests[0].Setups; !equalStrings(got, []string{"eShop.ACT_SeedOne"}) {
		t.Errorf("tests[0].Setups = %v", got)
	}
	if len(tests[1].Setups) != 0 {
		t.Errorf("tests[1].Setups = %v, want none — the first test's setup is not the file's",
			tests[1].Setups)
	}
}

// A header can only carry what a file-wide default can honour. @cleanup,
// @expect, @verify and @throws describe one test's execution, and a header that
// silently ignored them would be the same silent-absence bug this annotation is
// being fixed for.
func TestFileHeaderRefusesPerTestAnnotations(t *testing.T) {
	for _, tc := range []struct{ header, mentions string }{
		{" * @cleanup none", "@cleanup"},
		{" * @expect $x = 1", "@expect"},
		{" * @throws 'boom'", "@throws"},
	} {
		src := "/**\n * A file header.\n" + tc.header + "\n */\n\n/**\n * @test t\n */\ncall microflow M.A();\n/\n"
		_, err := parseMDLTests(src, "bad-header.test.mdl")
		if err == nil {
			t.Errorf("%s in a header: want an error, got none", tc.mentions)
			continue
		}
		if !strings.Contains(err.Error(), tc.mentions) {
			t.Errorf("error %q does not name %q", err, tc.mentions)
		}
	}
}

// The end of the chain: the setup is called before the test's statements, and
// the generated microflow parses.
func TestGenerateTestFlowsCallsSetupFirst(t *testing.T) {
	suite := &TestSuite{
		Name: "setup",
		Tests: []TestCase{{
			ID:      "test_1",
			Name:    "seeded",
			MDL:     "retrieve $Brands from eShop.CatalogBrand;",
			Setups:  []string{"eShop.ACT_SeedCatalog"},
			Expects: []Expect{expectOf("count($Brands) = 5")},
		}},
	}

	mdl := GenerateTestFlows(suite)
	if !strings.Contains(mdl, "CALL MICROFLOW eShop.ACT_SeedCatalog()") {
		t.Fatalf("the setup was not called:\n%s", mdl)
	}
	if strings.Index(mdl, "eShop.ACT_SeedCatalog") > strings.Index(mdl, "retrieve $Brands") {
		t.Errorf("the setup runs after the body it is supposed to prepare:\n%s", mdl)
	}
	if _, errs := visitor.Build(mdl); len(errs) > 0 {
		t.Fatalf("generated flow should parse, got %v\n%s", errs[0], mdl)
	}
}

// A setup that throws means the test never ran: it neither passed nor failed.
// Reporting that as a FAIL would blame the code under test for a broken fixture,
// which is the failure mode the annotation exists to prevent.
func TestSetupFailureIsReportedAsAnError(t *testing.T) {
	suite := &TestSuite{
		Name: "setup",
		Tests: []TestCase{{
			ID:     "test_1",
			Name:   "seeded",
			MDL:    "retrieve $Brands from eShop.CatalogBrand;",
			Setups: []string{"eShop.ACT_SeedCatalog"},
		}},
	}
	mdl := GenerateTestFlows(suite)
	if !strings.Contains(mdl, verdictSetupPrefix+"eShop.ACT_SeedCatalog") {
		t.Fatalf("no SETUP verdict on the error path:\n%s", mdl)
	}

	res := toResult(suite.Tests[0], &runResponse{
		OK:     true,
		Result: verdictSetupPrefix + "eShop.ACT_SeedCatalog",
	})
	if res.Status != StatusError {
		t.Errorf("status = %v, want ERROR — the test did not run", res.Status)
	}
	if !strings.Contains(res.Message, "eShop.ACT_SeedCatalog") {
		t.Errorf("message %q does not name the setup microflow", res.Message)
	}
}

// A @throws test gets its setup too: the fixture is not the thing expected to
// throw, and it must run before the verdict is pre-set to a failure.
func TestSetupRunsOnAThrowsTest(t *testing.T) {
	mdl := GenerateTestFlows(&TestSuite{
		Name: "setup",
		Tests: []TestCase{{
			ID:     "test_1",
			Name:   "rejects an empty order",
			MDL:    "call microflow eShop.ACT_Submit();",
			Setups: []string{"eShop.ACT_SeedCatalog"},
			Throws: "validation failed",
		}},
	})
	if !strings.Contains(mdl, "CALL MICROFLOW eShop.ACT_SeedCatalog()") {
		t.Fatalf("a @throws test lost its setup:\n%s", mdl)
	}
	if strings.Index(mdl, "eShop.ACT_SeedCatalog") > strings.Index(mdl, "expected an exception") {
		t.Errorf("the setup runs after the verdict is pre-set to a failure:\n%s", mdl)
	}
	if _, errs := visitor.Build(mdl); len(errs) > 0 {
		t.Fatalf("generated flow should parse, got %v\n%s", errs[0], mdl)
	}
}

// The legacy after-startup runner reports over the log protocol rather than a
// returned verdict, so it needs its own marker — and the parser has to know it,
// or a failed setup reads as "not executed".
func TestLegacyRunnerReportsSetupFailure(t *testing.T) {
	suite := &TestSuite{
		Name: "setup",
		Tests: []TestCase{{
			ID:     "test_1",
			Name:   "seeded",
			MDL:    "retrieve $Brands from eShop.CatalogBrand;",
			Setups: []string{"eShop.ACT_SeedCatalog"},
		}},
	}
	mdl := GenerateTestRunner(suite)
	if !strings.Contains(mdl, "CALL MICROFLOW eShop.ACT_SeedCatalog()") {
		t.Fatalf("the setup was not called:\n%s", mdl)
	}
	if !strings.Contains(mdl, "MXTEST:ERROR:test_1") {
		t.Fatalf("no ERROR line on the setup's error path:\n%s", mdl)
	}
	if _, errs := visitor.Build(mdl); len(errs) > 0 {
		t.Fatalf("generated runner should parse, got %v\n%s", errs[0], mdl)
	}

	log := strings.NewReader(
		"MXTEST: MXTEST:START:setup\n" +
			"MXTEST: MXTEST:RUN:test_1:seeded\n" +
			"MXTEST: MXTEST:ERROR:test_1:Setup failed: eShop.ACT_SeedCatalog\n" +
			"MXTEST: MXTEST:END:setup\n")
	sr := ParseLogResults(log, suite, false)
	if len(sr.Tests) != 1 {
		t.Fatalf("got %d result(s), want 1", len(sr.Tests))
	}
	if sr.Tests[0].Status != StatusError {
		t.Errorf("status = %v, want ERROR", sr.Tests[0].Status)
	}
	if !strings.Contains(sr.Tests[0].Message, "eShop.ACT_SeedCatalog") {
		t.Errorf("message %q does not name the setup microflow", sr.Tests[0].Message)
	}
	if sr.PassCount() != 0 || sr.AllPassed() {
		t.Errorf("a failed setup reported as passing: pass=%d allPassed=%v",
			sr.PassCount(), sr.AllPassed())
	}
}

// Control: a test with no @setup generates what it generated before. The whole
// feature must be invisible to a file that does not use it.
func TestNoSetupChangesNothing(t *testing.T) {
	tc := TestCase{
		ID:      "test_1",
		Name:    "plain",
		MDL:     "$r = CALL MICROFLOW M.A();",
		Expects: []Expect{expectOf("$r = 1")},
	}
	for _, mdl := range []string{
		GenerateTestFlows(&TestSuite{Name: "s", Tests: []TestCase{tc}}),
		GenerateTestRunner(&TestSuite{Name: "s", Tests: []TestCase{tc}}),
	} {
		if strings.Contains(mdl, "mxtest_setup") || strings.Contains(mdl, verdictSetupPrefix) ||
			strings.Contains(mdl, "MXTEST:ERROR:") {
			t.Errorf("a test with no setup grew setup scaffolding:\n%s", mdl)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
