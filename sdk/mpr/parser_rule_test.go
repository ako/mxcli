// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

// studioProRuleBSON reproduces Rules.Rule2 from the reference app
// (ako/TestApp, Mendix 11.13.0) — an enumeration-returning rule with an entity
// parameter, in the key order Studio Pro stores.
//
// The two deliberate omissions are the point of the fixture: a real rule
// document carries no AllowedModuleRoles and no ReturnType, so a parser that
// reaches for either is reading a key Mendix never wrote.
func studioProRuleBSON(t *testing.T) []byte {
	t.Helper()
	doc := bson.D{
		{Key: "$ID", Value: "rule-1"},
		{Key: "$Type", Value: "Microflows$Rule"},
		{Key: "ApplyEntityAccess", Value: false},
		{Key: "Documentation", Value: "decides the outcome"},
		{Key: "Excluded", Value: false},
		{Key: "ExportLevel", Value: "Hidden"},
		{Key: "Flows", Value: bson.A{int32(3)}},
		{Key: "MarkAsUsed", Value: false},
		{Key: "MicroflowReturnType", Value: bson.D{
			{Key: "$ID", Value: "rt-1"},
			{Key: "$Type", Value: "DataTypes$EnumerationType"},
			{Key: "Enumeration", Value: "Rules.RuleResult"},
		}},
		{Key: "Name", Value: "Rule2"},
		{Key: "ObjectCollection", Value: bson.D{
			{Key: "$ID", Value: "oc-1"},
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{
				int32(3),
				bson.D{
					{Key: "$ID", Value: "p-1"},
					{Key: "$Type", Value: "Microflows$MicroflowParameter"},
					{Key: "Name", Value: "pName"},
					{Key: "VariableType", Value: bson.D{
						{Key: "$ID", Value: "vt-1"},
						{Key: "$Type", Value: "DataTypes$ObjectType"},
						{Key: "Entity", Value: "Pages.Bus"},
					}},
				},
				bson.D{
					{Key: "$ID", Value: "end-1"},
					{Key: "$Type", Value: "Microflows$EndEvent"},
					{Key: "ReturnValue", Value: "Rules.RuleResult.Approved"},
				},
			}},
		}},
		{Key: "ReturnVariableName", Value: "Variable"},
	}
	b, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal rule fixture: %v", err)
	}
	return b
}

// The legacy engine reads a rule with the same fidelity as the codec engine:
// name, documentation, the enumeration return type, the entity parameter, and
// ReturnVariableName (which Studio Pro writes and a rewrite must not drop).
func TestParseRule_StudioProDocument(t *testing.T) {
	rule, err := testReader().parseRule("rule-1", "container-1", studioProRuleBSON(t))
	if err != nil {
		t.Fatalf("parseRule: %v", err)
	}

	if rule.Name != "Rule2" {
		t.Errorf("Name = %q, want Rule2", rule.Name)
	}
	if rule.Documentation != "decides the outcome" {
		t.Errorf("Documentation = %q", rule.Documentation)
	}
	if rule.ReturnVariableName != "Variable" {
		t.Errorf("ReturnVariableName = %q, want %q", rule.ReturnVariableName, "Variable")
	}
	if rule.TypeName != "Microflows$Rule" {
		t.Errorf("TypeName = %q, want Microflows$Rule", rule.TypeName)
	}

	enum, ok := rule.ReturnType.(*microflows.EnumerationType)
	if !ok {
		t.Fatalf("ReturnType = %T, want *microflows.EnumerationType — a rule may return an enumeration, not only Boolean", rule.ReturnType)
	}
	if enum.EnumerationQualifiedName != "Rules.RuleResult" {
		t.Errorf("enumeration = %q, want Rules.RuleResult", enum.EnumerationQualifiedName)
	}

	if len(rule.Parameters) != 1 {
		t.Fatalf("Parameters = %d, want 1", len(rule.Parameters))
	}
	if rule.Parameters[0].Name != "pName" {
		t.Errorf("parameter = %q, want pName", rule.Parameters[0].Name)
	}
	if rule.ObjectCollection == nil || len(rule.ObjectCollection.Objects) == 0 {
		t.Error("ObjectCollection did not come back")
	}
}
