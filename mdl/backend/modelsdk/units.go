// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// ListUnits and ListFolders expose the container tree. The executor's
// ContainerHierarchy (FindModuleID / BuildFolderPath) is built from
// ListModules + ListUnits + ListFolders, so renderers that resolve a
// document's module/folder from its ContainerID (e.g. SHOW MICROFLOWS, where
// flows are nested in folders) need these — without them the rows are silently
// dropped because folder→module resolution fails.

func (b *Backend) ListUnits() ([]*types.UnitInfo, error) {
	us, err := b.reader.ListUnits()
	if err != nil {
		return nil, err
	}
	out := make([]*types.UnitInfo, 0, len(us))
	for _, u := range us {
		out = append(out, &types.UnitInfo{
			ID:              model.ID(u.ID),
			ContainerID:     model.ID(u.ContainerID),
			ContainmentName: u.ContainmentName,
			Type:            u.Type,
		})
	}
	return out, nil
}

func (b *Backend) ListFolders() ([]*types.FolderInfo, error) {
	fs, err := b.reader.ListFolders()
	if err != nil {
		return nil, err
	}
	out := make([]*types.FolderInfo, 0, len(fs))
	for _, f := range fs {
		out = append(out, &types.FolderInfo{
			ID:          model.ID(f.ID),
			ContainerID: model.ID(f.ContainerID),
			Name:        f.Name,
		})
	}
	return out, nil
}
