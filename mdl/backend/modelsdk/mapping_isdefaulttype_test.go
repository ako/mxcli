// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// upstream #882. IsDefaultType belongs to the OBJECT mapping element type and to
// neither VALUE element type. mxcli wrote it on every value element.
//
// Authority, in order: `generated/metamodel` (built from Mendix's reflection
// data) declares isDefaultType on Import/ExportObjectMappingElement only, and
// modelsdk/gen exposes the accessor on those two types alone; and Studio Pro's
// own mappings in a blank app — FeedbackModule.IMM_PostResponse and
// EMM_PostFeedback — carry it on the object element and not on the value ones.
//
// This matters beyond tidiness: a property the type does not own is exactly the
// shape mxbuild's deserializer tolerates and Studio Pro refuses to open
// (System.InvalidOperationException at MprProperty.cs), so a green build is not
// evidence either way. See the overlay-writes rule in CLAUDE.md.
func TestMappingValueElementsOmitIsDefaultType(t *testing.T) {
	imp := importMappingElementToGen(&model.ImportMappingElement{
		Kind: "Object", Entity: "M.Root", ExposedName: "Root", JsonPath: "(Object)",
		Children: []*model.ImportMappingElement{
			{Kind: "Value", Attribute: "M.Root.Name", ExposedName: "Name", JsonPath: "(Object)|name", DataType: "String"},
		},
	}, "")
	exp := exportMappingElementToGen(&model.ExportMappingElement{
		Kind: "Object", Entity: "M.Root", ExposedName: "Root", JsonPath: "(Object)",
		Children: []*model.ExportMappingElement{
			{Kind: "Value", Attribute: "M.Root.Name", ExposedName: "Name", JsonPath: "(Object)|name", DataType: "String"},
		},
	}, "")

	for _, tc := range []struct {
		name string
		g    element.Element
	}{
		{"import", imp},
		{"export", exp},
	} {
		raw, err := (&codec.Encoder{}).Encode(tc.g)
		if err != nil {
			t.Fatalf("%s: encode: %v", tc.name, err)
		}
		var doc bson.Raw = raw

		if _, ok := doc.Lookup("IsDefaultType").BooleanOK(); !ok {
			t.Errorf("%s: the OBJECT element must keep IsDefaultType — it owns the property", tc.name)
		}
		children, ok := doc.Lookup("Children").ArrayOK()
		if !ok {
			t.Fatalf("%s: no Children array", tc.name)
		}
		vals, err := children.Values()
		if err != nil {
			t.Fatalf("%s: read Children: %v", tc.name, err)
		}
		var seen int
		for _, v := range vals {
			child, ok := v.DocumentOK()
			if !ok {
				continue
			}
			if rawStr(child, "ElementType") != "Value" {
				continue
			}
			seen++
			if _, present := child.Lookup("IsDefaultType").BooleanOK(); present {
				t.Errorf("%s: the VALUE element carries IsDefaultType, which its type does not own — "+
					"mxbuild tolerates the unknown property, Studio Pro does not", tc.name)
			}
		}
		if seen == 0 {
			t.Errorf("%s: no value element in the encoded document — the assertion proved nothing", tc.name)
		}
	}
}
