// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"
)

// Issue #266: `Attr = Module.MF(jsonField)` was already in the grammar and
// already built into the AST — nothing downstream read it. The model element had
// no Converter field and every writer hardcoded the BSON key to "", so the
// statement was accepted, the mapping was created, and the transform was
// silently absent. The visitor also never set JsonName for that alternative, so
// the member had no name to resolve against either.
//
// The export side had no spelling at all; 22 export value elements in the demo
// apps carry a converter, so the mirrored `jsonKey = Module.MF(Attr)` was added.
//
// The stored element carries ONE property — the microflow — and no separate
// parameter path: its input is the member the element already binds, which is
// why the syntax names the member inside the call.

const converterSetup = `create entity ` + testModule + `.ConvResponse (
  URL: String(500),
  Payload: String(500)
);
create json structure ` + testModule + `.JSON_Conv
  snippet '{"uuid": "u", "payload": "p"}';
create microflow ` + testModule + `.ConvUUID ( $uuid: String )
returns String
begin
  @start(100, 200)
  @position(400, 200)
  return 'https://example.com/' + $uuid;
end;
/
`

func TestMappingConverterMicroflow(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(converterSetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if err := env.executeMDL(`create import mapping ` + testModule + `.IM_Conv
  with json structure ` + testModule + `.JSON_Conv
{
  create ` + testModule + `.ConvResponse {
    URL = ` + testModule + `.ConvUUID(uuid),
    Payload = payload
  }
};`); err != nil {
		t.Fatalf("import converter rejected: %v", err)
	}

	kids := childElements(rootElement(t, env, "ImportMappings$ImportMapping", "IM_Conv"))
	if len(kids) != 2 {
		t.Fatalf("want 2 value elements, got %d", len(kids))
	}
	if got := lookupString(kids[0], "Converter"); got != testModule+".ConvUUID" {
		t.Errorf("Converter = %q, want %s.ConvUUID", got, testModule)
	}
	// The member must still have resolved — the converted element binds the
	// member named inside the call, not nothing.
	if got := lookupString(kids[0], "JsonPath"); got != "(Object)|uuid" {
		t.Errorf("converted element JsonPath = %q, want (Object)|uuid", got)
	}
	// Control: the sibling without a converter must carry an empty one, or the
	// assertion above would pass on a writer that stamps every element.
	if got := lookupString(kids[1], "Converter"); got != "" {
		t.Errorf("unconverted element Converter = %q, want empty", got)
	}

	if err := env.executeMDL(`create export mapping ` + testModule + `.EM_Conv
  with json structure ` + testModule + `.JSON_Conv
{
  ` + testModule + `.ConvResponse {
    uuid = URL,
    payload = ` + testModule + `.ConvUUID(Payload)
  }
};`); err != nil {
		t.Fatalf("export converter rejected: %v", err)
	}
	ekids := childElements(rootElement(t, env, "ExportMappings$ExportMapping", "EM_Conv"))
	if len(ekids) != 2 {
		t.Fatalf("want 2 export value elements, got %d", len(ekids))
	}
	if got := lookupString(ekids[0], "Converter"); got != "" {
		t.Errorf("unconverted export element Converter = %q, want empty", got)
	}
	if got := lookupString(ekids[1], "Converter"); got != testModule+".ConvUUID" {
		t.Errorf("export Converter = %q, want %s.ConvUUID", got, testModule)
	}

	// Both must round-trip: a converter DESCRIBE cannot print is the same silent
	// loss in a different place (#260).
	checkMappingRoundTrip(t, env, fixtureMapping{
		Module: testModule, Name: "IM_Conv", Type: "ImportMappings$ImportMapping",
	})
	checkMappingRoundTrip(t, env, fixtureMapping{
		Module: testModule, Name: "EM_Conv", Type: "ExportMappings$ExportMapping",
	})
}

// TestMappingConverterUnknownMicroflow: an unresolvable converter is refused
// rather than written through, the same rule as an unresolvable schema source
// (#259) — mxbuild reports one as CE1613 and the transform is silently gone.
func TestMappingConverterUnknownMicroflow(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(converterSetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	err := env.executeMDL(`create import mapping ` + testModule + `.IM_BadConv
  with json structure ` + testModule + `.JSON_Conv
{
  create ` + testModule + `.ConvResponse { URL = ` + testModule + `.NoSuchMicroflow(uuid) }
};`)
	if err == nil {
		t.Fatal("accepted an unknown converter microflow")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q does not say the microflow was not found", err)
	}
}
