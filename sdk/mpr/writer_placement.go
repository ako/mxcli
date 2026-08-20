// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// MoveDocument reparents a top-level document unit, whatever its type.
// The idempotence and write accounting live in moveUnitByID.
func (w *Writer) MoveDocument(unitID, containerID model.ID) error {
	if unitID == "" || containerID == "" {
		return fmt.Errorf("MoveDocument: unit and container are both required")
	}
	return w.moveUnitByID(string(unitID), string(containerID))
}

// FindDocumentUnit locates a document by module and name through the unit
// table, whatever its type. Mirrors the modelsdk engine's implementation.
//
// Only units contained as "Documents" are considered: a module also holds its
// domain model, security and settings, and folders share the table, so the
// containment filter is what stops a folder named like a document from being
// returned as one.
func (w *Writer) FindDocumentUnit(moduleName, name string) (*types.DocumentUnit, error) {
	modules, err := w.reader.ListModules()
	if err != nil {
		return nil, fmt.Errorf("FindDocumentUnit: list modules: %w", err)
	}
	var moduleID string
	for _, m := range modules {
		if m.Name == moduleName {
			moduleID = string(m.ID)
			break
		}
	}
	if moduleID == "" {
		return nil, nil
	}
	containers := buildContainerSet(w.reader, moduleID)

	units, err := w.reader.listUnitsByType("")
	if err != nil {
		return nil, fmt.Errorf("FindDocumentUnit: list units: %w", err)
	}
	for _, unit := range units {
		if unit.ContainmentName != "Documents" || !containers[unit.ContainerID] {
			continue
		}
		contents, err := w.reader.resolveContents(unit.ID, unit.Contents)
		if err != nil || len(contents) == 0 {
			continue
		}
		var raw bson.D
		if err := bson.Unmarshal(contents, &raw); err != nil {
			continue
		}
		matched := false
		for _, elem := range raw {
			if elem.Key == "Name" {
				if s, ok := elem.Value.(string); ok && s == name {
					matched = true
				}
				break
			}
		}
		if !matched {
			continue
		}
		return &types.DocumentUnit{
			ID:          model.ID(unit.ID),
			ContainerID: model.ID(unit.ContainerID),
			Name:        name,
			Type:        unit.Type,
			Kind:        types.DocumentKind(unit.Type),
		}, nil
	}
	return nil, nil
}
