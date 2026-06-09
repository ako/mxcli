// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
)

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
	setter, ok := el.(interface{ SetAllowedModuleRolesQualifiedNames([]string) })
	if !ok {
		return fmt.Errorf("UpdateAllowedRoles: unit %s (%s) has no allowed-roles list", unitID, el.TypeName())
	}
	setter.SetAllowedModuleRolesQualifiedNames(roles)
	contents, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		return fmt.Errorf("UpdateAllowedRoles: encode unit %s: %w", unitID, err)
	}
	if err := b.writer.UpdateRawUnit(string(unitID), contents); err != nil {
		return fmt.Errorf("UpdateAllowedRoles: update unit %s: %w", unitID, err)
	}
	return nil
}
