// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A DomainModels$AttributeRef whose Attribute is not Module.Entity.Attribute
// cannot be LOADED. Mendix rebuilds each stored reference into a typed
// identifier as it reads the unit, and a name that does not parse as one takes
// the loader down before any validation runs:
//
//	InvalidOperationException: An error occurred when trying to set the
//	'Attribute' property of a Attribute in a Page with ID ...
//	---> ArgumentNullException: Value cannot be null. (Parameter 'value')
//
// (`mx check`, Mendix 11.13.0). Excluding the document does not help — loading
// is not validating. Studio Pro qualifies every one: 73 of 73 across all 374
// units of a stock 11.13 project (71 in pages, one each in a snippet and a
// page template; no other unit type holds one).
//
// This sits at the write choke point for the same reason DuplicateElementIDError
// does. The page and snippet encoders refused a bare reference, but ALTER PAGE
// patches the stored tree and saves it through UpdateRawUnit, and a pluggable
// widget's template parameter inside a data container with no resolvable entity
// reached disk bare that way. Any raw write can.
//
// It refuses every bare reference in the unit, stored or new. A stored one
// cannot have come from Studio Pro, which cannot load it either; writing it back
// keeps the project unloadable, and the message names it so the statement that
// rewrites the unit can drop or qualify it.

// BareAttributeRefs names every DomainModels$AttributeRef in raw whose
// Attribute is non-empty and not Module.Entity.Attribute, with where it sits
// (the nearest named element's Name, then the property path). An empty
// Attribute is an unbound slot and is not reported. A document that cannot be
// read yields nothing, as DuplicateElementIDs does.
func BareAttributeRefs(raw []byte) []string {
	var bad []string
	var walk func(v bson.RawValue, path string)
	walk = func(v bson.RawValue, path string) {
		switch v.Type {
		case bson.TypeEmbeddedDocument:
			doc, ok := v.DocumentOK()
			if !ok {
				return
			}
			if t, ok := doc.Lookup("$Type").StringValueOK(); ok && t == "DomainModels$AttributeRef" {
				if a, ok := doc.Lookup("Attribute").StringValueOK(); ok && a != "" && strings.Count(a, ".") < 2 {
					bad = append(bad, fmt.Sprintf("%q at %s", a, path))
				}
			}
			name, _ := doc.Lookup("Name").StringValueOK()
			elems, _ := doc.Elements()
			for _, e := range elems {
				p := path + "/" + e.Key()
				if name != "" {
					p = path + "/" + name + "." + e.Key()
				}
				walk(e.Value(), p)
			}
		case bson.TypeArray:
			arr, ok := v.ArrayOK()
			if !ok {
				return
			}
			vals, _ := arr.Values()
			for _, x := range vals {
				walk(x, path)
			}
		}
	}
	if err := bson.Raw(raw).Validate(); err != nil {
		return nil
	}
	walk(bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: raw}, "")
	return bad
}

// BareAttributeRefError returns the error a write should fail with, or nil.
// unitLabel names the unit: an id is enough, a qualified name is better.
func BareAttributeRefError(unitLabel string, raw []byte) error {
	bad := BareAttributeRefs(raw)
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to write unit %s: attribute reference not qualified as "+
		"Module.Entity.Attribute — Mendix cannot load a project holding one: %s. Qualify it in the "+
		"script; inside a data container whose entity cannot be resolved (e.g. its data-source flow "+
		"is missing) there is nothing to qualify a bare name against",
		unitLabel, strings.Join(bad, "; "))
}
