// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"
	"time"
)

// ako/CapTrackV3 FINDINGS §29. A retrieve constraint written as an MDL STRING
// with the enum value quoted came out of the model truncated, with the clause
// after it gone:
//
//	RETRIEVE $Rows FROM Xp.Movement
//	  WHERE 'Status = ''Xp.Status.Confirmed'' and Kind = Xp.Kind.Addition';
//
//	describe microflow -> where Status = Xp.Status.;
//	mx check           -> [CE0161] "Error(s) in XPath constraint."
//	mxcli check        -> passed
//
// normalizeXPathEnumRefs ran its regex over the WHOLE constraint with no quote
// awareness, so a ref the author had already quoted was rewritten INSIDE its own
// quotes: `'Xp.Status.Confirmed'` became `''Confirmed''`. Every downstream reader
// lexes `''` as an empty string literal, so the parse stopped at `Status = ''`
// and ANTLR error recovery swallowed the stray word, the second `''` and the
// entire ` and Kind = …` tail.
//
// The report reached the right workaround — the bracket form takes enum values
// unquoted and survives — but the cost is the class this belongs to: mxbuild's
// CE0161 does not say WHICH clause, and a retrieve that silently returns more
// rows than it should reads as a data problem, not a serialization one.

func TestNormalizeXPathEnumRefs_LeavesAQuotedRefAlone(t *testing.T) {
	const in = "Status = 'Xp.Status.Confirmed' and Kind = Xp.Kind.Addition"
	const want = "Status = 'Xp.Status.Confirmed' and Kind = 'Addition'"

	if got := normalizeXPathEnumRefs(in); got != want {
		t.Errorf("normalizeXPathEnumRefs(%q)\n got  %q\n want %q", in, got, want)
	}
}

// THE CONTROL, and the reason the test above is worth having: the old behaviour
// produced a doubled quote. If this ever appears again the constraint is being
// corrupted, whatever the rest of the suite says.
func TestNormalizeXPathEnumRefs_NeverProducesADoubledQuote(t *testing.T) {
	for _, in := range []string{
		"Status = 'Xp.Status.Confirmed'",
		"Status = 'Xp.Status.Confirmed' and Kind = 'Xp.Kind.Addition'",
		"Code = 'a.b.c'",
	} {
		got := normalizeXPathEnumRefs(in)
		for i := 0; i+1 < len(got); i++ {
			if got[i] == '\'' && got[i+1] == '\'' {
				t.Errorf("normalizeXPathEnumRefs(%q) = %q — a doubled quote lexes as an "+
					"empty literal and drops the rest of the constraint", in, got)
				break
			}
		}
	}
}

// CONTROL: an ordinary string literal that happens to carry two dots — a
// filename, a version, a dotted code — is DATA, and rewriting it changes which
// rows the constraint matches. This is why the quoted ref is left as written
// rather than repaired: telling `'a.b.c'` from `'Xp.Status.Confirmed'` needs the
// project's enumerations, which this string-level pass does not have.
func TestNormalizeXPathEnumRefs_LeavesAnOrdinaryLiteralAlone(t *testing.T) {
	cases := map[string]string{
		"Code = 'a.b.c'":         "Code = 'a.b.c'",
		"Name = 'report.v1.csv'": "Name = 'report.v1.csv'",
		// A literal beside a real ref: only the unquoted one is rewritten.
		"Path = 'a.b.c' and Status = Xp.Status.Open": "Path = 'a.b.c' and Status = 'Open'",
	}
	for in, want := range cases {
		if got := normalizeXPathEnumRefs(in); got != want {
			t.Errorf("normalizeXPathEnumRefs(%q)\n got  %q\n want %q — a quoted literal is data", in, got, want)
		}
	}
}

// CONTROL: the case the function exists for still works. An UNQUOTED qualified
// enum ref becomes the bare value a database query compares against, which is
// what the bracket form has always produced.
func TestNormalizeXPathEnumRefs_StillRewritesAnUnquotedRef(t *testing.T) {
	cases := map[string]string{
		"Status = Xp.Status.Confirmed":                          "Status = 'Confirmed'",
		"Status = Xp.Status.Confirmed and Kind = Xp.Kind.Add":   "Status = 'Confirmed' and Kind = 'Add'",
		"[Status = XpathTest.OrderStatus.Open]":                 "[Status = 'Open']",
		"Status = Xp.Status.Open or Status = Xp.Status.Pending": "Status = 'Open' or Status = 'Pending'",
	}
	for in, want := range cases {
		if got := normalizeXPathEnumRefs(in); got != want {
			t.Errorf("normalizeXPathEnumRefs(%q)\n got  %q\n want %q", in, got, want)
		}
	}
}

// An unterminated literal must not stall the scanner. Malformed input reaches
// here from a hand-written constraint, and the loop advances past the end of the
// string rather than sitting on the opening quote forever.
func TestNormalizeXPathEnumRefs_UnclosedLiteralTerminates(t *testing.T) {
	done := make(chan string, 1)
	go func() { done <- normalizeXPathEnumRefs("Status = 'Xp.Status.Confirmed") }()
	select {
	case got := <-done:
		if got != "Status = 'Xp.Status.Confirmed" {
			t.Errorf("unclosed literal was rewritten: %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("normalizeXPathEnumRefs did not terminate on an unclosed literal")
	}
}
