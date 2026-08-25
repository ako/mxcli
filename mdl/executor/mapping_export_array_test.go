// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"testing"
)

// Issue #262: mxcli put the entity and association on BOTH levels of an export
// array, and the MDL made the author declare them twice. Studio Pro leaves the
// container BARE — 93 of the 93 entity-less object elements in the demo apps are
// array containers of that shape:
//
//	et=Array  entity=-  assoc=-  oh=Find   path=…|Versions
//	  et=Object entity=…  assoc=…  oh=Find   path=…|Versions|(Object)
//
// So an array is now declared exactly like a nested object and the container is
// generated — the same rule #248 applied to a root array.

const exportArraySetup = `create entity ` + testModule + `.EARoot ( Name: String(100) );
create entity ` + testModule + `.EAItem ( Code: String(100) );
create association ` + testModule + `.EAItem_EARoot
  from ` + testModule + `.EAItem to ` + testModule + `.EARoot
  type Reference;
create json structure ` + testModule + `.JSON_EA
  snippet '{"name": "n", "items": [{"code": "c"}]}';`

func TestExportNestedArrayContainerIsBare(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(exportArraySetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	// One declaration for the array — no repeated Assoc/Entity for the item.
	if err := env.executeMDL(`create export mapping ` + testModule + `.EM_Arr
  with json structure ` + testModule + `.JSON_EA
{
  ` + testModule + `.EARoot {
    name = Name,
    ` + testModule + `.EAItem_EARoot/` + testModule + `.EAItem as items {
      code = Code
    }
  }
};`); err != nil {
		t.Fatalf("export array rejected: %v", err)
	}

	root := rootElement(t, env, "ExportMappings$ExportMapping", "EM_Arr")
	var container map[string]string
	for _, child := range childElements(root) {
		if lookupString(child, "JsonPath") == "(Object)|items" {
			container = map[string]string{
				"elementType": lookupString(child, "ElementType"),
				"entity":      lookupString(child, "Entity"),
				"association": lookupString(child, "Association"),
				"handling":    lookupString(child, "ObjectHandling"),
			}
			items := childElements(child)
			if len(items) != 1 {
				t.Fatalf("container has %d children, want 1 (the item)", len(items))
			}
			if got := lookupString(items[0], "JsonPath"); got != "(Object)|items|(Object)" {
				t.Errorf("item JsonPath = %q", got)
			}
			if got := lookupString(items[0], "Entity"); got != testModule+".EAItem" {
				t.Errorf("item Entity = %q — the entity belongs to the ITEM", got)
			}
			if got := lookupString(items[0], "Association"); got != testModule+".EAItem_EARoot" {
				t.Errorf("item Association = %q", got)
			}
		}
	}
	if container == nil {
		t.Fatal("no array container at (Object)|items")
	}
	if container["elementType"] != "Array" {
		t.Errorf("container ElementType = %q, want Array", container["elementType"])
	}
	// The point of the issue: bare.
	if container["entity"] != "" {
		t.Errorf("container Entity = %q, want empty", container["entity"])
	}
	if container["association"] != "" {
		t.Errorf("container Association = %q, want empty", container["association"])
	}
	if container["handling"] != "Find" {
		t.Errorf("container ObjectHandling = %q, want Find", container["handling"])
	}

	// DESCRIBE has to unwrap the generated container again, or the mapping is
	// lossy from the day the shape changed (#260).
	checkMappingRoundTrip(t, env, fixtureMapping{
		Module: testModule, Name: "EM_Arr", Type: "ExportMappings$ExportMapping",
	})
}
