// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func encodeAction(t *testing.T, a pages.ClientAction) bsonv1.D {
	t.Helper()
	el, err := clientActionToGen(a)
	if err != nil {
		t.Fatalf("clientActionToGen: %v", err)
	}
	out, err := (&codec.Encoder{}).Encode(el.(element.Element))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var doc bsonv1.D
	if err := bsonv1.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return doc
}

// firstParamMapping returns the first entry of a ParameterMappings typed array
// (the list is prefixed with an int32 marker, so element 0 is skipped).
func firstParamMapping(t *testing.T, doc bsonv1.D) bsonv1.D {
	t.Helper()
	raw := docGet(doc, "ParameterMappings")
	arr, ok := raw.(bsonv1.A)
	if !ok || len(arr) < 2 {
		t.Fatalf("ParameterMappings not a non-empty typed array: %#v", raw)
	}
	pm, ok := arr[1].(bsonv1.D)
	if !ok {
		t.Fatalf("param mapping not a doc: %#v", arr[1])
	}
	return pm
}

// Bug 2 — the modelsdk engine could not write a nanoflow client action at all
// (default engine errored "client action *pages.NanoflowClientAction not yet
// supported"). It must now serialize a Forms$CallNanoflowClientAction with the
// nanoflow name and its parameter mappings.
func TestNanoflowClientAction_Serialized(t *testing.T) {
	a := &pages.NanoflowClientAction{
		NanoflowName: "M.NAV_Test",
		ParameterMappings: []*pages.NanoflowParameterMapping{
			{ParameterName: "Form", Variable: "$Form"},
		},
	}
	doc := encodeAction(t, a)

	if got := docGet(doc, "$Type"); got != "Forms$CallNanoflowClientAction" {
		t.Errorf("$Type = %v, want Forms$CallNanoflowClientAction", got)
	}
	if got := docGet(doc, "Nanoflow"); got != "M.NAV_Test" {
		t.Errorf("Nanoflow = %v, want M.NAV_Test", got)
	}
	pm := firstParamMapping(t, doc)
	if got := docGet(pm, "$Type"); got != "Forms$NanoflowParameterMapping" {
		t.Errorf("mapping $Type = %v", got)
	}
	if got := docGet(pm, "Parameter"); got != "M.NAV_Test.Form" {
		t.Errorf("Parameter = %v, want M.NAV_Test.Form (BY_NAME)", got)
	}
	if got := docGet(pm, "Expression"); got != "$Form" {
		t.Errorf("Expression = %v, want $Form", got)
	}
}

// Bug 2 (parallel fix) — the microflow client action nested a MicroflowSettings
// that never carried the argument mappings, so a microflow-button's parameters
// were silently dropped on the modelsdk engine (CE1571 in Studio Pro).
func TestMicroflowClientAction_PersistsParameters(t *testing.T) {
	a := &pages.MicroflowClientAction{
		MicroflowName: "M.ACT_Test",
		ParameterMappings: []*pages.MicroflowParameterMapping{
			{ParameterName: "Form", Variable: "$Form"},
		},
	}
	doc := encodeAction(t, a)

	settings, ok := docGet(doc, "MicroflowSettings").(bsonv1.D)
	if !ok {
		t.Fatalf("MicroflowSettings missing: %#v", docGet(doc, "MicroflowSettings"))
	}
	pm := firstParamMapping(t, settings)
	if got := docGet(pm, "Parameter"); got != "M.ACT_Test.Form" {
		t.Errorf("Parameter = %v, want M.ACT_Test.Form", got)
	}
	if got := docGet(pm, "Expression"); got != "$Form" {
		t.Errorf("Expression = %v, want $Form", got)
	}
}
