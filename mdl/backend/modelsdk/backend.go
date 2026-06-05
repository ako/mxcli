// SPDX-License-Identifier: Apache-2.0

// Package modelsdkbackend is the modelsdk-engine implementation of
// backend.FullBackend. It lives at a separate import path from the legacy
// mdl/backend/mpr (mprbackend) package so both engines can be linked at once
// and selected via the MXCLI_ENGINE seam (see cmd/mxcli/engine.go).
//
// Phase 1 (docs/plans/2026-06-05-adopt-modelsdk-engine.md) is a READ slice:
// it embeds *mock.MockBackend so the full 27-interface FullBackend surface is
// satisfied, and overrides only the connection + module read methods to drive
// the real modelsdk codec engine. Un-overridden methods fall through to the
// mock stubs (which return zero/nil and never panic). Write methods are NOT
// implemented yet — callers must not rely on them persisting; the CLI prints a
// read-only warning when this engine is selected.
package modelsdkbackend

import (
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// Compile-time guarantee that the read slice still satisfies the whole interface
// (via the embedded mock for everything it doesn't override).
var _ backend.FullBackend = (*Backend)(nil)

// Backend reads a Mendix project through the modelsdk engine.
type Backend struct {
	*mock.MockBackend // stubs for every not-yet-implemented FullBackend method
	reader            *mmpr.Reader
	path              string
}

// New constructs a read-slice backend. The embedded mock is non-nil so the
// promoted stub methods are safe to call before/without overrides.
func New() *Backend {
	return &Backend{MockBackend: &mock.MockBackend{}}
}

// --- ConnectionBackend ---

// Connect opens the project read-only through the modelsdk reader.
func (b *Backend) Connect(path string) error {
	r, err := mmpr.OpenWithOptions(path, mmpr.OpenOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	b.reader = r
	b.path = path
	return nil
}

// Disconnect closes the modelsdk reader.
func (b *Backend) Disconnect() error {
	if b.reader == nil {
		return nil
	}
	err := b.reader.Close()
	b.reader = nil
	return err
}

// Commit is a no-op for the read-only slice.
func (b *Backend) Commit() error { return nil }

func (b *Backend) IsConnected() bool { return b.reader != nil }

func (b *Backend) Path() string { return b.path }

func (b *Backend) Version() types.MPRVersion {
	if b.reader == nil {
		return 0
	}
	return types.MPRVersion(b.reader.Version())
}

func (b *Backend) ProjectVersion() *types.ProjectVersion {
	if b.reader == nil {
		return nil
	}
	pv := b.reader.ProjectVersion()
	if pv == nil {
		return nil
	}
	return &types.ProjectVersion{
		ProductVersion: pv.ProductVersion,
		BuildVersion:   pv.BuildVersion,
		FormatVersion:  pv.FormatVersion,
		SchemaHash:     pv.SchemaHash,
		MajorVersion:   pv.MajorVersion,
		MinorVersion:   pv.MinorVersion,
		PatchVersion:   pv.PatchVersion,
	}
}

func (b *Backend) GetMendixVersion() (string, error) {
	if b.reader == nil {
		return "", nil
	}
	return b.reader.GetMendixVersion()
}

// --- ModuleBackend (read only) ---

func (b *Backend) ListModules() ([]*model.Module, error) {
	infos, err := b.reader.ListModules()
	if err != nil {
		return nil, err
	}
	out := make([]*model.Module, 0, len(infos))
	for _, mi := range infos {
		out = append(out, moduleFromInfo(mi))
	}
	return out, nil
}

func (b *Backend) GetModuleByName(name string) (*model.Module, error) {
	mi, err := b.reader.GetModuleByName(name)
	if err != nil || mi == nil {
		return nil, err
	}
	return moduleFromInfo(mi), nil
}

func (b *Backend) GetModule(id model.ID) (*model.Module, error) {
	mi, err := b.reader.GetModule(string(id))
	if err != nil || mi == nil {
		return nil, err
	}
	return moduleFromInfo(mi), nil
}

// moduleFromInfo converts the modelsdk ModuleInfo (ID + Name) into our
// model.Module. Richer fields (FromAppStore, version, contained documents)
// need a full gen.Module decode and are deferred to a later phase.
func moduleFromInfo(mi *mmpr.ModuleInfo) *model.Module {
	m := &model.Module{Name: mi.Name}
	m.ID = model.ID(mi.ID)
	return m
}
