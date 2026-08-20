// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// MoveDocument reparents a top-level document unit. See
// backend.DocumentPlacementBackend for why this is type-agnostic; the writer
// handles the idempotence the interface requires.
func (b *Backend) MoveDocument(unitID, containerID model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("MoveDocument: not connected for writing")
	}
	if unitID == "" || containerID == "" {
		return fmt.Errorf("MoveDocument: unit and container are both required")
	}
	return b.writer.MoveUnit(string(unitID), string(containerID))
}

// FindDocumentUnit locates a document by module and name through the unit
// table, whatever its type.
//
// Only units contained as "Documents" are considered. A module also holds its
// domain model, its security and its settings, and folders sit in the same
// table — restricting to the document containment is what keeps a folder named
// like a document from being returned as one.
func (b *Backend) FindDocumentUnit(moduleName, name string) (*types.DocumentUnit, error) {
	if b.reader == nil {
		return nil, fmt.Errorf("FindDocumentUnit: not connected")
	}
	modules, err := b.reader.ListModules()
	if err != nil {
		return nil, fmt.Errorf("FindDocumentUnit: list modules: %w", err)
	}
	var moduleID string
	for _, m := range modules {
		if m.Name == moduleName {
			moduleID = m.ID
			break
		}
	}
	if moduleID == "" {
		return nil, nil
	}
	containers := b.containerSetForModule(moduleID)

	units, err := b.reader.ListUnits()
	if err != nil {
		return nil, fmt.Errorf("FindDocumentUnit: list units: %w", err)
	}
	for _, u := range units {
		if u.ContainmentName != "Documents" || !containers[u.ContainerID] {
			continue
		}
		raw, err := b.reader.GetRawUnitBytes(u.ID)
		if err != nil || len(raw) == 0 {
			continue
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			continue
		}
		if docNameOf(doc) != name {
			continue
		}
		return &types.DocumentUnit{
			ID:          model.ID(u.ID),
			ContainerID: model.ID(u.ContainerID),
			Name:        name,
			Type:        u.Type,
			Kind:        types.DocumentKind(u.Type),
		}, nil
	}
	return nil, nil
}
