// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"testing"
)

// upstream #922. A REST call storing its response in a file document round-trips
// on BOTH engines.
//
// Running it on both is the point. The two readers failed differently and would
// each have looked fine from inside itself: the legacy parser had no
// FileDocument case at all and returned nil handling (so the describer printed
// `returns String` AND dropped the output variable), while the modelsdk reader
// read the entity out of VariableType and then threw it away, keeping only a
// literal match on System.HttpResponse and treating everything else as String.
//
// Measured on mxbuild 11.6.6 before the fix: describe → exec rewrote the stored
// ResultHandlingType from FileDocument to String and the VariableType from
// ObjectType(MyFile) to StringType, and mx check still reported only the
// project's pre-existing baseline error — the corruption had no downstream
// signal at all.
func TestRoundtripRestCall_FileDocumentResult(t *testing.T) {
	for _, eng := range gateEngines {
		t.Run(eng.name, func(t *testing.T) { testRoundtripRestFileDocument(t, eng) })
	}
}

func testRoundtripRestFileDocument(t *testing.T, eng gateEngine) {
	env := setupTestEnvWithBackend(t, eng.factory)
	defer env.teardown()

	// Mendix rejects the base System.FileDocument as a return type (CE0362), so
	// the result must be a specialization — CE1540 permits FileDocument to be
	// specialized, which is what makes this feature expressible at all.
	entityMDL := `create or modify persistent entity ` + testModule + `.MyFile extends System.FileDocument ();`
	if err := env.executeMDL(entityMDL); err != nil {
		t.Fatalf("failed to create the file document specialization: %v", err)
	}

	createMDL := `create or modify microflow ` + testModule + `.MF_FetchFile (Location: String)
begin
  $fileResponseGet = rest call get '{1}' with ({1} = $Location)
    timeout 300
    returns ` + testModule + `.MyFile;
end;`

	env.assertContains(createMDL, []string{
		// The entity, not "String" — the reported symptom.
		"returns " + testModule + ".MyFile",
		// The output variable, which the legacy engine also used to lose because
		// its nil result handling took the variable fallback with it.
		"$fileResponseGet = ",
	})
}

// The neighbouring forms must keep round-tripping: `returns response` was
// reported alongside the file document case but is CORRECT as it stands.
// HttpResponse cannot be specialized (CE1540 permits only User, FileDocument,
// Image and Paging), so `response` already names the only type such a result can
// have and there is nothing for the describer to add.
func TestRoundtripRestCall_ResponseResultUnchanged(t *testing.T) {
	for _, eng := range gateEngines {
		t.Run(eng.name, func(t *testing.T) {
			env := setupTestEnvWithBackend(t, eng.factory)
			defer env.teardown()

			createMDL := `create or modify microflow ` + testModule + `.MF_FetchResponse (Location: String)
begin
  $httpResponseGet = rest call get '{1}' with ({1} = $Location)
    timeout 300
    returns response;
end;`

			env.assertContains(createMDL, []string{
				"returns response",
				"$httpResponseGet = ",
			})
		})
	}
}
