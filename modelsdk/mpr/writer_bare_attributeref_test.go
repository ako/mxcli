// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A DomainModels$AttributeRef whose Attribute is not Module.Entity.Attribute
// makes the PROJECT unloadable — `mx check` dies in the loader with
// ArgumentNullException setting 'Attribute' (Mendix 11.13.0) before any
// validation runs, and an excluded page does not help. The page and snippet
// encoders refused one, but ALTER PAGE patches the stored tree and saves it
// through UpdateRawUnit, so `alter page … insert … { image … (ImageUrlParams:
// [{1} = ImageB64]) }` inside a data view with no resolvable entity wrote one.
// The guard sits here, next to the duplicate-$ID one, and these tests go
// through the Writer because the wiring is what can come undone.

func pageWithAttributeRef(t *testing.T, attr string) []byte {
	t.Helper()
	bin := func(id string) bson.Binary { return bson.Binary{Subtype: 0x00, Data: uuidToBlob(id)} }
	b, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "$ID", Value: bin("22222222-2222-2222-2222-222222222222")},
		{Key: "Name", Value: "P"},
		{Key: "Widget", Value: bson.D{
			{Key: "$Type", Value: "Forms$TextBox"},
			{Key: "$ID", Value: bin("33333333-3333-3333-3333-333333333333")},
			{Key: "Name", Value: "textBox1"},
			{Key: "AttributeRef", Value: bson.D{
				{Key: "$Type", Value: "DomainModels$AttributeRef"},
				{Key: "$ID", Value: bin("44444444-4444-4444-4444-444444444444")},
				{Key: "Attribute", Value: attr},
				{Key: "EntityRef", Value: nil},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestUpdateUnitRefusesABareAttributeRef(t *testing.T) {
	const unitID = "55555555-5555-5555-5555-555555555555"
	w, unitPath := newV2WriterForCommitTest(t, unitID, pageWithAttributeRef(t, "MyModule.Customer.Name"))
	before, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read seeded unit: %v", err)
	}

	for _, bare := range []string{"Name", "Customer.Name"} {
		err := w.UpdateRawUnit(unitID, pageWithAttributeRef(t, bare))
		if err == nil {
			t.Fatalf("write accepted a unit holding the bare attribute reference %q", bare)
		}
		for _, want := range []string{unitID, `"` + bare + `"`, "textBox1", "Module.Entity.Attribute"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("message missing %q: %v", want, err)
			}
		}
	}

	after, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit after refusal: %v", err)
	}
	if string(after) != string(before) {
		t.Error("the refused write still changed the stored unit")
	}
}

func TestUpdateUnitAcceptsQualifiedAndEmptyAttributeRefs(t *testing.T) {
	// The control. A qualified reference is what Studio Pro stores (73 of 73
	// across a real project's units), and an EMPTY Attribute is a slot the
	// author has not bound yet — neither may be refused.
	const unitID = "66666666-6666-6666-6666-666666666666"
	w, unitPath := newV2WriterForCommitTest(t, unitID, pageWithAttributeRef(t, "MyModule.Customer.Name"))
	for _, ok := range []string{"MyModule.Customer.Email", ""} {
		if err := w.UpdateRawUnit(unitID, pageWithAttributeRef(t, ok)); err != nil {
			t.Fatalf("Attribute %q refused: %v", ok, err)
		}
	}
	after, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	if strings.Contains(string(after), "MyModule.Customer.Name") {
		t.Error("the accepted writes did not reach the stored unit")
	}
}
