// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UpdateQualifiedNameInAllUnits rewrites every reference to oldName across all
// units to newName — the cross-cutting rename primitive (e.g. after renaming a
// microflow, fix every caller). It is a raw-BSON traversal: each unit is decoded
// to an ordered document, every string value that equals oldName or is prefixed
// "oldName." is rewritten, and changed units are written back. Returns the number
// of units updated. Mirrors the legacy writer (but preserves field order via
// bson.D rather than an unordered map).
func (b *Backend) UpdateQualifiedNameInAllUnits(oldName, newName string) (int, error) {
	if b.writer == nil {
		return 0, fmt.Errorf("UpdateQualifiedNameInAllUnits: not connected for writing")
	}
	ids, err := b.reader.ListAllUnitIDs()
	if err != nil {
		return 0, fmt.Errorf("UpdateQualifiedNameInAllUnits: list units: %w", err)
	}
	updated := 0
	for _, id := range ids {
		raw, err := b.reader.GetRawUnitBytes(id)
		if err != nil || len(raw) == 0 {
			continue
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			continue
		}
		if replaced, ok := replaceQNInValue(doc, oldName, newName); ok {
			contents, err := bson.Marshal(replaced)
			if err != nil {
				return updated, fmt.Errorf("UpdateQualifiedNameInAllUnits: marshal %s: %w", id, err)
			}
			if err := b.writer.UpdateRawUnit(id, contents); err != nil {
				return updated, fmt.Errorf("UpdateQualifiedNameInAllUnits: update %s: %w", id, err)
			}
			updated++
		}
	}
	return updated, nil
}

// replaceQNInValue recursively rewrites qualified-name references in a decoded
// BSON value, returning the (possibly mutated) value and whether anything changed.
func replaceQNInValue(v any, oldName, newName string) (any, bool) {
	switch val := v.(type) {
	case string:
		return replaceQualifiedNameRef(val, oldName, newName)
	case primitive.D: // also matches bson.D (alias)
		changed := false
		for i, elem := range val {
			if nv, ok := replaceQNInValue(elem.Value, oldName, newName); ok {
				val[i].Value = nv
				changed = true
			}
		}
		return val, changed
	case primitive.A:
		changed := false
		for i, elem := range val {
			if nv, ok := replaceQNInValue(elem, oldName, newName); ok {
				val[i] = nv
				changed = true
			}
		}
		return val, changed
	case []any:
		changed := false
		for i, elem := range val {
			if nv, ok := replaceQNInValue(elem, oldName, newName); ok {
				val[i] = nv
				changed = true
			}
		}
		return val, changed
	}
	return v, false
}

// replaceQualifiedNameRef rewrites s when it equals oldName or is prefixed
// "oldName." (so "Mod.Old.Param" → "Mod.New.Param"), matching the legacy logic.
func replaceQualifiedNameRef(s, oldName, newName string) (any, bool) {
	if s == oldName {
		return newName, true
	}
	if strings.HasPrefix(s, oldName+".") {
		return newName + s[len(oldName):], true
	}
	return s, false
}
