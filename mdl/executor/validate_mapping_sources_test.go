// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func sourceRefsOf(t *testing.T, src string) []mappingSourceRef {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	return mappingSourceRefs(prog)
}

func names(vs ...string) map[string]bool {
	m := map[string]bool{}
	for _, v := range vs {
		m[v] = true
	}
	return m
}

// TestMappingSourceRefsCoversBothMappingKinds pins that the reference pass sees
// export mappings too. An export mapping names a schema source exactly the same
// way, and a rule that only walked import mappings would leave half the failure
// in place.
func TestMappingSourceRefsCoversBothMappingKinds(t *testing.T) {
	refs := sourceRefsOf(t, `create import mapping M.IMM_A with json structure M.JSON_A
{ create M.E { X = a } };
create export mapping M.EMM_B with xml schema M.B_Xsd
{ M.E { a = X } };`)

	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2: %+v", len(refs), refs)
	}
	if refs[0].kind != "JSON_STRUCTURE" || refs[0].name != "M.JSON_A" {
		t.Errorf("import ref = %+v", refs[0])
	}
	if refs[1].kind != "XML_SCHEMA" || refs[1].name != "M.B_Xsd" {
		t.Errorf("export ref = %+v", refs[1])
	}
}

// TestMappingSourceViolationsReportsUnknown covers the reported failure: mxcli
// check and check --references both passed, and MxBuild reported CE1613.
func TestMappingSourceViolationsReportsUnknown(t *testing.T) {
	refs := []mappingSourceRef{
		{"import mapping", "M.IMM_A", "JSON_STRUCTURE", "M.NoSuchJson"},
		{"export mapping", "M.EMM_B", "XML_SCHEMA", "M.NoSuchXsd"},
	}
	got := mappingSourceViolations(refs, names("M.JSON_A"), names("M.B_Xsd"))

	if len(got) != 2 {
		t.Fatalf("got %d errors, want 2: %v", len(got), got)
	}
	if !strings.Contains(got[0].Error(), "M.JSON_A") {
		t.Errorf("json error does not list what exists: %v", got[0])
	}
	if !strings.Contains(got[1].Error(), "M.B_Xsd") {
		t.Errorf("xml error does not list what exists: %v", got[1])
	}
}

// TestMappingSourceViolationsAcceptsKnown is the control.
func TestMappingSourceViolationsAcceptsKnown(t *testing.T) {
	refs := []mappingSourceRef{
		{"import mapping", "M.IMM_A", "JSON_STRUCTURE", "M.JSON_A"},
		{"export mapping", "M.EMM_B", "XML_SCHEMA", "M.B_Xsd"},
	}
	if got := mappingSourceViolations(refs, names("M.JSON_A"), names("M.B_Xsd")); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

// TestMappingSourceViolationsSkipMessageDefinitions pins that a third source
// kind is left alone rather than reported against the wrong set — it resolves
// against a collection document and has its own resolver in the create path.
func TestMappingSourceViolationsSkipMessageDefinitions(t *testing.T) {
	refs := []mappingSourceRef{
		{"import mapping", "M.IMM_A", "MESSAGE_DEFINITION", "M.Coll.Def"},
	}
	if got := mappingSourceViolations(refs, names("M.JSON_A"), names("M.B_Xsd")); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

// TestMappingSourceViolationsFailOpenOnEmptySet pins the direction that keeps
// this from being a regression. An empty XML set is the ORDINARY case — MDL
// cannot create an XML schema, and none of the nine demo apps in the corpus has
// one — and an empty JSON set means the project could not be read.
func TestMappingSourceViolationsFailOpenOnEmptySet(t *testing.T) {
	refs := []mappingSourceRef{
		{"import mapping", "M.IMM_A", "JSON_STRUCTURE", "M.Anything"},
		{"export mapping", "M.EMM_B", "XML_SCHEMA", "M.Anything"},
	}
	if got := mappingSourceViolations(refs, nil, nil); len(got) != 0 {
		t.Fatalf("got %v, want none — nothing was learned, so nothing should be asserted", got)
	}
}

// TestScriptJsonStructuresAreKnown pins the shape that would otherwise flag
// almost every script: create the structure, then the mapping over it. The
// structure is not in the project yet and must not be reported.
func TestScriptJsonStructuresAreKnown(t *testing.T) {
	prog, errs := visitor.Build(`create json structure M.JSON_New snippet '{"a": 1}';
create import mapping M.IMM_New with json structure M.JSON_New
{ create M.E { X = a } };`)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}

	known := scriptJsonStructures(prog)
	if !known["M.JSON_New"] {
		t.Fatal("a structure created in the script was not treated as known")
	}
	if got := mappingSourceViolations(mappingSourceRefs(prog), known, nil); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}
