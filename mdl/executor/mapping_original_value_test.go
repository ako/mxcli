// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// OriginalValue is the sample parsed out of a JSON structure's snippet. mxcli
// wrote it empty on every element, so a rewrite DELETED it from every mapping
// that had one — 2,322 of 3,042 value elements in the demo corpus carry one
// (ako/mxcli#379).
//
// Neither global default is right. The split is PER DOCUMENT: 145 mappings
// carry the sample on every element, 107 on none, 2 mixed. So a rewrite
// preserves what was stored instead of choosing, and a NEWLY authored mapping
// still writes empty — which is what #882 actually decided.

func importElem(path, value string, kids ...*model.ImportMappingElement) *model.ImportMappingElement {
	return &model.ImportMappingElement{JsonPath: path, OriginalValue: value, Children: kids}
}

func TestImportOriginalValuesAreCarriedThroughARewrite(t *testing.T) {
	stored := &model.ImportMapping{Elements: []*model.ImportMappingElement{
		importElem("(Object)", "",
			importElem("(Object)|title", `"hello"`),
			importElem("(Object)|nested", "",
				importElem("(Object)|nested|qty", `"3"`)),
		),
	}}
	rebuilt := &model.ImportMapping{Elements: []*model.ImportMappingElement{
		importElem("(Object)", "",
			importElem("(Object)|title", ""),
			importElem("(Object)|nested", "",
				importElem("(Object)|nested|qty", "")),
		),
	}}

	carryImportOriginalValues(rebuilt, stored)

	root := rebuilt.Elements[0]
	if got := root.Children[0].OriginalValue; got != `"hello"` {
		t.Errorf("title = %q, want \"hello\"", got)
	}
	// Nesting matters: a mapping's samples are not all at the top level.
	if got := root.Children[1].Children[0].OriginalValue; got != `"3"` {
		t.Errorf("nested qty = %q, want \"3\"", got)
	}
}

// TestOriginalValuesMatchOnJsonPath pins the matching key. Names can be renamed
// and order can change; a value element bound to a different path is a
// different element, so the path is what identifies it against the schema.
func TestOriginalValuesMatchOnJsonPath(t *testing.T) {
	stored := &model.ImportMapping{Elements: []*model.ImportMappingElement{
		importElem("(Object)", "", importElem("(Object)|title", `"hello"`)),
	}}
	rebuilt := &model.ImportMapping{Elements: []*model.ImportMappingElement{
		importElem("(Object)", "", importElem("(Object)|somethingElse", "")),
	}}

	carryImportOriginalValues(rebuilt, stored)

	if got := rebuilt.Elements[0].Children[0].OriginalValue; got != "" {
		t.Errorf("carried %q onto a different path — the sample belongs to the element it was measured on", got)
	}
}

// TestARewriteDoesNotOverwriteAnExplicitValue pins the precedence: what the
// rebuild produced wins, and the stored value only fills a gap.
func TestARewriteDoesNotOverwriteAnExplicitValue(t *testing.T) {
	stored := &model.ImportMapping{Elements: []*model.ImportMappingElement{
		importElem("(Object)|x", `"old"`),
	}}
	rebuilt := &model.ImportMapping{Elements: []*model.ImportMappingElement{
		importElem("(Object)|x", `"new"`),
	}}

	carryImportOriginalValues(rebuilt, stored)

	if got := rebuilt.Elements[0].OriginalValue; got != `"new"` {
		t.Errorf("OriginalValue = %q, want the rebuilt value", got)
	}
}

// TestANewMappingKeepsEmptyOriginalValues is the control for #882. With no
// stored document there is nothing to carry, so a newly authored mapping still
// writes empty — the decision that issue made, left intact.
func TestANewMappingKeepsEmptyOriginalValues(t *testing.T) {
	rebuilt := &model.ImportMapping{Elements: []*model.ImportMappingElement{
		importElem("(Object)|title", ""),
	}}

	carryImportOriginalValues(rebuilt, nil)

	if got := rebuilt.Elements[0].OriginalValue; got != "" {
		t.Errorf("OriginalValue = %q on a new mapping, want empty", got)
	}
}

// TestExportOriginalValuesAreCarriedToo pins the export twin, whose codec
// writer hardcoded "" rather than carrying the field at all.
func TestExportOriginalValuesAreCarriedToo(t *testing.T) {
	stored := &model.ExportMapping{Elements: []*model.ExportMappingElement{
		{JsonPath: "(Object)", Children: []*model.ExportMappingElement{
			{JsonPath: "(Object)|title", OriginalValue: `"hello"`},
		}},
	}}
	rebuilt := &model.ExportMapping{Elements: []*model.ExportMappingElement{
		{JsonPath: "(Object)", Children: []*model.ExportMappingElement{
			{JsonPath: "(Object)|title"},
		}},
	}}

	carryExportOriginalValues(rebuilt, stored)

	if got := rebuilt.Elements[0].Children[0].OriginalValue; got != `"hello"` {
		t.Errorf("title = %q, want \"hello\"", got)
	}
}

// TestSameJSONContentIgnoresFormatting pins the snippet half. describe
// pretty-prints, so describe -> exec rewrote a one-line snippet into a
// multi-line one — same JSON, different bytes, a diff for nothing.
func TestSameJSONContentIgnoresFormatting(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"formatting only", `{"title": "hello", "qty": "3"}`, "{\n  \"title\": \"hello\",\n  \"qty\": \"3\"\n}", true},
		{"key order", `{"a":1,"b":2}`, `{"b":2,"a":1}`, true},
		{"different value", `{"a":1}`, `{"a":2}`, false},
		{"added key", `{"a":1}`, `{"a":1,"b":2}`, false},
		// Anything that does not parse counts as different, so a malformed
		// snippet is replaced rather than silently kept.
		{"malformed", `{"a":`, `{"a":1}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameJSONContent(tc.a, tc.b); got != tc.want {
				t.Errorf("sameJSONContent = %v, want %v", got, tc.want)
			}
		})
	}
}
