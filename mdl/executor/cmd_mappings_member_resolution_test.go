// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// upstream #882. A JSON structure element carries TWO names and they routinely
// differ — Mendix derives the exposed name by capitalising the initial, and for
// an array's item object by suffixing "Item":
//
//	Path         "(Object)|uuid"   the raw JSON key — what the RUNTIME resolves by
//	ExposedName  "Uuid"            Mendix's derived name — what Studio Pro DISPLAYS
//
// Both are Mendix's own: the blank app's Studio-Pro-authored
// FeedbackModule.JSON_AppInsightsResponse stores ExposedName "Uuid" against Path
// "(Object)|uuid", and its IMM_PostResponse binds JsonPath "(Object)|uuid".
//
// mxcli's DESCRIBE prints the exposed name, but its builder resolved only raw
// keys — so mxcli's own output did not round-trip. Re-executing a DESCRIBE
// FABRICATED a path from the exposed name and mxbuild rejected it with CE5015;
// for an array the "|(Object)" item marker vanished entirely.

// jsonStructureFixture mirrors the shape in the issue: a lowercase-initial
// member, a camelCase one, a nested object, and an object array under an
// underscore-prefixed key.
func jsonStructureFixture() *jsonSchemaIndex {
	return newJSONSchemaIndex([]*types.JsonElement{{
		ExposedName: "Root", Path: "(Object)", ElementType: "Object", MinOccurs: 1, MaxOccurs: 1,
		Children: []*types.JsonElement{
			{ExposedName: "Total", Path: "(Object)|total", ElementType: "Value", MaxOccurs: 1},
			{ExposedName: "CamelCase", Path: "(Object)|camelCase", ElementType: "Value", MaxOccurs: 1},
			{
				ExposedName: "EntityInstances", Path: "(Object)|entityInstances", ElementType: "Object", MaxOccurs: 1,
				Children: []*types.JsonElement{
					{ExposedName: "__returnedCount", Path: "(Object)|entityInstances|__returnedCount", ElementType: "Value", MaxOccurs: 1},
					{
						ExposedName: "__Value", Path: "(Object)|entityInstances|__Value", ElementType: "Array", MaxOccurs: 1,
						Children: []*types.JsonElement{{
							ExposedName: "__ValueItem", Path: "(Object)|entityInstances|__Value|(Object)", ElementType: "Object", MaxOccurs: -1,
							Children: []*types.JsonElement{
								{ExposedName: "Name", Path: "(Object)|entityInstances|__Value|(Object)|name", ElementType: "Value", MaxOccurs: 1},
							},
						}},
					},
				},
			},
		},
	}})
}

func TestJSONSchemaIndexResolvesEitherSpelling(t *testing.T) {
	idx := jsonStructureFixture()

	for _, tc := range []struct {
		parent, name, wantPath string
		why                    string
	}{
		{"(Object)", "total", "(Object)|total", "the raw JSON key, as hand-written MDL spells it"},
		{"(Object)", "Total", "(Object)|total", "the exposed name, as DESCRIBE emits it"},
		{"(Object)", "camelCase", "(Object)|camelCase", "raw key with interior capitals"},
		{"(Object)", "CamelCase", "(Object)|camelCase", "exposed name of the same member"},
		{"(Object)", "entityInstances", "(Object)|entityInstances", "nested object, raw"},
		{"(Object)", "EntityInstances", "(Object)|entityInstances", "nested object, exposed"},
		{"(Object)|entityInstances", "__returnedCount", "(Object)|entityInstances|__returnedCount",
			"an underscore-prefixed key is not capitalised, so both spellings coincide"},
		{"(Object)|entityInstances", "__Value", "(Object)|entityInstances|__Value", "the array itself"},
		{"(Object)|entityInstances", "__ValueItem", "(Object)|entityInstances|__Value",
			"the array addressed by its ITEM's exposed name must resolve to the ARRAY, so the " +
				"caller still takes the |(Object) step to the item"},
	} {
		got := idx.resolve(tc.parent, tc.name)
		if got == nil {
			t.Errorf("resolve(%q, %q) = nil, want %s (%s)", tc.parent, tc.name, tc.wantPath, tc.why)
			continue
		}
		if got.Path != tc.wantPath {
			t.Errorf("resolve(%q, %q).Path = %q, want %q (%s)", tc.parent, tc.name, got.Path, tc.wantPath, tc.why)
		}
	}

	if got := idx.resolve("(Object)", "nope"); got != nil {
		t.Errorf("resolve of an absent member = %q, want nil — the caller must refuse, not invent a path", got.Path)
	}
}

// buildImportMappingElementModel must clone the structure's Path verbatim
// whichever spelling was authored. Fabricating one from the exposed name is what
// #882 was: `mxcli check` passed and mxbuild reported CE5015.
func TestImportMappingClonesTheStructurePathForBothSpellings(t *testing.T) {
	// The exposed-name spelling is exactly what `describe import mapping` emits.
	def := &ast.ImportMappingElementDef{
		Entity: "B.Root",
		Children: []*ast.ImportMappingElementDef{
			{Attribute: "Total", JsonName: "Total"},
			{Attribute: "Camel", JsonName: "CamelCase"},
			{
				Entity: "B.Inner", Association: "B.Root_Inner", JsonName: "EntityInstances",
				Children: []*ast.ImportMappingElementDef{
					{Attribute: "Cnt", JsonName: "__returnedCount"},
					{
						Entity: "B.Item", Association: "B.Inner_Item", JsonName: "__ValueItem",
						Children: []*ast.ImportMappingElementDef{{Attribute: "Nm", JsonName: "Name"}},
					},
				},
			},
		},
	}

	root, err := buildImportMappingElementModel("B", def, "", "(Object)", nil, jsonStructureFixture(), true)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	got := map[string]string{}
	var collect func(e *model.ImportMappingElement)
	collect = func(e *model.ImportMappingElement) {
		if e == nil {
			return
		}
		got[e.ExposedName] = e.JsonPath
		for _, c := range e.Children {
			collect(c)
		}
	}
	collect(root)

	want := map[string]string{
		"Root":            "(Object)",
		"Total":           "(Object)|total",
		"CamelCase":       "(Object)|camelCase",
		"EntityInstances": "(Object)|entityInstances",
		"__returnedCount": "(Object)|entityInstances|__returnedCount",
		// The array binds at the ITEM path — the "|(Object)" marker is the whole
		// point, and re-executing DESCRIBE used to drop it and leave
		// "(Object)|EntityInstances|__ValueItem".
		"__ValueItem": "(Object)|entityInstances|__Value|(Object)",
		"Name":        "(Object)|entityInstances|__Value|(Object)|name",
	}
	for name, wantPath := range want {
		if got[name] != wantPath {
			t.Errorf("%s: JsonPath = %q, want %q", name, got[name], wantPath)
		}
	}
}

// An unresolvable member is REFUSED. Before this it got a fabricated path that
// passed `mxcli check` and surfaced only in mxbuild (CE5015) or at runtime — the
// worst failure mode, because the tool that wrote it reported success.
func TestImportMappingRefusesAnUnknownMember(t *testing.T) {
	def := &ast.ImportMappingElementDef{
		Entity:   "B.Root",
		Children: []*ast.ImportMappingElementDef{{Attribute: "Camel", JsonName: "camelCse"}},
	}

	_, err := buildImportMappingElementModel("B", def, "", "(Object)", nil, jsonStructureFixture(), true)
	if err == nil {
		t.Fatal("a member that is in no JSON structure must be refused, not written with an invented path")
	}
	msg := err.Error()
	for _, want := range []string{"camelCse", "(Object)"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should name %q, got: %s", want, msg)
		}
	}
	// Both spellings are offered, because either is accepted and the author has
	// no way to know which one they mistyped.
	for _, want := range []string{"camelCase", "CamelCase"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should suggest %q, got: %s", want, msg)
		}
	}
}
