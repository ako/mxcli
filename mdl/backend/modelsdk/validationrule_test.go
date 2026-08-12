// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"testing"

	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestRuleTypeFromGen_ReportsWhatItFound is half of the data-loss fix.
//
// The reader used to collapse everything except Unique to "Required". Combined
// with a writer that also defaulted to Required, an ALTER ENTITY round trip
// turned a stored RegEx rule into a Required one: the pattern reference gone,
// the field merely mandatory, and mxbuild silent because both are valid rules.
func TestRuleTypeFromGen_ReportsWhatItFound(t *testing.T) {
	tests := []struct {
		info any
		want string
	}{
		{genDm.NewRequiredRuleInfo(), "Required"},
		{genDm.NewUniqueRuleInfo(), "Unique"},
		{genDm.NewRegExRuleInfo(), "RegEx"},
		{genDm.NewRangeRuleInfo(), "Range"},
		{genDm.NewMaxLengthRuleInfo(), "MaxLength"},
		{genDm.NewEqualsToRuleInfo(), "EqualsTo"},
	}
	for _, tt := range tests {
		el, ok := tt.info.(element.Element)
		if !ok {
			t.Fatalf("%T is not an element.Element", tt.info)
		}
		if got := ruleTypeFromGen(el); got != tt.want {
			t.Errorf("%s -> %q, want %q", el.TypeName(), got, tt.want)
		}
	}
}

// TestRuleInfoToGen_RefusesUnreproducibleTypes is the other half: the writer
// must return nil (a refusal) rather than a Required rule it did not mean.
func TestRuleInfoToGen_RefusesUnreproducibleTypes(t *testing.T) {
	for _, ruleType := range []string{"RegEx", "Range", "MaxLength", "EqualsTo"} {
		if ruleInfoToGen(ruleType) != nil {
			t.Errorf("ruleInfoToGen(%q) returned an element — a fallback silently downgrades the rule", ruleType)
		}
		if reproducibleRuleType(ruleType) {
			t.Errorf("reproducibleRuleType(%q) = true", ruleType)
		}
	}
	for _, ruleType := range []string{"Required", "Unique", ""} {
		if ruleInfoToGen(ruleType) == nil {
			t.Errorf("ruleInfoToGen(%q) = nil, want an element", ruleType)
		}
	}
}

func TestValidationRulesAreReproducible(t *testing.T) {
	e := &domainmodel.Entity{ValidationRules: []*domainmodel.ValidationRule{{Type: "Required"}}}
	if _, ok := validationRulesAreReproducible(e); !ok {
		t.Error("Required should be reproducible")
	}
	e.ValidationRules = append(e.ValidationRules, &domainmodel.ValidationRule{Type: "Range"})
	got, ok := validationRulesAreReproducible(e)
	if ok || got != "Range" {
		t.Errorf("got (%q, %v), want (Range, false)", got, ok)
	}
}
