// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	mdltypes "github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func jsonNameViolations(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	return ValidateJsonStructureNames(prog)
}

// TestUnknownCustomNameKeyIsReported pins the silence that hid ako/mxcli#272.
//
// An entry naming a key the snippet does not contain applied to nothing and
// reported nothing, so a typo was indistinguishable from not having written the
// entry — and `'data|(Object)' as 'Record'`, the obvious way to try to name an
// array's item, parsed and executed and did nothing at all.
func TestUnknownCustomNameKeyIsReported(t *testing.T) {
	got := jsonNameViolations(t, `create json structure M.JSON_A
  snippet '{"data": [{"id": 1}]}'
  custom name map ('data|(Object)' as 'Record');`)

	if len(got) != 1 || got[0].RuleID != "MDL-JSON01" {
		t.Fatalf("got %v, want one MDL-JSON01", got)
	}
	// Name what would have worked, the shape #882 established for members.
	if !strings.Contains(got[0].Message, "data") || !strings.Contains(got[0].Message, "id") {
		t.Errorf("message does not list the snippet's keys: %q", got[0].Message)
	}
}

// TestKnownCustomNameKeysAreAccepted is the control.
func TestKnownCustomNameKeysAreAccepted(t *testing.T) {
	got := jsonNameViolations(t, `create json structure M.JSON_A
  snippet '{"data": [{"id": 1}], "name": "x"}'
  custom name map ('data' as 'Records', 'id' as 'Ident', item of 'data' as 'Record');`)

	if len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

// TestItemOfANonArrayIsReported pins the second half: the key exists but does not
// reach an array, so there is no item element to name.
func TestItemOfANonArrayIsReported(t *testing.T) {
	got := jsonNameViolations(t, `create json structure M.JSON_A
  snippet '{"name": "x"}'
  custom name map (item of 'name' as 'Nope');`)

	if len(got) != 1 || got[0].RuleID != "MDL-JSON02" {
		t.Fatalf("got %v, want one MDL-JSON02", got)
	}
}

// TestItemOfRootRequiresARootArray covers the sentinel's own precondition.
func TestItemOfRootRequiresARootArray(t *testing.T) {
	bad := jsonNameViolations(t, `create json structure M.JSON_A
  snippet '{"name": "x"}'
  custom name map (item of 'Root' as 'Entry');`)
	if len(bad) != 1 || bad[0].RuleID != "MDL-JSON02" {
		t.Fatalf("got %v, want one MDL-JSON02", bad)
	}

	ok := jsonNameViolations(t, `create json structure M.JSON_B
  snippet '[{"id": 1}]'
  custom name map (item of 'Root' as 'Entry');`)
	if len(ok) != 0 {
		t.Fatalf("got %v on a real root array, want none", ok)
	}
}

// TestValidateProgramRunsTheJsonNameRule pins the wiring — `check` and `exec`
// both go through ValidateProgram, so a rule not appended there is a rule
// nothing calls.
func TestValidateProgramRunsTheJsonNameRule(t *testing.T) {
	prog, errs := visitor.Build(`create json structure M.JSON_A
  snippet '{"data": [{"id": 1}]}'
  custom name map ('nmae' as 'Name');`)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	for _, v := range ValidateProgram(prog, "") {
		if v.RuleID == "MDL-JSON01" {
			return
		}
	}
	t.Error("ValidateProgram does not run ValidateJsonStructureNames")
}

// TestDescribeCollectsChosenItemNames pins the round trip.
//
// An item element's path segment is the structural marker "(Object)" /
// "(Wrapper)", so collectCustomNames skips it. Without a second collector a
// named item was written on CREATE and dropped on DESCRIBE — and describe → exec,
// which is how a document is copied, silently renamed it back.
func TestDescribeCollectsChosenItemNames(t *testing.T) {
	elems, err := mdltypes.BuildJsonElementsFromSnippet(
		`{"data": [{"id": 1}], "tags": ["a"], "plain": [{"x": 1}]}`,
		nil,
		map[string]string{"data": "Record", "tags": "Label"})
	if err != nil {
		t.Fatal(err)
	}

	got := collectCustomItemNames(elems)
	if got["data"] != "Record" {
		t.Errorf("data item = %q, want Record", got["data"])
	}
	if got["tags"] != "Label" {
		t.Errorf("tags item = %q, want Label", got["tags"])
	}
	// `plain` was not named, so DESCRIBE must not emit an entry for it —
	// otherwise every structure would grow a clause restating its own defaults.
	if _, ok := got["plain"]; ok {
		t.Errorf("emitted an entry for a generated name: %v", got)
	}
}

// TestDescribeOmitsAGeneratedItemNameEvenWhenItLooksChosen is the sharp edge of
// the same rule: `tags` defaults to the singular `Tag`, so writing `item of
// 'tags' as 'Tag'` is a no-op and must not round-trip into the output.
func TestDescribeOmitsAGeneratedItemNameEvenWhenItLooksChosen(t *testing.T) {
	elems, err := mdltypes.BuildJsonElementsFromSnippet(
		`{"tags": ["a"]}`, nil, map[string]string{"tags": "Tag"})
	if err != nil {
		t.Fatal(err)
	}
	if got := collectCustomItemNames(elems); len(got) != 0 {
		t.Errorf("got %v, want none — Tag is what the builder generates for tags", got)
	}
}

// TestDescribeCollectsARootArrayItemName covers the sentinel on the way out.
func TestDescribeCollectsARootArrayItemName(t *testing.T) {
	elems, err := mdltypes.BuildJsonElementsFromSnippet(
		`[{"id": 1}]`, nil, map[string]string{mdltypes.RootArrayItemKey: "Entry"})
	if err != nil {
		t.Fatal(err)
	}
	if got := collectCustomItemNames(elems); got[mdltypes.RootArrayItemKey] != "Entry" {
		t.Errorf("got %v, want Root -> Entry", got)
	}
}
