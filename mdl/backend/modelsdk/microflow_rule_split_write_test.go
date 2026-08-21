// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ruleSplitMicroflow is the shape upstream #939 reported: a decision whose
// condition calls a rule, which MDL writes for `if Module.SomeRule(arg = $x)`.
func ruleSplitMicroflow() *microflows.Microflow {
	cond := &microflows.RuleSplitCondition{
		RuleQualifiedName: "Sample.Rule_IsActive",
		ParameterMappings: []*microflows.RuleCallParameterMapping{{
			ParameterName: "Sample.Rule_IsActive.IsActive",
			Argument:      "$IsActive",
		}},
	}
	cond.ID = model.ID("cond-1")
	cond.ParameterMappings[0].ID = model.ID("pm-1")

	split := &microflows.ExclusiveSplit{
		Caption:        "Sample.Rule_IsActive(IsActive = $IsActive)",
		SplitCondition: cond,
	}
	split.ID = model.ID("split-1")

	mf := &microflows.Microflow{
		Name: "MF_UseRule",
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{split},
		},
	}
	mf.ID = model.ID("mf-1")
	return mf
}

// TestMicroflowRoundTrip_RuleSplitCondition guards upstream #939. splitConditionToGen
// had no *microflows.RuleSplitCondition case, so the condition hit `default: return
// nil` and the caller skipped SetSplitCondition entirely — the decision was stored
// with no Condition at all. Measured on mxbuild 11.13.0: CE0080 "The 'Condition'
// property is required", with the Decision's caption still showing the call, while
// the same script on the legacy engine checked clean.
//
// The read side had been implemented for #723, which is what made this silent:
// `describe microflow` rendered `if true then` and the caption kept the original
// text, so the round-trip looked like valid MDL either way.
func TestMicroflowRoundTrip_RuleSplitCondition(t *testing.T) {
	got := roundTripMicroflow(t, ruleSplitMicroflow())

	var split *microflows.ExclusiveSplit
	if got.ObjectCollection != nil {
		for _, obj := range got.ObjectCollection.Objects {
			if s, ok := obj.(*microflows.ExclusiveSplit); ok {
				split = s
			}
		}
	}
	if split == nil {
		t.Fatal("ExclusiveSplit did not survive the round trip")
	}
	cond, ok := split.SplitCondition.(*microflows.RuleSplitCondition)
	if !ok {
		t.Fatalf("SplitCondition = %T, want *microflows.RuleSplitCondition — a nil condition "+
			"is mx check CE0080 and renders as `if true then`", split.SplitCondition)
	}
	if cond.RuleQualifiedName != "Sample.Rule_IsActive" {
		t.Errorf("RuleQualifiedName = %q, want Sample.Rule_IsActive", cond.RuleQualifiedName)
	}
	if len(cond.ParameterMappings) != 1 {
		t.Fatalf("ParameterMappings = %d, want 1", len(cond.ParameterMappings))
	}
	if pm := cond.ParameterMappings[0]; pm.ParameterName != "Sample.Rule_IsActive.IsActive" || pm.Argument != "$IsActive" {
		t.Errorf("parameter mapping = {%q, %q}, want {Sample.Rule_IsActive.IsActive, $IsActive}",
			pm.ParameterName, pm.Argument)
	}
}

// The round trip above passes whichever key the rule name is stored under, as long
// as both sides agree — so it cannot see the second half of #939: modelsdk/gen binds
// Microflows$RuleCall.Rule to the BSON key "Rule", and Mendix stores it as
// "Microflow" (rules share the microflow namespace). generated/metamodel
// (`json:"microflow"`), the legacy writer and modelsdk/gen/keyaudit_test.go all
// agree. Written as "Rule" the reference is invisible to Mendix and the decision is
// CE0080 again, for a different reason. Assert the storage key on the raw document.
func TestRuleSplitConditionUsesMicroflowStorageKey(t *testing.T) {
	raw, err := (&codec.Encoder{}).Encode(microflowToGen(ruleSplitMicroflow(), 11))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	objects, err := bson.Raw(raw).LookupErr("ObjectCollection", "Objects")
	if err != nil {
		t.Fatalf("no ObjectCollection.Objects: %v", err)
	}
	vals, err := objects.Array().Values()
	if err != nil {
		t.Fatalf("Objects array: %v", err)
	}
	var ruleCall bson.Raw
	for _, v := range vals {
		doc, ok := v.DocumentOK() // the leading int32 typed-array marker is not a document
		if !ok {
			continue
		}
		if rc, err := doc.LookupErr("SplitCondition", "RuleCall"); err == nil {
			ruleCall = rc.Document()
		}
	}
	if ruleCall == nil {
		t.Fatal("no SplitCondition.RuleCall on the encoded split")
	}

	if _, err := ruleCall.LookupErr("Rule"); err == nil {
		t.Error("RuleCall stores the reference under \"Rule\"; Mendix reads \"Microflow\" and " +
			"reports CE0080 \"The 'Condition' property is required\"")
	}
	name, err := ruleCall.LookupErr("Microflow")
	if err != nil {
		t.Fatalf("RuleCall has no \"Microflow\" key: %v", err)
	}
	if s, _ := name.StringValueOK(); s != "Sample.Rule_IsActive" {
		t.Errorf("Microflow = %q, want Sample.Rule_IsActive", s)
	}

	// Studio Pro and the legacy writer lead the ParameterMappings list with
	// typed-array marker 2, not the codec's default 3.
	mappings, err := ruleCall.LookupErr("ParameterMappings")
	if err != nil {
		t.Fatalf("RuleCall has no ParameterMappings: %v", err)
	}
	entries, err := mappings.Array().Values()
	if err != nil || len(entries) == 0 {
		t.Fatalf("ParameterMappings array: %v", err)
	}
	if m, ok := entries[0].Int32OK(); !ok || m != 2 {
		t.Errorf("ParameterMappings marker = %v, want int32 2", entries[0])
	}
}
