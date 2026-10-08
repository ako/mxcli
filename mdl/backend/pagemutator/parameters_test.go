// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

func pageWithParameter(name string, widgets ...bson.D) bson.D {
	raw := makeRawPage(widgets...)
	return append(raw, bson.E{Key: "Parameters", Value: bson.A{int32(3), bson.D{
		{Key: "$Type", Value: "Forms$PageParameter"},
		{Key: "Name", Value: name},
	}}})
}

// An expression that names the parameter is a use, by whole token only.
func TestDropParameter_RefusesExpressionUse(t *testing.T) {
	w := makeWidget("txt", "Forms$DynamicText")
	w = append(w, bson.E{Key: "VisibilityExpression", Value: "$Customer/Active and $CustomerList != empty"})
	m := New(pageWithParameter("Customer", w), "u", &recordingDeps{})
	err := m.DropParameter("Customer")
	if err == nil || !strings.Contains(err.Error(), "still used") {
		t.Fatalf("want a 'still used' refusal, got %v", err)
	}

	// Control: $CustomerList alone is a different name and does not block.
	w2 := makeWidget("txt", "Forms$DynamicText")
	w2 = append(w2, bson.E{Key: "VisibilityExpression", Value: "$CustomerList != empty"})
	m2 := New(pageWithParameter("Customer", w2), "u", &recordingDeps{})
	if err := m2.DropParameter("Customer"); err != nil {
		t.Fatalf("$CustomerList is not a use of $Customer: %v", err)
	}
}

func TestDropParameter_NotFoundNamesWhatExists(t *testing.T) {
	m := New(pageWithParameter("Customer"), "u", &recordingDeps{})
	err := m.DropParameter("Order")
	if err == nil || !strings.Contains(err.Error(), "$Customer") {
		t.Fatalf("want the declared parameters named, got %v", err)
	}
}

func TestAddParameter_RefusesVariableName(t *testing.T) {
	raw := append(makeRawPage(), bson.E{Key: "Variables", Value: bson.A{int32(3), bson.D{
		{Key: "$Type", Value: "Forms$LocalVariable"},
		{Key: "Name", Value: "Show"},
	}}})
	m := New(raw, "u", &recordingDeps{})
	if err := m.AddParameter(backendSpec("Show")); err == nil || !strings.Contains(err.Error(), "variable") {
		t.Fatalf("want a clash with the variable, got %v", err)
	}
}

func backendSpec(name string) backend.PageParameterSpec {
	return backend.PageParameterSpec{Name: name, PrimitiveType: "DataTypes$StringType"}
}
