// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// The export writers hardcoded three properties the import twin already read
// off the element, so no export mapping mxcli wrote matched its Studio Pro
// original — which kept every one of them in #260's silent-loss set even once
// its source kind was authorable.
//
// Studio Pro's values, measured on FeedbackModule.EXM_PostFeedback and
// MxGenAIConnector.EM_CohereEmbed_Request (11.13):
//
//	object root   MinOccurs 1          hardcoded 0   (#279)
//	value element MaxLength 0 / -1     hardcoded 0   (#277)
//	value element IsKey false          not written   (#277)
//
// MaxLength is the one to watch: it is 0 for a STRING element and -1 for a
// numeric one, mirroring the bound schema element exactly as MaxOccurs does, so
// a single hardcoded value cannot be right for both.

// assertVal compares a BSON field of any scalar type; the package's assertField
// only handles strings.
func assertVal(t *testing.T, m map[string]any, key string, want any) {
	t.Helper()
	got, ok := m[key]
	if !ok {
		t.Errorf("field %q: missing", key)
		return
	}
	if got != want {
		t.Errorf("field %q = %v (%T), want %v (%T)", key, got, got, want, want)
	}
}

func exportDoc(t *testing.T, em *model.ExportMapping) map[string]any {
	t.Helper()
	w := &Writer{}
	data, err := w.serializeExportMapping(em)
	if err != nil {
		t.Fatalf("serializeExportMapping: %v", err)
	}
	var raw map[string]any
	if err := bson.Unmarshal(data, &raw); err != nil {
		t.Fatalf("bson.Unmarshal: %v", err)
	}
	return raw
}

func TestExportMappingMirrorsSchemaFacets(t *testing.T) {
	em := &model.ExportMapping{
		BaseElement: model.BaseElement{ID: "em-1", TypeName: "ExportMappings$ExportMapping"},
		Name:        "EXM_Probe",
		Elements: []*model.ExportMappingElement{{
			Kind: "Object", Entity: "M.E", ObjectHandling: "Parameter",
			MinOccurs: 1, MaxOccurs: 1, JsonPath: "(Object)",
			Children: []*model.ExportMappingElement{
				{Kind: "Value", Attribute: "M.E.Name", DataType: "String",
					MinOccurs: 0, MaxOccurs: 1, MaxLength: 0, JsonPath: "(Object)|name"},
				{Kind: "Value", Attribute: "M.E.Width", DataType: "Integer",
					MinOccurs: 0, MaxOccurs: 1, MaxLength: -1, JsonPath: "(Object)|width"},
			},
		}},
	}

	root, ok := extractBsonArray(exportDoc(t, em)["Elements"])[0].(map[string]any)
	if !ok {
		t.Fatal("root element is not a document")
	}
	// A schema root has MinOccurs 1; the writer hardcoded 0.
	assertVal(t, root, "MinOccurs", int32(1))

	children := extractBsonArray(root["Children"])
	if len(children) != 2 {
		t.Fatalf("got %d children, want 2", len(children))
	}
	str, _ := children[0].(map[string]any)
	num, _ := children[1].(map[string]any)

	// Both were hardcoded to 0, which is right for the string and wrong for the
	// number — the reason a single constant cannot work here.
	assertVal(t, str, "MaxLength", int32(0))
	assertVal(t, num, "MaxLength", int32(-1))

	// Studio Pro writes IsKey on export value elements; it was not written.
	assertVal(t, str, "IsKey", false)
	assertVal(t, num, "IsKey", false)
}

// MessageDefinition2 is version-introduced (11.10+). It is CARRIED, never
// invented: writing it onto an older document is the shape mxbuild tolerates
// and Studio Pro refuses to open. nil means absent, which is not the same as
// present-and-empty — hence the pointer.
func TestExportMappingCarriesMessageDefinition2(t *testing.T) {
	base := func() *model.ExportMapping {
		return &model.ExportMapping{
			BaseElement: model.BaseElement{ID: "em-2", TypeName: "ExportMappings$ExportMapping"},
			Name:        "EXM_Probe",
		}
	}

	absent := exportDoc(t, base())
	if _, ok := absent["MessageDefinition2"]; ok {
		t.Error("nil carried the key through — a pre-11.10 document must not gain it")
	}

	em := base()
	empty := ""
	em.MessageDefinition2 = &empty
	present := exportDoc(t, em)
	v, ok := present["MessageDefinition2"]
	if !ok {
		t.Fatal("present-and-empty was dropped — that is what a blank 11.13 app stores")
	}
	if v != "" {
		t.Errorf("MessageDefinition2 = %v, want the empty string", v)
	}
}
