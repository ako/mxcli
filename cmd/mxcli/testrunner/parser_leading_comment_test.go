// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"
)

// #927 bug 1: a file-level javadoc comment before the first test made that test
// vanish from the suite — not as a pass, not as a fail, not as an error. Every
// later test then reported under the number of the one above it.
//
// The cause was `extractDocAndBody` taking the FIRST `/**` in a '/'-separated
// chunk as the test's doc comment. A file header is not separated from the test
// below it by a '/', so both live in one chunk: the header became the doc, it
// carried no @test, and the whole chunk — the real test included — was dropped.
func TestParseMDLTestsLeadingFileComment(t *testing.T) {
	content := `/**
 * File-level description with no statement of its own.
 */

/**
 * @test exactly 5 brands are seeded
 * @expect $BrandCount = 5
 * @cleanup none
 */
$BrandCount = CALL MICROFLOW eShop.QRY_CatalogBrandCount();
/

/**
 * @test exactly 4 types are seeded
 * @expect $TypeCount = 4
 * @cleanup none
 */
$TypeCount = CALL MICROFLOW eShop.QRY_CatalogTypeCount();
/
`
	tests, err := parseMDLTests(content, "repro.test.mdl")
	if err != nil {
		t.Fatalf("parseMDLTests: %v", err)
	}
	if len(tests) != 2 {
		t.Fatalf("got %d test(s), want 2 — the header swallowed one: %+v", len(tests), tests)
	}
	if tests[0].Name != "exactly 5 brands are seeded" {
		t.Errorf("tests[0].Name = %q, want the first test", tests[0].Name)
	}
	if len(tests[0].Expects) != 1 {
		t.Errorf("tests[0] has %d @expect(s), want 1", len(tests[0].Expects))
	}
	if tests[1].Name != "exactly 4 types are seeded" {
		t.Errorf("tests[1].Name = %q, want the second test", tests[1].Name)
	}
}

// The reported line must be the test's own, not the header's. Before the fix the
// position came from a substring search for the chunk's first 20 characters,
// which found the header — so both tests in the file above reported line 5.
func TestParseMDLTestsReportsEachTestsOwnLine(t *testing.T) {
	content := `-- a line comment header
/**
 * @test first
 */
CALL MICROFLOW M.A();
/

/**
 * @test second
 */
CALL MICROFLOW M.B();
/
`
	tests, err := parseMDLTests(content, "lines.test.mdl")
	if err != nil {
		t.Fatalf("parseMDLTests: %v", err)
	}
	if len(tests) != 2 {
		t.Fatalf("got %d test(s), want 2", len(tests))
	}
	for i, want := range []int{2, 8} {
		if tests[i].Line != want {
			t.Errorf("tests[%d] (%q) reported line %d, want %d",
				i, tests[i].Name, tests[i].Line, want)
		}
	}
}

// #927 bug 1b: the fusion came back when a `--` comment's PROSE spelled out the
// two javadoc delimiters — the workaround for bug 1 was to describe the bug in a
// line comment, and describing it re-triggered it. Comment delimiters were found
// by raw substring search, with no notion of already being inside a line comment.
func TestParseMDLTestsLineCommentMentioningJavadocDelimiters(t *testing.T) {
	content := `-- Do not put a /** ... */ docblock at the top of this file.
/**
 * @test the line comment above is prose, not a doc comment
 * @expect $n = 1
 */
$n = CALL MICROFLOW M.Count();
/
`
	tests, err := parseMDLTests(content, "prose.test.mdl")
	if err != nil {
		t.Fatalf("parseMDLTests: %v", err)
	}
	if len(tests) != 1 {
		t.Fatalf("got %d test(s), want 1 — the prose was read as a doc comment: %+v",
			len(tests), tests)
	}
	if tests[0].Name != "the line comment above is prose, not a doc comment" {
		t.Errorf("Name = %q", tests[0].Name)
	}
	if !strings.Contains(tests[0].MDL, "CALL MICROFLOW M.Count()") {
		t.Errorf("MDL = %q, want the body below the doc comment", tests[0].MDL)
	}
}

// The body must start after the doc comment, not after the file header — a
// header-only chunk must not donate its text to the test below it.
func TestParseMDLTestsBodyExcludesLeadingComments(t *testing.T) {
	content := `/** header */
/**
 * @test body boundary
 */
CALL MICROFLOW M.A();
/
`
	tests, err := parseMDLTests(content, "body.test.mdl")
	if err != nil {
		t.Fatalf("parseMDLTests: %v", err)
	}
	if len(tests) != 1 {
		t.Fatalf("got %d test(s), want 1", len(tests))
	}
	if tests[0].MDL != "CALL MICROFLOW M.A();" {
		t.Errorf("MDL = %q, want just the statement", tests[0].MDL)
	}
}

// Two @test doc comments in one chunk means a '/' separator is missing. Taking
// the last one would run the second test and drop the first exactly as bug 1
// did, so this is refused instead: an author who cannot see a test cannot tell a
// dropped one from a passing one.
func TestParseMDLTestsRefusesTwoTestsInOneBlock(t *testing.T) {
	content := `/**
 * @test first, whose separator is missing
 */
CALL MICROFLOW M.A();

/**
 * @test second
 */
CALL MICROFLOW M.B();
/
`
	_, err := parseMDLTests(content, "missing-separator.test.mdl")
	if err == nil {
		t.Fatal("want an error naming the missing '/' separator, got none")
	}
	for _, want := range []string{"first, whose separator is missing", "/"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// False-positive control: a file with no leading comment at all must still parse
// exactly as before — the fix must not change where a plain test's doc, body or
// line come from.
func TestParseMDLTestsNoLeadingCommentUnchanged(t *testing.T) {
	content := `/**
 * @test plain
 * @expect $r = true
 */
$r = CALL MICROFLOW M.A();
/
`
	tests, err := parseMDLTests(content, "plain.test.mdl")
	if err != nil {
		t.Fatalf("parseMDLTests: %v", err)
	}
	if len(tests) != 1 {
		t.Fatalf("got %d test(s), want 1", len(tests))
	}
	tc := tests[0]
	if tc.Name != "plain" || tc.Line != 1 || tc.MDL != "$r = CALL MICROFLOW M.A();" {
		t.Errorf("got name=%q line=%d mdl=%q", tc.Name, tc.Line, tc.MDL)
	}
	if len(tc.Expects) != 1 {
		t.Errorf("got %d @expect(s), want 1", len(tc.Expects))
	}
}

// A chunk that is only a comment — a file header followed by its own '/' — is
// not a test and must not become an error either.
func TestParseMDLTestsStandaloneCommentBlock(t *testing.T) {
	content := `/**
 * Suite header, terminated on its own.
 */
/

/**
 * @test the only test
 */
CALL MICROFLOW M.A();
/
`
	tests, err := parseMDLTests(content, "standalone.test.mdl")
	if err != nil {
		t.Fatalf("parseMDLTests: %v", err)
	}
	if len(tests) != 1 {
		t.Fatalf("got %d test(s), want 1", len(tests))
	}
	if tests[0].ID != "test_1" {
		t.Errorf("ID = %q, want test_1 — IDs number the tests, not the chunks", tests[0].ID)
	}
}
