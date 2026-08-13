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
//
// MaxLength and EqualsTo stay refused. MDL cannot author either, so a stored
// one only ever arrives via a read-modify-write, and neither is carried on the
// model: EqualsTo needs UseValue + a value + an attribute reference, and gen
// binds its value under the wrong key too ("EqualsToValue" where Studio Pro
// stores "value" — same generator fault as RegExIdentifier, unpatched because
// nothing here writes the type).
func TestRuleInfoToGen_RefusesUnreproducibleTypes(t *testing.T) {
	for _, ruleType := range []string{"MaxLength", "EqualsTo"} {
		vr := &domainmodel.ValidationRule{Type: ruleType}
		if ruleInfoToGen(vr) != nil {
			t.Errorf("ruleInfoToGen(%q) returned an element — a fallback silently downgrades the rule", ruleType)
		}
		if reproducibleRule(vr) {
			t.Errorf("reproducibleRule(%q) = true", ruleType)
		}
	}
	for _, ruleType := range []string{"Required", "Unique", ""} {
		if ruleInfoToGen(&domainmodel.ValidationRule{Type: ruleType}) == nil {
			t.Errorf("ruleInfoToGen(%q) = nil, want an element", ruleType)
		}
	}
}

// TestRuleInfoToGen_RefusesPayloadlessRules covers the trap that a rule type
// alone is not enough to rebuild the rule. A RegEx rule whose reference did not
// survive the read, or a Range with neither bound, must be refused — writing
// the bare RuleInfo would produce a rule Mendix accepts and that means nothing.
func TestRuleInfoToGen_RefusesPayloadlessRules(t *testing.T) {
	for _, vr := range []*domainmodel.ValidationRule{
		{Type: "RegEx"},
		{Type: "RegEx", Rule: &domainmodel.RegexValidationRuleInfo{}},
		{Type: "Range"},
		{Type: "Range", Rule: &domainmodel.RangeValidationRuleInfo{}},
	} {
		if got := ruleInfoToGen(vr); got != nil {
			t.Errorf("%s with rule %#v -> %T, want nil", vr.Type, vr.Rule, got)
		}
	}
}

// TestRuleInfoToGen_RegExCarriesTheReference is what the gen storage-name
// override buys: the regex rule can now be written, and it lands on the key
// Mendix reads ("RegExIdentifier" — see modelsdk/gen storagename_test.go).
func TestRuleInfoToGen_RegExCarriesTheReference(t *testing.T) {
	vr := &domainmodel.ValidationRule{
		Type: "RegEx",
		Rule: &domainmodel.RegexValidationRuleInfo{RegularExpressionQualifiedName: "MyModule.EmailPattern"},
	}
	el := ruleInfoToGen(vr)
	ri, ok := el.(*genDm.RegExRuleInfo)
	if !ok {
		t.Fatalf("got %T, want *genDm.RegExRuleInfo", el)
	}
	if got := ri.RegularExpressionQualifiedName(); got != "MyModule.EmailPattern" {
		t.Errorf("reference = %q, want MyModule.EmailPattern", got)
	}
}

// TestRuleInfoToGen_RangeDerivesTypeOfRange pins the enum Mendix stores
// alongside the bounds. There are only three values — a one-sided range is
// GreaterThanOrEqualTo or SmallerThanOrEqualTo, never a strict inequality, so
// MDL's `< x` / `> x` forms have no Mendix counterpart and are rejected in the
// executor rather than quietly widened to the inclusive rule.
func TestRuleInfoToGen_RangeDerivesTypeOfRange(t *testing.T) {
	lo, hi := "1", "100"
	tests := []struct {
		name string
		info *domainmodel.RangeValidationRuleInfo
		want string
	}{
		{"between", &domainmodel.RangeValidationRuleInfo{
			MinValue: &lo, MaxValue: &hi, UseMinValue: true, UseMaxValue: true}, "Between"},
		{"min only", &domainmodel.RangeValidationRuleInfo{
			MinValue: &lo, UseMinValue: true}, "GreaterThanOrEqualTo"},
		{"max only", &domainmodel.RangeValidationRuleInfo{
			MaxValue: &hi, UseMaxValue: true}, "SmallerThanOrEqualTo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			el := ruleInfoToGen(&domainmodel.ValidationRule{Type: "Range", Rule: tt.info})
			ri, ok := el.(*genDm.RangeRuleInfo)
			if !ok {
				t.Fatalf("got %T, want *genDm.RangeRuleInfo", el)
			}
			if got := ri.TypeOfRange(); got != tt.want {
				t.Errorf("TypeOfRange = %q, want %q", got, tt.want)
			}
			if ri.UseMinValue() != tt.info.UseMinValue || ri.UseMaxValue() != tt.info.UseMaxValue {
				t.Errorf("bounds flags not carried: min=%v max=%v", ri.UseMinValue(), ri.UseMaxValue())
			}
		})
	}
}

// TestRuleInfoRoundTrip is the guard that matters for ALTER ENTITY: whatever
// the reader takes off a stored rule, the writer must put back. The pair used
// to lose everything but the type name, which is how a RegEx rule came back as
// Required. An attribute-bounded Range is in here because MDL cannot author one
// — it can only ever arrive from a stored document, so nothing else would
// notice it being dropped.
func TestRuleInfoRoundTrip(t *testing.T) {
	lo, hi := "1", "100"
	tests := []struct {
		name string
		in   element.Element
		want *domainmodel.ValidationRule
	}{
		{
			name: "regex",
			in: func() element.Element {
				o := genDm.NewRegExRuleInfo()
				o.SetRegularExpressionQualifiedName("MyModule.EmailPattern")
				return o
			}(),
			want: &domainmodel.ValidationRule{Type: "RegEx"},
		},
		{
			name: "range between literals",
			in: func() element.Element {
				o := genDm.NewRangeRuleInfo()
				o.SetTypeOfRange("Between")
				o.SetUseMinValue(true)
				o.SetUseMaxValue(true)
				o.SetMinValue(lo)
				o.SetMaxValue(hi)
				return o
			}(),
			want: &domainmodel.ValidationRule{Type: "Range"},
		},
		{
			name: "range bounded by an attribute",
			in: func() element.Element {
				o := genDm.NewRangeRuleInfo()
				o.SetTypeOfRange("GreaterThanOrEqualTo")
				o.SetUseMinValue(true)
				o.SetMinAttributeQualifiedName("MyModule.Booking.StartDate")
				return o
			}(),
			want: &domainmodel.ValidationRule{Type: "Range"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := ruleInfoFromGen(tt.in)
			if payload == nil {
				t.Fatalf("reader dropped the payload for %s", tt.in.TypeName())
			}
			vr := &domainmodel.ValidationRule{Type: tt.want.Type, Rule: payload}

			out := ruleInfoToGen(vr)
			if out == nil {
				t.Fatalf("writer refused a rule the reader understood — round trip loses it")
			}
			if out.TypeName() != tt.in.TypeName() {
				t.Fatalf("type changed: %s -> %s", tt.in.TypeName(), out.TypeName())
			}

			switch want := tt.in.(type) {
			case *genDm.RegExRuleInfo:
				got := out.(*genDm.RegExRuleInfo)
				if got.RegularExpressionQualifiedName() != want.RegularExpressionQualifiedName() {
					t.Errorf("reference %q -> %q", want.RegularExpressionQualifiedName(), got.RegularExpressionQualifiedName())
				}
			case *genDm.RangeRuleInfo:
				got := out.(*genDm.RangeRuleInfo)
				for _, f := range []struct {
					name      string
					want, got string
				}{
					{"TypeOfRange", want.TypeOfRange(), got.TypeOfRange()},
					{"MinValue", want.MinValue(), got.MinValue()},
					{"MaxValue", want.MaxValue(), got.MaxValue()},
					{"MinAttribute", want.MinAttributeQualifiedName(), got.MinAttributeQualifiedName()},
					{"MaxAttribute", want.MaxAttributeQualifiedName(), got.MaxAttributeQualifiedName()},
				} {
					if f.want != f.got {
						t.Errorf("%s %q -> %q", f.name, f.want, f.got)
					}
				}
				if got.UseMinValue() != want.UseMinValue() || got.UseMaxValue() != want.UseMaxValue() {
					t.Errorf("bound flags not preserved")
				}
			}
		})
	}
}

func TestValidationRulesAreReproducible(t *testing.T) {
	e := &domainmodel.Entity{ValidationRules: []*domainmodel.ValidationRule{{Type: "Required"}}}
	if _, ok := validationRulesAreReproducible(e); !ok {
		t.Error("Required should be reproducible")
	}
	e.ValidationRules = append(e.ValidationRules, &domainmodel.ValidationRule{Type: "EqualsTo"})
	got, ok := validationRulesAreReproducible(e)
	if ok || got != "EqualsTo" {
		t.Errorf("got (%q, %v), want (EqualsTo, false)", got, ok)
	}
}
