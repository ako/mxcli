// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// widget_convert.go moves a stored widget subtree between the two representations the
// codebase already has for the same thing:
//
//	bson.D          — how a widget is stored in a unit (ordered, IDs as 16-byte binary)
//	map[string]any  — what widgets.AugmentTemplate operates on (IDs as hex strings)
//
// The point is to run the SIX reconciliation passes AugmentTemplate already performs
// (enum values, property metadata, ValueType scalars, the AllowUpload envelope,
// PropertyType order, definition attributes) against a stored instance, instead of
// maintaining a second, weaker set of hand-rolled mutations. Hand-rolling produced a
// sync that left 47 Captions, 32 Categories and every ValueType/Translations wrong,
// because those are reconciled by passes it never called.
//
// # Key order
//
// map[string]any is unordered and Mendix cares about BSON key order (a documented
// CE0463 cause). Stored documents are ordered alphabetically — a PropertyType reads
// $ID, $Type, Caption, Category, Description, IsDefault, PropertyKey, ValueType — so
// converting back with sorted keys reproduces it. That assumption is not taken on
// faith: TestWidgetRoundTripIsByteStable converts every widget in a real project and
// asserts the re-encoded bytes are identical.

// widgetToMap converts stored BSON into the map form, rendering binary IDs as hex.
func widgetToMap(v any) any {
	switch t := v.(type) {
	case bson.D:
		out := make(map[string]any, len(t))
		for _, e := range t {
			out[e.Key] = widgetToMap(e.Value)
		}
		return out
	case bson.A:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = widgetToMap(item)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = widgetToMap(item)
		}
		return out
	case primitive.Binary:
		return bsonutil.BsonBinaryToID(t)
	case []byte:
		return bsonutil.BsonBinaryToID(primitive.Binary{Subtype: 0x00, Data: t})
	case int32:
		// The template pipeline models Mendix's array markers and small ints as
		// float64; keep one numeric representation so comparisons behave.
		return float64(t)
	case int64:
		return float64(t)
	}
	return v
}

// mapToWidgetDoc converts back, restoring binary IDs and alphabetical key order.
func mapToWidgetDoc(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(bson.D, 0, len(keys))
		for _, k := range keys {
			out = append(out, bson.E{Key: k, Value: mapValueToWidgetBSON(k, t[k])})
		}
		return out
	case []any:
		out := make(bson.A, len(t))
		for i, item := range t {
			out[i] = mapToWidgetDoc(item)
		}
		return out
	}
	return v
}

func mapValueToWidgetBSON(key string, v any) any {
	if s, ok := v.(string); ok && isWidgetIDField(key) {
		if b, err := bsonutil.IDToBsonBinaryErr(s); err == nil {
			return b
		}
	}
	return mapToWidgetDoc(v)
}

// isWidgetIDField names the fields Mendix stores as binary GUIDs. $ID plus the
// *Pointer references (TypePointer binds a WidgetProperty to its WidgetPropertyType).
func isWidgetIDField(key string) bool {
	return key == "$ID" || strings.HasSuffix(key, "Pointer")
}

// ensureUniqueWidgetIDs guarantees every $ID in a widget subtree is distinct.
//
// This is not defensive tidying — without it `mxcli widget sync` CORRUPTS the project.
// AugmentTemplate adds a property to an object-list property (a DataGrid2 column set)
// by giving every list entry a copy of the same constructed node. Those copies carry
// the same placeholder ID, so remapping placeholder->UUID by VALUE assigns all of them
// one UUID. The result loads and passes `mx check`, then fails at save time with
//
//	System.InvalidOperationException: Duplicate Guid in unit page template '...'
//
// and — because `mx update-widgets` collapses mprcontents/ BEFORE it fails to save —
// leaves the project both flattened and unloadable ("Root unit not found"). Reported
// against PR #89 on a real project; the template pipeline never hit it because a
// template has exactly one list entry.
//
// Scoping matters. A TypePointer that repeats across list entries is CORRECT: every
// column's WidgetProperty for a given key points at the one shared WidgetPropertyType.
// So only $ID is made unique, and when an $ID is regenerated, references to it are
// rewritten only within the same subtree — never across siblings.
func ensureUniqueWidgetIDs(v any, seen map[string]bool) any {
	return uniquifyIDs(v, seen, map[string]string{})
}

func uniquifyIDs(v any, seen map[string]bool, scope map[string]string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		// The node's own $ID first, so children can be rewritten against it.
		if id, ok := t["$ID"].(string); ok {
			if seen[id] {
				fresh := types.GenerateID()
				scope[id] = fresh
				out["$ID"] = fresh
				seen[fresh] = true
			} else {
				seen[id] = true
				out["$ID"] = id
			}
		}
		for k, val := range t {
			if k == "$ID" {
				continue
			}
			if s, ok := val.(string); ok && isWidgetIDField(k) {
				if fresh, remapped := scope[s]; remapped {
					out[k] = fresh
					continue
				}
				out[k] = s
				continue
			}
			out[k] = uniquifyIDs(val, seen, scope)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			// Each list entry is its own reference scope: sibling entries legitimately
			// share pointers to the schema, but must not share $IDs.
			child := map[string]string{}
			for k, val := range scope {
				child[k] = val
			}
			out[i] = uniquifyIDs(item, seen, child)
		}
		return out
	}
	return v
}

// widgetIDsAreUnique reports the first $ID that occurs more than once in an encoded
// unit — the check that must pass before anything is written to disk.
func widgetIDsAreUnique(doc bson.D) (string, bool) {
	seen := map[string]bool{}
	var dup string
	var walk func(any)
	walk = func(v any) {
		if dup != "" {
			return
		}
		switch t := v.(type) {
		case bson.D:
			if id, ok := idOf(t); ok {
				if seen[id] {
					dup = id
					return
				}
				seen[id] = true
			}
			for _, e := range t {
				walk(e.Value)
			}
		case bson.A:
			for _, item := range t {
				walk(item)
			}
		}
	}
	walk(doc)
	return dup, dup == ""
}
