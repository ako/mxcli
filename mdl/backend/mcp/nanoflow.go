// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Nanoflows cannot be *created* over MCP: PED's ped_create_document rejects the
// document type outright ("Document type 'Microflows$Nanoflow' cannot be created.
// Did you mean: Microflows$Microflow?") — its create whitelist covers microflows
// but not nanoflows, despite the two sharing a structure. CREATE / CREATE OR MODIFY
// are therefore rejected with an actionable error. DROP of an existing nanoflow
// still works via Concord's delete_document, like microflows; reads delegate to the
// local .mpr.

// CreateNanoflow is rejected — PED won't create Microflows$Nanoflow documents.
func (b *Backend) CreateNanoflow(nf *microflows.Nanoflow) error {
	return errNanoflowCreate(nf.Name)
}

// UpdateNanoflow (CREATE OR MODIFY) is rejected for the same reason.
func (b *Backend) UpdateNanoflow(nf *microflows.Nanoflow) error {
	return errNanoflowCreate(nf.Name)
}

func errNanoflowCreate(name string) error {
	return fmt.Errorf("nanoflow %q cannot be created via the MCP backend — Studio Pro's MCP server refuses Microflows$Nanoflow in ped_create_document (its create whitelist covers microflows but not nanoflows); author it against a local .mpr or in Studio Pro", name)
}

// DeleteNanoflow drops an existing nanoflow via Concord's delete_document (PED has
// no delete tool), like microflows. Requires --mcp-concord.
func (b *Backend) DeleteNanoflow(id model.ID) error {
	nf, err := b.GetNanoflow(id)
	if err != nil {
		return fmt.Errorf("resolve nanoflow %s for DROP: %w", id, err)
	}
	modName, err := b.moduleNameForContainer(nf.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve module for nanoflow %q: %w", nf.Name, err)
	}
	return b.concordDeleteDocument(modName, nf.Name)
}

// Nanoflow reads delegate to the local reader (nanoflows can't be created over MCP,
// so there is nothing session-local to merge).
func (b *Backend) ListNanoflows() ([]*microflows.Nanoflow, error) { return b.reader.ListNanoflows() }

func (b *Backend) GetNanoflow(id model.ID) (*microflows.Nanoflow, error) {
	return b.reader.GetNanoflow(id)
}
