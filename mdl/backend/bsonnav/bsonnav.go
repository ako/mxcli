// SPDX-License-Identifier: Apache-2.0

// Package bsonnav provides generic bson.D navigation helpers shared by the page
// and workflow mutators. These operate on raw (decoded) Mendix BSON documents —
// bson v1 (go.mongodb.org/mongo-driver/bson), the version both mutators decode
// into — and understand the Mendix array convention (an int32 type marker at
// index 0 of every list).
package bsonnav

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// DGet returns the value for a key in a bson.D, or nil if not found.
func DGet(doc bson.D, key string) any {
	for _, elem := range doc {
		if elem.Key == key {
			return elem.Value
		}
	}
	return nil
}

// DGetDoc returns a nested bson.D field value, or nil.
func DGetDoc(doc bson.D, key string) bson.D {
	v := DGet(doc, key)
	if d, ok := v.(bson.D); ok {
		return d
	}
	return nil
}

// DGetString returns a string field value, or "".
func DGetString(doc bson.D, key string) string {
	v := DGet(doc, key)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// DSet sets a field value in a bson.D in place. Returns true if found.
// NOTE: callers generally do not check the return value because the keys
// are structurally guaranteed by the widgetFinder traversal. If a key
// is absent, the mutation is silently skipped — this is intentional for
// optional fields (e.g. Appearance, DataSource) that may not be present
// on every widget type.
//
// DSet CANNOT ADD an absent key: doc is a bson.D held by value, so appending
// would not be visible to the caller. A mutation that has to work on a document
// which may not carry the key yet must go through DSetArrayIn (or write the
// grown child back into its parent itself) and check the result — otherwise the
// write is a silent no-op. That is what made ALTER STYLING report success and
// store nothing when the target widget had no DesignProperties array
// (upstream #931).
func DSet(doc bson.D, key string, value any) bool {
	for i := range doc {
		if doc[i].Key == key {
			doc[i].Value = value
			return true
		}
	}
	return false
}

// DGetArrayElements extracts Mendix array elements from a bson.D field value.
// Strips the int32 type marker at index 0.
func DGetArrayElements(val any) []any {
	arr := ToBsonA(val)
	if len(arr) == 0 {
		return nil
	}
	if _, ok := arr[0].(int32); ok {
		return arr[1:]
	}
	if _, ok := arr[0].(int); ok {
		return arr[1:]
	}
	return arr
}

// ToBsonA converts various BSON array types to []any.
func ToBsonA(v any) []any {
	switch arr := v.(type) {
	case bson.A:
		return []any(arr)
	case []any:
		return arr
	default:
		return nil
	}
}

// DSetArray sets a Mendix-style BSON array field, preserving the int32 marker.
func DSetArray(doc bson.D, key string, elements []any) bool {
	marker, ok := arrayMarker(DGet(doc, key))
	if !ok {
		// No marker to preserve. Writing the elements bare would produce a list
		// whose first entry Mendix reads as the typed-array marker — measured on
		// 11.13, a DesignProperties array written that way makes the project
		// unloadable ("Type OptionDesignPropertyValue does not contain a
		// constructor with a parameter of type Appearance") and
		// create-module-package die with "Unknown export error". Refusing is the
		// safe direction; a caller that legitimately creates the property passes
		// the marker explicitly via DSetArrayIn.
		if len(elements) > 0 {
			return false
		}
		return DSet(doc, key, bson.A{})
	}
	return DSet(doc, key, withMarker(marker, elements))
}

// DSetArrayIn sets an array property on a child document of parent, CREATING it
// with the given typed-array marker when the child does not carry it yet.
//
// It exists because DSet cannot add an absent key to a bson.D held by value: the
// grown child has to be written back into its parent, which needs the parent.
// Returns false when parent has no such child document — nothing was written.
func DSetArrayIn(parent bson.D, childKey, arrayKey string, elements []any, marker int32) bool {
	child := DGetDoc(parent, childKey)
	if child == nil {
		return false
	}
	existing, ok := arrayMarker(DGet(child, arrayKey))
	if !ok {
		existing = marker
	}
	value := withMarker(existing, elements)
	if DSet(child, arrayKey, value) {
		return true
	}
	// The child does not carry the key at all — append and write the grown child
	// back into the parent.
	return DSet(parent, childKey, append(child, bson.E{Key: arrayKey, Value: value}))
}

// arrayMarker returns the leading typed-array marker of a Mendix array value.
func arrayMarker(val any) (int32, bool) {
	arr := ToBsonA(val)
	if len(arr) == 0 {
		return 0, false
	}
	switch m := arr[0].(type) {
	case int32:
		return m, true
	case int:
		return int32(m), true
	}
	return 0, false
}

// withMarker builds a Mendix array value: the typed-array marker followed by the
// elements.
func withMarker(marker int32, elements []any) bson.A {
	result := make(bson.A, 0, len(elements)+1)
	result = append(result, marker)
	return append(result, elements...)
}

// ExtractBinaryIDFromDoc extracts a binary ID string from a bson.D field.
func ExtractBinaryIDFromDoc(val any) string {
	switch bin := val.(type) {
	case primitive.Binary:
		return types.BlobToUUID(bin.Data)
	case []byte:
		return types.BlobToUUID(bin)
	default:
		return ""
	}
}
