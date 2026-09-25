// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Studio Pro's stored shape (Administration.Account_Edit, Mendix 11.13.0):
//
//	ConditionalVisibilitySettings: Forms$ConditionalVisibilitySettings
//	  Attribute: "Administration.Account.IsLocalUser"
//	  Conditions [marker=2]:
//	    - Enumerations$Condition { AttributeValue: "true",  EditableVisible: false }
//	    - Enumerations$Condition { AttributeValue: "false", EditableVisible: true }
//	  Expression: ""
//	  IgnoreSecurity: false
//	  ModuleRoles [marker=1]: []
//	  SourceVariable: null
//
// Markers: Conditions is [2] and ModuleRoles [1] on all 12 conditional
// settings in that project, empty or not; mxcli wrote [3] for both.
func TestConditionalVisibilityToGen_AttributeConditions(t *testing.T) {
	d := encodeToD(t, conditionalVisibilityToGen(&pages.ConditionalVisibilitySettings{
		Attribute: "Administration.Account.IsLocalUser",
		Conditions: []pages.ValueCondition{
			{Value: "true", Visible: false},
			{Value: "false", Visible: true},
		},
	}))
	get := func(k string) any {
		for _, e := range d {
			if e.Key == k {
				return e.Value
			}
		}
		t.Fatalf("key %q missing: %v", k, d)
		return nil
	}
	if get("Attribute") != "Administration.Account.IsLocalUser" {
		t.Errorf("Attribute = %v", get("Attribute"))
	}
	conds, ok := get("Conditions").(bson.A)
	if !ok || len(conds) != 3 || conds[0] != int32(2) {
		t.Fatalf("Conditions = %#v, want [2, cond, cond]", get("Conditions"))
	}
	first, _ := conds[1].(bson.D)
	want := map[string]any{"$Type": "Enumerations$Condition", "AttributeValue": "true", "EditableVisible": false}
	for k, v := range want {
		found := false
		for _, e := range first {
			if e.Key == k {
				found = true
				if e.Value != v {
					t.Errorf("condition[0].%s = %v, want %v", k, e.Value, v)
				}
			}
		}
		if !found {
			t.Errorf("condition[0] lacks %s: %v", k, first)
		}
	}
	if roles, _ := get("ModuleRoles").(bson.A); len(roles) != 1 || roles[0] != int32(1) {
		t.Errorf("ModuleRoles = %#v, want [1]", get("ModuleRoles"))
	}
}

// The expression form keeps its shape, with the corrected empty markers.
func TestConditionalVisibilityToGen_ExpressionMarkers(t *testing.T) {
	d := encodeToD(t, conditionalVisibilityToGen(&pages.ConditionalVisibilitySettings{Expression: "$currentObject/ImageB64 != empty"}))
	for _, e := range d {
		switch e.Key {
		case "Conditions":
			if a, _ := e.Value.(bson.A); len(a) != 1 || a[0] != int32(2) {
				t.Errorf("empty Conditions = %#v, want [2]", e.Value)
			}
		case "ModuleRoles":
			if a, _ := e.Value.(bson.A); len(a) != 1 || a[0] != int32(1) {
				t.Errorf("empty ModuleRoles = %#v, want [1]", e.Value)
			}
		case "Attribute":
			if e.Value != "" {
				t.Errorf("Attribute = %v, want \"\"", e.Value)
			}
		}
	}
}
