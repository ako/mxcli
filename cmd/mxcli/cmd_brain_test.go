// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
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
	run("add", "-A")
	run("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "seed")

	all := []string{brain.ProjectShard, "Finance", "Sales"}

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
}

func day() time.Time { return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC) }
