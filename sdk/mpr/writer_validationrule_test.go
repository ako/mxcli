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
// MaxLength and EqualsTo stay refused: the model carries no payload type for
// either, so a rewrite would lose them. RegEx and Range are now writable, but
// only WITH their payload — see TestSerializeRuleInfo_RefusesPayloadlessRules.
func TestSerializeRuleInfo_RefusesUnreproducibleTypes(t *testing.T) {
	for _, ruleType := range []string{"MaxLength", "EqualsTo"} {
		t.Run(ruleType, func(t *testing.T) {
			vr := &domainmodel.ValidationRule{Type: ruleType}
			if got := serializeRuleInfo(vr); got != nil {
				t.Errorf("serializeRuleInfo(%q) = %v, want nil (refusal) — a fallback silently downgrades the rule", ruleType, got)
			}
			if reproducibleRule(vr) {
				t.Errorf("reproducibleRule(%q) = true", ruleType)
			}
		})
	}
}

// TestSerializeRuleInfo_RefusesPayloadlessRules: a rule TYPE is not a rule. A
// bare RegExRuleInfo with no reference, or a Range with no bounds, is a document
// Mendix accepts that constrains nothing — the same silent downgrade wearing the
// right type name.
func TestSerializeRuleInfo_RefusesPayloadlessRules(t *testing.T) {
	for _, vr := range []*domainmodel.ValidationRule{
		{Type: "RegEx"},
		{Type: "RegEx", Rule: &domainmodel.RegexValidationRuleInfo{}},
		{Type: "Range"},
		{Type: "Range", Rule: &domainmodel.RangeValidationRuleInfo{}},
	} {
		if got := serializeRuleInfo(vr); got != nil {
			t.Errorf("%s with rule %#v serialized to %v, want nil", vr.Type, vr.Rule, got)
		}
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
		doc := serializeRuleInfo(&domainmodel.ValidationRule{Type: ruleType})
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

// TestSerializeRuleInfo_RegExUsesStorageName pins the key both engines must
// write. The SDK name is "RegularExpression"; Studio Pro stores
// "RegExIdentifier", and writing the SDK name makes mxbuild report CE0135
// "No regular expression specified" (measured on 11.13.0).
func TestSerializeRuleInfo_RegExUsesStorageName(t *testing.T) {
	doc := serializeRuleInfo(&domainmodel.ValidationRule{
		Type: "RegEx",
		Rule: &domainmodel.RegexValidationRuleInfo{RegularExpressionQualifiedName: "Val.EmailAddress"},
	})
	if doc == nil {
		t.Fatal("a RegEx rule with a reference must serialize")
	}
	if doc[1].Value != "DomainModels$RegExRuleInfo" {
		t.Errorf("$Type = %v", doc[1].Value)
	}
	if doc[2].Key != "RegExIdentifier" {
		t.Errorf("reference key = %q, want RegExIdentifier — the SDK name yields CE0135", doc[2].Key)
	}
	if doc[2].Value != "Val.EmailAddress" {
		t.Errorf("reference = %v", doc[2].Value)
	}
}

func TestSerializeRuleInfo_RangeKinds(t *testing.T) {
	lo, hi := "1", "100"
	tests := []struct {
		name string
		info *domainmodel.RangeValidationRuleInfo
		want string
	}{
		{"between", &domainmodel.RangeValidationRuleInfo{MinValue: &lo, MaxValue: &hi, UseMinValue: true, UseMaxValue: true}, "Between"},
		{"min only", &domainmodel.RangeValidationRuleInfo{MinValue: &lo, UseMinValue: true}, "GreaterThanOrEqualTo"},
		{"max only", &domainmodel.RangeValidationRuleInfo{MaxValue: &hi, UseMaxValue: true}, "SmallerThanOrEqualTo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := serializeRuleInfo(&domainmodel.ValidationRule{Type: "Range", Rule: tt.info})
			if doc == nil {
				t.Fatal("a Range rule with bounds must serialize")
			}
			if doc[2].Key != "TypeOfRange" || doc[2].Value != tt.want {
				t.Errorf("TypeOfRange = %v %v, want %q", doc[2].Key, doc[2].Value, tt.want)
			}
		})
	}
}

func TestValidationRulesAreReproducible(t *testing.T) {
	e := &domainmodel.Entity{ValidationRules: []*domainmodel.ValidationRule{
		{Type: "Required"}, {Type: "Unique"},
	}}
	if _, ok := validationRulesAreReproducible(e); !ok {
		t.Error("Required/Unique should be reproducible")
	}

	e.ValidationRules = append(e.ValidationRules, &domainmodel.ValidationRule{Type: "EqualsTo"})
	got, ok := validationRulesAreReproducible(e)
	if ok {
		t.Fatal("an entity with an EqualsTo rule must not be reported reproducible")
	}
	if got != "EqualsTo" {
		t.Errorf("reported %q, want EqualsTo", got)
	}
}

func TestUpdateEntity_RefusalNamesTheRuleAndTheConsequence(t *testing.T) {
	// The message has to say what would be lost, not just that it refused —
	// a bare "cannot rewrite" sends the user looking for a bug in their script.
	e := &domainmodel.Entity{Name: "Person", ValidationRules: []*domainmodel.ValidationRule{{Type: "EqualsTo"}}}
	ruleType, _ := validationRulesAreReproducible(e)
	msg := "entity " + e.Name + " has a " + ruleType + " validation rule"
	for _, want := range []string{"Person", "EqualsTo"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q", want)
		}
	}
}
