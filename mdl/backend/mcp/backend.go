// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/mpr"
)

// Backend executes domain-model writes against a live Studio Pro via its MCP
// ("PED") server, while serving reads from the local mounted .mpr file.
//
// It is a hybrid: PED has no way to enumerate modules or run the catalog, so
// reads (ListModules, GetDomainModel, SHOW/DESCRIBE) come from a read-only
// local reader, and writes (CreateEntity, …) go over MCP so Studio Pro stays
// the authoritative serializer and the project can remain open. See
// docs/11-proposals/PROPOSAL_mcp_backend.md.
//
// The embedded unsupportedBackend makes every FullBackend method error by
// default; Backend overrides only the operations in the entity slice. This is
// deliberate: an un-overridden write must NOT fall through to the local .mpr
// (that would edit the file Studio Pro holds in memory) — it errors instead.
type Backend struct {
	unsupportedBackend

	mcpURL string
	dial   string

	client *Client
	reader *mpr.Reader
	path   string
	server ServerInfo

	// schemaFetched records element types already fetched via ped_get_schema
	// this session (the contract asks for a schema fetch before create/add).
	schemaFetched map[string]bool
}

// compile-time guarantee that Backend (and its embedded base) satisfies the
// full backend contract.
var _ backend.FullBackend = (*Backend)(nil)

// New returns an unconnected MCP backend that will issue model writes to the
// MCP server at mcpURL. dial optionally overrides the TCP address actually
// connected to (empty = derive from the URL; localhost maps to
// host.docker.internal from a devcontainer). Call Connect with the local .mpr
// path to open the read side.
func New(mcpURL, dial string) *Backend {
	return &Backend{mcpURL: mcpURL, dial: dial, schemaFetched: map[string]bool{}}
}

// Connect opens the local .mpr read-only (for reads/enumeration) and completes
// the MCP handshake with Studio Pro (for writes).
func (b *Backend) Connect(path string) error {
	r, err := mpr.Open(path) // read-only: never lock the file Studio Pro owns
	if err != nil {
		return fmt.Errorf("open local project %q: %w", path, err)
	}
	c, err := NewClient(ClientOptions{URL: b.mcpURL, Dial: b.dial})
	if err != nil {
		r.Close()
		return err
	}
	si, err := c.Initialize()
	if err != nil {
		r.Close()
		return fmt.Errorf("connect to MCP server %q: %w", b.mcpURL, err)
	}
	b.reader = r
	b.client = c
	b.server = si
	b.path = path
	return nil
}

// ServerInfo returns the connected MCP server identity (after Connect).
func (b *Backend) ServerInfo() ServerInfo { return b.server }

// ---------------------------------------------------------------------------
// ConnectionBackend
// ---------------------------------------------------------------------------

func (b *Backend) Disconnect() error {
	if b.reader != nil {
		err := b.reader.Close()
		b.reader = nil
		b.client = nil
		b.path = ""
		return err
	}
	return nil
}

// Commit is a no-op: PED applies each operation to Studio Pro's in-memory model
// immediately. There is no MCP flush-to-disk tool — the user saves in Studio
// Pro. See the "consistency hole" section of the proposal.
func (b *Backend) Commit() error { return nil }

func (b *Backend) IsConnected() bool { return b.client != nil && b.reader != nil }
func (b *Backend) Path() string      { return b.path }

func (b *Backend) Version() types.MPRVersion             { return types.MPRVersion(b.reader.Version()) }
func (b *Backend) ProjectVersion() *types.ProjectVersion { return b.reader.ProjectVersion() }
func (b *Backend) GetMendixVersion() (string, error)     { return b.reader.GetMendixVersion() }

// ---------------------------------------------------------------------------
// Reads — delegated to the local read-only reader (hybrid model).
//
// Caveat: these reflect the last-saved on-disk state. Edits applied via MCP
// this session that Studio Pro keeps in memory are not visible here until the
// user saves (the "consistency hole"). For the entity slice this is acceptable;
// a dirty-set read router is future work.
// ---------------------------------------------------------------------------

func (b *Backend) ListModules() ([]*model.Module, error)        { return b.reader.ListModules() }
func (b *Backend) GetModule(id model.ID) (*model.Module, error) { return b.reader.GetModule(id) }
func (b *Backend) GetModuleByName(name string) (*model.Module, error) {
	return b.reader.GetModuleByName(name)
}

func (b *Backend) ListDomainModels() ([]*domainmodel.DomainModel, error) {
	return b.reader.ListDomainModels()
}
func (b *Backend) GetDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error) {
	return b.reader.GetDomainModel(moduleID)
}
func (b *Backend) GetDomainModelByID(id model.ID) (*domainmodel.DomainModel, error) {
	return b.reader.GetDomainModelByID(id)
}

// ReconcileMemberAccesses is a no-op for the MCP backend. It is the executor's
// finalize-time sync that keeps entity/association member-access rules
// consistent on disk. Studio Pro maintains that consistency itself for PED
// edits, and this slice never pushes access rules over PED, so there is
// nothing to reconcile.
func (b *Backend) ReconcileMemberAccesses(_ model.ID, _ string) (int, error) {
	return 0, nil
}
