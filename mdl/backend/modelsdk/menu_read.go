// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	genMenus "github.com/mendixlabs/mxcli/modelsdk/gen/menus"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// ListMenuDocuments reads every standalone Menus$MenuDocument unit.
//
// A menu document's entries are the same Menus$MenuItem elements a navigation
// profile holds, so the recursive conversion reuses navMenuItemFromGen — which
// already handles the three icon variants and the client-action dispatch. The
// only structural difference is that a menu document wraps its entries in a
// Menus$MenuItemCollection rather than holding them directly.
func (b *Backend) ListMenuDocuments() ([]*types.MenuDocument, error) {
	if b.reader == nil {
		return nil, fmt.Errorf("ListMenuDocuments: not connected")
	}
	units, err := mprread.ListUnitsWithContainer[*genMenus.MenuDocument](b.reader)
	if err != nil {
		return nil, err
	}

	out := make([]*types.MenuDocument, 0, len(units))
	for _, u := range units {
		g := u.Element
		md := &types.MenuDocument{
			ID:            model.ID(g.ID()),
			ContainerID:   model.ID(u.ContainerID),
			Name:          g.Name(),
			Documentation: g.Documentation(),
			ExportLevel:   g.ExportLevel(),
			Excluded:      g.Excluded(),
		}
		if coll, ok := g.ItemCollection().(*genMenus.MenuItemCollection); ok && coll != nil {
			for _, el := range coll.ItemsItems() {
				if item := navMenuItemFromGen(el); item != nil {
					md.Items = append(md.Items, item)
				}
			}
		}
		out = append(out, md)
	}
	return out, nil
}

// GetMenuDocumentByQualifiedName finds a menu document by module + name.
func (b *Backend) GetMenuDocumentByQualifiedName(moduleName, name string) (*types.MenuDocument, error) {
	all, err := b.ListMenuDocuments()
	if err != nil {
		return nil, err
	}
	for _, md := range all {
		if md.Name == name && b.moduleNameFor(md.ID) == moduleName {
			return md, nil
		}
	}
	return nil, fmt.Errorf("menu not found: %s.%s", moduleName, name)
}
