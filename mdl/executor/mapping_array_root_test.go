// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Issue #248: a mapping over an array-rooted JSON structure could not be
// authored at all. Both executors hardcoded the root lookup to "(Object)", while
// an array-rooted structure is built at "(Array)" with its item at
// "(Array)|(Object)" (mdl/types/json_utils.go, buildElementFromRawRootArray), so
// the root resolved to nothing and every member was then reported "not a member
// of the JSON structure at (Object), which has no members there" — a path that
// structure never had.
//
// The fix takes the structure's own root rather than assuming its path. The
// existing array branch then does the rest: it was already stepping from an
// Array element to its "|(Object)" item for NESTED arrays, and that step is
// what a root array needs too.
//
// Knock-on: `returns mapping … as list of` and the list-only ranges (`first`,
// `offset`) are unusable without a list-rooted mapping, and
// mdl-examples/bug-tests/519-rest-mapping-as-list-of.mdl could not execute.

const arrayRootSetup = `create entity ` + testModule + `.ArrRootItem (
  Ident: Integer,
  Label: String(100)
);
create json structure ` + testModule + `.JSON_ArrRoot
  snippet '[{"id": 1, "name": "a"}]';
create json structure ` + testModule + `.JSON_ObjRoot
  snippet '{"id": 1, "name": "a"}';`

func TestImportMappingArrayRoot(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(arrayRootSetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Control: the same mapping body over an OBJECT-rooted structure. If this
	// ever breaks, the array case below proves nothing.
	if err := env.executeMDL(`create import mapping ` + testModule + `.IM_ObjRoot
  with json structure ` + testModule + `.JSON_ObjRoot
{
  create ` + testModule + `.ArrRootItem {
    Ident = id key,
    Label = name
  }
};`); err != nil {
		t.Fatalf("control (object root) failed — the fixture, not the fix, is broken: %v", err)
	}
	if got := rootJSONPath(t, env, "ImportMappings$ImportMapping", "IM_ObjRoot"); got != "(Object)" {
		t.Fatalf("control root JsonPath = %q, want (Object)", got)
	}

	if err := env.executeMDL(`create import mapping ` + testModule + `.IM_ArrRoot
  with json structure ` + testModule + `.JSON_ArrRoot
{
  create ` + testModule + `.ArrRootItem {
    Ident = id key,
    Label = name
  }
};`); err != nil {
		t.Fatalf("array-rooted import mapping rejected (#248): %v", err)
	}

	// The root element binds the array's ITEM, the same shape a nested array
	// produces and the one Studio Pro stores for an array-rooted mapping.
	if got := rootJSONPath(t, env, "ImportMappings$ImportMapping", "IM_ArrRoot"); got != "(Array)|(Object)" {
		t.Errorf("array root JsonPath = %q, want (Array)|(Object)", got)
	}

	// The members must have resolved against the item, not been invented: a
	// fabricated path passes `mxcli check` and fails only in mxbuild or at
	// runtime (#882).
	for _, want := range []string{"(Array)|(Object)|id", "(Array)|(Object)|name"} {
		if !hasValuePath(t, env, "ImportMappings$ImportMapping", "IM_ArrRoot", want) {
			t.Errorf("no value element bound at %q", want)
		}
	}

	// And it must round-trip, or the newly authorable mapping just joins the 112
	// silent-loss cases this fix was meant to shrink (#260).
	checkMappingRoundTrip(t, env, fixtureMapping{
		Module: testModule, Name: "IM_ArrRoot", Type: "ImportMappings$ImportMapping",
	})
}

func TestExportMappingArrayRoot(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(arrayRootSetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if err := env.executeMDL(`create export mapping ` + testModule + `.EM_ObjRoot
  with json structure ` + testModule + `.JSON_ObjRoot
{
  ` + testModule + `.ArrRootItem {
    id = Ident,
    name = Label
  }
};`); err != nil {
		t.Fatalf("control (object root) failed — the fixture, not the fix, is broken: %v", err)
	}

	if err := env.executeMDL(`create export mapping ` + testModule + `.EM_ArrRoot
  with json structure ` + testModule + `.JSON_ArrRoot
{
  ` + testModule + `.ArrRootItem {
    id = Ident,
    name = Label
  }
};`); err != nil {
		t.Fatalf("array-rooted export mapping rejected (#248): %v", err)
	}

	// An array-rooted EXPORT mapping is two elements: a bare Array container at
	// the structure root, and the item that carries the entity. Measured on
	// SnowflakeIntegration.EXM_SensorData (Evora, 11.13) — the import side
	// collapses to the item alone, the export side does not.
	if got := rootJSONPath(t, env, "ExportMappings$ExportMapping", "EM_ArrRoot"); got != "(Array)" {
		t.Errorf("array root JsonPath = %q, want (Array)", got)
	}
	root := rootElement(t, env, "ExportMappings$ExportMapping", "EM_ArrRoot")
	if got := lookupString(root, "ElementType"); got != "Array" {
		t.Errorf("container ElementType = %q, want Array", got)
	}
	if got := lookupString(root, "Entity"); got != "" {
		t.Errorf("container Entity = %q, want empty — the entity belongs to the item", got)
	}
	if got := lookupString(root, "ObjectHandling"); got != "Find" {
		t.Errorf("container ObjectHandling = %q, want Find", got)
	}
	items := childElements(root)
	if len(items) != 1 {
		t.Fatalf("container has %d children, want 1 (the item)", len(items))
	}
	if got := lookupString(items[0], "JsonPath"); got != "(Array)|(Object)" {
		t.Errorf("item JsonPath = %q, want (Array)|(Object)", got)
	}
	if got := lookupString(items[0], "Entity"); got != testModule+".ArrRootItem" {
		t.Errorf("item Entity = %q, want %s.ArrRootItem", got, testModule)
	}
	if got := lookupString(items[0], "ObjectHandling"); got != "Parameter" {
		t.Errorf("item ObjectHandling = %q, want Parameter", got)
	}
	for _, want := range []string{"(Array)|(Object)|id", "(Array)|(Object)|name"} {
		if !hasValuePath(t, env, "ExportMappings$ExportMapping", "EM_ArrRoot", want) {
			t.Errorf("no value element bound at %q", want)
		}
	}

	// DESCRIBE has to unwrap the container again, or this mapping is lossy from
	// the day it became authorable (#260).
	checkMappingRoundTrip(t, env, fixtureMapping{
		Module: testModule, Name: "EM_ArrRoot", Type: "ExportMappings$ExportMapping",
	})
}

// rootJSONPath returns the JsonPath of a mapping's single root element.
func rootJSONPath(t *testing.T, env *testEnv, typeName, name string) string {
	t.Helper()
	return lookupString(rootElement(t, env, typeName, name), "JsonPath")
}

// rootElement returns a mapping's single root element.
func rootElement(t *testing.T, env *testEnv, typeName, name string) bson.D {
	t.Helper()
	_, raw := storedUnit(t, env.projectPath, typeName, name)
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	for _, e := range doc {
		if e.Key != "Elements" {
			continue
		}
		for _, child := range mappingChildren(e.Value) {
			return child
		}
	}
	t.Fatalf("%s has no root element", name)
	return nil
}

// childElements returns an element's child documents.
func childElements(d bson.D) []bson.D {
	for _, e := range d {
		if e.Key == "Children" {
			return mappingChildren(e.Value)
		}
	}
	return nil
}

// hasValuePath reports whether any value element in the mapping binds jsonPath.
func hasValuePath(t *testing.T, env *testEnv, typeName, name, jsonPath string) bool {
	t.Helper()
	_, raw := storedUnit(t, env.projectPath, typeName, name)
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	found := false
	var walk func(v any)
	walk = func(v any) {
		for _, elem := range mappingChildren(v) {
			if strings.HasSuffix(lookupString(elem, "$Type"), "ValueMappingElement") &&
				lookupString(elem, "JsonPath") == jsonPath {
				found = true
			}
			for _, e := range elem {
				if e.Key == "Children" {
					walk(e.Value)
				}
			}
		}
	}
	for _, e := range doc {
		if e.Key == "Elements" {
			walk(e.Value)
		}
	}
	return found
}

// mappingChildren returns the documents of a Mendix typed array, dropping the
// leading list marker.
func mappingChildren(v any) []bson.D {
	arr, ok := v.(bson.A)
	if !ok {
		return nil
	}
	var out []bson.D
	for _, item := range arr {
		if d, ok := item.(bson.D); ok {
			out = append(out, d)
		}
	}
	return out
}

func lookupString(d bson.D, key string) string {
	for _, e := range d {
		if e.Key == key {
			s, _ := e.Value.(string)
			return s
		}
	}
	return ""
}
