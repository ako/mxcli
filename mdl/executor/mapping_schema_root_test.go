// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"
)

// Issue #267: a mapping's root element does not have to be the schema's root —
// Studio Pro lets you start the mapping at any element and stores that as the
// root element's JsonPath (13 of the 327 demo-app documents do). MDL had no
// syntax for it, and DESCRIBE printed such a mapping as if it were rooted at the
// document root: output that parsed and executed, binding members that do not
// exist at that level.
//
// `root a/b/c` selects the element, written in member names. The path MAY pass
// through an array — the mapping is then rooted at the item — where a value
// reference may not (CE0256).

const schemaRootSetup = `create entity ` + testModule + `.SRMessage ( Content: String(500) );
create json structure ` + testModule + `.JSON_SR
  snippet '{"id": "x", "choices": [{"index": 0, "message": {"role": "a", "content": "hi"}}]}';`

func TestMappingNestedSchemaRoot(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(schemaRootSetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Control: no clause means the structure's own root, unchanged behaviour.
	if err := env.executeMDL(`create import mapping ` + testModule + `.IM_SRRoot
  with json structure ` + testModule + `.JSON_SR
{
  create ` + testModule + `.SRMessage { Content = id }
};`); err != nil {
		t.Fatalf("control (no root clause) failed: %v", err)
	}
	if got := rootJSONPath(t, env, "ImportMappings$ImportMapping", "IM_SRRoot"); got != "(Object)" {
		t.Fatalf("control root JsonPath = %q, want (Object)", got)
	}

	if err := env.executeMDL(`create import mapping ` + testModule + `.IM_SRNested
  with json structure ` + testModule + `.JSON_SR root choices/message
{
  create ` + testModule + `.SRMessage { Content = content }
};`); err != nil {
		t.Fatalf("nested root rejected (#267): %v", err)
	}

	// `choices` is an array, so the path steps through its item on the way to
	// `message` — the whole point of the clause.
	if got := rootJSONPath(t, env, "ImportMappings$ImportMapping", "IM_SRNested"); got != "(Object)|choices|(Object)|message" {
		t.Errorf("nested root JsonPath = %q, want (Object)|choices|(Object)|message", got)
	}
	// Members must resolve against the SELECTED root, not the document root.
	if !hasValuePath(t, env, "ImportMappings$ImportMapping", "IM_SRNested",
		"(Object)|choices|(Object)|message|content") {
		t.Error("no value element under the selected root — the member resolved elsewhere")
	}

	checkMappingRoundTrip(t, env, fixtureMapping{
		Module: testModule, Name: "IM_SRNested", Type: "ImportMappings$ImportMapping",
	})
}

// TestMappingSchemaRootRefusals: an unresolvable root is refused and names what
// would have worked, rather than being written through to fail in the build
// (#259's rule).
func TestMappingSchemaRootRefusals(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	if err := env.executeMDL(schemaRootSetup); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	err := env.executeMDL(`create import mapping ` + testModule + `.IM_SRBad
  with json structure ` + testModule + `.JSON_SR root choices/nosuchthing
{
  create ` + testModule + `.SRMessage { Content = content }
};`)
	if err == nil {
		t.Fatal("accepted a root path that does not resolve")
	}
	if !strings.Contains(err.Error(), "available:") {
		t.Errorf("error %q does not list the members that would have worked", err)
	}
}
