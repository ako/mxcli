// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

// refuseBareAttributeRefs refuses a page or snippet whose encoded form holds a
// DomainModels$AttributeRef that is not Module.Entity.Attribute.
//
// Mendix rebuilds each stored reference into a typed identifier as it loads,
// and an attribute that does not parse as one takes the loader down before any
// validation runs: a bare `ImageB64` image parameter left `mx check` unable to
// load the project (ArgumentNullException setting 'Attribute', 11.13.0) — the
// page was excluded, which does not help, as loading is not validating. Studio
// Pro qualifies every one (72 of 72 across a stock project's pages, snippets
// and layouts). A bare name reaches here when nothing could qualify it — inside
// a data container whose flow the project lacks — so this is the last line
// under the check that refuses it first (checkUnscopedBindings).
func refuseBareAttributeRefs(contents []byte) error {
	var bad []string
	var walk func(v bson.RawValue, path string)
	walk = func(v bson.RawValue, path string) {
		switch v.Type {
		case bson.TypeEmbeddedDocument:
			doc := v.Document()
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
			vals, _ := v.Array().Values()
			for _, x := range vals {
				walk(x, path)
			}
		}
	}
	walk(bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: contents}, "")
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("attribute reference not qualified as Module.Entity.Attribute — Mendix cannot load a "+
		"project holding one, so it is not written: %s. Qualify it in the script; inside a data container "+
		"whose flow the project lacks there is no entity to resolve a bare name against", strings.Join(bad, "; "))
}
