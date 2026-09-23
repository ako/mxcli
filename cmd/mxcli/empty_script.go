// SPDX-License-Identifier: Apache-2.0

package main

import "strings"

// unparsableInput reports input that has content but produced no statements.
//
// `visitor.Build` returns zero statements AND zero errors for text the grammar
// cannot begin to parse, so it is indistinguishable from an empty file — which
// is why `check` said "Check passed!" and `exec` applied nothing, both at exit 0
// (ako/mxcli#618). A malformed statement that *starts* with a keyword is caught
// normally; this is only for input the parser never got into.
//
// It returns the first line that is neither blank nor a comment, so the message
// can name where reading went wrong rather than just asserting that it did.
//
// src must be the text that was PARSED, not the file as read. For a .test.mdl
// the two differ — check parses testrunner's rendering — and quoting a line the
// parser was never given is how this message starts lying. See
// noTestsDeclaredError for the case that difference creates.
func unparsableInput(src string, statements int) (string, bool) {
	if statements > 0 {
		return "", false
	}
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		// A file of nothing but comments is a legitimately empty script, and so
		// is a blank one; neither should be refused.
		if line == "" || strings.HasPrefix(line, "--") || strings.HasPrefix(line, "//") {
			continue
		}
		return line, true
	}
	return "", false
}

// unparsableInputError is the shared message. Both gates say the same thing,
// because a reader who hits it at one of them will try the other.
func unparsableInputError(path, line string) string {
	return "Error: " + path + " produced no statements, but it is not empty.\n" +
		"  The parser could not begin reading it, so there is nothing to " +
		"check or apply.\n" +
		"  First line that did not parse: " + clipLine(line) + "\n" +
		"  A truncated file, a lost heredoc, or the wrong path looks exactly " +
		"like this."
}

// noTestsDeclaredError is the diagnosis for a .test.mdl / .test.md file that
// declares no @test block.
//
// It is NOT the #618 message, although the two arrive at the same place. A test
// file is not top-level MDL: testrunner renders its blocks as microflows and
// `check` parses the RENDERING, so a file with no @test block renders to nothing
// and yields zero statements with the parser never having seen a line of the
// author's text. Saying "the parser could not begin reading it" there, and
// quoting a source line the parser was never given, would be false.
//
// Found by the #618 guard firing on mdl-examples/doctype-tests/
// 15-fragment-examples.test.mdl, which declares no tests, is 416 lines long, and
// had been reporting PASS in `make check-mdl` while nothing in it was checked —
// the same silent no-op #618 is about, one layer up. `mxcli test run` already
// refuses such a file ("no tests found"); this makes `check` agree.
func noTestsDeclaredError(path string) string {
	return "Error: " + path + " declares no tests, but it is not empty.\n" +
		"  A test file is checked as the microflows its @test blocks become, so " +
		"one with\n" +
		"  no @test block is checked by nothing and `mxcli test run` will not " +
		"run it.\n" +
		"  Add a /** @test name */ block, or name the file .mdl if it is ordinary MDL."
}

func clipLine(s string) string {
	const max = 80
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
