// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"sort"
	"strings"
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1317 — a flow argument bound through Expression (a literal,
// $currentObject, a path, a primitive parameter) was written WITHOUT the
// Variable key. Studio Pro always writes all three keys and nulls the unused
// slot, so opening the change in the Changes panel crashed with
//
//	Objects with ID … of type Forms$MicroflowParameterMapping do not have the
//	same properties. baseNames = Expression, Parameter, Variable;
//	newNames = Parameter, Expression
//
// mx check and mxbuild are both clean on it; only the Changes-panel diff sees
// the key set. Same class as #1180 (DomainModels$NoGeneralization).

// mappingKeys returns the non-$ keys of a parameter mapping, sorted.
func mappingKeys(pm bsonv1.D) []string {
	var keys []string
	for _, e := range pm {
		if !strings.HasPrefix(e.Key, "$") {
			keys = append(keys, e.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

// assertStudioProMappingShape checks the key set Studio Pro writes for a
// Forms$MicroflowParameterMapping / Forms$NanoflowParameterMapping at 11.12.2
// (the baseNames of the reported crash), with Variable null when unused.
func assertStudioProMappingShape(t *testing.T, pm bsonv1.D) {
	t.Helper()
	got := strings.Join(mappingKeys(pm), ", ")
	if want := "Expression, Parameter, Variable"; got != want {
		t.Fatalf("mapping keys = %s; Studio Pro writes %s — the Changes panel "+
			"throws \"do not have the same properties\" on the difference", got, want)
	}
	if v := docGet(pm, "Variable"); v != nil {
		t.Errorf("Variable = %#v, want null for an Expression-bound argument", v)
	}
}

func TestMicroflowActionExpressionArgWritesNullVariable(t *testing.T) {
	a := &pages.MicroflowClientAction{
		MicroflowName: "LearningJourney.ACT_ExcludeItemStep",
		ParameterMappings: []*pages.MicroflowParameterMapping{
			{ParameterName: "LearningJourney", Expression: "$LearningJourney"},
		},
	}
	// The reported path: a pluggable widget's named action slot, serialized
	// through the child serializer the page mutator calls.
	doc := codecChildSerializer{}.SerializeClientAction(a)
	settings, ok := docGet(doc, "MicroflowSettings").(bsonv1.D)
	if !ok {
		t.Fatalf("MicroflowSettings missing: %#v", doc)
	}
	assertStudioProMappingShape(t, firstParamMapping(t, settings))
}

func TestNanoflowActionExpressionArgWritesNullVariable(t *testing.T) {
	a := &pages.NanoflowClientAction{
		NanoflowName: "MyModule.ACT_Toggle_NF",
		ParameterMappings: []*pages.NanoflowParameterMapping{
			{ParameterName: "Flag", Expression: "true"},
		},
	}
	assertStudioProMappingShape(t, firstParamMapping(t, encodeAction(t, a)))
}

// Control: the Variable-bound form already wrote all three keys (#1140) and
// must keep its PageVariable rather than be nulled by the default.
func TestMicroflowActionVariableArgKeepsPageVariable(t *testing.T) {
	a := &pages.MicroflowClientAction{
		MicroflowName: "LearningJourney.ACT_ExcludeItemStep",
		ParameterMappings: []*pages.MicroflowParameterMapping{
			{ParameterName: "LearningJourney", Variable: "$LearningJourney", VariableKind: "parameter"},
		},
	}
	settings, _ := docGet(encodeAction(t, a), "MicroflowSettings").(bsonv1.D)
	pm := firstParamMapping(t, settings)
	if got := strings.Join(mappingKeys(pm), ", "); got != "Expression, Parameter, Variable" {
		t.Fatalf("mapping keys = %s", got)
	}
	if paramVariable(pm) == nil {
		t.Fatalf("Variable = %#v, want a Forms$PageVariable", docGet(pm, "Variable"))
	}
}
