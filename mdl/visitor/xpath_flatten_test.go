// SPDX-License-Identifier: Apache-2.0

package visitor

import "testing"

// The property the pair has to hold, and the reason DESCRIBE can keep emitting
// one-line MDL while the model stores the constraint broken up: flattening what
// the formatter produced gives back exactly the constraint that went in.
//
// It holds exactly wherever the formatter only added line breaks. Where it also
// made an implied grouping explicit, the weaker statement below is the one that
// applies.
func TestFlattenXPathConstraint_UndoesFormatting(t *testing.T) {
	for _, in := range []string{
		"[RequestTitle = 'abc']",
		"[Status = 'Open' and Priority = 'High' and Category = 'Electrical' and Severity > 3 and Archived = false]",
		"[not(Status = 'Closed' and Priority = 'Low' and Category = 'Electrical' and Archived = true)]",
		"[Owner/Module.Owner_User/System.User = '[%CurrentUser%]' and Status = 'Open' and Priority = 'High' and Severity > 3]",
	} {
		wrapped := FormatXPathConstraint(in)
		if got := FlattenXPathConstraint(wrapped); got != in {
			t.Errorf("Flatten(Format(x)) != x\n  x: %s\n got: %s\nwrapped:\n%s", in, got, wrapped)
		}
	}
}

// Where the formatter parenthesised a mixed and/or chain, flattening cannot give
// the original text back — the parentheses are the point. What still has to hold
// is what the DESCRIBE round trip actually rests on: describing a stored
// constraint and executing that description re-derives the same stored text.
func TestFlattenXPathConstraint_RoundTripIsStableThroughAddedParentheses(t *testing.T) {
	in := "[Archived = false and (Status = 'Open' and Priority = 'High' and Category = 'Electrical' or Escalated = true and Severity > 3)]"

	stored := FormatXPathConstraint(in)
	described := FlattenXPathConstraint(stored)
	if described == in {
		t.Fatal("fixture no longer exercises added parentheses — pick one that does")
	}
	if reStored := FormatXPathConstraint(described); reStored != stored {
		t.Errorf("describing and re-executing moved the stored constraint\n  first:\n%s\n second:\n%s", stored, reStored)
	}

	// And the parentheses did not change what it matches.
	before, ok := ParseXPathConstraint(in)
	if !ok {
		t.Fatal("fixture does not parse")
	}
	after, ok := ParseXPathConstraint(described)
	if !ok {
		t.Fatalf("the described form does not parse: %s", described)
	}
	if got, want := fullyParenthesised(after), fullyParenthesised(before); got != want {
		t.Errorf("flattening changed the expression\n got %s\nwant %s", got, want)
	}
}

// Sibling groups are joined by a newline when formatted; flattening puts them
// back side by side, which is how Mendix stores them.
func TestFlattenXPathConstraint_RejoinsPredicateGroups(t *testing.T) {
	in := "[Status = 'Open' and Priority = 'High' and Category = 'Electrical' and Severity > 3][Archived = false]"

	if got := FlattenXPathConstraint(FormatXPathConstraint(in)); got != in {
		t.Errorf("Flatten(Format(x)) != x\n  x: %s\n got: %s", in, got)
	}
}

// Whitespace inside a string literal is data. Collapsing it would change which
// rows the constraint matches.
func TestFlattenXPathConstraint_KeepsWhitespaceInsideLiterals(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"[Name = 'two  spaces']", "[Name = 'two  spaces']"},
		{"[\n  Name = 'two  spaces'\n  and Status = 'Open'\n]", "[Name = 'two  spaces' and Status = 'Open']"},
		{"[Name = 'it''s  here']", "[Name = 'it''s  here']"},
		{"[Name = 'a ] b' and Status = 'Open']", "[Name = 'a ] b' and Status = 'Open']"},
	} {
		if got := FlattenXPathConstraint(tc.in); got != tc.want {
			t.Errorf("FlattenXPathConstraint(%q)\n got %q\nwant %q", tc.in, got, tc.want)
		}
	}
}

// A constraint with no whitespace at all is returned as-is, and one that is
// already flat is unchanged.
func TestFlattenXPathConstraint_LeavesFlatInputAlone(t *testing.T) {
	for _, in := range []string{
		"",
		"[Active]",
		"[Status = 'Open' and Priority = 'High']",
	} {
		if got := FlattenXPathConstraint(in); got != in {
			t.Errorf("FlattenXPathConstraint(%q) = %q, want it unchanged", in, got)
		}
	}
}
