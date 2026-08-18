// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestAttributeToGen_CalculatedValue is the #917 write half. Without the
// CalculatedValue arm the attribute fell through to the default StoredValue, so
// `calculated by` reported success and the binding never reached the model —
// an attribute that stays empty at runtime in a project that builds at 0 errors.
func TestAttributeToGen_CalculatedValue(t *testing.T) {
	attr := &domainmodel.Attribute{
		Name: "Total",
		Type: &domainmodel.IntegerAttributeType{},
		Value: &domainmodel.AttributeValue{
			Type:          "CalculatedValue",
			MicroflowName: "MyFirstModule.CalcTotal",
			PassEntity:    true,
		},
	}

	out := attributeToGen(attr, false)
	cv, ok := out.Value().(*genDm.CalculatedValue)
	if !ok {
		t.Fatalf("value is %T, want *genDm.CalculatedValue — the binding was discarded", out.Value())
	}
	if got := cv.MicroflowQualifiedName(); got != "MyFirstModule.CalcTotal" {
		t.Errorf("microflow = %q, want %q", got, "MyFirstModule.CalcTotal")
	}
	if !cv.PassEntity() {
		t.Error("PassEntity = false, want true for a microflow that takes the owning entity")
	}
}

// TestAttributeToGen_CalculatedValue_NoPassEntity pins the parameterless shape.
// Both are accepted by mxbuild (measured on 11.13.0, 0 errors), so PassEntity
// must follow the signature rather than being hardcoded.
func TestAttributeToGen_CalculatedValue_NoPassEntity(t *testing.T) {
	attr := &domainmodel.Attribute{
		Name: "Total",
		Type: &domainmodel.IntegerAttributeType{},
		Value: &domainmodel.AttributeValue{
			Type:          "CalculatedValue",
			MicroflowName: "MyFirstModule.CalcNoParam",
		},
	}

	cv, ok := attributeToGen(attr, false).Value().(*genDm.CalculatedValue)
	if !ok {
		t.Fatal("value is not a CalculatedValue")
	}
	if cv.PassEntity() {
		t.Error("PassEntity = true for a parameterless microflow")
	}
}

// TestAttributeFromGen_CalculatedValue is the #917 read half: reading the
// binding back is what makes a read-modify-write safe. Without it an unrelated
// ALTER on the same entity silently converts the attribute to a stored one,
// destroying a binding the user made in Studio Pro.
func TestAttributeFromGen_CalculatedValue(t *testing.T) {
	g := genDm.NewAttribute()
	g.SetName("Total")
	cv := genDm.NewCalculatedValue()
	cv.SetMicroflowQualifiedName("MyFirstModule.CalcTotal")
	cv.SetPassEntity(true)
	g.SetValue(cv)

	attr := attributeFromGen(g)
	if attr.Value == nil {
		t.Fatal("attribute value is nil — the binding did not survive the read")
	}
	if attr.Value.Type != "CalculatedValue" {
		t.Errorf("value type = %q, want CalculatedValue", attr.Value.Type)
	}
	if attr.Value.MicroflowName != "MyFirstModule.CalcTotal" {
		t.Errorf("microflow = %q, want MyFirstModule.CalcTotal", attr.Value.MicroflowName)
	}
	if !attr.Value.PassEntity {
		t.Error("PassEntity did not survive the read")
	}
}
