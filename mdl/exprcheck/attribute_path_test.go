// SPDX-License-Identifier: Apache-2.0

package exprcheck

import "testing"

// pathCatalog is a small Shop model: Order has Total (Decimal), Status
// (Enumeration Shop.OrderStatus) and Shipped (Boolean); Customer has Name
// (String). Shop.Order_Customer joins them.
type pathCatalog struct{}

func (pathCatalog) AttributeKind(entityQN, attr string) (TypeKind, bool) {
	switch entityQN + "." + attr {
	case "Shop.Order.Total":
		return KindDecimal, true
	case "Shop.Order.Status":
		return KindEnumeration, true
	case "Shop.Order.Shipped":
		return KindBoolean, true
	case "Shop.Customer.Name":
		return KindString, true
	}
	return KindUnknown, false
}

func (pathCatalog) AttributeEnumQN(entityQN, attr string) (string, bool) {
	if entityQN == "Shop.Order" && attr == "Status" {
		return "Shop.OrderStatus", true
	}
	return "", false
}

func (pathCatalog) EnumCases(enumQN string) ([]string, bool) {
	if enumQN == "Shop.OrderStatus" {
		return []string{"Open", "Shipped", "Closed"}, true
	}
	return nil, false
}

func (pathCatalog) MicroflowReturn(string) (TypeKind, bool)        { return KindUnknown, false }
func (pathCatalog) MicroflowParam(string, string) (TypeKind, bool) { return KindUnknown, false }

// pathScope types $O as an Order and $C as a Customer, and knows one
// association.
type pathScope struct{}

func (pathScope) VariableEntity(name string) (string, bool) {
	switch name {
	case "O":
		return "Shop.Order", true
	case "C":
		return "Shop.Customer", true
	}
	return "", false
}

func (pathScope) AssociationTarget(assocQN, from string) (string, bool) {
	if assocQN != "Shop.Order_Customer" {
		return "", false
	}
	switch from {
	case "Shop.Order":
		return "Shop.Customer", true
	case "Shop.Customer":
		return "Shop.Order", true
	}
	return "", false
}

func pathCtx() Context {
	return Context{Catalog: pathCatalog{}, Entities: pathScope{}, Slots: DefaultSlotResolver()}
}

func kindOf(t *testing.T, src string, ctx Context) TypeKind {
	t.Helper()
	e, _ := NewParser().Parse(src, ctx)
	return inferKind(e, ctx)
}

// TestAttributePathResolvesToItsKind is the gap this file closes. inferKind
// returned KindUnknown for every AttributePathExpr, so `$obj/Attr` typed to
// nothing and every rule downstream of it stayed quiet.
func TestAttributePathResolvesToItsKind(t *testing.T) {
	ctx := pathCtx()
	tests := []struct {
		src  string
		want TypeKind
	}{
		{"$O/Total", KindDecimal},
		{"$O/Status", KindEnumeration},
		{"$O/Shipped", KindBoolean},
		// Through an association hop, which an expression does not spell the
		// intermediate entity for — unlike XPath.
		{"$O/Shop.Order_Customer/Name", KindString},
		// Reverse direction resolves too.
		{"$C/Shop.Order_Customer/Total", KindDecimal},
	}
	for _, tc := range tests {
		if got := kindOf(t, tc.src, ctx); got != tc.want {
			t.Errorf("%s inferred %v, want %v", tc.src, got, tc.want)
		}
	}
}

// TestAttributePathUnknownsStayUnknown pins the failure direction: anything the
// seams cannot answer types to nothing, which suppresses the rule that asked
// rather than guessing at it.
func TestAttributePathUnknownsStayUnknown(t *testing.T) {
	ctx := pathCtx()
	for _, src := range []string{
		"$Unknown/Total",                    // variable not in scope
		"$O/NoSuchAttribute",                // attribute not on the entity
		"$O/Shop.Mystery/Name",              // unresolvable association
		"$C/Total",                          // right name, wrong entity
		"$O/Shop.Order_Customer/NoSuchAttr", // hop resolves, attribute does not
	} {
		if got := kindOf(t, src, ctx); got != KindUnknown {
			t.Errorf("%s inferred %v, want KindUnknown", src, got)
		}
	}
}

// TestAttributePathNeedsBothSeams pins that the resolution is off unless the
// caller supplied both halves — a Context with no EntityScope must behave
// exactly as it did before.
func TestAttributePathNeedsBothSeams(t *testing.T) {
	if got := kindOf(t, "$O/Total", Context{Catalog: pathCatalog{}}); got != KindUnknown {
		t.Errorf("with no EntityScope, inferred %v, want KindUnknown", got)
	}
	if got := kindOf(t, "$O/Total", Context{Entities: pathScope{}}); got != KindUnknown {
		t.Errorf("with no Catalog, inferred %v, want KindUnknown", got)
	}
}

// TestEnumComparedToStringLiteral is the case the proposal opens with, and the
// one a person actually writes. It has no slot to key off — the only thing that
// says "this is an enumeration" is the attribute path on the other side.
func TestEnumComparedToStringLiteral(t *testing.T) {
	ctx := pathCtx()
	for _, src := range []string{
		"$O/Status = 'Open'",
		"$O/Status != 'Open'",
		"'Open' = $O/Status", // operand order must not matter
	} {
		_, hs := NewParser().Parse(src, ctx)
		if len(hs) != 1 {
			t.Fatalf("%s produced %d hints, want 1: %+v", src, len(hs), hs)
		}
		if hs[0].Code != "E001" {
			t.Errorf("%s reported %s, want E001 — the same code as the slot form", src, hs[0].Code)
		}
		if hs[0].Fix != "Shop.OrderStatus.Open" {
			t.Errorf("%s suggested %q, want the qualified enum value", src, hs[0].Fix)
		}
		if hs[0].Reference == nil || len(hs[0].Reference.EnumValues) != 3 {
			t.Errorf("%s did not carry the enum's legal values: %+v", src, hs[0].Reference)
		}
	}
}

// TestEnumComparisonLeavesValidFormsAlone is the control. Without it the test
// above would pass against a rule that flagged every comparison.
func TestEnumComparisonLeavesValidFormsAlone(t *testing.T) {
	ctx := pathCtx()
	for _, src := range []string{
		"$O/Status = Shop.OrderStatus.Open", // the correct spelling
		"$C/Name = 'Ada'",                   // a String attribute really is compared to a string
		"$O/Total = 10",                     // no string literal at all
		"$Unknown/Status = 'Open'",          // unresolvable variable — no guessing
		"'Open' = 'Open'",                   // two literals, no attribute
	} {
		if _, hs := NewParser().Parse(src, ctx); len(hs) != 0 {
			t.Errorf("%s was flagged: %+v", src, hs)
		}
	}
}

// TestEnumComparisonNeedsTheCatalog pins that the rule cannot fire without the
// seams, so a project-less check is unaffected.
func TestEnumComparisonNeedsTheCatalog(t *testing.T) {
	if _, hs := NewParser().Parse("$O/Status = 'Open'", Context{}); len(hs) != 0 {
		t.Errorf("a syntax-only context reported %+v", hs)
	}
}

// TestConcatWithResolvedPath pins that the existing E004 rule benefits from the
// resolution too — a Boolean or Enumeration operand cannot be concatenated,
// while Mendix does auto-convert a numeric one (which is why Total is silent).
func TestConcatWithResolvedPath(t *testing.T) {
	ctx := pathCtx()
	if _, hs := NewParser().Parse("'status: ' + $O/Status", ctx); len(hs) != 1 || hs[0].Code != "E004" {
		t.Errorf("concatenating an Enumeration was not flagged: %+v", hs)
	}
	if _, hs := NewParser().Parse("'total: ' + $O/Total", ctx); len(hs) != 0 {
		t.Errorf("concatenating a Decimal was flagged, but Mendix auto-converts it: %+v", hs)
	}
}
