// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// studioProRuleKeys is the exact key set of a Studio Pro-authored rule document,
// measured on Rules.Rule1 and Rules.Rule2 (ako/TestApp, Mendix 11.13.0). An
// mxcli-authored rule must match it — no more (inventing microflow-only model
// the user does not have) and no less.
//
// ExportLevel is the reason this test exists: the first authored rule was
// missing exactly that one key, and mx check passed anyway.
var studioProRuleKeys = []string{
	"$ID", "$Type",
	"ApplyEntityAccess", "Documentation", "Excluded", "ExportLevel", "Flows",
	"MarkAsUsed", "MicroflowReturnType", "Name", "ObjectCollection", "ReturnVariableName",
}

func TestAuthoredRuleMatchesStudioProKeySet(t *testing.T) {
	rule := &microflows.Rule{
		Name:               "Rule_NameNotEmpty",
		ReturnType:         &microflows.BooleanType{},
		ReturnVariableName: "Variable",
	}

	encoded, err := (&codec.Encoder{}).Encode(ruleToGen(rule, 11))
	if err != nil {
		t.Fatalf("encode rule: %v", err)
	}
	elems, err := bson.Raw(encoded).Elements()
	if err != nil {
		t.Fatalf("read encoded rule: %v", err)
	}

	var got []string
	for _, e := range elems {
		got = append(got, e.Key())
	}
	sort.Strings(got)
	want := append([]string(nil), studioProRuleKeys...)
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("authored rule has %d keys %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("key %d = %q, want %q (full set: %v)", i, got[i], want[i], got)
		}
	}
}

// ExportLevel specifically: both reference rules store "Hidden", and both engines
// already hardcode it for microflows.
func TestAuthoredRuleSetsExportLevel(t *testing.T) {
	encoded, err := (&codec.Encoder{}).Encode(ruleToGen(&microflows.Rule{
		Name:       "R",
		ReturnType: &microflows.BooleanType{},
	}, 11))
	if err != nil {
		t.Fatalf("encode rule: %v", err)
	}
	val, err := bson.Raw(encoded).LookupErr("ExportLevel")
	if err != nil {
		t.Fatalf("authored rule has no ExportLevel; Studio Pro writes it on every rule")
	}
	if s, ok := val.StringValueOK(); !ok || s != "Hidden" {
		t.Errorf("ExportLevel = %v, want \"Hidden\"", val)
	}
}
