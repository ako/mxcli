// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Issue #265: an import mapping can take an INPUT OBJECT — Mendix stores it as
// ParameterType, a DataTypes$ObjectType naming the entity — and a custom handler
// binds it with `Param: parameter` (#264). 10 of the 327 demo-app mappings
// declare one, and MDL had no syntax for it, so `Param: parameter` referred to
// something that could not exist.
//
// Import only: 190 of the 200 import mappings store the DataTypes$UnknownType
// marker and the other 10 an ObjectType, while an export mapping carries no
// ParameterType property at all (0 of 127) — its parameter IS its root object.

const mappingParamSetup = `create entity ` + testModule + `.PChunkCollection ( Name: String(100) );
create entity ` + testModule + `.PChunk ( Text: String(200) );
create association ` + testModule + `.PChunk_PChunkCollection
  from ` + testModule + `.PChunk to ` + testModule + `.PChunkCollection
  type Reference;
create json structure ` + testModule + `.JSON_P
  snippet '{"id": "x", "embeddings": [{"text": "t", "idx": 1}]}';
create microflow ` + testModule + `.P_FindChunk ( $Collection: ` + testModule + `.PChunkCollection, $Index: Integer )
returns ` + testModule + `.PChunk
begin
  @start(100, 200)
  @position(400, 200)
  return empty;
end;
/
`

func TestMappingInputParameter(t *testing.T) {
	env := newCustomHandlerEnv(t) // modelsdk: the custom handler needs it (#264)
	defer env.teardown()

	if err := env.executeMDL(mappingParamSetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Control: no clause means no input object, which Mendix stores as the
	// UnknownType MARKER rather than by omitting the property.
	if err := env.executeMDL(`create import mapping ` + testModule + `.IM_PNone
  with json structure ` + testModule + `.JSON_P
{
  create ` + testModule + `.PChunkCollection { Name = id }
};`); err != nil {
		t.Fatalf("control (no parameter) failed: %v", err)
	}
	if got := parameterType(t, env, "IM_PNone"); got["$Type"] != "DataTypes$UnknownType" {
		t.Errorf("control ParameterType = %q, want DataTypes$UnknownType", got["$Type"])
	}
	if got := parameterType(t, env, "IM_PNone")["Entity"]; got != "" {
		t.Errorf("control carries Entity %q — an UnknownType names none", got)
	}

	if err := env.executeMDL(`create import mapping ` + testModule + `.IM_PParam
  with json structure ` + testModule + `.JSON_P
  parameter ` + testModule + `.PChunkCollection
{
  create ` + testModule + `.PChunkCollection {
    Name = id,
    find ` + testModule + `.PChunk_PChunkCollection/` + testModule + `.PChunk
      by ` + testModule + `.P_FindChunk ( Collection: parameter, Index: idx )
      = embeddings {
        Text = text
      }
  }
};`); err != nil {
		t.Fatalf("parameter clause rejected (#265): %v", err)
	}

	pt := parameterType(t, env, "IM_PParam")
	if pt["$Type"] != "DataTypes$ObjectType" {
		t.Errorf("ParameterType $Type = %q, want DataTypes$ObjectType", pt["$Type"])
	}
	if pt["Entity"] != testModule+".PChunkCollection" {
		t.Errorf("ParameterType Entity = %q", pt["Entity"])
	}

	// DESCRIBE has to print the clause, or the `Param: parameter` handler in the
	// body describes to something that cannot be re-executed.
	out, derr := env.describeMDL(`describe import mapping ` + testModule + `.IM_PParam;`)
	if derr != nil {
		t.Fatalf("describe failed: %v", derr)
	}
	if !strings.Contains(out, "parameter "+testModule+".PChunkCollection") {
		t.Errorf("DESCRIBE does not print the parameter clause:\n%s", out)
	}

	checkMappingRoundTrip(t, env, fixtureMapping{
		Module: testModule, Name: "IM_PParam", Type: "ImportMappings$ImportMapping",
	})
}

// TestMappingParameterRefusals: both halves of the pair are refused rather than
// written through, since mxbuild reports each and the mapping looks fine until
// then (#259's rule).
func TestMappingParameterRefusals(t *testing.T) {
	env := newCustomHandlerEnv(t)
	defer env.teardown()

	if err := env.executeMDL(mappingParamSetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// `Param: parameter` with no declared input object — mxbuild reports CE0279
	// "Parameter is used in object handling microflow while the mapping does not
	// specify one." (measured on 11.13 with the guard stubbed).
	err := env.executeMDL(`create import mapping ` + testModule + `.IM_PUndeclared
  with json structure ` + testModule + `.JSON_P
{
  create ` + testModule + `.PChunkCollection {
    find ` + testModule + `.PChunk_PChunkCollection/` + testModule + `.PChunk
      by ` + testModule + `.P_FindChunk ( Collection: parameter, Index: idx )
      = embeddings { Text = text }
  }
};`)
	if err == nil {
		t.Fatal("accepted `Param: parameter` on a mapping that declares none")
	}
	if !strings.Contains(err.Error(), "CE0279") {
		t.Errorf("error %q does not name the build error it prevents", err)
	}

	// An entity that does not exist.
	err = env.executeMDL(`create import mapping ` + testModule + `.IM_PBad
  with json structure ` + testModule + `.JSON_P
  parameter ` + testModule + `.NoSuchEntity
{
  create ` + testModule + `.PChunkCollection { Name = id }
};`)
	if err == nil {
		t.Fatal("accepted a parameter entity that does not exist")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q does not say the entity was not found", err)
	}
}

// parameterType returns the mapping's stored ParameterType sub-document as a
// flat map, so a test can assert both the variant and the entity it names.
func parameterType(t *testing.T, env *testEnv, name string) map[string]string {
	t.Helper()
	_, raw := storedUnit(t, env.projectPath, "ImportMappings$ImportMapping", name)
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	out := map[string]string{}
	for _, e := range doc {
		if e.Key != "ParameterType" {
			continue
		}
		sub, ok := e.Value.(bson.D)
		if !ok {
			t.Fatalf("%s: ParameterType is %T, want a sub-document", name, e.Value)
		}
		for _, f := range sub {
			if s, ok := f.Value.(string); ok {
				out[f.Key] = s
			}
		}
		return out
	}
	t.Fatalf("%s stores no ParameterType — Studio Pro writes it on every import mapping", name)
	return nil
}

// TestMappingParameterLegacyEngine: unlike the custom handler it feeds (#264),
// the parameter itself is serializable by BOTH engines — the legacy writer has
// always emitted a ParameterType sub-document, it just hardcoded the UnknownType
// variant. So a mapping that declares an input object without a custom handler
// is authorable on legacy too, and refusing it there would be gratuitous.
func TestMappingParameterLegacyEngine(t *testing.T) {
	env := setupTestEnv(t) // legacy backend
	defer env.teardown()

	if err := env.executeMDL(mappingParamSetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if err := env.executeMDL(`create import mapping ` + testModule + `.IM_PLegacy
  with json structure ` + testModule + `.JSON_P
  parameter ` + testModule + `.PChunkCollection
{
  create ` + testModule + `.PChunkCollection { Name = id }
};`); err != nil {
		t.Fatalf("legacy engine rejected the parameter clause: %v", err)
	}
	pt := parameterType(t, env, "IM_PLegacy")
	if pt["$Type"] != "DataTypes$ObjectType" || pt["Entity"] != testModule+".PChunkCollection" {
		t.Errorf("legacy ParameterType = %v", pt)
	}

	checkMappingRoundTrip(t, env, fixtureMapping{
		Module: testModule, Name: "IM_PLegacy", Type: "ImportMappings$ImportMapping",
	})
}
