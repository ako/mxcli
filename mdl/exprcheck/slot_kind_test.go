// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"strings"
	"testing"
)

// The slot-expectation table had existed with NOTHING READING IT — slotKind()
// was defined and never called — so a slot declared `{Kind: KindString}`
// constrained nothing. `LOG WARNING 42` passed, and so did every non-String log
// template parameter, which mxbuild reports as CE0117 "Error(s) in expression"
// (mendixlabs/mxcli#1043).

// fakeEntities answers the object half of inference: a variable the entity
// scope knows holds an object.
type fakeEntities map[string]string

func (f fakeEntities) VariableEntity(name string) (string, bool) {
	qn, ok := f[strings.TrimPrefix(name, "$")]
	return qn, ok
}
func (fakeEntities) AssociationTarget(string, string) (string, bool) { return "", false }

func parseIn(t *testing.T, src, slot string, opts ...func(*Context)) []Hint {
	t.Helper()
	ctx := Context{SlotPath: slot, Slots: DefaultSlotResolver()}
	for _, o := range opts {
		o(&ctx)
	}
	_, hs := NewParser().Parse(src, ctx)
	return hs
}

func codes(hs []Hint) []string {
	var out []string
	for _, h := range hs {
		out = append(out, h.Code)
	}
	return out
}

// Measured on 11.13.0 by executing and building each: Boolean, DateTime,
// Decimal and Integer log parameters all fail CE0117; a String one is clean;
// toString(...) around any of them is clean.
func TestSlotKind_LogTemplateParamMustBeString(t *testing.T) {
	for _, src := range []string{"42", "true", "1.5"} {
		hs := parseIn(t, src, "LogStmt.TemplateParam")
		if !hasCode(hs, "E009") {
			t.Errorf("%q in a String slot was accepted: %v", src, codes(hs))
		}
	}

	// CONTROL: a String is fine, and so is the conversion mxbuild accepts.
	for _, src := range []string{"'text'", "toString(42)"} {
		if hs := parseIn(t, src, "LogStmt.TemplateParam"); hasCode(hs, "E009") {
			t.Errorf("%q was rejected in a String slot: %v", src, hs)
		}
	}
}

// The case #1043 was actually filed about. A variable holding an OBJECT infers
// Unknown without the entity scope, and Unknown is tolerated everywhere — so
// the object went unreported.
func TestSlotKind_AnObjectIsNotAString(t *testing.T) {
	withEnts := func(c *Context) { c.Entities = fakeEntities{"Customer": "Sales.Customer"} }

	hs := parseIn(t, "$Customer", "LogStmt.TemplateParam", withEnts)
	if !hasCode(hs, "E009") {
		t.Errorf("an object in a String slot was accepted: %v", codes(hs))
	}

	// CONTROL: without the entity scope the kind is genuinely unknown, and
	// unknown must stay silent rather than be guessed at.
	if hs := parseIn(t, "$Customer", "LogStmt.TemplateParam"); hasCode(hs, "E009") {
		t.Errorf("reported a variable whose kind could not be established: %v", hs)
	}
}

// CONTROL for the scope of the whole mechanism. A slot whose expectation has to
// be resolved per call site (ResolveBy) is NOT enforced here: the adapter
// encodes the resolved target by appending it to the slot path, so the table
// entry does not apply and enforcing it would compare against the wrong kind.
func TestSlotKind_ResolveBySlotsAreNotEnforced(t *testing.T) {
	for _, slot := range []string{
		"ChangeItem.Value",
		"ChangeItem.Value:Sales.Order.Qty",
		"ReturnStmt.Value",
		"CallArgument.Value",
	} {
		if hs := parseIn(t, "42", slot); hasCode(hs, "E009") {
			t.Errorf("slot %q was enforced against its placeholder kind: %v", slot, hs)
		}
	}
}

// `empty` is Mendix's null rather than a kind of its own, so it satisfies any
// slot. Without this every `LOG … WITH ({1} = empty)` would be reported.
func TestSlotKind_EmptySatisfiesAnySlot(t *testing.T) {
	if hs := parseIn(t, "empty", "LogStmt.TemplateParam"); hasCode(hs, "E009") {
		t.Errorf("empty was rejected in a String slot: %v", hs)
	}
}

// CONTROL: a slot with no table entry constrains nothing.
func TestSlotKind_UnknownSlotIsSilent(t *testing.T) {
	if hs := parseIn(t, "42", "NoSuchSlot.Value"); hasCode(hs, "E009") {
		t.Errorf("an unmapped slot was enforced: %v", hs)
	}
}

// ---------------------------------------------------------------------------
// E013 — a bare word as a member's value (mendixlabs/mxcli#1044)
// ---------------------------------------------------------------------------

// Mendix expressions have no bare identifiers, so the parser reads one as a
// variable, it resolves to nothing, and the kind comes out Unknown — which is
// tolerated everywhere. That is why `CHANGE $Order (Status = Closed)` passed
// check and exec and arrived as CE0117.
func TestBareIdentifier_ReportedAsAMemberValue(t *testing.T) {
	for _, slot := range []string{
		"ChangeItem.Value:Sales.Order.Status",
		"CreateItem.Value:Sales.Order.Status",
	} {
		hs := parseIn(t, "Closed", slot)
		if !hasCode(hs, "E013") {
			t.Errorf("slot %q accepted a bare word: %v", slot, codes(hs))
		}
		for _, h := range hs {
			if h.Code == "E013" && !strings.Contains(h.Fix, "'Closed'") {
				t.Errorf("the fix should offer the quoted form: %s", h.Fix)
			}
		}
	}
}

// THE CONTROL that scopes the rule. A bare name NESTED in a list-operation
// predicate is legal MDL — `FILTER($L, Status = 'Open')` resolves `Status`
// against the item under test — so a rule that fired on any bare identifier
// would reject working scripts. Only a bare word that is the WHOLE expression
// of a member value can only be a mistake.
func TestBareIdentifier_NotReportedWhenNested(t *testing.T) {
	for _, tc := range []struct{ src, slot string }{
		{"FILTER($L, Status = 'Open')", "MfSetStmt.Value"},
		{"FIND($L, Code = 'x')", "ChangeItem.Value:Sales.Order.Ref"},
		{"Status = 'Open'", "ChangeItem.Value:Sales.Order.Flag"},
	} {
		if hs := parseIn(t, tc.src, tc.slot); hasCode(hs, "E013") {
			t.Errorf("%q reported a nested bare name: %v", tc.src, hs)
		}
	}
}

// CONTROL: outside a member value the rule is silent, because a bare word may
// be legitimate there and this rule has only been established for one position.
func TestBareIdentifier_OnlyInMemberValues(t *testing.T) {
	for _, slot := range []string{"IfStmt.Condition", "LogStmt.Message", "MfSetStmt.Value"} {
		if hs := parseIn(t, "Closed", slot); hasCode(hs, "E013") {
			t.Errorf("slot %q was reported: %v", slot, hs)
		}
	}
}

// CONTROL: every spelling that IS a Mendix expression must pass.
func TestBareIdentifier_AcceptsRealExpressions(t *testing.T) {
	for _, src := range []string{
		"'Closed'",                 // a string literal
		"$Closed",                  // a variable
		"Sales.OrderStatus.Closed", // a qualified enumeration value
		"toString($Order/Qty)",     // a call
		"true",                     // a keyword literal
		"empty",                    // Mendix's null
		"[%CurrentDateTime%]",      // a token
	} {
		if hs := parseIn(t, src, "ChangeItem.Value:Sales.Order.Status"); hasCode(hs, "E013") {
			t.Errorf("%q was reported as a bare word: %v", src, hs)
		}
	}
}
