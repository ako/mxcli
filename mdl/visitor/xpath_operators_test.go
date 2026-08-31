// SPDX-License-Identifier: Apache-2.0

package visitor

import "testing"

// mxcli-formula1 FINDINGS §80: `RETRIEVE … WHERE a = x AND b = y` stored an
// uppercase `AND` in the XPath constraint, which mxbuild rejects:
//
//	ERROR at Formula1Backend, Microflow 'ZZ_AndProbe2',
//	  Retrieve object(s) activity 'Retrieve list of LiveForecast from database':
//	  Error(s) in XPath constraint.                                    (CE0161)
//
// XPath 1.0 spells its operators in lower case only, and MDL's lexer accepts
// any case (`AND: A N D;`), so the two have to be reconciled on the way out.
//
// The finding is careful about something worth repeating: an earlier note
// blamed the CASING alone, and a literal-only reproducer built cleanly. The
// trigger is a VARIABLE reference on one side. Measured, and reproduced here on
// 11.13 before the fix:
//
//	WHERE SessionKey = '1' AND AtLap = 2                  -> stored `and`, builds
//	WHERE SessionKey = $N/SessionKey AND AtLap = $N/AtLap -> stored `AND`, CE0161
//	…the same with lowercase `and`                        -> builds
//
// Two rendering paths, and only one normalises. `expressionToXPath` lowercases
// the operator as it walks the parse tree; but `buildRetrieveWhereExpression`
// freezes the RAW SOURCE whenever the clause contains a `/` — which every
// variable path does — and `expressionToXPath`'s SourceExpr case hands that text
// back verbatim. So the casing only survives when a path is present, which is
// exactly the correlation the finding measured and could not explain.
//
// Normalising at FormatXPathConstraint fixes all three of mxcli's constraint
// writers at once (retrieve, page data source, entity access rule), which is
// why it goes there rather than in the retrieve builder.

func TestNormalizeXPathOperators_LowercasesBooleanOperators(t *testing.T) {
	cases := []struct{ in, want string }{
		// The reported case: a variable path, so the raw source is preserved.
		{"[SessionKey = $N/SessionKey AND AtLap = $N/AtLap]",
			"[SessionKey = $N/SessionKey and AtLap = $N/AtLap]"},
		{"[a = 1 OR b = 2]", "[a = 1 or b = 2]"},
		{"[NOT(a = 1)]", "[not(a = 1)]"},
		// Mixed case, which the lexer accepts just as readily.
		{"[a = 1 And b = 2]", "[a = 1 and b = 2]"},
		{"[a = 1 Or b = 2]", "[a = 1 or b = 2]"},
		// Several in one constraint.
		{"[a = 1 AND b = 2 OR c = 3]", "[a = 1 and b = 2 or c = 3]"},
	}
	for _, tc := range cases {
		if got := NormalizeXPathOperators(tc.in); got != tc.want {
			t.Errorf("NormalizeXPathOperators(%q)\n got %q\nwant %q", tc.in, got, tc.want)
		}
	}
}

// A STRING LITERAL is data, not syntax. Rewriting inside one changes what the
// constraint matches — a silent wrong-rows bug, which is worse than the CE0161
// this fixes.
func TestNormalizeXPathOperators_LeavesStringLiteralsAlone(t *testing.T) {
	cases := []string{
		"[Name = 'A AND B']",
		"[Name = 'AND']",
		"[Name = 'NOT SET' AND Active = true()]",
		// A doubled quote is an escaped quote inside the literal, not its end.
		"[Name = 'it''s AND then' ]",
	}
	for _, in := range cases {
		got := NormalizeXPathOperators(in)
		want := in
		if in == "[Name = 'NOT SET' AND Active = true()]" {
			want = "[Name = 'NOT SET' and Active = true()]" // the operator OUTSIDE is normalised
		}
		if got != want {
			t.Errorf("NormalizeXPathOperators(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

// An identifier that merely CONTAINS an operator's letters is not an operator.
// This is the whole reason the replacement is token-based rather than a
// string substitution.
func TestNormalizeXPathOperators_LeavesIdentifiersAlone(t *testing.T) {
	for _, in := range []string{
		"[Brand = 'x']",
		"[Andrew = 1]",
		"[Module.Handover = 1]",
		"[NOTES != empty]",
		"[Sales.Order_Andon/Sales.Andon/Name = 'x']",
		"[ORDERS = 1]",
	} {
		if got := NormalizeXPathOperators(in); got != in {
			t.Errorf("NormalizeXPathOperators(%q) rewrote an identifier: %q", in, got)
		}
	}
}

// Idempotent, and a no-op on what already worked — the literal-only form the
// finding measured as building cleanly must come through untouched.
func TestNormalizeXPathOperators_IsANoOpOnCorrectInput(t *testing.T) {
	for _, in := range []string{
		"[SessionKey = '1' and AtLap = 2]",
		"[a = 1 or not(b = 2)]",
		"",
		"[]",
	} {
		if got := NormalizeXPathOperators(in); got != in {
			t.Errorf("NormalizeXPathOperators(%q) = %q, want it unchanged", in, got)
		}
	}
	// Applying it twice changes nothing more.
	once := NormalizeXPathOperators("[a = 1 AND b = 2]")
	if twice := NormalizeXPathOperators(once); twice != once {
		t.Errorf("not idempotent: %q then %q", once, twice)
	}
}

// The end the finding is about: the constraint mxcli actually stores. Every
// constraint writer goes through FormatXPathConstraint, so the normalisation
// has to survive both its branches — the short one that returns the caller's
// own bytes, and the wrapping one.
func TestFormatXPathConstraint_NormalisesTheOperator(t *testing.T) {
	short := "[SessionKey = $N/SessionKey AND AtLap = $N/AtLap]"
	if got, want := FormatXPathConstraint(short), "[SessionKey = $N/SessionKey and AtLap = $N/AtLap]"; got != want {
		t.Errorf("short constraint:\n got %q\nwant %q", got, want)
	}

	// Long enough to be wrapped: the operator must be lower case there too, and
	// the wrapping (upstream #979) must still happen.
	long := "[SessionKey = $Newest/SessionKey AND AtLap = $Newest/AtLap AND Status = 'Published' AND Archived = false()]"
	got := FormatXPathConstraint(long)
	if containsUpperOperator(got) {
		t.Errorf("wrapped constraint kept an uppercase operator:\n%s", got)
	}
	if !hasNewline(got) {
		t.Errorf("a constraint over the width budget should still be wrapped:\n%s", got)
	}
}

func containsUpperOperator(s string) bool {
	for _, op := range []string{" AND ", " OR ", "NOT("} {
		if idx := indexOf(s, op); idx >= 0 {
			return true
		}
	}
	return false
}

func hasNewline(s string) bool { return indexOf(s, "\n") >= 0 }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
