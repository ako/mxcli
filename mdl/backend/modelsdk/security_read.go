// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	genSec "github.com/mendixlabs/mxcli/modelsdk/gen/security"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// ListModuleSecurity reads every module's security document (its module roles)
// through the codec engine. Mirrors the legacy reader.ListModuleSecurity.
func (b *Backend) ListModuleSecurity() ([]*security.ModuleSecurity, error) {
	units, err := mprread.ListUnitsWithContainer[*genSec.ModuleSecurity](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*security.ModuleSecurity, 0, len(units))
	for _, u := range units {
		out = append(out, moduleSecurityFromGen(u.Element, u.ContainerID))
	}
	return out, nil
}

// GetModuleSecurity returns the module-security document whose container is
// moduleID (its module roles).
func (b *Backend) GetModuleSecurity(moduleID model.ID) (*security.ModuleSecurity, error) {
	units, err := mprread.ListUnitsWithContainer[*genSec.ModuleSecurity](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if u.ContainerID == moduleID {
			return moduleSecurityFromGen(u.Element, u.ContainerID), nil
		}
	}
	return nil, fmt.Errorf("module security not found for module: %s", moduleID)
}

// moduleSecurityFromGen converts a gen ModuleSecurity element to the semantic type.
func moduleSecurityFromGen(ms *genSec.ModuleSecurity, containerID model.ID) *security.ModuleSecurity {
	out := &security.ModuleSecurity{ContainerID: containerID}
	out.ID = model.ID(ms.ID())
	for _, el := range ms.ModuleRolesItems() {
		r, ok := el.(*genSec.ModuleRole)
		if !ok {
			continue
		}
		mr := &security.ModuleRole{Name: r.Name(), Description: r.Description()}
		mr.ID = model.ID(r.ID())
		out.ModuleRoles = append(out.ModuleRoles, mr)
	}
	return out
}
