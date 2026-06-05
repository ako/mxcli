// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func newPersistentEntity(name string, attrs ...*domainmodel.Attribute) *domainmodel.Entity {
	return &domainmodel.Entity{
		Name:        name,
		Persistable: true,
		Location:    model.Point{X: 100, Y: 100},
		Attributes:  attrs,
	}
}

func attr(name string, t domainmodel.AttributeType) *domainmodel.Attribute {
	return &domainmodel.Attribute{Name: name, Type: t}
}

func TestBuildEntityValue_PlainEntity(t *testing.T) {
	b := &Backend{}
	e := newPersistentEntity("Order",
		attr("Total", &domainmodel.DecimalAttributeType{}),
		attr("Title", &domainmodel.StringAttributeType{Length: 200}),
		attr("Count", &domainmodel.IntegerAttributeType{}),
	)
	v, err := b.buildEntityValue(e)
	if err != nil {
		t.Fatalf("buildEntityValue: %v", err)
	}
	if v.SType != "DomainModels$Entity" || v.Name != "Order" {
		t.Fatalf("unexpected entity header: %+v", v)
	}
	if v.Location == nil || v.Location.X != 100 || v.Location.Y != 100 {
		t.Fatalf("unexpected location: %+v", v.Location)
	}
	want := []struct{ name, typ string }{
		{"Total", "Decimal"}, {"Title", "String"}, {"Count", "Integer"},
	}
	if len(v.Attributes) != len(want) {
		t.Fatalf("got %d attributes, want %d", len(v.Attributes), len(want))
	}
	for i, w := range want {
		got := v.Attributes[i]
		if got.SType != "DomainModels$Attribute" || got.Name != w.name || got.Type != w.typ {
			t.Errorf("attr[%d] = %+v, want name=%s type=%s", i, got, w.name, w.typ)
		}
	}

	// The serialized value must use the PED keys $Type/name/type.
	raw, _ := json.Marshal(v)
	for _, key := range []string{`"$Type":"DomainModels$Entity"`, `"name":"Order"`, `"type":"Decimal"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("serialized entity missing %s: %s", key, raw)
		}
	}
}

func TestBuildEntityValue_EnumerationRef(t *testing.T) {
	b := &Backend{}
	e := newPersistentEntity("Ticket",
		attr("Status", &domainmodel.EnumerationAttributeType{EnumerationRef: "MyFirstModule.StatusEnum"}),
	)
	v, err := b.buildEntityValue(e)
	if err != nil {
		t.Fatalf("buildEntityValue: %v", err)
	}
	a := v.Attributes[0]
	if a.Type != "Enumeration" || a.EnumerationName != "MyFirstModule.StatusEnum" {
		t.Fatalf("enum attr mapped wrong: %+v", a)
	}
}

func TestBuildEntityValue_BooleanFalseDefaultAllowed(t *testing.T) {
	b := &Backend{}
	boolAttr := attr("IsPaid", &domainmodel.BooleanAttributeType{})
	boolAttr.Value = &domainmodel.AttributeValue{DefaultValue: "false"} // auto-added by the executor
	e := newPersistentEntity("Invoice", boolAttr)
	if _, err := b.buildEntityValue(e); err != nil {
		t.Fatalf("Boolean false default should be allowed (dropped), got: %v", err)
	}
}

func TestBuildEntityValue_RejectsUnsupportedFeatures(t *testing.T) {
	b := &Backend{}
	cases := map[string]func(*domainmodel.Entity){
		"non-persistent":  func(e *domainmodel.Entity) { e.Persistable = false },
		"indexes":         func(e *domainmodel.Entity) { e.Indexes = []*domainmodel.Index{{}} },
		"validation rule": func(e *domainmodel.Entity) { e.ValidationRules = []*domainmodel.ValidationRule{{}} },
		"event handler":   func(e *domainmodel.Entity) { e.EventHandlers = []*domainmodel.EventHandler{{}} },
		"system owner":    func(e *domainmodel.Entity) { e.HasOwner = true },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			e := newPersistentEntity("X", attr("A", &domainmodel.StringAttributeType{}))
			mutate(e)
			if _, err := b.buildEntityValue(e); err == nil {
				t.Fatalf("expected error for %s, got nil", name)
			}
		})
	}
}

func TestBuildAttributeValue_RejectsCalculatedAndNonDefaultValues(t *testing.T) {
	b := &Backend{}
	calc := attr("Full", &domainmodel.StringAttributeType{})
	calc.Value = &domainmodel.AttributeValue{Type: "CalculatedValue"}
	if _, err := b.buildAttributeValue(calc); err == nil {
		t.Error("expected calculated attribute to be rejected")
	}

	deflt := attr("Qty", &domainmodel.IntegerAttributeType{})
	deflt.Value = &domainmodel.AttributeValue{DefaultValue: "5"}
	if _, err := b.buildAttributeValue(deflt); err == nil {
		t.Error("expected non-Boolean default to be rejected")
	}
}

func TestAssociationMultiplicity(t *testing.T) {
	cases := []struct {
		typ   domainmodel.AssociationType
		owner domainmodel.AssociationOwner
		want  string
	}{
		{domainmodel.AssociationTypeReference, domainmodel.AssociationOwnerDefault, "one_to_many"},
		{domainmodel.AssociationTypeReference, domainmodel.AssociationOwnerBoth, "one_to_one"},
		{domainmodel.AssociationTypeReferenceSet, domainmodel.AssociationOwnerBoth, "many_to_many"},
	}
	for _, c := range cases {
		got, err := associationMultiplicity(&domainmodel.Association{Name: "A", Type: c.typ, Owner: c.owner})
		if err != nil || got != c.want {
			t.Errorf("type=%s owner=%s => %q,%v; want %q", c.typ, c.owner, got, err, c.want)
		}
	}
}

func TestGuardAssociationFeatures_RejectsCustomDeleteBehavior(t *testing.T) {
	a := &domainmodel.Association{
		Name:                "A",
		ChildDeleteBehavior: &domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorTypeDeleteMeAndReferences},
	}
	if err := guardAssociationFeatures(a); err == nil {
		t.Error("expected custom (cascade) delete behavior to be rejected")
	}
	// default keep-references is allowed
	a.ChildDeleteBehavior.Type = domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences
	if err := guardAssociationFeatures(a); err != nil {
		t.Errorf("keep-references should be allowed, got: %v", err)
	}
}

func TestPedUpdate_SendsAssociationGUIDs(t *testing.T) {
	f := newFakePED(t, func(string, map[string]any) (string, bool) { return "SUCCESS", false })
	b := &Backend{client: f.connectClient(t)}

	err := b.pedUpdate("ObjListV10", pedOpEntry{
		Path: "/associations",
		Operation: pedOperation{Type: "add", Value: pedAssociation{
			SType: "DomainModels$Association", Name: "SalesData_Location",
			ParentEntity: "1f5aa90f-6b84-46d1-86cc-84e5a7ba8311",
			ChildEntity:  "ec764fad-d840-45f5-b595-038662842e51",
			Multiplicity: "one_to_many",
		}},
	})
	if err != nil {
		t.Fatalf("pedUpdate: %v", err)
	}
	call, _ := f.callByName("ped_update_document")
	raw, _ := json.Marshal(call.Args["operations"])
	for _, want := range []string{
		`"path":"/associations"`,
		`"parentEntity":"1f5aa90f-6b84-46d1-86cc-84e5a7ba8311"`,
		`"childEntity":"ec764fad-d840-45f5-b595-038662842e51"`,
		`"multiplicity":"one_to_many"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("association op missing %s: %s", want, raw)
		}
	}
}

func TestPedAttributeType(t *testing.T) {
	ok := map[string]string{
		"String": "String", "Integer": "Integer", "Boolean": "Boolean",
		"DateTime": "DateTime", "Date": "DateTime", "Enumeration": "Enumeration",
		"AutoNumber": "AutoNumber", "Long": "Long", "Decimal": "Decimal",
		"Binary": "Binary", "HashedString": "HashedString",
	}
	for in, want := range ok {
		got, err := pedAttributeType(in)
		if err != nil || got != want {
			t.Errorf("pedAttributeType(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := pedAttributeType("Reference"); err == nil {
		t.Error("expected unsupported type to error")
	}
}
