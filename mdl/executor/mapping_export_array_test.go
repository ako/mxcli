// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
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

// TestExportGroupElement covers the OTHER half of #262: an entity-less grouping
// node — a JSON object with no Mendix object behind it.
//
// It may hold OBJECT elements only. Every one of the 10 such elements in the
// demo apps does, and a value under one is rejected by mxbuild with CE0061 "No
// entity selected." That constraint was found the hard way: a first attempt put
// a value there, mxbuild refused it, and the syntax was withdrawn on the
// assumption that the shape itself was invalid. Transplanting the shipped
// document (MxGenAIConnector.EM_ConverseRequest) into an 11.13 project and
// checking it — 0 errors — showed the shape was fine and the reproduction was
// not.
func TestExportGroupElement(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(exportArraySetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := env.executeMDL(`create json structure ` + testModule + `.JSON_EAG
  snippet '{"name": "n", "wrapper": {"items": [{"code": "c"}]}}';
create export mapping ` + testModule + `.EM_Group
  with json structure ` + testModule + `.JSON_EAG
{
  ` + testModule + `.EARoot {
    name = Name,
    group as wrapper {
      ` + testModule + `.EAItem_EARoot/` + testModule + `.EAItem as items {
        code = Code
      }
    }
  }
};`); err != nil {
		t.Fatalf("group element rejected: %v", err)
	}

	root := rootElement(t, env, "ExportMappings$ExportMapping", "EM_Group")
	var group bson.D
	for _, child := range childElements(root) {
		if lookupString(child, "JsonPath") == "(Object)|wrapper" {
			group = child
		}
	}
	if group == nil {
		t.Fatal("no group element at (Object)|wrapper")
	}
	if got := lookupString(group, "Entity"); got != "" {
		t.Errorf("group Entity = %q, want empty", got)
	}
	if got := lookupString(group, "ElementType"); got != "Object" {
		t.Errorf("group ElementType = %q, want Object (an Array container is a different shape)", got)
	}
	if got := lookupString(group, "ObjectHandling"); got != "Find" {
		t.Errorf("group ObjectHandling = %q, want Find", got)
	}

	checkMappingRoundTrip(t, env, fixtureMapping{
		Module: testModule, Name: "EM_Group", Type: "ExportMappings$ExportMapping",
	})
}

// TestExportGroupRefusesValueChild pins the constraint with the error the author
// should get instead of mxbuild's CE0061.
func TestExportGroupRefusesValueChild(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(exportArraySetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	err := env.executeMDL(`create export mapping ` + testModule + `.EM_BadGroup
  with json structure ` + testModule + `.JSON_EA
{
  ` + testModule + `.EARoot {
    group as items { name = Name }
  }
};`)
	if err == nil {
		t.Fatal("accepted a value under a group — mxbuild reports CE0061")
	}
	if !strings.Contains(err.Error(), "CE0061") {
		t.Errorf("error %q does not name the build error it prevents", err)
	}
}

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
