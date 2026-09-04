// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/types"
)

func newTestCatalog(t *testing.T, inserts ...string) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cat.Close() })
	for _, ins := range inserts {
		if _, err := cat.Query(ins); err != nil {
			t.Fatalf("%s: %v", ins, err)
		}
	}
	return cat
}

// A MODULE row in the objects view carries an empty ModuleName — it *is* the
// module. Without the special case the misfiling check would compare a module
// anchor's shard against "" and report every one of them as misfiled.
func TestResolverNamesTheModuleForAModuleAnchor(t *testing.T) {
	cat := newTestCatalog(t,
		`INSERT INTO modules_data (Id, Name, QualifiedName) VALUES ('m1', 'Sales', 'Sales')`)
	r := &catalogResolver{cat: cat, be: &mock.MockBackend{}}

	a, err := brain.ParseAnchor("@Sales")
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.Resolve(a)
	if err != nil {
		t.Fatal(err)
	}
	if res.State != brain.Resolved {
		t.Fatalf("state = %v, want resolved", res.State)
	}
	if res.Module != "Sales" {
		t.Errorf("module = %q, want Sales — a module anchor must report its own name", res.Module)
	}
	if brain.MisfiledIn("Sales", []string{res.Module}) {
		t.Error("a module anchor must not misfile its own shard")
	}
}

// The fallback is what separates "gone" from "of a type the index does not
// cover". Both directions are asserted, because a fallback that always says
// NotIndexable would hide real staleness just as badly.
func TestResolverSeparatesNotIndexableFromNotFound(t *testing.T) {
	cat := newTestCatalog(t,
		`INSERT INTO modules_data (Id, Name, QualifiedName) VALUES ('m1', 'Sales', 'Sales')`)

	present := &mock.MockBackend{
		FindDocumentUnitFunc: func(moduleName, name string) (*types.DocumentUnit, error) {
			return &types.DocumentUnit{Name: name, Kind: "scheduled event"}, nil
		},
	}
	absent := &mock.MockBackend{
		FindDocumentUnitFunc: func(moduleName, name string) (*types.DocumentUnit, error) {
			return nil, nil
		},
	}
	a, err := brain.ParseAnchor("@Sales.NightlyRun")
	if err != nil {
		t.Fatal(err)
	}

	res, err := (&catalogResolver{cat: cat, be: present}).Resolve(a)
	if err != nil {
		t.Fatal(err)
	}
	if res.State != brain.NotIndexable {
		t.Errorf("a document the catalog does not index must be NotIndexable, got %v", res.State)
	}
	if res.Kind != "scheduled event" {
		t.Errorf("kind = %q, want the unit's own kind", res.Kind)
	}

	res, err = (&catalogResolver{cat: cat, be: absent}).Resolve(a)
	if err != nil {
		t.Fatal(err)
	}
	if res.State != brain.NotFound {
		t.Errorf("a document that is not there at all must be NotFound, got %v", res.State)
	}
}

// A module anchor never takes the fallback: modules are always indexed, so a
// miss is a miss. Asserted because the fallback would otherwise report a
// deleted module as merely "not indexable" and never fail.
func TestMissingModuleIsNotFoundNotNotIndexable(t *testing.T) {
	cat := newTestCatalog(t)
	be := &mock.MockBackend{
		FindDocumentUnitFunc: func(moduleName, name string) (*types.DocumentUnit, error) {
			t.Fatal("a module anchor must not reach the document fallback")
			return nil, nil
		},
	}
	a, _ := brain.ParseAnchor("@Gone")
	res, err := (&catalogResolver{cat: cat, be: be}).Resolve(a)
	if err != nil {
		t.Fatal(err)
	}
	if res.State != brain.NotFound {
		t.Errorf("state = %v, want not found", res.State)
	}
}

func TestChangedShardsMapsPathsToShards(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")

	store := brain.NewStore(dir)
	if _, err := store.Init(); err != nil {
		t.Fatal(err)
	}
	for _, shard := range []string{"Sales", "Finance"} {
		e, err := brain.NewEntry("An entry", []string{"@" + shard + ".Thing"}, day())
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Promote(e, shard); err != nil {
			t.Fatal(err)
		}
	}
	// A plan slice too: its shard carries the plan/ prefix, so mapping a path
	// by basename alone made every edited slice invisible to --changed. The
	// first version of this test used only module shards and missed it.
	req, err := brain.NewRequirement("A requirement", []string{"@Sales.Thing"}, "01-accounts", day())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Promote(req, req.Shard()); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "seed")

	all := []string{brain.ProjectShard, "Finance", "Sales", brain.PlanShard("01-accounts")}

	// Control: a clean tree changes nothing, so nothing is checked.
	got, err := changedShards(dir, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("clean tree selected %v", got)
	}

	// Touch exactly one shard; only it should come back.
	path := filepath.Join(dir, "docs", "brain", "modules", "Sales.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, []byte("\nedited\n")...), 0644); err != nil {
		t.Fatal(err)
	}
	got, err = changedShards(dir, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "Sales" {
		t.Fatalf("got %v, want [Sales]", got)
	}

	// The same for a plan slice, which is the case the basename mapping broke.
	planPath := filepath.Join(dir, "docs", "brain", "plan", "01-accounts.md")
	pb, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, append(pb, []byte("\nedited\n")...), 0644); err != nil {
		t.Fatal(err)
	}
	got, err = changedShards(dir, all)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "Sales" || got[1] != brain.PlanShard("01-accounts") {
		t.Fatalf("got %v, want [Sales plan/01-accounts]", got)
	}
}

func day() time.Time { return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC) }

// The store's project.md is documented as loaded every session, and its cap —
// the tightest in the store — is justified by exactly that. Routing to it
// through the skill alone does not make it true: a skill is triggered by
// symptom, so a session that never hits the symptom never reads the project's
// decisions. The generated CLAUDE.md is the only thing that makes the claim
// mechanical, which is why it is asserted here rather than left to review.
func TestGeneratedClaudeMDRoutesToTheBrainAtSessionStart(t *testing.T) {
	md := generateClaudeMD("Demo", "Demo.mpr")
	for _, want := range []string{
		"docs/brain/project.md",       // the unconditional read
		"docs/brain/modules/<Module>", // the on-demand shards
		"brain plan",                  // how to pick work up
	} {
		if !strings.Contains(md, want) {
			t.Errorf("generated CLAUDE.md does not mention %q — project.md is then loaded only when a symptom happens to trigger the skill, and the cap that assumes otherwise is unfounded", want)
		}
	}
	// It has to come before the bulk of the file, or "read this first" is a
	// claim the document's own ordering contradicts.
	if i := strings.Index(md, "docs/brain/project.md"); i < 0 || i > len(md)/3 {
		t.Errorf("the brain section is at byte %d of %d; it is meant to be read first", i, len(md))
	}
}
