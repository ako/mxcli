// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// checkDocumentAccessRolesSameModule rejects granting a document (page,
// microflow, nanoflow, or a published OData/REST service) access to a module role
// from a different module than the document. Mendix stores document access as
// references to the document's OWN module roles only — Studio Pro's role picker
// only offers the containing module's roles — so a cross-module reference builds
// with CE0148 ("reselect roles") even though it is otherwise well-formed. Without
// this guard the grant writes an invalid reference that passes mxcli but fails the
// Mendix build with an opaque error. The MOVE path enforces the same rule by
// remapping to same-named target-module roles (see remapDocumentAccessRoles); on
// an explicit GRANT we reject instead, so a wrong role or wrong document is not
// silently substituted.
//
// docKind/docName are used only for the message; docModule is the document's module.
func checkDocumentAccessRolesSameModule(docKind, docModule, docName string, roles []ast.QualifiedName) error {
	for _, role := range roles {
		if role.Module != docModule {
			return mdlerrors.NewValidationf(
				"cannot grant %s %s.%s access to %s.%s: a %s can only reference module roles from its own module %q "+
					"(Mendix rejects a cross-module document role with CE0148 \"reselect roles\"). "+
					"Grant a %s module role instead — e.g. %s.%s if it exists — and map the user role to it.",
				docKind, docModule, docName, role.Module, role.Name,
				docKind, docModule,
				docModule, docModule, role.Name)
		}
	}
	return nil
}

const (
	autoDocumentRoleName        = "User"
	autoDocumentRoleDescription = "Auto-created default role for mxcli document access"
)

// defaultDocumentAccessRoles returns a conservative fallback role set for newly
// created pages/microflows when the target module has no module roles at all.
//
// Mendix accepts document access only when it references a role from the same
// module; using an existing role from another module causes CE0148 on freshly
// created documents. To keep mx-check green, auto-create a local `User` module
// role only for modules that currently have zero roles. Modules that already
// manage their own roles keep the existing "no access by default" behavior.
func defaultDocumentAccessRoles(ctx *ExecContext, module *model.Module) []model.ID {
	if module == nil {
		return nil
	}

	ms, err := ctx.Backend.GetModuleSecurity(module.ID)
	if err != nil || ms == nil {
		return nil
	}
	if moduleUsesAutoDocumentRole(ms) {
		return []model.ID{model.ID(module.Name + "." + autoDocumentRoleName)}
	}
	if len(ms.ModuleRoles) > 0 {
		return nil
	}

	if err := ctx.Backend.AddModuleRole(ms.ID, autoDocumentRoleName, autoDocumentRoleDescription); err != nil {
		return nil
	}
	return []model.ID{model.ID(module.Name + "." + autoDocumentRoleName)}
}

func moduleUsesAutoDocumentRole(ms *security.ModuleSecurity) bool {
	if ms == nil {
		return false
	}
	return len(ms.ModuleRoles) == 1 &&
		ms.ModuleRoles[0].Name == autoDocumentRoleName &&
		ms.ModuleRoles[0].Description == autoDocumentRoleDescription
}

func remapDocumentAccessRoles(ctx *ExecContext, targetModule *model.Module, currentRoles []model.ID) []model.ID {
	if targetModule == nil {
		return nil
	}

	ms, err := ctx.Backend.GetModuleSecurity(targetModule.ID)
	if err != nil || ms == nil {
		return nil
	}
	if len(ms.ModuleRoles) == 0 || moduleUsesAutoDocumentRole(ms) {
		return defaultDocumentAccessRoles(ctx, targetModule)
	}

	targetRoleNames := make(map[string]bool, len(ms.ModuleRoles))
	for _, role := range ms.ModuleRoles {
		targetRoleNames[role.Name] = true
	}

	var remapped []model.ID
	seen := make(map[string]bool)
	for _, qualifiedRole := range currentRoles {
		roleName := string(qualifiedRole)
		if idx := strings.LastIndex(roleName, "."); idx >= 0 {
			roleName = roleName[idx+1:]
		}
		if !targetRoleNames[roleName] {
			continue
		}
		targetQualifiedRole := targetModule.Name + "." + roleName
		if seen[targetQualifiedRole] {
			continue
		}
		seen[targetQualifiedRole] = true
		remapped = append(remapped, model.ID(targetQualifiedRole))
	}

	return remapped
}

func documentRoleStrings(roles []model.ID) []string {
	values := make([]string, 0, len(roles))
	for _, role := range roles {
		values = append(values, string(role))
	}
	return values
}

func cloneRoleIDs(roles []model.ID) []model.ID {
	if len(roles) == 0 {
		return nil
	}
	cloned := make([]model.ID, len(roles))
	copy(cloned, roles)
	return cloned
}

// pruneInvalidUserRoles removes user roles that no longer have any non-System
// module role assignments. Mendix rejects those roles with CE0157.
func pruneInvalidUserRoles(ctx *ExecContext, ps *security.ProjectSecurity) error {
	if latest, err := ctx.Backend.GetProjectSecurity(); err == nil {
		ps = latest
	} else if ps == nil {
		return err
	}

	for _, userRole := range ps.UserRoles {
		hasNonSystemRole := false
		for _, moduleRole := range userRole.ModuleRoles {
			if !strings.HasPrefix(moduleRole, "System.") {
				hasNonSystemRole = true
				break
			}
		}
		if hasNonSystemRole {
			continue
		}
		if err := ctx.Backend.RemoveUserRole(ps.ID, userRole.Name); err != nil {
			return err
		}
		if !ctx.Quiet {
			fmt.Fprintf(ctx.Output, "Dropped invalid user role: %s\n", userRole.Name)
		}
	}

	return nil
}
