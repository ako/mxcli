// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/modelsdk/canon"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// The fixture is eleven real Studio Pro mapping documents (see
// testdata/mapping-fixtures/README.md). The round trip under test is the one
// users actually perform:
//
//	describe import mapping X   ->   edit   ->   exec
//
// DESCRIBE emits `create or modify <same name>`, so re-executing its own output
// rewrites the same document. A faithful describer therefore leaves the stored
// document semantically unchanged; anything else has silently rewritten the
// user's mapping into a different one.
//
// Comparison is `canon.Equal`, the same canonical form the write path elides on
// (ADR-0008) — element `$ID`s are normalised away, because a rebuild mints fresh
// ones and a byte comparison would fail for every mapping including the faithful
// ones.
//
// knownLossy records what is broken TODAY, with the issue that tracks it. The
// test fails in both directions: a mapping that starts round-tripping must be
// struck off (that is a fix landing), and one that stops must be explained. See
// docs/11-proposals/PROPOSAL_mapping_coverage.md for the measurement this came
// from — 112 of 327 real mappings are in this class.
var knownLossy = map[string]string{
	// Constructs MDL still cannot express.
	// #265's `parameter GenAICommons.ChunkCollection` is fixed here and prints.
	// What blocks it now is #260 item 2 in the IMPORT printer: this mapping's
	// array container carries its OWN entity, association and custom handler
	// (Studio Pro's two-level import array), and printImportMappingElement
	// branches on `Kind == "Object"` so an "Array" falls into the value branch
	// and prints ` = embeddings`. Fixing only the dispatch would be worse than
	// the parse error: the import builder GENERATES the item level, so the
	// object form would re-execute to a one-level mapping — silent loss where
	// there is now a loud failure. Needs an import spelling for the two-level
	// array, as #262 kept for export.
	"MxGenAIConnector.IM_CohereEmbed_Response": "#260 two-level import array container (entity on the Array element)",
	// #268's wrapper is fixed here; what blocks it now is an OBJECT element with
	// a nested member path (`= meta/pagination`), which the grammar has never
	// accepted — #260 item 1.
	"KrogerAPI.IM_ProductList": "#260 object element with a nested member path",
	// #268's wrapper and #261's backup are fixed here; the residue is the export
	// writer's property set — #277 (IsKey), #279 (MinOccurs).
	"MxGenAIConnector.EM_CohereEmbed_Request": "#277 IsKey; #279 MinOccurs",
	// #267 is fixed for this one — its `root chunks` clause round-trips — but it
	// carries two shapes that have never parsed: an object element with an
	// entity and NO association (DESCRIBE prints `./Entity`) and an entity-less
	// import container (`= metadata`). Both are #260 describe-only spellings.
	"MxGenAIConnector.IM_Collection_RetrieveNearestNeighbors": "#260 association-less and entity-less import object elements",

	// Not constructs: the property SET a rebuild writes (#279). #266's converter
	// and #261's backup are fixed on both of these — what remains is
	// MessageDefinition2 being dropped, plus #277 on the export twin.
	"FeedbackModule.IMM_PostResponse": "#279 rebuild drops MessageDefinition2",
	"FeedbackModule.EXM_PostFeedback": "#279 MessageDefinition2/MinOccurs; #277 IsKey/MaxLength",
	// The same family in the opposite direction: this document predates
	// MappingSourceReference (the only one of the twelve without the key) and
	// the rebuild writes the current shape, which is what Studio Pro does on
	// save too. Kept so the version difference stays visible.
	"MendixSSO.AppRolesResponse": "#279 rebuild adds MappingSourceReference, which 11 of 12 real mappings carry",
}

// The positive control is authored by mxcli itself rather than transplanted: a
// mapping mxcli wrote, described and re-executed MUST be stable, so a green run
// proves the harness can actually pass. Without it, "everything is lossy" and
// "the comparison never looked" are indistinguishable — the trap CLAUDE.md
// warns about.
//
// KrogerAPI.IM_Location is the second control, and the stronger one: a REAL
// Studio Pro document that uses nothing outside MDL's range and does round-trip.
// It only round-trips because its JSON structure is transplanted verbatim — with
// the structure regenerated from its snippet it drifted, which is how #272 was
// found (mxcli gives the root MinOccurs 0 where Studio Pro writes 1, and names an
// array item "DataItem" where Studio Pro singularises to "Datum"). Both controls
// earn their place: the synthetic one proves the harness, the real one proves the
// fixture is loaded faithfully.
const controlJSON = `create json structure ` + testModule + `.JSON_RoundtripControl
  snippet '{"id": 1, "name": "a"}';`

const controlEntity = `create entity ` + testModule + `.RoundtripControl (
  Ident: Integer,
  Label: String(100)
);`

const controlMappingMDL = `create import mapping ` + testModule + `.IM_RoundtripControl
  with json structure ` + testModule + `.JSON_RoundtripControl
{
  create ` + testModule + `.RoundtripControl {
    Ident = id key,
    Label = name
  }
};`

const controlMapping = testModule + ".IM_RoundtripControl"

type fixtureMapping struct {
	Module   string   `json:"module"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	File     string   `json:"file"`
	Source   string   `json:"source"`
	Blockers []string `json:"blockers"`
}

type fixtureManifest struct {
	Modules []string `json:"modules"`
	// Documents transplanted verbatim alongside the mappings: JSON structures
	// and message definitions. A structure rebuilt from its snippet is not the
	// same document (#272), and its differences leak into every mapping over it,
	// so regenerating them would make this test measure the structure builder.
	Documents []fixtureMapping `json:"documents"`
	Mappings  []fixtureMapping `json:"mappings"`
}

func (m fixtureMapping) qualifiedName() string { return m.Module + "." + m.Name }

func (m fixtureMapping) kind() string {
	if strings.HasPrefix(m.Type, "Import") {
		return "import"
	}
	return "export"
}

func TestMappingFixtureRoundTrip(t *testing.T) {
	// The modelsdk engine, not setupTestEnv's legacy default: it is what users
	// get (cmd/mxcli/engine.go defaults to it), and it is the only engine that
	// reads message definitions (#263) — on legacy the fixture's
	// message-definition mappings are refused by design.
	env := setupTestEnvWithBackend(t, func() backend.FullBackend { return modelsdkbackend.New() })
	defer env.teardown()

	manifest, loaded := loadMappingFixture(t, env)

	// Positive control first: if this does not round-trip, nothing below means
	// anything and the failure is in the harness, not the fixture.
	for _, mdl := range []string{controlJSON, controlEntity, controlMappingMDL} {
		if err := env.executeMDL(mdl); err != nil {
			t.Fatalf("control setup failed: %v", err)
		}
	}
	t.Run(controlMapping, func(t *testing.T) {
		checkMappingRoundTrip(t, env, fixtureMapping{
			Module: testModule, Name: "IM_RoundtripControl",
			Type: "ImportMappings$ImportMapping",
		})
	})

	seen := map[string]bool{}
	for _, m := range manifest.Mappings {
		if !loaded[m.qualifiedName()] {
			continue
		}
		seen[m.qualifiedName()] = true
		t.Run(m.qualifiedName(), func(t *testing.T) {
			checkMappingRoundTrip(t, env, m)
		})
	}

	for qn := range knownLossy {
		if !seen[qn] {
			t.Errorf("knownLossy lists %s, which the fixture does not contain — "+
				"stale entry, remove it", qn)
		}
	}
}

// checkMappingRoundTrip describes one mapping, re-executes that output, and
// compares the stored document before and after.
func checkMappingRoundTrip(t *testing.T, env *testEnv, m fixtureMapping) {
	t.Helper()
	qn := m.qualifiedName()
	reason, lossyExpected := knownLossy[qn]

	unitID, before := storedUnit(t, env.projectPath, m.Type, m.Name)

	described, err := env.describeMDL(fmt.Sprintf("describe %s mapping %s", m.kind(), qn))
	if err != nil {
		failOrLog(t, lossyExpected, reason, "describe failed: %v", err)
		return
	}
	if strings.TrimSpace(described) == "" {
		failOrLog(t, lossyExpected, reason, "describe produced no output")
		return
	}

	if err := env.executeMDL(described); err != nil {
		// A parse error here is the loud half of #260: DESCRIBE emitted MDL its
		// own grammar rejects.
		failOrLog(t, lossyExpected, reason, "re-executing DESCRIBE output failed: %v\n--- output ---\n%s",
			err, described)
		return
	}

	_, after := storedUnit(t, env.projectPath, m.Type, m.Name)
	equal, err := canon.Equal(before, after)
	if err != nil {
		t.Fatalf("canonical compare failed for %s (unit %s): %v", qn, unitID, err)
	}

	switch {
	case equal && lossyExpected:
		t.Errorf("%s now round-trips faithfully — remove it from knownLossy (was: %s)",
			qn, reason)
	case !equal && !lossyExpected:
		t.Errorf("%s does not round-trip: DESCRIBE output rebuilt a different document.\n"+
			"--- DESCRIBE output ---\n%s", qn, described)
	case !equal && lossyExpected:
		t.Logf("still lossy, as expected: %s", reason)
	}
}

// failOrLog reports a hard failure unless the mapping is a known-lossy one, in
// which case the failure IS the documented behaviour.
func failOrLog(t *testing.T, expected bool, reason, format string, args ...any) {
	t.Helper()
	if expected {
		t.Logf("still lossy, as expected (%s): %s", reason, fmt.Sprintf(format, args...))
		return
	}
	t.Errorf(format, args...)
}

// ---------------------------------------------------------------------------
// Fixture loading
// ---------------------------------------------------------------------------

// loadMappingFixture creates the fixture's modules, runs deps.mdl, and
// transplants each mapping document verbatim. It returns the manifest and the
// set of mappings actually loaded — one already present in the base project
// (FeedbackModule ships two) is left alone rather than duplicated, and is still
// exercised, since the project's copy is the same Studio Pro document.
func loadMappingFixture(t *testing.T, env *testEnv) (fixtureManifest, map[string]bool) {
	t.Helper()
	dir := filepath.Join("testdata", "mapping-fixtures")

	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest fixtureManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	// Modules one at a time: a module the base project already has must not
	// abort the rest, and `exec` refuses a whole script that contains a known
	// error rather than applying it partly.
	for _, mod := range manifest.Modules {
		_ = env.executeMDL(fmt.Sprintf("create module %s;", mod))
	}

	deps, err := os.ReadFile(filepath.Join(dir, "deps.mdl"))
	if err != nil {
		t.Fatalf("read deps.mdl: %v", err)
	}
	if err := env.executeMDL(string(deps)); err != nil {
		t.Fatalf("deps.mdl failed — the fixture cannot be loaded: %v", err)
	}

	present := storedDocumentNames(t, env.projectPath)
	modules := moduleUnitIDs(t, env.projectPath)

	w, err := mmpr.NewWriter(env.projectPath)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer w.Close()

	loaded := map[string]bool{}
	// Verbatim documents first — a mapping resolves its source by qualified name.
	for _, m := range append(append([]fixtureMapping{}, manifest.Documents...),
		manifest.Mappings...) {
		if m.File == "" {
			continue
		}
		isMapping := strings.HasSuffix(m.Type, "Mapping")
		if isMapping {
			loaded[m.qualifiedName()] = true
		}
		if present[m.Type+"/"+m.Name] {
			continue // already in the base project; use its copy
		}
		container, ok := modules[m.Module]
		if !ok {
			t.Fatalf("module %s missing after deps.mdl", m.Module)
		}
		contents, err := os.ReadFile(filepath.Join(dir, m.File))
		if err != nil {
			t.Fatalf("read %s: %v", m.File, err)
		}
		// The unit's ID *is* the document's own $ID. Minting a fresh one gives a
		// document that `show` lists and `describe` cannot find, because the
		// backend resolves a document's module from its ID.
		if err := w.InsertUnit(documentID(t, contents), container, "Documents", m.Type, contents); err != nil {
			t.Fatalf("transplant %s: %v", m.qualifiedName(), err)
		}
	}
	return manifest, loaded
}

// documentID reads a stored document's own $ID.
func documentID(t *testing.T, contents []byte) string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		t.Fatalf("decode fixture document: %v", err)
	}
	for _, e := range doc {
		if e.Key == "$ID" {
			if b, ok := e.Value.(bson.Binary); ok {
				return mmpr.BsonBinaryToID(b)
			}
			t.Fatalf("$ID is %T, want bson.Binary", e.Value)
		}
	}
	t.Fatal("fixture document has no $ID")
	return ""
}

// storedUnit returns the unit id and raw stored bytes of a mapping document.
func storedUnit(t *testing.T, project, typeName, name string) (string, []byte) {
	t.Helper()
	r, err := mmpr.Open(project)
	if err != nil {
		t.Fatalf("open project: %v", err)
	}
	defer r.Close()

	ids, err := r.ListAllUnitIDs()
	if err != nil {
		t.Fatalf("list units: %v", err)
	}
	sort.Strings(ids)
	for _, id := range ids {
		b, err := r.GetRawUnitBytes(id)
		if err != nil || len(b) == 0 {
			continue
		}
		tn, n := documentTypeAndName(b)
		if tn == typeName && n == name {
			return id, b
		}
	}
	t.Fatalf("no %s named %s in the project", typeName, name)
	return "", nil
}

// storedDocumentNames is the set of mapping, JSON-structure and
// message-definition documents the project already has, keyed "<type>/<name>".
func storedDocumentNames(t *testing.T, project string) map[string]bool {
	t.Helper()
	r, err := mmpr.Open(project)
	if err != nil {
		t.Fatalf("open project: %v", err)
	}
	defer r.Close()

	out := map[string]bool{}
	ids, err := r.ListAllUnitIDs()
	if err != nil {
		t.Fatalf("list units: %v", err)
	}
	for _, id := range ids {
		b, err := r.GetRawUnitBytes(id)
		if err != nil || len(b) == 0 {
			continue
		}
		switch tn, n := documentTypeAndName(b); tn {
		case "ImportMappings$ImportMapping", "ExportMappings$ExportMapping",
			"JsonStructures$JsonStructure", "MessageDefinitions$MessageDefinition":
			out[tn+"/"+n] = true
		}
	}
	return out
}

// moduleUnitIDs maps module name -> unit id.
func moduleUnitIDs(t *testing.T, project string) map[string]string {
	t.Helper()
	r, err := mmpr.Open(project)
	if err != nil {
		t.Fatalf("open project: %v", err)
	}
	defer r.Close()

	out := map[string]string{}
	ids, err := r.ListAllUnitIDs()
	if err != nil {
		t.Fatalf("list units: %v", err)
	}
	for _, id := range ids {
		b, err := r.GetRawUnitBytes(id)
		if err != nil || len(b) == 0 {
			continue
		}
		tn, n := documentTypeAndName(b)
		if tn == "Projects$Module" || tn == "Projects$ModuleImpl" {
			out[n] = id
		}
	}
	return out
}

// documentTypeAndName reads $Type and Name off a stored document without
// decoding the whole tree.
func documentTypeAndName(contents []byte) (string, string) {
	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return "", ""
	}
	var typeName, name string
	for _, e := range doc {
		switch e.Key {
		case "$Type":
			typeName, _ = e.Value.(string)
		case "Name":
			name, _ = e.Value.(string)
		}
	}
	return typeName, name
}
