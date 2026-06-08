// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// MoveEnumeration reparents an enumeration unit to its (already-updated) target
// container module. Enumerations are top-level units, so this is a unit reparent.
func (b *Backend) MoveEnumeration(enum *model.Enumeration) error {
	if enum == nil {
		return fmt.Errorf("MoveEnumeration: nil enumeration")
	}
	if b.writer == nil {
		return fmt.Errorf("MoveEnumeration: not connected for writing")
	}
	return b.writer.MoveUnit(string(enum.ID), string(enum.ContainerID))
}

// MoveConstant reparents a constant unit to its target container module.
func (b *Backend) MoveConstant(c *model.Constant) error {
	if c == nil {
		return fmt.Errorf("MoveConstant: nil constant")
	}
	if b.writer == nil {
		return fmt.Errorf("MoveConstant: not connected for writing")
	}
	return b.writer.MoveUnit(string(c.ID), string(c.ContainerID))
}

// --- Guarded gaps -----------------------------------------------------------
// These operations are not yet implemented on the codec path. They are guarded
// (per ADR-0005: refuse rather than silently drop) so the modelsdk engine fails
// honestly instead of leaving a half-applied/broken model. Full implementations
// are tracked in docs/plans/2026-06-05-adopt-modelsdk-engine.md.

const errModelSDKUnsupported = "%s: not yet supported by the modelsdk engine (needs %s) — use the legacy engine"

// MoveEntity is cross-domain-model: it removes the entity from the source DM,
// adds it to the target DM, and converts dangling associations to cross-module
// associations. Pending CreateCrossAssociation + reference rewrites.
func (b *Backend) MoveEntity(entity *domainmodel.Entity, sourceDMID, targetDMID model.ID, sourceModuleName, targetModuleName string) ([]string, error) {
	return nil, fmt.Errorf(errModelSDKUnsupported, "MoveEntity", "cross-DM move + cross-association conversion")
}

// CreateCrossAssociation creates a cross-module association (ParentID by-id +
// remote child by qualified name + delete behaviors). Pending the converter.
func (b *Backend) CreateCrossAssociation(domainModelID model.ID, ca *domainmodel.CrossModuleAssociation) error {
	return fmt.Errorf(errModelSDKUnsupported, "CreateCrossAssociation", "cross-association converter")
}

// CreateViewEntitySourceDocument creates the OQL source document that backs a
// view entity. Pending the source-document converter + entity view-source wiring.
func (b *Backend) CreateViewEntitySourceDocument(moduleID model.ID, moduleName, docName, oqlQuery, documentation string) (model.ID, error) {
	return "", fmt.Errorf(errModelSDKUnsupported, "CreateViewEntitySourceDocument", "view-entity OQL source documents")
}
