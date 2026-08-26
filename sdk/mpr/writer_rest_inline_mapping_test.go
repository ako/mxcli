// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// The INLINE REST mapping is a separate serializer from the mapping-DOCUMENT
// one, and two defects lived here that the document work never touched
// (reported by the mxcli-rest project, findings #36 and #37):
//
//  1. A multi-segment member (`Title = fields/Title`) was stored as ONE member
//     whose name contains a slash — "(Object)|fields/Title" — instead of
//     "(Object)|fields|Title". Every gate passed and the value was empty at
//     runtime, which is the worst possible failure mode for this.
//  2. Every nested child was hardcoded ObjectHandling "Create", including in an
//     EXPORT body, where mxbuild refuses to LOAD the project at all.
//
// Four Studio Pro-authored inline response mappings in the demo apps confirm
// the stored form is a full pipe path (e.g.
// "(Object)|results|bindings|(Object)|caseId|value").

func inlineRoot(t *testing.T, doc bson.D) map[string]any {
	t.Helper()
	for _, e := range doc {
		if e.Key == "RootMappingElement" {
			raw, err := bson.Marshal(e.Value)
			if err != nil {
				t.Fatalf("marshal root: %v", err)
			}
			var m map[string]any
			if err := bson.Unmarshal(raw, &m); err != nil {
				t.Fatalf("unmarshal root: %v", err)
			}
			return m
		}
	}
	t.Fatal("no RootMappingElement")
	return nil
}

func inlineChildren(t *testing.T, elem map[string]any) []map[string]any {
	t.Helper()
	arr, ok := elem["Children"].(bson.A)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, v := range arr {
		m, ok := v.(map[string]any)
		if !ok {
			continue // the int32 typed-array marker
		}
		out = append(out, m)
	}
	return out
}

// TestInlineResponseMappingMultiSegmentPath is finding #36.
func TestInlineResponseMappingMultiSegmentPath(t *testing.T) {
	doc := serializeRestImplicitMappingResponse("RestLab.FlatProbe", []*model.RestResponseMapping{
		{Attribute: "ItemId", ExposedName: "id"},
		{Attribute: "Title", ExposedName: "fields/Title"},
	})

	children := inlineChildren(t, inlineRoot(t, doc))
	if len(children) != 2 {
		t.Fatalf("got %d children, want 2", len(children))
	}
	if got := children[0]["JsonPath"]; got != "(Object)|id" {
		t.Errorf("single-segment JsonPath = %q, want (Object)|id — unchanged behaviour", got)
	}
	// The defect: "(Object)|fields/Title" is one member with a slash in its
	// name, so nothing binds and the column is silently empty.
	if got := children[1]["JsonPath"]; got != "(Object)|fields|Title" {
		t.Errorf("multi-segment JsonPath = %q, want (Object)|fields|Title", got)
	}
	// ExposedName is a label, and Studio Pro stores the last segment.
	if got := children[1]["ExposedName"]; got != "Title" {
		t.Errorf("ExposedName = %q, want Title", got)
	}
}

// TestInlineExportBodyNestedHandling is finding #37. "Create" on an export
// object element is not a check error — mxbuild throws before the check, and
// the project cannot be opened.
func TestInlineExportBodyNestedHandling(t *testing.T) {
	doc := serializeRestImplicitMappingBody("RestLab.Task", []*model.RestResponseMapping{{
		Entity:      "RestLab.TaskFields",
		Association: "RestLab.Task_TaskFields",
		ExposedName: "fields",
		Children:    []*model.RestResponseMapping{{Attribute: "Title", ExposedName: "Title"}},
	}})

	root := inlineRoot(t, doc)
	if got := root["ObjectHandling"]; got != "Parameter" {
		t.Errorf("export root ObjectHandling = %q, want Parameter", got)
	}
	if got := root["$Type"]; got != "ExportMappings$ObjectMappingElement" {
		t.Fatalf("export root $Type = %q", got)
	}

	nested := inlineChildren(t, root)
	if len(nested) != 1 {
		t.Fatalf("got %d nested elements, want 1", len(nested))
	}
	if got := nested[0]["ObjectHandling"]; got != "Find" {
		t.Errorf("nested export ObjectHandling = %q, want Find — mxbuild refuses to LOAD "+
			"a project whose export object element is Create", got)
	}
	// An export element has nothing to create, so the backup is Error.
	if got := nested[0]["ObjectHandlingBackup"]; got != "Error" {
		t.Errorf("nested export ObjectHandlingBackup = %q, want Error", got)
	}
}

// TestInlineImportNestedHandlingUnchanged is the control for the one above: the
// import direction legitimately creates, and must not be changed by the fix.
func TestInlineImportNestedHandlingUnchanged(t *testing.T) {
	doc := serializeRestImplicitMappingResponse("RestLab.Task", []*model.RestResponseMapping{{
		Entity:      "RestLab.TaskFields",
		Association: "RestLab.Task_TaskFields",
		ExposedName: "fields",
		Children:    []*model.RestResponseMapping{{Attribute: "Title", ExposedName: "Title"}},
	}})

	nested := inlineChildren(t, inlineRoot(t, doc))
	if len(nested) != 1 {
		t.Fatalf("got %d nested elements, want 1", len(nested))
	}
	if got := nested[0]["ObjectHandling"]; got != "Create" {
		t.Errorf("nested import ObjectHandling = %q, want Create", got)
	}
	if got := nested[0]["JsonPath"]; got != "(Object)|fields" {
		t.Errorf("nested JsonPath = %q", got)
	}
	// The value under it resolves against the nested path, not the root.
	values := inlineChildren(t, nested[0])
	if len(values) != 1 || values[0]["JsonPath"] != "(Object)|fields|Title" {
		t.Errorf("nested value JsonPath = %v", values)
	}
}

// TestInlineNestedObjectMultiSegmentPath: an OBJECT element can carry a
// multi-segment member too, and it was broken the same way.
func TestInlineNestedObjectMultiSegmentPath(t *testing.T) {
	doc := serializeRestImplicitMappingResponse("RestLab.Root", []*model.RestResponseMapping{{
		Entity:      "RestLab.Binding",
		Association: "RestLab.Binding_Root",
		ExposedName: "results/bindings",
		Children:    []*model.RestResponseMapping{{Attribute: "Value", ExposedName: "caseId/value"}},
	}})

	nested := inlineChildren(t, inlineRoot(t, doc))
	if len(nested) != 1 {
		t.Fatalf("got %d nested elements, want 1", len(nested))
	}
	if got := nested[0]["JsonPath"]; got != "(Object)|results|bindings" {
		t.Errorf("nested object JsonPath = %q, want (Object)|results|bindings", got)
	}
	if got := nested[0]["ExposedName"]; got != "bindings" {
		t.Errorf("nested object ExposedName = %q, want bindings", got)
	}
	values := inlineChildren(t, nested[0])
	if len(values) != 1 {
		t.Fatalf("got %d values, want 1", len(values))
	}
	if got := values[0]["JsonPath"]; got != "(Object)|results|bindings|caseId|value" {
		t.Errorf("value JsonPath = %q", got)
	}
}
