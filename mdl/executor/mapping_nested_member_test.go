// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// upstream #927. Studio Pro can bind a leaf several levels below the object
// element it belongs to, with no entity for the levels in between — tick a
// nested leaf without ticking its parents and it is offered on the nearest kept
// object element. It is stored as ONE multi-segment JsonPath.
//
// mxcli printed only the last segment, so DESCRIBE reported a mapping the
// project did not contain: a value element at "(Object)|customer|name" under an
// object element at "(Object)" came out as `CustomerName = name`, and
// re-executing that failed with `"name" is not a member of the JSON structure at
// (Object)`. The description of an existing model was simply wrong.
//
// Measured on mxbuild 11.13: the collapsed import mapping builds at 0 errors, so
// this is a real shape mxcli has to read, not a hypothetical.
func TestMappingMemberName_RelativeToParent(t *testing.T) {
	cases := []struct {
		name       string
		parentPath string
		jsonPath   string
		exposed    string
		want       string
	}{
		{
			name:       "direct child is unchanged (the overwhelmingly common case)",
			parentPath: "(Object)",
			jsonPath:   "(Object)|orderId",
			exposed:    "OrderId",
			want:       "orderId",
		},
		{
			name:       "one level collapsed",
			parentPath: "(Object)",
			jsonPath:   "(Object)|customer|name",
			exposed:    "Name",
			want:       "customer/name",
		},
		{
			name:       "two levels collapsed",
			parentPath: "(Object)",
			jsonPath:   "(Object)|customer|contact|email",
			exposed:    "Email",
			want:       "customer/contact/email",
		},
		{
			name:       "collapsed under a nested object element",
			parentPath: "(Object)|order",
			jsonPath:   "(Object)|order|customer|name",
			exposed:    "Name",
			want:       "customer/name",
		},
		{
			name:       "an array's item object is still addressed by the array's key",
			parentPath: "(Object)",
			jsonPath:   "(Object)|items|(Object)",
			exposed:    "ItemsItem",
			want:       "items",
		},
		{
			name:       "children of an array item are relative to the item path",
			parentPath: "(Object)|items|(Object)",
			jsonPath:   "(Object)|items|(Object)|sku",
			exposed:    "Sku",
			want:       "sku",
		},
		{
			name:       "no JsonPath at all (XML schema / message definition) keeps the exposed name",
			parentPath: "(Object)",
			jsonPath:   "",
			exposed:    "Whatever",
			want:       "Whatever",
		},
		{
			name:       "unknown parent falls back to the last segment rather than printing a path",
			parentPath: "(Object)|somewhereElse",
			jsonPath:   "(Object)|customer|name",
			exposed:    "Name",
			want:       "name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mappingMemberName(tc.parentPath, tc.jsonPath, tc.exposed); got != tc.want {
				t.Errorf("mappingMemberName(%q, %q, %q) = %q, want %q",
					tc.parentPath, tc.jsonPath, tc.exposed, got, tc.want)
			}
		})
	}
}

// The index resolves a `/`-separated member one segment at a time, so each step
// keeps resolve's tolerance for the raw key or the exposed name (#882).
func TestJSONSchemaIndex_ResolvePath(t *testing.T) {
	idx := newJSONSchemaIndex(orderSchemaElements())

	t.Run("single segment behaves like resolve", func(t *testing.T) {
		got, arr := idx.resolvePath("(Object)", "orderId")
		if arr != nil {
			t.Fatalf("unexpected array level %v", arr)
		}
		if got == nil || got.Path != "(Object)|orderId" {
			t.Fatalf("got %v, want the orderId element", got)
		}
	})

	t.Run("multi-segment reaches the nested leaf", func(t *testing.T) {
		got, arr := idx.resolvePath("(Object)", "customer/contact/email")
		if arr != nil {
			t.Fatalf("unexpected array level %v", arr)
		}
		if got == nil || got.Path != "(Object)|customer|contact|email" {
			t.Fatalf("got %v, want the nested email element", got)
		}
	})

	t.Run("exposed names work at every step", func(t *testing.T) {
		got, _ := idx.resolvePath("(Object)", "Customer/Contact/Email")
		if got == nil || got.Path != "(Object)|customer|contact|email" {
			t.Fatalf("got %v, want the nested email element via exposed names", got)
		}
	})

	t.Run("a missing segment resolves to nothing rather than a fabricated path", func(t *testing.T) {
		got, arr := idx.resolvePath("(Object)", "customer/nope")
		if got != nil || arr != nil {
			t.Fatalf("got (%v, %v), want (nil, nil) — a fabricated path fails only later, in the build", got, arr)
		}
	})

	// Measured on mxbuild 11.13 by patching the path into a stored mapping:
	// CE0256 "Between value mapping 'sku' and parent element '(Object)' is a
	// schema element with wrong occurrence (0..*)". A value genuinely cannot be
	// pulled through many items, so the caller refuses instead of writing it.
	t.Run("an intermediate 0..* element is reported, not traversed", func(t *testing.T) {
		got, arr := idx.resolvePath("(Object)", "items/sku")
		if got != nil {
			t.Errorf("resolved through an array to %v; mxbuild rejects that with CE0256", got)
		}
		if arr == nil || arr.Path != "(Object)|items" {
			t.Fatalf("array level = %v, want the items element so the error can name it", arr)
		}
	})
}

// orderSchemaElements is the element tree a JSON structure over
// {"orderId":…, "customer":{"name":…, "contact":{"email":…}}, "items":[{"sku":…}]}
// produces: two collapsible object levels and one array, which is the shape both
// halves of #927 turn on.
func orderSchemaElements() []*types.JsonElement {
	email := &types.JsonElement{ExposedName: "Email", Path: "(Object)|customer|contact|email", ElementType: "Value", MaxOccurs: 1}
	contact := &types.JsonElement{ExposedName: "Contact", Path: "(Object)|customer|contact", ElementType: "Object", MaxOccurs: 1,
		Children: []*types.JsonElement{email}}
	name := &types.JsonElement{ExposedName: "Name", Path: "(Object)|customer|name", ElementType: "Value", MaxOccurs: 1}
	customer := &types.JsonElement{ExposedName: "Customer", Path: "(Object)|customer", ElementType: "Object", MaxOccurs: 1,
		Children: []*types.JsonElement{name, contact}}
	orderID := &types.JsonElement{ExposedName: "OrderId", Path: "(Object)|orderId", ElementType: "Value", MaxOccurs: 1}
	sku := &types.JsonElement{ExposedName: "Sku", Path: "(Object)|items|(Object)|sku", ElementType: "Value", MaxOccurs: 1}
	item := &types.JsonElement{ExposedName: "ItemsItem", Path: "(Object)|items|(Object)", ElementType: "Object", MaxOccurs: 1,
		Children: []*types.JsonElement{sku}}
	items := &types.JsonElement{ExposedName: "Items", Path: "(Object)|items", ElementType: "Array", MaxOccurs: -1,
		Children: []*types.JsonElement{item}}
	root := &types.JsonElement{ExposedName: "Root", Path: "(Object)", ElementType: "Object", MaxOccurs: 1,
		Children: []*types.JsonElement{orderID, customer, items}}
	return []*types.JsonElement{root}
}
