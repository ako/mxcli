// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
)

// Text the grammar cannot begin to parse yields ZERO statements and ZERO errors
// — `visitor.Build` cannot tell it from an empty file — so `check` reported
// "Check passed!" and `exec` applied nothing, both at exit 0 (ako/mxcli#618).
//
// A truncated file, a lost heredoc, or a path that resolved to the wrong kind of
// file therefore passes both gates and moves nothing. For a workflow whose
// premise is that mdlsource/ replays, that is the worst available outcome: the
// replay reports success and the model does not change.

func TestGarbageIsNotAnEmptyScript(t *testing.T) {
	line, bad := unparsableInput("this is not valid mdl at all;\n/\n", 0)
	if !bad {
		t.Fatal("a file of pure garbage is accepted as an empty script")
	}
	if line != "this is not valid mdl at all;" {
		t.Errorf("complaint names %q; it should name the first line that did not parse", line)
	}
}

// THE CONTROLS. Each of these legitimately yields zero statements and must stay
// a pass, or the guard would refuse files that are fine.
func TestGenuinelyEmptyInputIsStillAccepted(t *testing.T) {
	for name, src := range map[string]string{
		"empty":            "",
		"whitespace":       "   \n\n\t\n",
		"comments only":    "-- set up the domain model\n-- (nothing yet)\n",
		"comments + blank": "\n-- TODO\n\n",
	} {
		if _, bad := unparsableInput(src, 0); bad {
			t.Errorf("%s: refused, but it is a legitimately empty script", name)
		}
	}
}

// A file that parsed is never the guard's business, whatever it contains.
func TestParsedInputIsNeverFlagged(t *testing.T) {
	if _, bad := unparsableInput("this is not valid mdl at all;", 1); bad {
		t.Error("input that produced statements was flagged; the guard is only for zero")
	}
}

// A .test.mdl is not top-level MDL: `check` parses testrunner's RENDERING of
// its @test blocks. A file with no @test block renders to nothing, so it reaches
// the same zero-statement state from a completely different cause — the parser
// was handed an empty rendering, not the author's text.
//
// This is not hypothetical. The #618 guard fired on
// mdl-examples/doctype-tests/15-fragment-examples.test.mdl and reported "the
// parser could not begin reading it. First line that did not parse: create
// module FragTest;" — a line the parser was never given. The file parses fine
// as ordinary MDL (18 statements, and one real MDL-PAGE20 violation nobody had
// ever seen, because being named .test.mdl meant `make check-mdl` had been
// reporting PASS on 416 unchecked lines).
//
// So the predicate `check` branches on is the RENDERING being empty, and it is
// what this asserts — the message is downstream of getting that right.
func TestTestFileWithNoTestBlocksRendersEmpty(t *testing.T) {
	src := "-- a demo of some syntax\ncreate module FragTest;\ncreate entity FragTest.Customer (Name: String);\n"

	checked, err := testrunner.CheckSource(src, "demo.test.mdl")
	if err != nil {
		t.Fatalf("CheckSource: %v", err)
	}
	if strings.TrimSpace(checked.MDL) != "" {
		t.Errorf("a file with no @test block rendered to %q; check would parse that "+
			"instead of reporting that nothing is checked", checked.MDL)
	}

	// CONTROL: the same statements inside a @test block DO render, so the
	// emptiness above is the missing annotation and not the content.
	withTest := "/**\n * @test customer entity exists\n */\n" + src
	checkedTest, err := testrunner.CheckSource(withTest, "demo.test.mdl")
	if err != nil {
		t.Fatalf("CheckSource with @test: %v", err)
	}
	if strings.TrimSpace(checkedTest.MDL) == "" {
		t.Fatal("control failed: a file WITH an @test block also rendered empty, " +
			"so the predicate does not distinguish the two")
	}
}

// The two messages must not be confusable: one of them names a source line, and
// that is exactly the claim that was false for a test file.
func TestNoTestsMessageDoesNotClaimAParseFailure(t *testing.T) {
	msg := noTestsDeclaredError("demo.test.mdl")
	if strings.Contains(msg, "could not begin reading") ||
		strings.Contains(msg, "did not parse") {
		t.Errorf("the no-tests message borrows #618's parse-failure wording:\n%s", msg)
	}
	if !strings.Contains(msg, "@test") {
		t.Errorf("the no-tests message does not say what is missing:\n%s", msg)
	}
}
