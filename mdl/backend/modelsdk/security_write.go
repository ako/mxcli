// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genSec "github.com/mendixlabs/mxcli/modelsdk/gen/security"
)

// loadModuleSecurityGen decodes a Security$ModuleSecurity unit by ID.
func (b *Backend) loadModuleSecurityGen(unitID model.ID) (*genSec.ModuleSecurity, error) {
	raw, err := b.reader.GetRawUnitBytes(string(unitID))
	if err != nil {
		return nil, fmt.Errorf("read module security unit %s: %w", unitID, err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("decode module security unit %s: %w", unitID, err)
	}
	ms, ok := el.(*genSec.ModuleSecurity)
	if !ok {
		return nil, fmt.Errorf("unit %s is not a ModuleSecurity (%s)", unitID, el.TypeName())
	}
	return ms, nil
}

// AddModuleRole adds (or, case-insensitively, updates) a module role on a module's
// Security$ModuleSecurity document. Mirrors legacy: a case-insensitive duplicate is
// overwritten in place (keeping its ID stable so references stay valid).
func (b *Backend) AddModuleRole(unitID model.ID, roleName, description string) error {
	if b.writer == nil {
		return fmt.Errorf("AddModuleRole: not connected for writing")
	}
	ms, err := b.loadModuleSecurityGen(unitID)
	if err != nil {
		return err
	}
	for _, el := range ms.ModuleRolesItems() {
		if r, ok := el.(*genSec.ModuleRole); ok && strings.EqualFold(r.Name(), roleName) {
			r.SetName(roleName)
			r.SetDescription(description)
			return b.persistUnit(unitID, ms)
		}
	}
	r := genSec.NewModuleRole()
	assignID(r)
	r.SetName(roleName)
	r.SetDescription(description)
	ms.AddModuleRoles(r)
	return b.persistUnit(unitID, ms)
}

// RemoveModuleRole removes a module role (by case-insensitive name) from a module's
// Security$ModuleSecurity document.
func (b *Backend) RemoveModuleRole(unitID model.ID, roleName string) error {
	if b.writer == nil {
		return fmt.Errorf("RemoveModuleRole: not connected for writing")
	}
	ms, err := b.loadModuleSecurityGen(unitID)
	if err != nil {
		return err
	}
	for i, el := range ms.ModuleRolesItems() {
		if r, ok := el.(*genSec.ModuleRole); ok && strings.EqualFold(r.Name(), roleName) {
			ms.RemoveModuleRoles(i)
			return b.persistUnit(unitID, ms)
		}
	}
	return nil // not present — nothing to do
}

// persistUnit re-encodes a mutated unit element and writes it back.
func (b *Backend) persistUnit(unitID model.ID, el element.Element) error {
	contents, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		return fmt.Errorf("encode unit %s: %w", unitID, err)
	}
	if err := b.writer.UpdateRawUnit(string(unitID), contents); err != nil {
		return fmt.Errorf("update unit %s: %w", unitID, err)
	}
	return nil
}

// UpdateAllowedRoles sets the AllowedModuleRoles of a document unit (which module
// roles may run a microflow / open a page, etc.) to the given role qualified
// names. It decodes the unit, sets just that property, and re-encodes — the codec
// passes the rest of the document through unchanged, so this is a surgical patch
// regardless of the unit's document type.
func (b *Backend) UpdateAllowedRoles(unitID model.ID, roles []string) error {
	if b.writer == nil {
		return fmt.Errorf("UpdateAllowedRoles: not connected for writing")
	}
	raw, err := b.reader.GetRawUnitBytes(string(unitID))
	if err != nil {
		return fmt.Errorf("UpdateAllowedRoles: read unit %s: %w", unitID, err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		return fmt.Errorf("UpdateAllowedRoles: decode unit %s: %w", unitID, err)
	}
	// Microflows/nanoflows expose SetAllowedModuleRolesQualifiedNames; pages/
	// snippets use SetAllowedRolesQualifiedNames (different gen Go name, same
	// AllowedModuleRoles storage key).
	switch s := el.(type) {
	case interface{ SetAllowedModuleRolesQualifiedNames([]string) }:
		s.SetAllowedModuleRolesQualifiedNames(roles)
	case interface{ SetAllowedRolesQualifiedNames([]string) }:
		s.SetAllowedRolesQualifiedNames(roles)
	default:
		return fmt.Errorf("UpdateAllowedRoles: unit %s (%s) has no allowed-roles list", unitID, el.TypeName())
	}
	contents, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		return fmt.Errorf("UpdateAllowedRoles: encode unit %s: %w", unitID, err)
	}
	if err := b.writer.UpdateRawUnit(string(unitID), contents); err != nil {
		return fmt.Errorf("UpdateAllowedRoles: update unit %s: %w", unitID, err)
	}
	return nil
}
