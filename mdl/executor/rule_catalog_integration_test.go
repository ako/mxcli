// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/modelsdk/mpr"
	"go.mongodb.org/mongo-driver/bson"
)

// ruleKeys is Microflows$Rule's property list. A rule is a microflow minus nine
// properties, so the fixture below is built by keeping exactly these.
var ruleKeys = []string{
	"Name", "Documentation", "Excluded", "ExportLevel", "ObjectCollection",
	"Flows", "MicroflowReturnType", "MarkAsUsed", "ReturnVariableName", "ApplyEntityAccess",
}

// convertMicroflowUnitToRule rewrites an mxcli-created microflow document as a
// Microflows$Rule, in place. MDL cannot yet author a rule (that is slice 3), and
// no module in a blank Mendix app ships one, so this is how the test gets a rule
// whose body calls something.
func convertMicroflowUnitToRule(t *testing.T, projectPath, unitID string) {
	t.Helper()

	r, err := mpr.Open(projectPath)
	if err != nil {
		t.Fatalf("open project: %v", err)
	}
	raw, err := r.GetRawUnitBytes(unitID)
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	var containerBlob []byte
	if err := r.DB().QueryRow(`SELECT ContainerID FROM Unit WHERE UnitID = ?`,
		mpr.IDToBsonBinary(unitID).Data).Scan(&containerBlob); err != nil {
		t.Fatalf("read container: %v", err)
	}
	containerID := mpr.BlobToUUID(containerBlob)
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}

	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal unit: %v", err)
	}
	get := func(k string) (any, bool) {
		for _, e := range doc {
			if e.Key == k {
				return e.Value, true
			}
		}
		return nil, false
	}

	id, _ := get("$ID")
	out := bson.D{{Key: "$ID", Value: id}, {Key: "$Type", Value: "Microflows$Rule"}}
	for _, k := range ruleKeys {
		if v, ok := get(k); ok {
			out = append(out, bson.E{Key: k, Value: v})
		}
	}
	contents, err := bson.Marshal(out)
	if err != nil {
		t.Fatalf("marshal rule: %v", err)
	}

	w, err := mpr.NewWriter(projectPath)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer w.Close()
	if err := w.DeleteUnit(unitID); err != nil {
		t.Fatalf("delete microflow unit: %v", err)
	}
	if err := w.InsertUnit(unitID, containerID, "Documents", "Microflows$Rule", contents); err != nil {
		t.Fatalf("insert rule unit: %v", err)
	}
}

// A microflow called only from inside a rule's body must not read as dead code.
//
// Before rules were catalog objects whose bodies are walked, the reference graph
// never entered a rule, so every document a rule called was invisible: `show
// callers` reported none and GRAPH_DEAD_ASSETS listed it. Measured on the
// reference app before the fix, Rules.OnlyCalledByRule was reported dead.
//
// The unreferenced microflow is the control: it must stay dead, so the test
// cannot pass by the dead-asset view simply going empty.
func TestRuleBodyReferencesAreNotDeadCode(t *testing.T) {
	env := setupTestEnv(t)

	if err := env.executeMDL(`
create or modify microflow TestModule.CalledFromRule () returns Boolean
begin
  return true;
end
/
create or modify microflow TestModule.NeverCalled () returns Boolean
begin
  return true;
end
/
create or modify microflow TestModule.WillBecomeRule () returns Boolean
begin
  $Ok = call microflow TestModule.CalledFromRule ();
  return $Ok;
end
/
`); err != nil {
		t.Fatalf("create fixtures: %v", err)
	}

	unitID := microflowUnitID(t, env, "WillBecomeRule")
	convertMicroflowUnitToRule(t, env.projectPath, unitID)

	// Reconnect so the executor's reader sees the rewritten unit, then rebuild.
	if err := env.executor.Execute(&ast.ConnectStmt{Path: env.projectPath}); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	buildCatalogFull(t, env)

	if n := countRefs(t, env, "TestModule.WillBecomeRule", "TestModule.CalledFromRule", "call"); n != 1 {
		t.Errorf("call edge from the rule to CalledFromRule = %d, want 1 — a rule's body must be walked for references", n)
	}

	dead := deadAssetNames(t, env)
	if dead["TestModule.CalledFromRule"] {
		t.Error("CalledFromRule is reported dead, but a rule calls it")
	}
	if !dead["TestModule.NeverCalled"] {
		t.Error("control failed: NeverCalled should be dead, so the assertion above is not passing vacuously")
	}
}

// microflowUnitID resolves a microflow's unit id by name.
func microflowUnitID(t *testing.T, env *testEnv, name string) string {
	t.Helper()
	mfs, err := env.executor.Backend().ListMicroflows()
	if err != nil {
		t.Fatalf("list microflows: %v", err)
	}
	for _, mf := range mfs {
		if mf.Name == name {
			return string(mf.ID)
		}
	}
	t.Fatalf("microflow %s not found", name)
	return ""
}

// deadAssetNames returns the qualified names GRAPH_DEAD_ASSETS reports.
func deadAssetNames(t *testing.T, env *testEnv) map[string]bool {
	t.Helper()
	result, err := env.executor.catalog.Query("SELECT QualifiedName FROM graph_dead_assets")
	if err != nil {
		t.Fatalf("dead-asset query failed: %v", err)
	}
	out := map[string]bool{}
	for _, row := range result.Rows {
		out[fmt.Sprintf("%v", row[0])] = true
	}
	return out
}
