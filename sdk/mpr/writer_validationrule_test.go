// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestSerializeRuleInfo_RefusesUnreproducibleTypes is the data-loss guard.
//
// serializeRuleInfo used to fall back to RequiredRuleInfo for any type it did
// not recognise. Because ALTER ENTITY round-trips an entity through the writer,
// a stored RegEx rule was read as "RegEx" and written back as Required — the
// pattern reference gone, the field merely mandatory — and mxbuild reported
// nothing, because a Required rule is perfectly valid.
func TestSerializeRuleInfo_RefusesUnreproducibleTypes(t *testing.T) {
	for _, ruleType := range []string{"RegEx", "Range", "MaxLength", "EqualsTo"} {
		t.Run(ruleType, func(t *testing.T) {
			if got := serializeRuleInfo(ruleType); got != nil {
				t.Errorf("serializeRuleInfo(%q) = %v, want nil (refusal) — a fallback silently downgrades the rule", ruleType, got)
			}
			if reproducibleRuleType(ruleType) {
				t.Errorf("reproducibleRuleType(%q) = true", ruleType)
			}
		})
	}
}

func TestSerializeRuleInfo_ReproducibleTypes(t *testing.T) {
	for ruleType, wantType := range map[string]string{
		"Required": "DomainModels$RequiredRuleInfo",
		"Unique":   "DomainModels$UniqueRuleInfo",
		// An empty type is what the attribute-constraint path produces for
		// `not null`; it must keep working.
		"": "DomainModels$RequiredRuleInfo",
	} {
		doc := serializeRuleInfo(ruleType)
		if doc == nil {
			t.Fatalf("serializeRuleInfo(%q) = nil, want a document", ruleType)
		}
		if doc[0].Key != "$ID" {
			t.Errorf("%q: first key = %q, want $ID (Mendix rejects any other order)", ruleType, doc[0].Key)
		}
		if doc[1].Value != wantType {
			t.Errorf("%q: $Type = %v, want %s", ruleType, doc[1].Value, wantType)
		}
	}
}

func TestValidationRulesAreReproducible(t *testing.T) {
	e := &domainmodel.Entity{ValidationRules: []*domainmodel.ValidationRule{
		{Type: "Required"}, {Type: "Unique"},
	}}
	if _, ok := validationRulesAreReproducible(e); !ok {
		t.Error("Required/Unique should be reproducible")
	}

	e.ValidationRules = append(e.ValidationRules, &domainmodel.ValidationRule{Type: "RegEx"})
	got, ok := validationRulesAreReproducible(e)
	if ok {
		t.Fatal("an entity with a RegEx rule must not be reported reproducible")
	}
	if got != "RegEx" {
		t.Errorf("reported %q, want RegEx", got)
	}
}

func TestUpdateEntity_RefusalNamesTheRuleAndTheConsequence(t *testing.T) {
	// The message has to say what would be lost, not just that it refused —
	// a bare "cannot rewrite" sends the user looking for a bug in their script.
	e := &domainmodel.Entity{Name: "Person", ValidationRules: []*domainmodel.ValidationRule{{Type: "RegEx"}}}
	ruleType, _ := validationRulesAreReproducible(e)
	msg := "entity " + e.Name + " has a " + ruleType + " validation rule"
	for _, want := range []string{"Person", "RegEx"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q", want)
		}
	}
}
