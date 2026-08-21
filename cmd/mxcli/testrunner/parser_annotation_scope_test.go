// SPDX-License-Identifier: Apache-2.0

package testrunner

import "testing"

// An annotation is a javadoc tag: it opens the line. Matching one anywhere in a
// line made prose that quotes a tag into a real annotation — which is not a
// hypothetical, it is what the #927 repro file hit on its first run: a sentence
// reading `@expect count($Var) = N` on a bare retrieve` became that file's own
// assertion, and its error then reported against a test nobody had written.
func TestParseAnnotationsIgnoresTagsQuotedInProse(t *testing.T) {
	a := parseAnnotations(`/**
 * @test prose must not assert
 * Writing ` + "`@expect $x = 1`" + ` in a sentence is documentation, and so is
 * saying that @cleanup none would apply here, or that @throws 'boom' is how
 * an error is expected, or referring to @verify select count(*) as n from M.E = 1.
 * @expect $real = true
 */`)

	if a.Test != "prose must not assert" {
		t.Errorf("Test = %q", a.Test)
	}
	if len(a.Expects) != 1 || a.Expects[0].Raw != "$real = true" {
		t.Errorf("Expects = %+v, want only the annotation that opens its line", a.Expects)
	}
	if len(a.AssertionErrors) != 0 {
		t.Errorf("AssertionErrors = %v, want none — prose was parsed as an assertion",
			a.AssertionErrors)
	}
	if a.Cleanup != CleanupRollback {
		t.Errorf("Cleanup = %q, want the default: the prose mention must not set it", a.Cleanup)
	}
	if a.Throws != "" {
		t.Errorf("Throws = %q, want empty", a.Throws)
	}
	if len(a.Verify) != 0 {
		t.Errorf("Verify = %+v, want none", a.Verify)
	}
}

// False-positive control: the real spellings must all still be read, at every
// indentation a javadoc block uses.
func TestParseAnnotationsReadsTagsThatOpenTheirLine(t *testing.T) {
	a := parseAnnotations(`/**
 * @test still parsed
 *     @expect $r = 1
@verify select count(*) as n from M.E = 1
 * @cleanup none
 * @setup seed
 */`)

	if a.Test != "still parsed" {
		t.Errorf("Test = %q", a.Test)
	}
	if len(a.Expects) != 1 {
		t.Errorf("Expects = %+v, want 1", a.Expects)
	}
	if len(a.Verify) != 1 {
		t.Errorf("Verify = %+v, want 1", a.Verify)
	}
	if a.Cleanup != CleanupNone {
		t.Errorf("Cleanup = %q, want none", a.Cleanup)
	}
	if len(a.Setups) != 1 || a.Setups[0] != "seed" {
		t.Errorf("Setups = %v, want [seed]", a.Setups)
	}
}

// A single-line doc comment is the other shape a tag opens.
func TestParseAnnotationsSingleLineDoc(t *testing.T) {
	a := parseAnnotations(`/** @test one-liner */`)
	if a.Test != "one-liner" {
		t.Errorf("Test = %q, want one-liner", a.Test)
	}
}
