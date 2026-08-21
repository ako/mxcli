// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDT "github.com/mendixlabs/mxcli/modelsdk/gen/datatypes"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// studioProRule is Rules.Rule2 from the reference app (ako/TestApp, Mendix
// 11.13.0), authored in Studio Pro: an enumeration-returning rule with an entity
// parameter. It is built through gen rather than pasted as BSON so the encode
// and decode key bindings are both exercised.
//
// Two properties of the real document are load-bearing here and are asserted
// below: a rule stores NO AllowedModuleRoles (it is not independently callable),
// and its return type lives under MicroflowReturnType — gen's sibling
// ReturnType string is not written by Studio Pro and must not be read.
func studioProRule(t *testing.T) *genMf.Rule {
	t.Helper()
	r := genMf.NewRule()
	r.SetID(element.ID("22222222-2222-2222-2222-222222222222"))
	r.SetName("Rule2")
	r.SetDocumentation("")
	r.SetExportLevel("Hidden")
	r.SetReturnVariableName("Variable")

	enum := genDT.NewEnumerationType()
	enum.SetID(element.ID("33333333-3333-3333-3333-333333333333"))
	enum.SetEnumerationQualifiedName("Rules.RuleResult")
	r.SetMicroflowReturnType(enum)

	oc := genMf.NewMicroflowObjectCollection()
	oc.SetID(element.ID("44444444-4444-4444-4444-444444444444"))

	param := genMf.NewMicroflowParameter()
	param.SetID(element.ID("55555555-5555-5555-5555-555555555555"))
	param.SetName("pName")
	objType := genDT.NewObjectType()
	objType.SetID(element.ID("66666666-6666-6666-6666-666666666666"))
	objType.SetEntityQualifiedName("Pages.Bus")
	param.SetParameterType(objType)
	oc.AddObjects(param)

	r.SetObjectCollection(oc)
	return r
}

// A rule round-trips through the codec with its enumeration return type and its
// parameter intact. The enumeration case matters: microflows.Rule's doc comment
// used to claim the return type is "always boolean", which Rules.Rule2 disproves.
func TestRuleFromGen_EnumerationReturnAndEntityParameter(t *testing.T) {
	encoded, err := (&codec.Encoder{}).Encode(studioProRule(t))
	if err != nil {
		t.Fatalf("encode rule: %v", err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(encoded))
	if err != nil {
		t.Fatalf("decode rule: %v", err)
	}
	g, ok := el.(*genMf.Rule)
	if !ok {
		t.Fatalf("decoded %T, want *genMf.Rule", el)
	}

	rule := ruleFromGen(g, model.ID("container-1"))
	if rule.Name != "Rule2" {
		t.Errorf("Name = %q, want Rule2", rule.Name)
	}
	if rule.ReturnVariableName != "Variable" {
		t.Errorf("ReturnVariableName = %q, want %q — Studio Pro writes it and a rewrite must not drop it",
			rule.ReturnVariableName, "Variable")
	}
	enum, ok := rule.ReturnType.(*microflows.EnumerationType)
	if !ok {
		t.Fatalf("ReturnType = %T, want *microflows.EnumerationType (a rule may return an enumeration, not only Boolean)", rule.ReturnType)
	}
	if enum.EnumerationQualifiedName != "Rules.RuleResult" {
		t.Errorf("enumeration = %q, want Rules.RuleResult", enum.EnumerationQualifiedName)
	}
	if len(rule.Parameters) != 1 || rule.Parameters[0].Name != "pName" {
		t.Fatalf("Parameters = %+v, want one named pName", rule.Parameters)
	}
}

// The storage keys a rule does NOT carry. Measured on both Studio Pro reference
// rules: neither AllowedModuleRoles nor ReturnType appears, where a microflow in
// the same app stores nine properties a rule has no concept of. Writing either
// would be inventing model the user does not have.
func TestEncodedRuleOmitsMicroflowOnlyKeys(t *testing.T) {
	encoded, err := (&codec.Encoder{}).Encode(studioProRule(t))
	if err != nil {
		t.Fatalf("encode rule: %v", err)
	}
	raw := bson.Raw(encoded)
	for _, key := range []string{
		"AllowedModuleRoles", "ReturnType", "AllowConcurrentExecution",
		"ConcurrenyErrorMessage", "ConcurrencyErrorMicroflow", "StableId",
		"Url", "UrlSearchParameters", "MicroflowActionInfo", "WorkflowActionInfo",
	} {
		if _, err := raw.LookupErr(key); err == nil {
			t.Errorf("encoded rule carries %q; Studio Pro writes only the ten rule properties", key)
		}
	}
	// Control: the keys it must carry are present, so the loop above is not
	// passing because the document is empty.
	for _, key := range []string{"Name", "ObjectCollection", "MicroflowReturnType", "ReturnVariableName"} {
		if _, err := raw.LookupErr(key); err != nil {
			t.Errorf("encoded rule is missing %q", key)
		}
	}
}
