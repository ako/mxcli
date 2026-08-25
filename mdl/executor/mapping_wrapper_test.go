// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Issue #268: an array of PRIMITIVES — `"tags": ["a","b"]` — maps to one entity
// per string with the value on an attribute. Mendix models it with a Wrapper
// element whose single child binds the "(Value)" marker; 34 of the 327 demo-app
// documents contain one, and MDL could express none of them.
//
// Two halves, and the first was a prerequisite: mxcli's JSON structure builder
// wrote the wrong PATHS for a primitive array (the wrapper at "|(Object)" and the
// value at "|(Object)|", a trailing empty segment), so nothing could resolve
// against it. The mapping builders then stepped from an array to a hardcoded
// "|(Object)" rather than to the array's actual child.
//
// No new syntax: an array of primitives is written like any other array and the
// wrapper level is generated, the same way the item level is.

const wrapperSetup = `create entity ` + testModule + `.WProduct ( Name: String(100) );
create entity ` + testModule + `.WTag ( Value: String(100) );
create association ` + testModule + `.WTag_WProduct
  from ` + testModule + `.WTag to ` + testModule + `.WProduct
  type Reference;
create json structure ` + testModule + `.JSON_W
  snippet '{"name": "n", "tags": ["a", "b"]}';`

// TestJSONStructurePrimitiveArrayPaths pins the structure half. The markers are
// what a mapping resolves against, so getting them wrong makes the construct
// unreachable rather than merely different.
func TestJSONStructurePrimitiveArrayPaths(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(wrapperSetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	_, raw := storedUnit(t, env.projectPath, "JsonStructures$JsonStructure", "JSON_W")
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode structure: %v", err)
	}
	paths := map[string]string{}
	var walk func(v any)
	walk = func(v any) {
		for _, e := range mappingChildren(v) {
			paths[lookupString(e, "ExposedName")] = lookupString(e, "Path")
			for _, f := range e {
				if f.Key == "Children" {
					walk(f.Value)
				}
			}
		}
	}
	for _, e := range doc {
		if e.Key == "Elements" {
			walk(e.Value)
		}
	}
	if got := paths["Tag"]; got != "(Object)|tags|(Wrapper)" {
		t.Errorf("wrapper path = %q, want (Object)|tags|(Wrapper)", got)
	}
	if got := paths["Value"]; got != "(Object)|tags|(Wrapper)|(Value)" {
		t.Errorf("value path = %q, want (Object)|tags|(Wrapper)|(Value)", got)
	}
}

func TestMappingPrimitiveArrayWrapper(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(wrapperSetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := env.executeMDL(`create import mapping ` + testModule + `.IM_W
  with json structure ` + testModule + `.JSON_W
{
  create ` + testModule + `.WProduct {
    Name = name,
    create ` + testModule + `.WTag_WProduct/` + testModule + `.WTag = tags {
      Value = Value
    }
  }
};`); err != nil {
		t.Fatalf("primitive array rejected (#268): %v", err)
	}

	root := rootElement(t, env, "ImportMappings$ImportMapping", "IM_W")
	var wrapper map[string]string
	for _, child := range childElements(root) {
		if lookupString(child, "JsonPath") == "(Object)|tags|(Wrapper)" {
			wrapper = map[string]string{
				"type":        lookupString(child, "$Type"),
				"elementType": lookupString(child, "ElementType"),
				"entity":      lookupString(child, "Entity"),
			}
			vals := childElements(child)
			if len(vals) != 1 {
				t.Fatalf("wrapper has %d children, want 1 (the value)", len(vals))
			}
			if got := lookupString(vals[0], "JsonPath"); got != "(Object)|tags|(Wrapper)|(Value)" {
				t.Errorf("value JsonPath = %q", got)
			}
			if got := lookupString(vals[0], "Attribute"); got != testModule+".WTag.Value" {
				t.Errorf("value Attribute = %q", got)
			}
		}
	}
	if wrapper == nil {
		t.Fatal("no wrapper element at (Object)|tags|(Wrapper)")
	}
	// A Wrapper is an OBJECT mapping element. Both writers used to dispatch on
	// Kind Object/Array only, so it serialized as a Value with no attribute —
	// mxbuild reports CE5015 "… is not a Value element."
	if wrapper["type"] != "ImportMappings$ObjectMappingElement" {
		t.Errorf("wrapper $Type = %q, want an ObjectMappingElement", wrapper["type"])
	}
	if wrapper["elementType"] != "Wrapper" {
		t.Errorf("wrapper ElementType = %q, want Wrapper", wrapper["elementType"])
	}
	if wrapper["entity"] != testModule+".WTag" {
		t.Errorf("wrapper Entity = %q", wrapper["entity"])
	}

	checkMappingRoundTrip(t, env, fixtureMapping{
		Module: testModule, Name: "IM_W", Type: "ImportMappings$ImportMapping",
	})
}

func TestExportPrimitiveArrayWrapper(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(wrapperSetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := env.executeMDL(`create export mapping ` + testModule + `.EM_W
  with json structure ` + testModule + `.JSON_W
{
  ` + testModule + `.WProduct {
    name = Name,
    ` + testModule + `.WTag_WProduct/` + testModule + `.WTag as tags {
      Value = Value
    }
  }
};`); err != nil {
		t.Fatalf("primitive array rejected on export: %v", err)
	}

	// Export keeps the container, as it does for an array of objects (#262).
	root := rootElement(t, env, "ExportMappings$ExportMapping", "EM_W")
	var found bool
	for _, child := range childElements(root) {
		if lookupString(child, "JsonPath") != "(Object)|tags" {
			continue
		}
		if got := lookupString(child, "ElementType"); got != "Array" {
			t.Errorf("container ElementType = %q, want Array", got)
		}
		for _, w := range childElements(child) {
			if lookupString(w, "JsonPath") == "(Object)|tags|(Wrapper)" {
				found = true
				if got := lookupString(w, "ElementType"); got != "Wrapper" {
					t.Errorf("wrapper ElementType = %q, want Wrapper", got)
				}
			}
		}
	}
	if !found {
		t.Fatal("no wrapper under the array container")
	}

	checkMappingRoundTrip(t, env, fixtureMapping{
		Module: testModule, Name: "EM_W", Type: "ExportMappings$ExportMapping",
	})
}
