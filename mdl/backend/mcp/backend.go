// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"os"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
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

	// concord is an optional second MCP client to the Concord extension server,
	// used ONLY for capabilities the built-in PED server lacks (delete, save,
	// validate, run). nil unless configured via WithConcord. PED stays the
	// authoring path; Concord is the gap-filler.
	concordURL  string
	concordDial string
	concord     *Client
	// saveOnExit flushes Studio Pro (Concord save_all) on Disconnect — PED has no
	// save tool, so this is how `mxcli --mcp ... --mcp-save` persists writes.
	saveOnExit bool
	// checkOnExit runs Concord's check_model on Disconnect and prints the report
	// (`--mcp-check`), so MCP-authored changes are validated.
	checkOnExit bool

	// schemaFetched records element types already fetched via ped_get_schema
	// this session (the contract asks for a schema fetch before create/add).
	schemaFetched map[string]bool

	// dirty holds module names whose live (in-memory) domain model has diverged
	// from the on-disk .mpr because of writes this session. Reads of a dirty
	// module are reconstructed from MCP instead of the stale local reader —
	// this is the dirty-set read router that closes the consistency hole.
	dirty map[string]bool

	// synthetic maps the synthetic IDs handed out by reconstructed reads back to
	// the PED-addressable element name (entity or association name). PED never
	// exposes real $IDs, so reconstructed elements get synthetic IDs; the write
	// helpers resolve those back to names through this map before falling back
	// to the local reader.
	synthetic map[model.ID]string

	// sessionEnums holds enumerations created over MCP this session. They are
	// not on disk yet, so ListEnumerations/GetEnumeration merge them in — this
	// is what lets "create enumeration X; create entity (a: X)" work in one run.
	// Enumerations are create-only via PED (no delete tool), so a registry is
	// enough; no live reconstruction is needed.
	sessionEnums []*model.Enumeration

	// sessionMicroflows holds microflows created over MCP this session, merged
	// into ListMicroflows/GetMicroflow for the same reason as sessionEnums
	// (duplicate detection and create-then-reference within one run).
	sessionMicroflows []*microflows.Microflow

	// sessionPages holds pages created over MCP this session, merged into
	// ListPages (the executor's duplicate/role checks read it).
	sessionPages []*pages.Page

	// customWidgets holds the high-level pg `object` recorded for each pluggable
	// widget built this session, keyed by the CustomWidget's ID. The pluggable
	// widget engine builds widgets via LoadWidgetTemplate → an mcpWidgetBuilder
	// that records semantic property ops here instead of mutating BSON;
	// mapPageWidget then looks the object up to emit CustomWidgets$CustomWidget.
	customWidgets map[model.ID]*mcpCustomWidget

	// sessionModules holds modules created over MCP this session, merged into
	// ListModules/GetModule(ByName) so that a module-dependent op later in the
	// same run (e.g. "create module X; create enumeration X.Y") can resolve the
	// freshly created module, which the local reader does not yet know about.
	sessionModules []*model.Module
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
	return &Backend{
		mcpURL:        mcpURL,
		dial:          dial,
		schemaFetched: map[string]bool{},
		dirty:         map[string]bool{},
		synthetic:     map[model.ID]string{},
	}
}

// WithConcord configures the optional second MCP client to the Concord extension
// server (gap-filler for capabilities PED lacks). saveOnExit makes Disconnect run
// Concord's save_all so PED-authored in-memory writes are persisted. Returns the
// receiver for chaining.
func (b *Backend) WithConcord(url, dial string, saveOnExit, checkOnExit bool) *Backend {
	b.concordURL = url
	b.concordDial = dial
	b.saveOnExit = saveOnExit
	b.checkOnExit = checkOnExit
	return b
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

	// Optional Concord gap-filler client. A failure here is fatal only because the
	// user explicitly asked for it (--mcp-concord); PED authoring already succeeded.
	if b.concordURL != "" {
		cc, err := NewClient(ClientOptions{URL: b.concordURL, Dial: b.concordDial})
		if err != nil {
			return fmt.Errorf("create Concord client: %w", err)
		}
		if _, err := cc.Initialize(); err != nil {
			return fmt.Errorf("connect to Concord MCP server %q: %w", b.concordURL, err)
		}
		b.concord = cc
	}
	return nil
}

// ServerInfo returns the connected MCP server identity (after Connect).
func (b *Backend) ServerInfo() ServerInfo { return b.server }

// ---------------------------------------------------------------------------
// ConnectionBackend
// ---------------------------------------------------------------------------

func (b *Backend) Disconnect() error {
	// Persist PED's in-memory writes via Concord before tearing down (PED has no
	// save tool). A save failure must not be silent — surface it on stderr — but
	// it does not block teardown (the writes still live in Studio Pro's memory).
	if b.saveOnExit && b.concord != nil {
		if err := b.SaveAll(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: --mcp-save failed, changes remain unsaved in Studio Pro: %v\n", err)
		}
	}
	// Validate the (in-memory) model after writes if requested. Diagnostic, so it
	// prints to stderr and never blocks teardown.
	if b.checkOnExit && b.concord != nil {
		if r, err := b.CheckModel(""); err != nil {
			fmt.Fprintf(os.Stderr, "warning: --mcp-check failed: %v\n", err)
		} else {
			writeCheckReport(os.Stderr, r)
		}
	}
	if b.reader != nil {
		err := b.reader.Close()
		b.reader = nil
		b.client = nil
		b.concord = nil
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

func (b *Backend) ListModules() ([]*model.Module, error) {
	local, err := b.reader.ListModules()
	if err != nil {
		return nil, err
	}
	if len(b.sessionModules) == 0 {
		return local, nil
	}
	seen := make(map[string]bool, len(b.sessionModules))
	out := make([]*model.Module, 0, len(local)+len(b.sessionModules))
	for _, m := range b.sessionModules {
		seen[m.Name] = true
		out = append(out, m)
	}
	for _, m := range local {
		if !seen[m.Name] {
			out = append(out, m)
		}
	}
	return out, nil
}

func (b *Backend) GetModule(id model.ID) (*model.Module, error) {
	for _, m := range b.sessionModules {
		if m.ID == id {
			return m, nil
		}
	}
	return b.reader.GetModule(id)
}

func (b *Backend) GetModuleByName(name string) (*model.Module, error) {
	for _, m := range b.sessionModules {
		if m.Name == name {
			return m, nil
		}
	}
	return b.reader.GetModuleByName(name)
}

func (b *Backend) ListDomainModels() ([]*domainmodel.DomainModel, error) {
	return b.reader.ListDomainModels()
}

// GetDomainModel returns a module's domain model. If the module was written
// this session (dirty), it is reconstructed from Studio Pro's live in-memory
// model so in-session edits are visible; otherwise it comes from the local
// reader (last-saved state).
func (b *Backend) GetDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error) {
	mod, err := b.reader.GetModule(moduleID)
	if err == nil && b.dirty[mod.Name] {
		return b.reconstructDomainModel(mod.Name, moduleID)
	}
	return b.reader.GetDomainModel(moduleID)
}

// GetDomainModelByID mirrors GetDomainModel but is keyed by the domain model's
// own ID; it resolves the owning module and applies the same dirty routing.
func (b *Backend) GetDomainModelByID(id model.ID) (*domainmodel.DomainModel, error) {
	localDM, err := b.reader.GetDomainModelByID(id)
	if err != nil {
		return nil, err
	}
	if mod, err := b.reader.GetModule(localDM.ContainerID); err == nil && b.dirty[mod.Name] {
		return b.reconstructDomainModel(mod.Name, localDM.ContainerID)
	}
	return localDM, nil
}

// ReconcileMemberAccesses is a no-op for the MCP backend. It is the executor's
// finalize-time sync that keeps entity/association member-access rules
// consistent on disk. Studio Pro maintains that consistency itself for PED
// edits, and this slice never pushes access rules over PED, so there is
// nothing to reconcile.
func (b *Backend) ReconcileMemberAccesses(_ model.ID, _ string) (int, error) {
	return 0, nil
}
