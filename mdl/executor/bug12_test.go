// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestRuleTargetsAttribute guards the AttributeID matcher used by MODIFY
// ATTRIBUTE … NULLABLE (Bug 12a). A validation rule's AttributeID is either the
// bare attribute UUID (freshly-created rules) or the fully-qualified attribute
// name Module.Entity.Attr (read-back rules); NULLABLE must match both to drop
// the Required rule.
func TestRuleTargetsAttribute(t *testing.T) {
	attr := &domainmodel.Attribute{Name: "Foo"}
	attr.ID = model.ID("65cc13a2-b73d-49a9-9542-3fbb91ef52b2")

	cases := []struct {
		ruleAttrID string
		want       bool
	}{
		{"65cc13a2-b73d-49a9-9542-3fbb91ef52b2", true}, // bare UUID (create)
		{"T12.ZZTmp.Foo", true},                        // qualified name (read-back)
		{"Foo", true},                                  // bare name
		{"T12.ZZTmp.Bar", false},                       // different attribute
		{"other-uuid", false},
		{"", false},
	}
	for _, c := range cases {
		if got := ruleTargetsAttribute(c.ruleAttrID, attr); got != c.want {
			t.Errorf("ruleTargetsAttribute(%q, Foo) = %v, want %v", c.ruleAttrID, got, c.want)
		}
	}
}

// TestSetAttributeValidationRule verifies the add/remove/preserve semantics used
// by MODIFY ATTRIBUTE. Bug 12a.
func TestSetAttributeValidationRule(t *testing.T) {
	foo := &domainmodel.Attribute{Name: "Foo"}
	foo.ID = model.ID("foo-id")
	bar := &domainmodel.Attribute{Name: "Bar"}
	bar.ID = model.ID("bar-id")

	// Start with Required on Foo (read-back qualified form) and Required on Bar.
	entity := &domainmodel.Entity{
		Attributes: []*domainmodel.Attribute{foo, bar},
		ValidationRules: []*domainmodel.ValidationRule{
			{AttributeID: "M.E.Foo", Type: "Required"},
			{AttributeID: "M.E.Bar", Type: "Required"},
		},
	}

	// NULLABLE on Foo: drop Foo's Required, keep Bar's.
	setAttributeValidationRule(entity, foo, "M.E.Foo", "Required", false, "")
	if n := countRules(entity, "Required"); n != 1 {
		t.Fatalf("after NULLABLE Foo: %d Required rules, want 1 (Bar's)", n)
	}
	if hasRuleForAttr(entity, "Required", foo) {
		t.Error("Foo's Required rule should have been removed")
	}
	if !hasRuleForAttr(entity, "Required", bar) {
		t.Error("Bar's Required rule must be preserved")
	}

	// NOT NULL on Foo again: re-add (idempotent — no duplicates).
	setAttributeValidationRule(entity, foo, "M.E.Foo", "Required", true, "")
	setAttributeValidationRule(entity, foo, "M.E.Foo", "Required", true, "")
	if n := countRulesForAttr(entity, "Required", foo); n != 1 {
		t.Errorf("Foo Required rules = %d, want exactly 1 (idempotent)", n)
	}
}

func countRules(e *domainmodel.Entity, typ string) int {
	n := 0
	for _, vr := range e.ValidationRules {
		if vr.Type == typ {
			n++
		}
	}
	return n
}
func countRulesForAttr(e *domainmodel.Entity, typ string, a *domainmodel.Attribute) int {
	n := 0
	for _, vr := range e.ValidationRules {
		if vr.Type == typ && ruleTargetsAttribute(string(vr.AttributeID), a) {
			n++
		}
	}
	return n
}
func hasRuleForAttr(e *domainmodel.Entity, typ string, a *domainmodel.Attribute) bool {
	return countRulesForAttr(e, typ, a) > 0
}
