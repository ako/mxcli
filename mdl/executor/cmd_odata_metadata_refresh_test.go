// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// edmxWith renders a minimal EDMX document carrying the named entity types, so
// a test can change the contract the way a backend release does — by adding an
// entity set — rather than by editing an opaque blob.
func edmxWith(names ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><edmx:Edmx xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx" Version="4.0"><edmx:DataServices><Schema xmlns="http://docs.oasis-open.org/odata/ns/edm" Namespace="Test">`)
	for _, n := range names {
		b.WriteString(`<EntityType Name="` + n + `"><Key><PropertyRef Name="ID"/></Key><Property Name="ID" Type="Edm.Int32"/></EntityType>`)
	}
	b.WriteString(`</Schema></edmx:DataServices></edmx:Edmx>`)
	return b.String()
}

// odataModifyFixture wires a mock project holding one OData client whose cached
// contract is whatever the file at contractPath held when the fixture was built
// — i.e. the state CREATE ODATA CLIENT leaves behind. The returned pointer is
// the service the executor mutates; updated records what reached the backend.
type odataModifyFixture struct {
	ctx        *ExecContext
	out        *strings.Builder
	svc        *model.ConsumedODataService
	seedHash   string
	seedCached string
	updated    **model.ConsumedODataService
}

// newODataModifyFixture takes the project directory separately from the contract
// path: a relative MetadataUrl resolves against the .mpr, not against wherever
// the contract happens to sit.
func newODataModifyFixture(t *testing.T, projectDir, contractPath string) *odataModifyFixture {
	t.Helper()

	cached, hash, err := fetchODataMetadata("file://"+contractPath, nil)
	if err != nil {
		t.Fatalf("seeding the cached contract failed: %v", err)
	}

	mod := mkModule("MyModule")
	svc := &model.ConsumedODataService{
		BaseElement:  model.BaseElement{ID: nextID("cos")},
		ContainerID:  mod.ID,
		Name:         "NowApi",
		ODataVersion: "4.0",
		MetadataUrl:  "file://" + contractPath,
		Metadata:     cached,
		MetadataHash: hash,
		Validated:    true,
	}
	h := mkHierarchy(mod)
	withContainer(h, svc.ContainerID, mod.ID)

	var updated *model.ConsumedODataService
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{mod}, nil
		},
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
			return []*model.ConsumedODataService{svc}, nil
		},
		UpdateConsumedODataServiceFunc: func(s *model.ConsumedODataService) error {
			updated = s
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h),
		withMprPath(filepath.Join(projectDir, "app.mpr")))

	var sb strings.Builder
	ctx.Output = &sb

	return &odataModifyFixture{
		ctx: ctx, out: &sb, svc: svc,
		seedHash: hash, seedCached: cached, updated: &updated,
	}
}

func (f *odataModifyFixture) modify(t *testing.T, metadataUrl string) {
	t.Helper()
	stmt := &ast.CreateODataClientStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "NowApi"},
		CreateOrModify: true,
		MetadataUrl:    metadataUrl,
	}
	if err := createODataClient(f.ctx, stmt); err != nil {
		t.Fatalf("create or modify odata client: %v", err)
	}
	if *f.updated == nil {
		t.Fatal("the modify branch never reached the backend")
	}
}

// mxcli-formula1: CREATE OR REPLACE / CREATE OR MODIFY updated every property of
// an OData client except the cached contract, which stayed at the snapshot taken
// when the client was created. A consumed service that gained entity sets was
// therefore invisible: the refreshed contract file on disk had five, SHOW
// CONTRACT ENTITIES kept showing three, the statement reported "Unchanged", and
// the CREATE EXTERNAL ENTITIES that followed imported the old shape without
// saying so. The only way out was DROP + recreate, which invalidates the client
// ID the existing external entities point at.
func TestCreateOrModifyODataClient_RefreshesCachedContract(t *testing.T) {
	dir := t.TempDir()
	contract := filepath.Join(dir, "f1-now-metadata.xml")
	if err := os.WriteFile(contract, []byte(edmxWith("Session", "Driver", "Order")), 0o600); err != nil {
		t.Fatal(err)
	}

	f := newODataModifyFixture(t, dir, contract)

	// The backend gains two entity sets and the contract file is refreshed.
	if err := os.WriteFile(contract, []byte(edmxWith("Session", "Driver", "Order", "Trace", "Stints")), 0o600); err != nil {
		t.Fatal(err)
	}

	f.modify(t, contract)

	got := *f.updated
	for _, want := range []string{"Trace", "Stints"} {
		if !strings.Contains(got.Metadata, `Name="`+want+`"`) {
			t.Errorf("cached contract is missing entity type %q — it is still the snapshot from creation", want)
		}
	}
	if got.MetadataHash == f.seedHash {
		t.Error("MetadataHash still matches the contract cached at creation")
	}
	if !got.Validated {
		t.Error("Validated should stay true after a successful refresh")
	}
	if out := f.out.String(); !strings.Contains(out, "Refreshed $metadata: 5 entity types") {
		t.Errorf("no refresh reported to the user; output was:\n%s", out)
	}
}

// The control: an unchanged contract must not be rewritten, or every re-run of a
// script would churn the document and the statement would stop reporting
// "Unchanged". Same code path, only the file is left alone.
func TestCreateOrModifyODataClient_UnchangedContractIsNotRewritten(t *testing.T) {
	dir := t.TempDir()
	contract := filepath.Join(dir, "f1-now-metadata.xml")
	if err := os.WriteFile(contract, []byte(edmxWith("Session", "Driver", "Order")), 0o600); err != nil {
		t.Fatal(err)
	}

	f := newODataModifyFixture(t, dir, contract)

	f.modify(t, contract)

	got := *f.updated
	if got.Metadata != f.seedCached || got.MetadataHash != f.seedHash {
		t.Error("an unchanged contract was rewritten, so the write can no longer be elided")
	}
	if out := f.out.String(); strings.Contains(out, "Refreshed $metadata") {
		t.Errorf("a refresh was reported for an unchanged contract:\n%s", out)
	}
}

// A relative MetadataUrl has to be resolved against the project directory on the
// modify path too. It was stored raw, so a client written with './contracts/…'
// ended up with a URL Studio Pro cannot open and that fetchODataMetadata — which
// documents its input as already normalized — could not read back.
func TestCreateOrModifyODataClient_NormalizesRelativeMetadataUrl(t *testing.T) {
	dir := t.TempDir()
	contracts := filepath.Join(dir, "contracts")
	if err := os.MkdirAll(contracts, 0o755); err != nil {
		t.Fatal(err)
	}
	contract := filepath.Join(contracts, "f1-now-metadata.xml")
	if err := os.WriteFile(contract, []byte(edmxWith("Session")), 0o600); err != nil {
		t.Fatal(err)
	}

	f := newODataModifyFixture(t, dir, contract)

	if err := os.WriteFile(contract, []byte(edmxWith("Session", "Trace")), 0o600); err != nil {
		t.Fatal(err)
	}

	// The MprPath the fixture set is dir/app.mpr, so this is the spelling a
	// script in the project actually uses.
	f.modify(t, "./contracts/f1-now-metadata.xml")

	got := *f.updated
	if !strings.HasPrefix(got.MetadataUrl, "file://") {
		t.Errorf("MetadataUrl = %q, want an absolute file:// URL", got.MetadataUrl)
	}
	if !strings.Contains(got.Metadata, `Name="Trace"`) {
		t.Error("the relative URL did not resolve to the contract on disk, so nothing was refreshed")
	}
}
