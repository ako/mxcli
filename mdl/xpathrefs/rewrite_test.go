// SPDX-License-Identifier: Apache-2.0

package xpathrefs

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// testModel is a small hand-built domain: Person and Order, joined by
// Sales.Order_Person (FROM Order, TO Person).
type testModel struct{}

func (testModel) IsEntity(qn string) bool {
	switch qn {
	case "Sales.Person", "Sales.Order", "Sales.Contact":
		return true
	}
	return false
}

func (testModel) AssociationTarget(qn, from string) (string, bool) {
	// Order_Person: FROM Sales.Order, TO Sales.Person. Traversable either way.
	if qn != "Sales.Order_Person" {
		return "", false
	}
	switch from {
	case "Sales.Order":
		return "Sales.Person", true
	case "Sales.Person":
		return "Sales.Order", true
	}
	return "", false
}

func rw(t *testing.T, constraint, target string) (string, bool) {
	t.Helper()
	return rewriteConstraint(constraint, target, "Sales.Person", "FirstName", "GivenName", testModel{})
}

func TestRewriteTopLevelAttribute(t *testing.T) {
	got, ok := rw(t, "[FirstName = 'Ada']", "Sales.Person")
	if !ok {
		t.Fatal("the constraint was reported as not understood")
	}
	if want := "[GivenName = 'Ada']"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRewritePreservesEverythingElse pins that the edit is a token replacement,
// not a re-render. Re-rendering would normalise the spacing and operator casing
// of a constraint the rename has no business touching, and any parser/renderer
// disagreement would corrupt it.
func TestRewritePreservesEverythingElse(t *testing.T) {
	in := "[   FirstName='Ada'   and LastName  =  'Lovelace' ]"
	got, ok := rw(t, in, "Sales.Person")
	if !ok {
		t.Fatal("the constraint was reported as not understood")
	}
	want := "[   GivenName='Ada'   and LastName  =  'Lovelace' ]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRewriteSkipsStringLiterals pins that the name inside quotes is data, not a
// reference — the single most likely way a naive scan corrupts a project.
func TestRewriteSkipsStringLiterals(t *testing.T) {
	got, ok := rw(t, "[FirstName = 'FirstName']", "Sales.Person")
	if !ok {
		t.Fatal("the constraint was reported as not understood")
	}
	if want := "[GivenName = 'FirstName']"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRewriteSiblingGroups pins that Mendix's concatenated groups are each
// handled — the shape that silently dropped every group but the first in #772.
func TestRewriteSiblingGroups(t *testing.T) {
	got, ok := rw(t, "[FirstName = 'Ada'][LastName = 'L'][FirstName != 'Bob']", "Sales.Person")
	if !ok {
		t.Fatal("the constraint was reported as not understood")
	}
	want := "[GivenName = 'Ada'][LastName = 'L'][GivenName != 'Bob']"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRewriteAcrossAssociationHop pins the case the qualified-name scanner can
// never reach: the constraint belongs to Order, and only the hop to Person makes
// the bare step ours.
func TestRewriteAcrossAssociationHop(t *testing.T) {
	got, ok := rw(t, "[Sales.Order_Person/Sales.Person/FirstName = 'Ada']", "Sales.Order")
	if !ok {
		t.Fatal("the constraint was reported as not understood")
	}
	want := "[Sales.Order_Person/Sales.Person/GivenName = 'Ada']"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRewriteLeavesOtherEntitysAttributeAlone is the correctness case that makes
// this package worth having: Order has a FirstName too, and it must not move.
func TestRewriteLeavesOtherEntitysAttributeAlone(t *testing.T) {
	got, ok := rw(t, "[FirstName = 'Ada']", "Sales.Order")
	if !ok {
		t.Error("an unambiguous constraint on another entity was reported as not understood")
	}
	if want := "[FirstName = 'Ada']"; got != want {
		t.Errorf("another entity's attribute was renamed: got %q, want %q", got, want)
	}
}

// TestRewriteRefusesMixedMeanings pins the refusal. In one group the bare name
// means Order's attribute and Person's attribute; a token-level edit cannot tell
// them apart, so nothing is written and the caller is told.
func TestRewriteRefusesMixedMeanings(t *testing.T) {
	in := "[FirstName = 'Ada' and Sales.Order_Person/Sales.Person/FirstName = 'Ada']"
	got, ok := rw(t, in, "Sales.Order")
	if ok {
		t.Error("a group where the bare name means two different attributes was accepted")
	}
	if got != in {
		t.Errorf("the group was rewritten anyway: got %q", got)
	}
}

// TestRewriteRefusesUnknownAssociation pins that an unresolvable hop blocks the
// rewrite rather than being assumed to land on the renamed entity.
func TestRewriteRefusesUnknownAssociation(t *testing.T) {
	in := "[Sales.Mystery/FirstName = 'Ada']"
	got, ok := rw(t, in, "Sales.Person")
	if ok {
		t.Error("a path through an unresolvable association was accepted")
	}
	if got != in {
		t.Errorf("the group was rewritten anyway: got %q", got)
	}
}

// TestRewriteRefusesUnparseableGroup pins that a group mxcli cannot read is
// reported, not passed over in silence — silence is what makes a half-rename
// look complete.
func TestRewriteRefusesUnparseableGroup(t *testing.T) {
	in := "[[FirstName]]"
	got, ok := rw(t, in, "Sales.Person")
	if ok {
		t.Error("an unparseable group that names the attribute was accepted")
	}
	if got != in {
		t.Errorf("the group was rewritten anyway: got %q", got)
	}
}

// TestRewriteIgnoresUnrelatedUnparseableGroup pins the other side of that: a
// group mxcli cannot read but which never mentions the attribute is not the
// rename's problem and must not be reported.
func TestRewriteIgnoresUnrelatedUnparseableGroup(t *testing.T) {
	in := "[[Nonsense]]"
	got, ok := rw(t, in, "Sales.Person")
	if !ok {
		t.Error("an unparseable group unrelated to the attribute was reported")
	}
	if got != in {
		t.Errorf("got %q, want it untouched", got)
	}
}

// TestRewriteCountInvariantCatchesADroppedRegion is the test for the safety
// check that makes a lenient parser usable as a gate.
//
// visitor.ParseXPathConstraint runs with ANTLR's error listeners removed, so it
// recovers and can return a tree that omits part of its input — here everything
// after the first comparison. The dropped text holds a second occurrence of the
// name that the resolution therefore never saw, and a lexical edit would rewrite
// it anyway. The counts disagree, so nothing is written.
func TestRewriteCountInvariantCatchesADroppedRegion(t *testing.T) {
	in := "[LastName = 'L' FirstName]"
	if _, ok := visitor.ParseXPathConstraint(in); !ok {
		t.Skip("the parser now rejects this shape; the invariant is exercised elsewhere")
	}
	got, ok := rw(t, in, "Sales.Person")
	if ok {
		t.Error("a group whose parse dropped an occurrence of the name was accepted")
	}
	if got != in {
		t.Errorf("the group was rewritten anyway: got %q", got)
	}
}

// TestRewriteQualifiedNameIsNotABareStep pins that the three-part qualified form
// is the string scanner's job, not this one's — and that this package does not
// double-rewrite what the scanner already handled.
func TestRewriteQualifiedNameIsNotABareStep(t *testing.T) {
	in := "[Sales.Person.FirstName = 'Ada']"
	got, _ := rw(t, in, "Sales.Person")
	if got != in {
		t.Errorf("a qualified name was treated as a bare step: got %q", got)
	}
}

func TestRewriteNoMentionIsUntouched(t *testing.T) {
	in := "[LastName = 'Lovelace']"
	got, ok := rw(t, in, "Sales.Person")
	if !ok {
		t.Error("a constraint that never names the attribute was reported")
	}
	if got != in {
		t.Errorf("got %q, want it untouched", got)
	}
}

// TestRewriteNonBracketConstraintIsReported pins that a stored value this
// package cannot even split into groups is reported when it names the attribute.
func TestRewriteNonBracketConstraintIsReported(t *testing.T) {
	if _, ok := rw(t, "FirstName = 'Ada'", "Sales.Person"); ok {
		t.Error("a constraint with no bracket group was silently accepted")
	}
	if _, ok := rw(t, "LastName = 'L'", "Sales.Person"); !ok {
		t.Error("a constraint with no bracket group and no mention was reported")
	}
}

func TestReplaceIdentifier(t *testing.T) {
	tests := []struct {
		in   string
		want string
		n    int
	}{
		{"[A = 'x']", "[B = 'x']", 1},
		{"[A = 'A']", "[B = 'A']", 1},
		{"[Mod.A = 'x']", "[Mod.A = 'x']", 0},
		{"[A.Sub = 'x']", "[A.Sub = 'x']", 0},
		{"[AA = 'x']", "[AA = 'x']", 0},
		{"[A_1 = 'x']", "[A_1 = 'x']", 0},
		{"[A = A]", "[B = B]", 2},
		{"[contains(A, 'x')]", "[contains(B, 'x')]", 1},
	}
	for _, tc := range tests {
		got, n := replaceIdentifier(tc.in, "A", "B")
		if got != tc.want || n != tc.n {
			t.Errorf("replaceIdentifier(%q) = (%q, %d), want (%q, %d)", tc.in, got, n, tc.want, tc.n)
		}
	}
}
