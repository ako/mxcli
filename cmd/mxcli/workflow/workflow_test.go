// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const vibeJSON = `{
  "id": "vibe", "title": "Vibe code", "summary": "Small changes, quality kept",
  "command": "/workflow-vibe", "skill": "workflow-vibe",
  "gates": [
    {"id": "intent", "title": "Intent", "kind": "intent", "blocking": true},
    {"id": "security", "title": "Security", "kind": "lint", "blocking": true, "rules": ["SEC", "MPR007"], "args": ["lint", "-p", "{mpr}", "--format", "json"]},
    {"id": "tests", "title": "Tests", "kind": "command", "blocking": true, "requiresPath": "tests", "args": ["test", "tests/", "-p", "{mpr}"]},
    {"id": "quality", "title": "Quality", "kind": "score", "args": ["report", "-p", "{mpr}", "--format", "json"]},
    {"id": "note", "title": "Change note", "kind": "note", "blocking": true}
  ]
}`

func defs(t *testing.T) []Definition {
	t.Helper()
	d, err := LoadDefinitions(fstest.MapFS{"skills/workflow-vibe/workflow.json": {Data: []byte(vibeJSON)}}, "skills")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// fakeRunner answers mxcli invocations by their first argument.
type fakeRunner struct {
	exit  map[string]int
	out   map[string]string
	calls [][]string
}

func (f *fakeRunner) Run(args []string) (int, string, string, error) {
	f.calls = append(f.calls, args)
	return f.exit[args[0]], f.out[args[0]], "", nil
}

func TestLoadDefinitions_ReadsWorkflowFilesAndLetsAProjectOverride(t *testing.T) {
	fsys := fstest.MapFS{
		"embedded/workflow-vibe/workflow.json": {Data: []byte(vibeJSON)},
		"embedded/other-skill/SKILL.md":        {Data: []byte("---\nname: other-skill\n---\n")},
		"project/workflow-vibe/workflow.json":  {Data: []byte(strings.Replace(vibeJSON, `"Vibe code"`, `"Our vibe"`, 1))},
	}
	d, err := LoadDefinitions(fsys, "embedded", "project")
	if err != nil {
		t.Fatal(err)
	}
	if len(d) != 1 || d[0].Title != "Our vibe" || len(d[0].Gates) != 5 {
		t.Fatalf("want one definition, the project's, with 5 gates; got %+v", d)
	}
}

func TestEvaluate_GatesDecideFromCommandsAndTheRecord(t *testing.T) {
	dir := t.TempDir()
	run := &Run{Workflow: "vibe", Intent: "add an order status"}
	r := &fakeRunner{
		out: map[string]string{"report": `{"overallScore": 91.5}`, "lint": lintAfter},
	}
	base := 95.0
	run.BaselineScore = &base
	res := Evaluate(defs(t)[0], run, r, "app.mpr", dir)

	got := map[string]string{}
	for _, g := range res {
		got[g.Gate] = g.Status
	}
	want := map[string]string{
		"intent":   StatusPass,
		"security": StatusFail, // SEC001 findings and no baseline: all of them are new
		"tests":    StatusFail, // no tests/ directory
		"quality":  StatusWarn, // 91.5 < 95 baseline, and the gate does not block
		"note":     StatusFail,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("gate %s = %q, want %q", k, got[k], v)
		}
	}
	// {mpr} is substituted, and tests/ is not run when it does not exist.
	for _, c := range r.calls {
		if c[0] == "test" {
			t.Error("the tests gate ran mxcli test with no tests/ directory")
		}
		for _, a := range c {
			if a == "{mpr}" {
				t.Errorf("{mpr} not substituted in %v", c)
			}
		}
	}
}

func TestEvaluate_AllPassingGatesLetARunClose(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := &Run{Workflow: "vibe", Intent: "x", Note: "added Order.Status, granted to User, tested"}
	r := &fakeRunner{out: map[string]string{"report": `{"overallScore": 96}`, "lint": `{"violations":[]}`}}
	base := 95.0
	run.BaselineScore = &base
	res := Evaluate(defs(t)[0], run, r, "app.mpr", dir)
	run.Checks = append(run.Checks, Check{Results: res})
	if blocked := BlockingFailures(defs(t)[0], run); len(blocked) != 0 {
		t.Errorf("want no blocking failures, got %v", blocked)
	}
}

func TestBlockingFailures_NeedsACheckFirst(t *testing.T) {
	run := &Run{Workflow: "vibe", Intent: "x", Note: "y"}
	if blocked := BlockingFailures(defs(t)[0], run); len(blocked) == 0 {
		t.Error("a run that was never checked must not close")
	}
}

func TestStore_RoundTripsRunsOneFileEach(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	a, err := s.Create("vibe", "Add an order status", time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != "2026-10-13-vibe-add-an-order-status" {
		t.Errorf("id = %q", a.ID)
	}
	a.Note = "done"
	if err := s.Save(a); err != nil {
		t.Fatal(err)
	}
	open, err := s.Open()
	if err != nil || len(open) != 1 || open[0].Note != "done" {
		t.Fatalf("Open() = %+v, %v", open, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "brain", "workflows", a.ID+".json")); err != nil {
		t.Errorf("the run is not one file under docs/brain/workflows: %v", err)
	}
}

// A docs/brain/ that mxcli did not write is someone else's notes: the store
// refuses rather than add a folder to it (the brain's own rule).
func TestStore_RefusesAForeignBrain(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs", "brain"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "brain", "README.md"), []byte("# our notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(dir).Create("vibe", "x", time.Now()); err == nil {
		t.Error("wrote a run into a docs/brain/ mxcli did not create")
	}
}

const lintBefore = `{"violations":[
  {"ruleId":"SEC001","severity":"warning","message":"Persistent entity 'CxLayout.AuthToken' has no access rules","module":"CxLayout","document":"AuthToken"},
  {"ruleId":"CONV011","severity":"warning","message":"naming","module":"Sales","document":"x"}
]}`

const lintAfter = `{"violations":[
  {"ruleId":"SEC001","severity":"warning","message":"Persistent entity 'CxLayout.AuthToken' has no access rules","module":"CxLayout","document":"AuthToken"},
  {"ruleId":"SEC001","severity":"warning","message":"Persistent entity 'Sales.Order' has no access rules","module":"Sales","document":"Order","suggestion":"GRANT <Role> ON Sales.Order (READ *)"},
  {"ruleId":"CONV011","severity":"warning","message":"naming","module":"Sales","document":"y"}
]}`

// The security gate holds a change to "adds no security finding": findings
// that were there when the run started do not block it (a template module's
// entity with no access rules would otherwise block every run in the project),
// a new one does, and findings of other rules never count.
func TestEvaluate_LintGateBlocksOnlyFindingsTheChangeAdded(t *testing.T) {
	def := defs(t)[0]
	run := &Run{Workflow: "vibe", Intent: "x"}
	run.BaselineFindings = Findings(def, &fakeRunner{out: map[string]string{"lint": lintBefore}}, "app.mpr")
	if len(run.BaselineFindings) != 1 {
		t.Fatalf("baseline = %v, want only the SEC001 finding", run.BaselineFindings)
	}

	same := Evaluate(def, run, &fakeRunner{exit: map[string]int{"lint": 0}, out: map[string]string{"lint": lintBefore, "report": `{"overallScore":1}`}}, "app.mpr", t.TempDir())
	if status(same, "security") != StatusPass {
		t.Errorf("pre-existing finding blocked the gate: %+v", same)
	}

	added := Evaluate(def, run, &fakeRunner{out: map[string]string{"lint": lintAfter, "report": `{"overallScore":1}`}}, "app.mpr", t.TempDir())
	if status(added, "security") != StatusFail {
		t.Errorf("a new SEC001 finding did not block: %+v", added)
	}
	if d := detail(added, "security"); !strings.Contains(d, "Sales.Order") || strings.Contains(d, "CxLayout") {
		t.Errorf("detail should name the new finding only, got %q", d)
	}
}

func status(res []GateResult, gate string) string {
	for _, r := range res {
		if r.Gate == gate {
			return r.Status
		}
	}
	return ""
}

func detail(res []GateResult, gate string) string {
	for _, r := range res {
		if r.Gate == gate {
			return r.Detail
		}
	}
	return ""
}
