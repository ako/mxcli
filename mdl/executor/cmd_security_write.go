// SPDX-License-Identifier: Apache-2.0

// Package executor - Security write commands (CREATE/DROP/ALTER/GRANT/REVOKE)
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// execCreateModuleRole handles CREATE MODULE ROLE Module.RoleName [DESCRIPTION '...'].
func execCreateModuleRole(ctx *ExecContext, s *ast.CreateModuleRoleStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	ms, err := ctx.Backend.GetModuleSecurity(module.ID)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("read module security for %s", s.Name.Module), err)
	}

	// Check if role already exists. Mendix treats role names case-insensitively
	// (CE0123), so match that way. An auto-provisioned role collides with any
	// user-requested casing; let AddModuleRole overwrite to adopt the caller's
	// casing so later case-sensitive lookups (GRANT ACCESS TO x.user) succeed.
	for _, mr := range ms.ModuleRoles {
		if !strings.EqualFold(mr.Name, s.Name.Name) {
			continue
		}
		if mr.Description == autoDocumentRoleDescription {
			oldQualified := s.Name.Module + "." + mr.Name
			newQualified := s.Name.Module + "." + s.Name.Name
			if err := ctx.Backend.AddModuleRole(ms.ID, s.Name.Name, s.Description); err != nil {
				return mdlerrors.NewBackend("create module role", err)
			}
			// If the casing actually changed, propagate the rename across every
			// unit that referenced the old name (AllowedModuleRoles on microflows,
			// pages, published REST services, etc.). Without this, mx check fails
			// with CE1613 "selected module role X no longer exists".
			if oldQualified != newQualified {
				if _, err := ctx.Backend.UpdateQualifiedNameInAllUnits(oldQualified, newQualified); err != nil {
					return mdlerrors.NewBackend(fmt.Sprintf("rename references %s -> %s", oldQualified, newQualified), err)
				}
			}
			if !ctx.Quiet {
				fmt.Fprintf(ctx.Output, "Module role %s.%s already exists (auto-provisioned)\n", s.Name.Module, s.Name.Name)
			}
			return nil
		}
		if s.CreateOrModify {
			// Re-running a security script must not fail on a role that is
			// already there. AddModuleRole overwrites, so this also adopts a new
			// description and the caller's casing.
			if err := ctx.Backend.AddModuleRole(ms.ID, s.Name.Name, s.Description); err != nil {
				return mdlerrors.NewBackend("modify module role", err)
			}
			if !ctx.Quiet {
				ctx.ReportMutation("Modified", "module role: %s.%s", s.Name.Module, s.Name.Name)
			}
			return nil
		}
		return mdlerrors.NewAlreadyExists("module role", s.Name.Module+"."+s.Name.Name)
	}

	if err := ctx.Backend.AddModuleRole(ms.ID, s.Name.Name, s.Description); err != nil {
		return mdlerrors.NewBackend("create module role", err)
	}

	fmt.Fprintf(ctx.Output, "Created module role: %s.%s\n", s.Name.Module, s.Name.Name)
	return nil
}

// execDropModuleRole handles DROP MODULE ROLE Module.RoleName.
// Cascade-removes the role from all entity access rules, microflow/nanoflow/page
// allowed roles, and OData service allowed roles before deleting the role itself.
func execDropModuleRole(ctx *ExecContext, s *ast.DropModuleRoleStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	module, err := findModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	ms, err := ctx.Backend.GetModuleSecurity(module.ID)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("read module security for %s", s.Name.Module), err)
	}

	// Check role exists
	found := false
	for _, mr := range ms.ModuleRoles {
		if mr.Name == s.Name.Name {
			found = true
			break
		}
	}
	if !found {
		return mdlerrors.NewNotFound("module role", s.Name.Module+"."+s.Name.Name)
	}

	qualifiedRole := s.Name.Module + "." + s.Name.Name

	// Cascade: remove role from entity access rules
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err == nil {
		if n, err := ctx.Backend.RemoveRoleFromAllEntities(dm.ID, qualifiedRole); err != nil {
			return mdlerrors.NewBackend("cascade-remove entity access rules", err)
		} else if n > 0 {
			fmt.Fprintf(ctx.Output, "Removed %s from %d entity access rule(s)\n", qualifiedRole, n)
		}
	}

	// Cascade: remove role from microflow/nanoflow/page allowed roles
	h, err := getHierarchy(ctx)
	if err == nil {
		// Microflows
		if mfs, err := ctx.Backend.ListMicroflows(); err == nil {
			for _, mf := range mfs {
				modID := h.FindModuleID(mf.ContainerID)
				if modID != module.ID {
					continue
				}
				if removed, err := ctx.Backend.RemoveFromAllowedRoles(mf.ID, qualifiedRole); err == nil && removed {
					fmt.Fprintf(ctx.Output, "Removed %s from microflow %s allowed roles\n", qualifiedRole, mf.Name)
				}
			}
		}

		// Nanoflows
		if nfs, err := ctx.Backend.ListNanoflows(); err == nil {
			for _, nf := range nfs {
				modID := h.FindModuleID(nf.ContainerID)
				if modID != module.ID {
					continue
				}
				if removed, err := ctx.Backend.RemoveFromAllowedRoles(nf.ID, qualifiedRole); err == nil && removed {
					fmt.Fprintf(ctx.Output, "Removed %s from nanoflow %s allowed roles\n", qualifiedRole, nf.Name)
				}
			}
		}

		// Pages
		if pgs, err := ctx.Backend.ListPages(); err == nil {
			for _, pg := range pgs {
				modID := h.FindModuleID(pg.ContainerID)
				if modID != module.ID {
					continue
				}
				if removed, err := ctx.Backend.RemoveFromAllowedRoles(pg.ID, qualifiedRole); err == nil && removed {
					fmt.Fprintf(ctx.Output, "Removed %s from page %s allowed roles\n", qualifiedRole, pg.Name)
				}
			}
		}

		// OData services
		if svcs, err := ctx.Backend.ListPublishedODataServices(); err == nil {
			for _, svc := range svcs {
				modID := h.FindModuleID(svc.ContainerID)
				if modID != module.ID {
					continue
				}
				if removed, err := ctx.Backend.RemoveFromAllowedRoles(svc.ID, qualifiedRole); err == nil && removed {
					fmt.Fprintf(ctx.Output, "Removed %s from OData service %s allowed roles\n", qualifiedRole, svc.Name)
				}
			}
		}
	}

	// Cascade: remove role from user roles in ProjectSecurity
	if ps, err := ctx.Backend.GetProjectSecurity(); err == nil {
		if n, err := ctx.Backend.RemoveModuleRoleFromAllUserRoles(ps.ID, qualifiedRole); err == nil && n > 0 {
			fmt.Fprintf(ctx.Output, "Removed %s from %d user role(s)\n", qualifiedRole, n)
		}
		if err := pruneInvalidUserRoles(ctx, ps); err != nil {
			return mdlerrors.NewBackend("cleanup invalid user roles", err)
		}
	}

	// Finally, remove the role itself
	if err := ctx.Backend.RemoveModuleRole(ms.ID, s.Name.Name); err != nil {
		return mdlerrors.NewBackend("drop module role", err)
	}

	fmt.Fprintf(ctx.Output, "Dropped module role: %s.%s\n", s.Name.Module, s.Name.Name)
	return nil
}

// execCreateUserRole handles CREATE [OR MODIFY] USER ROLE Name (ModuleRoles) [MANAGE ALL ROLES].
func execCreateUserRole(ctx *ExecContext, s *ast.CreateUserRoleStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil {
		return mdlerrors.NewBackend("read project security", err)
	}

	// Build qualified module role names
	var moduleRoleNames []string
	for _, mr := range s.ModuleRoles {
		qn := mr.Module + "." + mr.Name
		moduleRoleNames = append(moduleRoleNames, qn)
	}

	// Check if role already exists
	for _, ur := range ps.UserRoles {
		if ur.Name == s.Name {
			if !s.CreateOrModify {
				return mdlerrors.NewAlreadyExists("user role", s.Name)
			}
			// Additive: ensure specified module roles are present
			if err := ctx.Backend.AlterUserRoleModuleRoles(ps.ID, s.Name, true, moduleRoleNames); err != nil {
				return mdlerrors.NewBackend("update user role", err)
			}
			ctx.ReportMutation("Modified", "user role: %s", s.Name)
			return nil
		}
	}

	if err := ctx.Backend.AddUserRole(ps.ID, s.Name, moduleRoleNames, s.ManageAllRoles); err != nil {
		return mdlerrors.NewBackend("create user role", err)
	}

	fmt.Fprintf(ctx.Output, "Created user role: %s\n", s.Name)
	return nil
}

// execAlterUserRole handles ALTER USER ROLE Name ADD/REMOVE MODULE ROLES (...).
func execAlterUserRole(ctx *ExecContext, s *ast.AlterUserRoleStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil {
		return mdlerrors.NewBackend("read project security", err)
	}

	// Check user role exists
	found := false
	for _, ur := range ps.UserRoles {
		if ur.Name == s.Name {
			found = true
			break
		}
	}
	if !found {
		return mdlerrors.NewNotFound("user role", s.Name)
	}

	// Build qualified module role names
	var moduleRoleNames []string
	for _, mr := range s.ModuleRoles {
		moduleRoleNames = append(moduleRoleNames, mr.Module+"."+mr.Name)
	}

	if err := ctx.Backend.AlterUserRoleModuleRoles(ps.ID, s.Name, s.Add, moduleRoleNames); err != nil {
		return mdlerrors.NewBackend("alter user role", err)
	}

	action := "Added"
	prep := "to"
	if !s.Add {
		action = "Removed"
		prep = "from"
	}
	fmt.Fprintf(ctx.Output, "%s module roles %s %s user role %s\n", action, strings.Join(moduleRoleNames, ", "), prep, s.Name)
	return nil
}

// execDropUserRole handles DROP USER ROLE Name.
func execDropUserRole(ctx *ExecContext, s *ast.DropUserRoleStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil {
		return mdlerrors.NewBackend("read project security", err)
	}

	// Check user role exists
	found := false
	for _, ur := range ps.UserRoles {
		if ur.Name == s.Name {
			found = true
			break
		}
	}
	if !found {
		return mdlerrors.NewNotFound("user role", s.Name)
	}

	if err := ctx.Backend.RemoveUserRole(ps.ID, s.Name); err != nil {
		return mdlerrors.NewBackend("drop user role", err)
	}

	fmt.Fprintf(ctx.Output, "Dropped user role: %s\n", s.Name)
	return nil
}

// execGrantEntityAccess handles GRANT roles ON Module.Entity (rights) [WHERE '...'].
func execGrantEntityAccess(ctx *ExecContext, s *ast.GrantEntityAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	module, err := findModule(ctx, s.Entity.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Verify entity exists
	entity := dm.FindEntityByName(s.Entity.Name)
	if entity == nil {
		return mdlerrors.NewNotFound("entity", s.Entity.Module+"."+s.Entity.Name)
	}

	// Validate all roles exist before creating any access rules
	for _, role := range s.Roles {
		if err := validateModuleRole(ctx, role); err != nil {
			return err
		}
	}

	// Build role name list
	var roleNames []string
	for _, role := range s.Roles {
		roleNames = append(roleNames, role.Module+"."+role.Name)
	}

	// Parse access rights from the statement.
	// Note: Mendix has no AllowRead/AllowWrite properties on AccessRule.
	// Read/write access is determined by DefaultMemberAccessRights and MemberAccesses.
	allowCreate, allowDelete := false, false
	defaultMemberAccess := "None"
	var readMembers, writeMembers []string // nil = all (wildcard)
	for _, right := range s.Rights {
		switch right.Type {
		case ast.EntityAccessCreate:
			allowCreate = true
		case ast.EntityAccessDelete:
			allowDelete = true
		case ast.EntityAccessReadAll:
			if defaultMemberAccess == "None" {
				defaultMemberAccess = "ReadOnly"
			}
		case ast.EntityAccessReadMembers:
			readMembers = right.Members
		case ast.EntityAccessWriteAll:
			defaultMemberAccess = "ReadWrite"
		case ast.EntityAccessWriteMembers:
			writeMembers = right.Members
		}
	}

	// Build MemberAccess entries for all entity attributes and associations.
	// Mendix requires explicit MemberAccess entries for every member — an empty
	// MemberAccesses array triggers CE0066 "Entity access is out of date".
	var memberAccesses []types.EntityMemberAccess

	// Build sets for specific member overrides (when READ (Name, Email) syntax is used)
	writeMemberSet := make(map[string]bool)
	for _, m := range writeMembers {
		writeMemberSet[m] = true
	}
	readMemberSet := make(map[string]bool)
	for _, m := range readMembers {
		readMemberSet[m] = true
	}

	// Create entries for every attribute of the entity's access surface — its own
	// AND those inherited through the generalization chain. Enumerating only
	// entity.Attributes meant a GRANT naming an inherited member produced no entry
	// at all while still reporting success, and left the rule incomplete so Mendix
	// reported CE0066 (mendixlabs/mxcli#758). Each reference is qualified against
	// the entity that DECLARES the member; qualifying an inherited one against this
	// entity is CE1613 "The selected attribute no longer exists".
	entityQN := module.Name + "." + s.Entity.Name
	members := EntityMembers(ctx, entityQN)
	grantedMembers := map[string]bool{}
	for _, mem := range members {
		rights := defaultMemberAccess
		if writeMemberSet[mem.Name] {
			rights = "ReadWrite"
		} else if readMemberSet[mem.Name] {
			rights = "ReadOnly"
		}
		// Calculated attributes cannot have write rights (CE6592)
		if mem.IsCalculated && (rights == "ReadWrite" || rights == "WriteOnly") {
			rights = "ReadOnly"
		}
		grantedMembers[mem.Name] = true
		memberAccesses = append(memberAccesses, types.EntityMemberAccess{
			AttributeRef: mem.Ref,
			AccessRights: rights,
		})
	}

	// Audit members are stored as flags on the entity, not as entries in
	// entity.Attributes, so the member walk above never yields them — naming
	// `createdDate` in a GRANT was rejected as "entity has no member(s)", which
	// is simply wrong: Mendix does consider them members (issuetracker #20).
	//
	// What Mendix does NOT have is a MemberAccess for them. An entity storing
	// audit members checks clean with no entry, and mxbuild rejects a rule that
	// carries one with CE0066 (verified on 11.12.1). Their access therefore comes
	// from the rule's default and cannot be set per member — so naming one is
	// accepted (it is a real member), but asking for rights that differ from the
	// default is refused rather than silently dropped.
	for _, sys := range storedAuditMembers(entity) {
		grantedMembers[sys] = true
		rights, clause := "", ""
		if writeMemberSet[sys] {
			rights, clause = "ReadWrite", "write"
		} else if readMemberSet[sys] {
			rights, clause = "ReadOnly", "read"
		}
		if rights != "" && rights != defaultMemberAccess {
			return mdlerrors.NewValidationf(
				"%s.%s is a Mendix audit member: its access follows the rule's default and cannot be set per member "+
					"(Mendix stores no member access for it, and a rule that carries one fails the build with CE0066). "+
					"Drop it from the %s (...) list and let `read *` / `write *` cover it, or change the rule's default.",
				entityQN, sys, clause)
		}
	}

	// Create entries for associations this entity owns. ParentID = FROM entity
	// (FK owner), and for a Default-owner association MemberAccess belongs only
	// on that side — adding it to the TO side triggers CE0066.
	//
	// `OWNER Both` is the exception: both ends own the association and Mendix
	// expects a MemberAccess entry on each. Emitting it only on the FROM side
	// left the TO entity's rule incomplete, which is CE0066 "Entity access is
	// out of date" — the reported symptom (issuetracker #20). Verified against
	// mxbuild 11.12.1: the same script with `OWNER Default` checks clean, so the
	// owner mode is the trigger, not the reference set.
	addAssociationAccess := func(name, ref string) {
		rights := defaultMemberAccess
		if writeMemberSet[name] {
			rights = "ReadWrite"
		} else if readMemberSet[name] {
			rights = "ReadOnly"
		}
		grantedMembers[name] = true
		memberAccesses = append(memberAccesses, types.EntityMemberAccess{
			AssociationRef: ref,
			AccessRights:   rights,
		})
	}
	for _, assoc := range dm.Associations {
		ownedHere := assoc.ParentID == entity.ID ||
			(assoc.Owner == domainmodel.AssociationOwnerBoth && assoc.ChildID == entity.ID)
		if ownedHere {
			addAssociationAccess(assoc.Name, module.Name+"."+assoc.Name)
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.ParentID == entity.ID {
			addAssociationAccess(ca.Name, module.Name+"."+ca.Name)
		}
	}
	// A cross-module association owned by both ends is stored in the FROM
	// entity's module, so this entity — the TO end — has to look for it there.
	for _, other := range otherModuleBothOwnerAssociations(ctx, module.Name, entityQN) {
		addAssociationAccess(other.Name, other.Ref)
	}
	// Associations declared on an ancestor. Mendix inheritance is multi-table:
	// a specialization has ALL of its generalization's members, associations
	// included, and its access rule needs an entry for each — exactly as it does
	// for inherited attributes (#758).
	//
	// Leaving them out was CE0066 "Entity access is out of date" on the
	// specialization's own module, and it made the rule OpenAIConnector ships
	// impossible to express in MDL: `OpenAIDeployedModel extends
	// GenAICommons.DeployedModel`, and `DeployedModel_InputModality` is declared
	// on the parent, so the grant naming it was refused as "no such member"
	// (mxcli-chat FINDINGS §26). Reproduced on a two-entity fixture with no
	// marketplace module in sight: `GRANT … ON Derived (READ *, WRITE *)` gave
	// CE0066 while the same rule on the base entity checked clean.
	//
	// The reference is qualified against the module that DECLARES the
	// association, not this entity's — the same rule the attribute walk follows.
	for _, inh := range inheritedAssociations(ctx, entityQN) {
		if grantedMembers[inh.Name] {
			continue // an association of this entity's own shadows it
		}
		addAssociationAccess(inh.Name, inh.Ref)
	}

	// A member named in the GRANT that matched nothing used to be dropped in
	// silence — the command reported success and the access simply was not there,
	// which is why REVOKE + GRANT could not repair a damaged rule (#758). Name it
	// instead. Inherited members now resolve, so anything still unmatched is a typo
	// or a member of another entity.
	if unknown := unmatchedGrantMembers(readMembers, writeMembers, grantedMembers); len(unknown) > 0 {
		return mdlerrors.NewValidationf(
			"entity %s has no member(s) %s; grant only names members of the entity or of an entity it inherits from",
			entityQN, strings.Join(unknown, ", "))
	}

	// Add MemberAccess entries for system associations (owner, changedBy).
	// When an entity has HasOwner/HasChangedBy, Mendix implicitly adds
	// System.owner/System.changedBy associations that require MemberAccess.
	if entity.HasOwner {
		memberAccesses = append(memberAccesses, types.EntityMemberAccess{
			AssociationRef: "System.owner",
			AccessRights:   defaultMemberAccess,
		})
	}
	if entity.HasChangedBy {
		memberAccesses = append(memberAccesses, types.EntityMemberAccess{
			AssociationRef: "System.changedBy",
			AccessRights:   defaultMemberAccess,
		})
	}

	// A constraint too long to read on one line is broken at its boolean joints;
	// one that already fits comes back unchanged (upstream #979). It is formatted
	// ONCE, here, because the stored text is also what identifies the rule: a rule
	// is keyed by role plus constraint (#936), so echoing the result back with the
	// unformatted spelling would look for a rule that is not there.
	xpathConstraint := visitor.FormatXPathConstraint(s.XPathConstraint)

	if err := ctx.Backend.AddEntityAccessRule(backend.EntityAccessRuleParams{
		UnitID:              dm.ID,
		EntityName:          s.Entity.Name,
		RoleNames:           roleNames,
		AllowCreate:         allowCreate,
		AllowDelete:         allowDelete,
		DefaultMemberAccess: defaultMemberAccess,
		XPathConstraint:     xpathConstraint,
		MemberAccesses:      memberAccesses,
	}); err != nil {
		return mdlerrors.NewBackend("grant entity access", err)
	}

	// Reconcile MemberAccesses on pre-existing rules for this entity's domain model
	if count, err := ctx.Backend.ReconcileMemberAccesses(dm.ID, module.Name); err != nil {
		return mdlerrors.NewBackend("reconcile member accesses", err)
	} else if count > 0 && !ctx.Quiet {
		fmt.Fprintf(ctx.Output, "Reconciled %d access rule(s) in module %s\n", count, module.Name)
	}

	ctx.trackModifiedDomainModel(module.ID, module.Name)
	fmt.Fprintf(ctx.Output, "Granted access on %s.%s to %s\n", s.Entity.Module, s.Entity.Name, strings.Join(roleNames, ", "))
	if !ctx.Quiet {
		fmt.Fprint(ctx.Output, formatAccessRuleResult(ctx, s.Entity.Module, s.Entity.Name, roleNames, xpathConstraint, false))
	}
	return nil
}

// execRevokeEntityAccess handles REVOKE roles ON Module.Entity [(rights...)].
func execRevokeEntityAccess(ctx *ExecContext, s *ast.RevokeEntityAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	module, err := findModule(ctx, s.Entity.Module)
	if err != nil {
		return err
	}

	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	// Verify entity exists
	entity := dm.FindEntityByName(s.Entity.Name)
	if entity == nil {
		return mdlerrors.NewNotFound("entity", s.Entity.Module+"."+s.Entity.Name)
	}

	// Validate all roles exist before modifying any access rules
	for _, role := range s.Roles {
		if err := validateModuleRole(ctx, role); err != nil {
			return err
		}
	}

	// Build role name list
	var roleNames []string
	for _, role := range s.Roles {
		roleNames = append(roleNames, role.Module+"."+role.Name)
	}

	if len(s.Rights) > 0 {
		// Partial revoke — downgrade specific rights
		revocation := types.EntityAccessRevocation{}
		for _, right := range s.Rights {
			switch right.Type {
			case ast.EntityAccessCreate:
				revocation.RevokeCreate = true
			case ast.EntityAccessDelete:
				revocation.RevokeDelete = true
			case ast.EntityAccessReadAll:
				revocation.RevokeReadAll = true
			case ast.EntityAccessWriteAll:
				revocation.RevokeWriteAll = true
			case ast.EntityAccessReadMembers:
				for _, m := range right.Members {
					revocation.RevokeReadMembers = append(revocation.RevokeReadMembers,
						module.Name+"."+s.Entity.Name+"."+m)
				}
			case ast.EntityAccessWriteMembers:
				for _, m := range right.Members {
					revocation.RevokeWriteMembers = append(revocation.RevokeWriteMembers,
						module.Name+"."+s.Entity.Name+"."+m)
				}
			}
		}

		modified, err := ctx.Backend.RevokeEntityMemberAccess(dm.ID, s.Entity.Name, roleNames, revocation)
		if err != nil {
			return mdlerrors.NewBackend("revoke entity access", err)
		}

		if modified == 0 {
			fmt.Fprintf(ctx.Output, "No access rules found matching %s on %s.%s\n", strings.Join(roleNames, ", "), s.Entity.Module, s.Entity.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Revoked partial access on %s.%s from %s\n", s.Entity.Module, s.Entity.Name, strings.Join(roleNames, ", "))
			if !ctx.Quiet {
				fmt.Fprint(ctx.Output, formatAccessRuleResult(ctx, s.Entity.Module, s.Entity.Name, roleNames, "", true))
			}
		}
	} else {
		// Full revoke — remove entire access rule
		modified, err := ctx.Backend.RemoveEntityAccessRule(dm.ID, s.Entity.Name, roleNames)
		if err != nil {
			return mdlerrors.NewBackend("revoke entity access", err)
		}

		if modified == 0 {
			fmt.Fprintf(ctx.Output, "No access rules found matching %s on %s.%s\n", strings.Join(roleNames, ", "), s.Entity.Module, s.Entity.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Revoked access on %s.%s from %s\n", s.Entity.Module, s.Entity.Name, strings.Join(roleNames, ", "))
			if !ctx.Quiet {
				fmt.Fprint(ctx.Output, "  Result: (no access)\n")
			}
		}
	}
	ctx.trackModifiedDomainModel(module.ID, module.Name)
	return nil
}

// execGrantMicroflowAccess handles GRANT EXECUTE ON MICROFLOW Module.MF TO roles.
func execGrantMicroflowAccess(ctx *ExecContext, s *ast.GrantMicroflowAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the microflow
	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return mdlerrors.NewBackend("list microflows", err)
	}

	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Microflow.Module || mf.Name != s.Microflow.Name {
			continue
		}

		// Reject cross-module role grants before they reach the model (CE0148 guard).
		if err := checkDocumentAccessRolesSameModule("microflow", modName, mf.Name, s.Roles); err != nil {
			return err
		}

		// Validate all roles exist
		for _, role := range s.Roles {
			if err := validateModuleRole(ctx, role); err != nil {
				return err
			}
		}

		// Merge new roles with existing (skip duplicates)
		existing := make(map[string]bool)
		var merged []string
		for _, r := range mf.AllowedModuleRoles {
			existing[string(r)] = true
			merged = append(merged, string(r))
		}
		var added []string
		for _, role := range s.Roles {
			qn := role.Module + "." + role.Name
			if !existing[qn] {
				merged = append(merged, qn)
				added = append(added, qn)
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(mf.ID, merged); err != nil {
			return mdlerrors.NewBackend("update microflow access", err)
		}

		if len(added) == 0 {
			fmt.Fprintf(ctx.Output, "All specified roles already have execute access on %s.%s\n", modName, mf.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Granted execute access on %s.%s to %s\n", modName, mf.Name, strings.Join(added, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("microflow", s.Microflow.Module+"."+s.Microflow.Name)
}

// execRevokeMicroflowAccess handles REVOKE EXECUTE ON MICROFLOW Module.MF FROM roles.
func execRevokeMicroflowAccess(ctx *ExecContext, s *ast.RevokeMicroflowAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the microflow
	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return mdlerrors.NewBackend("list microflows", err)
	}

	for _, mf := range mfs {
		modID := h.FindModuleID(mf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Microflow.Module || mf.Name != s.Microflow.Name {
			continue
		}

		// Build set of roles to remove
		toRemove := make(map[string]bool)
		for _, role := range s.Roles {
			toRemove[role.Module+"."+role.Name] = true
		}

		// Filter out removed roles
		var remaining []string
		var removed []string
		for _, r := range mf.AllowedModuleRoles {
			if toRemove[string(r)] {
				removed = append(removed, string(r))
			} else {
				remaining = append(remaining, string(r))
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(mf.ID, remaining); err != nil {
			return mdlerrors.NewBackend("update microflow access", err)
		}

		if len(removed) == 0 {
			fmt.Fprintf(ctx.Output, "None of the specified roles had execute access on %s.%s\n", modName, mf.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Revoked execute access on %s.%s from %s\n", modName, mf.Name, strings.Join(removed, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("microflow", s.Microflow.Module+"."+s.Microflow.Name)
}

// execGrantNanoflowAccess handles GRANT EXECUTE ON NANOFLOW Module.NF TO roles.
func execGrantNanoflowAccess(ctx *ExecContext, s *ast.GrantNanoflowAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	nfs, err := ctx.Backend.ListNanoflows()
	if err != nil {
		return mdlerrors.NewBackend("list nanoflows", err)
	}

	for _, nf := range nfs {
		modID := h.FindModuleID(nf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Nanoflow.Module || nf.Name != s.Nanoflow.Name {
			continue
		}

		// Reject cross-module role grants before they reach the model (CE0148 guard).
		if err := checkDocumentAccessRolesSameModule("nanoflow", modName, nf.Name, s.Roles); err != nil {
			return err
		}

		for _, role := range s.Roles {
			if err := validateModuleRole(ctx, role); err != nil {
				return err
			}
		}

		existing := make(map[string]bool)
		var merged []string
		for _, r := range nf.AllowedModuleRoles {
			existing[string(r)] = true
			merged = append(merged, string(r))
		}
		var added []string
		for _, role := range s.Roles {
			qn := role.Module + "." + role.Name
			if !existing[qn] {
				merged = append(merged, qn)
				added = append(added, qn)
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(nf.ID, merged); err != nil {
			return mdlerrors.NewBackend("update nanoflow access", err)
		}

		if len(added) == 0 {
			fmt.Fprintf(ctx.Output, "All specified roles already have execute access on %s.%s\n", modName, nf.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Granted execute access on %s.%s to %s\n", modName, nf.Name, strings.Join(added, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("nanoflow", s.Nanoflow.Module+"."+s.Nanoflow.Name)
}

// execRevokeNanoflowAccess handles REVOKE EXECUTE ON NANOFLOW Module.NF FROM roles.
func execRevokeNanoflowAccess(ctx *ExecContext, s *ast.RevokeNanoflowAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	nfs, err := ctx.Backend.ListNanoflows()
	if err != nil {
		return mdlerrors.NewBackend("list nanoflows", err)
	}

	for _, nf := range nfs {
		modID := h.FindModuleID(nf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Nanoflow.Module || nf.Name != s.Nanoflow.Name {
			continue
		}

		toRemove := make(map[string]bool)
		for _, role := range s.Roles {
			toRemove[role.Module+"."+role.Name] = true
		}

		var remaining []string
		var removed []string
		for _, r := range nf.AllowedModuleRoles {
			if toRemove[string(r)] {
				removed = append(removed, string(r))
			} else {
				remaining = append(remaining, string(r))
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(nf.ID, remaining); err != nil {
			return mdlerrors.NewBackend("update nanoflow access", err)
		}

		if len(removed) == 0 {
			fmt.Fprintf(ctx.Output, "None of the specified roles had execute access on %s.%s\n", modName, nf.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Revoked execute access on %s.%s from %s\n", modName, nf.Name, strings.Join(removed, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("nanoflow", s.Nanoflow.Module+"."+s.Nanoflow.Name)
}

// execGrantPageAccess handles GRANT VIEW ON PAGE Module.Page TO roles.
func execGrantPageAccess(ctx *ExecContext, s *ast.GrantPageAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the page
	pages, err := ctx.Backend.ListPages()
	if err != nil {
		return mdlerrors.NewBackend("list pages", err)
	}

	for _, pg := range pages {
		modID := h.FindModuleID(pg.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Page.Module || pg.Name != s.Page.Name {
			continue
		}

		// Reject cross-module role grants before they reach the model (CE0148 guard).
		if err := checkDocumentAccessRolesSameModule("page", modName, pg.Name, s.Roles); err != nil {
			return err
		}

		// Validate all roles exist
		for _, role := range s.Roles {
			if err := validateModuleRole(ctx, role); err != nil {
				return err
			}
		}

		// Merge new roles with existing (skip duplicates)
		existing := make(map[string]bool)
		var merged []string
		for _, r := range pg.AllowedRoles {
			existing[string(r)] = true
			merged = append(merged, string(r))
		}
		var added []string
		for _, role := range s.Roles {
			qn := role.Module + "." + role.Name
			if !existing[qn] {
				merged = append(merged, qn)
				added = append(added, qn)
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(pg.ID, merged); err != nil {
			return mdlerrors.NewBackend("update page access", err)
		}

		if len(added) == 0 {
			fmt.Fprintf(ctx.Output, "All specified roles already have view access on %s.%s\n", modName, pg.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Granted view access on %s.%s to %s\n", modName, pg.Name, strings.Join(added, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("page", s.Page.Module+"."+s.Page.Name)
}

// execRevokePageAccess handles REVOKE VIEW ON PAGE Module.Page FROM roles.
func execRevokePageAccess(ctx *ExecContext, s *ast.RevokePageAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the page
	pages, err := ctx.Backend.ListPages()
	if err != nil {
		return mdlerrors.NewBackend("list pages", err)
	}

	for _, pg := range pages {
		modID := h.FindModuleID(pg.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Page.Module || pg.Name != s.Page.Name {
			continue
		}

		// Build set of roles to remove
		toRemove := make(map[string]bool)
		for _, role := range s.Roles {
			toRemove[role.Module+"."+role.Name] = true
		}

		// Filter out removed roles
		var remaining []string
		var removed []string
		for _, r := range pg.AllowedRoles {
			if toRemove[string(r)] {
				removed = append(removed, string(r))
			} else {
				remaining = append(remaining, string(r))
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(pg.ID, remaining); err != nil {
			return mdlerrors.NewBackend("update page access", err)
		}

		if len(removed) == 0 {
			fmt.Fprintf(ctx.Output, "None of the specified roles had view access on %s.%s\n", modName, pg.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Revoked view access on %s.%s from %s\n", modName, pg.Name, strings.Join(removed, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("page", s.Page.Module+"."+s.Page.Name)
}

// execGrantWorkflowAccess handles GRANT EXECUTE ON WORKFLOW Module.WF TO roles.
// Mendix workflows do not have a document-level AllowedModuleRoles field (unlike
// microflows and pages), so this operation is not supported.
func execGrantWorkflowAccess(ctx *ExecContext, s *ast.GrantWorkflowAccessStmt) error {
	return mdlerrors.NewUnsupported("grant execute on workflow is not supported: Mendix workflows do not have document-level AllowedModuleRoles (unlike microflows and pages). Workflow access is controlled through the microflow that triggers the workflow and UserTask targeting")
}

// execRevokeWorkflowAccess handles REVOKE EXECUTE ON WORKFLOW Module.WF FROM roles.
// Mendix workflows do not have a document-level AllowedModuleRoles field (unlike
// microflows and pages), so this operation is not supported.
func execRevokeWorkflowAccess(ctx *ExecContext, s *ast.RevokeWorkflowAccessStmt) error {
	return mdlerrors.NewUnsupported("revoke execute on workflow is not supported: Mendix workflows do not have document-level AllowedModuleRoles (unlike microflows and pages). Workflow access is controlled through the microflow that triggers the workflow and UserTask targeting")
}

// validateModuleRole checks that a module role exists in the project.
func validateModuleRole(ctx *ExecContext, role ast.QualifiedName) error {
	module, err := findModule(ctx, role.Module)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("module not found for role %s.%s", role.Module, role.Name), err)
	}

	ms, err := ctx.Backend.GetModuleSecurity(module.ID)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("read module security for %s", role.Module), err)
	}

	if ms != nil {
		for _, mr := range ms.ModuleRoles {
			if mr.Name == role.Name {
				return nil
			}
		}
	}

	return mdlerrors.NewNotFound("module role", role.Module+"."+role.Name)
}

// execAlterProjectSecurity handles ALTER PROJECT SECURITY LEVEL/DEMO USERS/GUEST ACCESS.
func execAlterProjectSecurity(ctx *ExecContext, s *ast.AlterProjectSecurityStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil {
		return mdlerrors.NewBackend("read project security", err)
	}

	if s.SecurityLevel != "" {
		// Map from display name to BSON value
		var bsonLevel string
		switch s.SecurityLevel {
		case "Production":
			bsonLevel = security.SecurityLevelProduction
		case "Prototype":
			bsonLevel = security.SecurityLevelPrototype
		case "Off":
			bsonLevel = security.SecurityLevelOff
		default:
			return mdlerrors.NewUnsupported(fmt.Sprintf("unknown security level: %s", s.SecurityLevel))
		}

		if err := ctx.Backend.SetProjectSecurityLevel(ps.ID, bsonLevel); err != nil {
			return mdlerrors.NewBackend("set security level", err)
		}
		fmt.Fprintf(ctx.Output, "Set project security level to %s\n", s.SecurityLevel)
	}

	if s.DemoUsersEnabled != nil {
		if err := ctx.Backend.SetProjectDemoUsersEnabled(ps.ID, *s.DemoUsersEnabled); err != nil {
			return mdlerrors.NewBackend("set demo users", err)
		}
		state := "disabled"
		if *s.DemoUsersEnabled {
			state = "enabled"
		}
		fmt.Fprintf(ctx.Output, "Demo users %s\n", state)
	}

	if s.GuestAccessEnabled != nil {
		if err := applyGuestAccess(ctx, ps, s); err != nil {
			return err
		}
	}

	return nil
}

// applyGuestAccess handles ALTER PROJECT SECURITY GUEST ACCESS ON|OFF [ROLE r].
//
// Two things Mendix does not do for us, and one it does:
//
//   - mxbuild raises CE0133 ("No user role for anonymous users selected even
//     though the feature anonymous users is enabled") when access is on with no
//     role, so ON is refused here rather than producing a project that will not
//     build. A stored role satisfies it, which is why ROLE is optional.
//   - mxbuild does NOT check that the role exists — a nonexistent one builds
//     with the same error count as a valid one — so a typo would otherwise be a
//     silently broken anonymous configuration. Validate it here.
//   - OFF leaves the stored role in place. Guest access off with a role set is
//     valid, and dropping it would lose the operator's choice on a toggle.
func applyGuestAccess(ctx *ExecContext, ps *security.ProjectSecurity, s *ast.AlterProjectSecurityStmt) error {
	enabled := *s.GuestAccessEnabled
	role := s.GuestUserRole

	if role != "" {
		known := make([]string, 0, len(ps.UserRoles))
		var match string
		for _, ur := range ps.UserRoles {
			known = append(known, ur.Name)
			if strings.EqualFold(ur.Name, role) {
				match = ur.Name
			}
		}
		if match == "" {
			return mdlerrors.NewNotFoundMsg("user role", role, fmt.Sprintf(
				"user role not found: %s (project user roles: %s). Mendix does not validate "+
					"this reference, so an unknown role would build cleanly and leave anonymous "+
					"visitors with no access",
				role, strings.Join(known, ", ")))
		}
		// Store the role under its declared casing, not the caller's.
		role = match
	} else if enabled && ps.GuestUserRole == "" {
		return mdlerrors.NewValidation(
			"GUEST ACCESS ON requires a role: no anonymous user role is configured, and Mendix " +
				"rejects anonymous access without one (CE0133). Use ALTER PROJECT SECURITY " +
				"GUEST ACCESS ON ROLE <UserRole>")
	}

	if err := ctx.Backend.SetProjectGuestAccess(ps.ID, enabled, role); err != nil {
		return mdlerrors.NewBackend("set guest access", err)
	}

	if !enabled {
		fmt.Fprintf(ctx.Output, "Guest access disabled\n")
		return nil
	}
	if role == "" {
		role = ps.GuestUserRole
	}
	fmt.Fprintf(ctx.Output, "Guest access enabled for user role %s\n", role)
	return nil
}

// execCreateDemoUser handles CREATE [OR MODIFY] DEMO USER 'name' PASSWORD 'pw' [ENTITY Module.Entity] (Roles).
func execCreateDemoUser(ctx *ExecContext, s *ast.CreateDemoUserStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil {
		return mdlerrors.NewBackend("read project security", err)
	}

	// Validate password against project password policy
	if err := ps.PasswordPolicy.ValidatePassword(s.Password); err != nil {
		return mdlerrors.NewValidationf("password policy violation for demo user '%s': %v\nhint: check your project's password policy with show project security", s.UserName, err)
	}

	// Check if user already exists
	for _, du := range ps.DemoUsers {
		if du.UserName == s.UserName {
			if !s.CreateOrModify {
				return mdlerrors.NewAlreadyExists("demo user", s.UserName)
			}
			// Additive: merge roles, update password. Drop and re-create with merged roles.
			mergedRoles := du.UserRoles
			existingSet := make(map[string]bool)
			for _, r := range mergedRoles {
				existingSet[r] = true
			}
			for _, r := range s.UserRoles {
				if !existingSet[r] {
					mergedRoles = append(mergedRoles, r)
				}
			}
			entity := du.Entity
			if s.Entity != "" {
				entity = s.Entity
			}
			if err := ctx.Backend.RemoveDemoUser(ps.ID, s.UserName); err != nil {
				return mdlerrors.NewBackend("update demo user", err)
			}
			if err := ctx.Backend.AddDemoUser(ps.ID, s.UserName, s.Password, entity, mergedRoles); err != nil {
				return mdlerrors.NewBackend("update demo user", err)
			}
			ctx.ReportMutation("Modified", "demo user: %s", s.UserName)
			return nil
		}
	}

	// Resolve entity: use explicit value or auto-detect from domain models
	entity := s.Entity
	if entity == "" {
		detected, err := detectUserEntity(ctx)
		if err != nil {
			return err
		}
		entity = detected
	}

	if err := ctx.Backend.AddDemoUser(ps.ID, s.UserName, s.Password, entity, s.UserRoles); err != nil {
		return mdlerrors.NewBackend("create demo user", err)
	}

	fmt.Fprintf(ctx.Output, "Created demo user: %s (entity: %s)\n", s.UserName, entity)
	warnDemoUsersInert(ctx, ps.SecurityLevel)
	return nil
}

// warnDemoUsersInert says so when demo users cannot materialise.
//
// With Security Level Off — the level a blank mxcli template ships with — the
// runtime creates no accounts, serves no login page and enforces none of the
// row-level rules, so the demo users sit in the model and never appear. Nothing
// said this: `CREATE DEMO USER` reported success and `SHOW PROJECT SECURITY`
// reported "Demo Users Enabled: true", while the running app had zero accounts.
// (mxcli-todo findings #15)
func warnDemoUsersInert(ctx *ExecContext, level string) {
	if level != security.SecurityLevelOff {
		return
	}
	fmt.Fprintf(ctx.Output, "  Note: project security level is Off, so the runtime creates no accounts "+
		"and this demo user will not appear in the app.\n"+
		"  Raise it first: alter project security level prototype;\n")
}

// detectUserEntity finds the entity that generalizes System.User.
// storedAuditMembers returns the audit members the entity actually stores, under
// the names Mendix uses for them. They are entity FLAGS rather than entries in
// entity.Attributes, so a member walk never yields them and a GRANT naming one
// was rejected as "no member" (issuetracker #20).
func storedAuditMembers(e *domainmodel.Entity) []string {
	if e == nil {
		return nil
	}
	var out []string
	if e.HasCreatedDate {
		out = append(out, "createdDate")
	}
	if e.HasChangedDate {
		out = append(out, "changedDate")
	}
	if e.HasOwner {
		out = append(out, "owner")
	}
	if e.HasChangedBy {
		out = append(out, "changedBy")
	}
	return out
}

// namedAssociation is an association reachable from an entity, with the
// qualified reference to store in a MemberAccess.
type namedAssociation struct{ Name, Ref string }

// otherModuleBothOwnerAssociations finds cross-module associations owned by BOTH
// ends whose TO end is entityQN. They are stored in the FROM entity's module, so
// scanning only this entity's own domain model misses them.
func otherModuleBothOwnerAssociations(ctx *ExecContext, thisModule, entityQN string) []namedAssociation {
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	var out []namedAssociation
	for _, dm := range dms {
		modName := h.GetModuleName(h.FindModuleID(dm.ContainerID))
		if modName == "" || modName == thisModule {
			continue
		}
		for _, ca := range dm.CrossAssociations {
			if ca.Owner == domainmodel.AssociationOwnerBoth && ca.ChildRef == entityQN {
				out = append(out, namedAssociation{Name: ca.Name, Ref: modName + "." + ca.Name})
			}
		}
	}
	return out
}

func detectUserEntity(ctx *ExecContext) (string, error) {
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return "", mdlerrors.NewBackend("list modules", err)
	}
	moduleNameByID := make(map[model.ID]string, len(modules))
	for _, m := range modules {
		moduleNameByID[m.ID] = m.Name
	}

	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return "", mdlerrors.NewBackend("list domain models", err)
	}

	var candidates []string
	for _, dm := range dms {
		moduleName := moduleNameByID[dm.ContainerID]
		for _, ent := range dm.Entities {
			if ent.GeneralizationRef == "System.User" {
				candidates = append(candidates, moduleName+"."+ent.Name)
			}
		}
	}

	switch len(candidates) {
	case 0:
		return "", mdlerrors.NewValidation("no entity found that generalizes System.User; use entity clause to specify one")
	case 1:
		return candidates[0], nil
	default:
		return "", mdlerrors.NewValidationf("multiple entities generalize System.User: %s; use entity clause to specify one", joinCandidates(candidates))
	}
}

func joinCandidates(candidates []string) string {
	result := candidates[0]
	for i := 1; i < len(candidates); i++ {
		result += ", " + candidates[i]
	}
	return result
}

// execDropDemoUser handles DROP DEMO USER 'name'.
func execDropDemoUser(ctx *ExecContext, s *ast.DropDemoUserStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	ps, err := ctx.Backend.GetProjectSecurity()
	if err != nil {
		return mdlerrors.NewBackend("read project security", err)
	}

	// Check if user exists
	found := false
	for _, du := range ps.DemoUsers {
		if du.UserName == s.UserName {
			found = true
			break
		}
	}
	if !found {
		return mdlerrors.NewNotFound("demo user", s.UserName)
	}

	if err := ctx.Backend.RemoveDemoUser(ps.ID, s.UserName); err != nil {
		return mdlerrors.NewBackend("drop demo user", err)
	}

	fmt.Fprintf(ctx.Output, "Dropped demo user: %s\n", s.UserName)
	return nil
}

// ============================================================================
// GRANT/REVOKE ACCESS ON ODATA SERVICE
// ============================================================================

// execGrantODataServiceAccess handles GRANT ACCESS ON ODATA SERVICE Module.Svc TO roles.
func execGrantODataServiceAccess(ctx *ExecContext, s *ast.GrantODataServiceAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the published OData service
	services, err := ctx.Backend.ListPublishedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list published OData services", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Service.Module || svc.Name != s.Service.Name {
			continue
		}

		// Reject cross-module role grants before they reach the model (CE0148 guard).
		if err := checkDocumentAccessRolesSameModule("OData service", modName, svc.Name, s.Roles); err != nil {
			return err
		}

		// Validate all roles exist
		for _, role := range s.Roles {
			if err := validateModuleRole(ctx, role); err != nil {
				return err
			}
		}

		// Merge new roles with existing (skip duplicates)
		existing := make(map[string]bool)
		var merged []string
		for _, r := range svc.AllowedModuleRoles {
			existing[r] = true
			merged = append(merged, r)
		}
		var added []string
		for _, role := range s.Roles {
			qn := role.Module + "." + role.Name
			if !existing[qn] {
				merged = append(merged, qn)
				added = append(added, qn)
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(svc.ID, merged); err != nil {
			return mdlerrors.NewBackend("update OData service access", err)
		}

		if len(added) == 0 {
			fmt.Fprintf(ctx.Output, "All specified roles already have access on OData service %s.%s\n", modName, svc.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Granted access on OData service %s.%s to %s\n", modName, svc.Name, strings.Join(added, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("published OData service", s.Service.Module+"."+s.Service.Name)
}

// execRevokeODataServiceAccess handles REVOKE ACCESS ON ODATA SERVICE Module.Svc FROM roles.
func execRevokeODataServiceAccess(ctx *ExecContext, s *ast.RevokeODataServiceAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	// Find the published OData service
	services, err := ctx.Backend.ListPublishedODataServices()
	if err != nil {
		return mdlerrors.NewBackend("list published OData services", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Service.Module || svc.Name != s.Service.Name {
			continue
		}

		// Build set of roles to remove
		toRemove := make(map[string]bool)
		for _, role := range s.Roles {
			toRemove[role.Module+"."+role.Name] = true
		}

		// Filter out removed roles
		var remaining []string
		var removed []string
		for _, r := range svc.AllowedModuleRoles {
			if toRemove[r] {
				removed = append(removed, r)
			} else {
				remaining = append(remaining, r)
			}
		}

		if err := ctx.Backend.UpdateAllowedRoles(svc.ID, remaining); err != nil {
			return mdlerrors.NewBackend("update OData service access", err)
		}

		if len(removed) == 0 {
			fmt.Fprintf(ctx.Output, "None of the specified roles had access on OData service %s.%s\n", modName, svc.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Revoked access on OData service %s.%s from %s\n", modName, svc.Name, strings.Join(removed, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("published OData service", s.Service.Module+"."+s.Service.Name)
}

// ============================================================================
// GRANT/REVOKE ACCESS ON PUBLISHED REST SERVICE
// ============================================================================

// execGrantPublishedRestServiceAccess handles GRANT ACCESS ON PUBLISHED REST SERVICE Module.Svc TO roles.
func execGrantPublishedRestServiceAccess(ctx *ExecContext, s *ast.GrantPublishedRestServiceAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	if err := checkFeature(ctx, "integration", "published_rest_grant_revoke",
		"grant access on published rest service",
		"upgrade your project to 10.0+"); err != nil {
		return err
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	services, err := ctx.Backend.ListPublishedRestServices()
	if err != nil {
		return mdlerrors.NewBackend("list published rest services", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Service.Module || svc.Name != s.Service.Name {
			continue
		}

		// Reject cross-module role grants before they reach the model (CE0148 guard).
		if err := checkDocumentAccessRolesSameModule("published REST service", modName, svc.Name, s.Roles); err != nil {
			return err
		}

		// Validate all roles exist
		for _, role := range s.Roles {
			if err := validateModuleRole(ctx, role); err != nil {
				return err
			}
		}

		// Merge new roles with existing (skip duplicates)
		existing := make(map[string]bool)
		var merged []string
		for _, r := range svc.AllowedRoles {
			existing[r] = true
			merged = append(merged, r)
		}
		var added []string
		for _, role := range s.Roles {
			qn := role.Module + "." + role.Name
			if !existing[qn] {
				merged = append(merged, qn)
				added = append(added, qn)
			}
		}

		if err := ctx.Backend.UpdatePublishedRestServiceRoles(svc.ID, merged); err != nil {
			return mdlerrors.NewBackend("update published rest service access", err)
		}

		if len(added) == 0 {
			fmt.Fprintf(ctx.Output, "All specified roles already have access on published rest service %s.%s\n", modName, svc.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Granted access on published rest service %s.%s to %s\n", modName, svc.Name, strings.Join(added, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("published rest service", s.Service.Module+"."+s.Service.Name)
}

// execRevokePublishedRestServiceAccess handles REVOKE ACCESS ON PUBLISHED REST SERVICE Module.Svc FROM roles.
func execRevokePublishedRestServiceAccess(ctx *ExecContext, s *ast.RevokePublishedRestServiceAccessStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	services, err := ctx.Backend.ListPublishedRestServices()
	if err != nil {
		return mdlerrors.NewBackend("list published rest services", err)
	}

	for _, svc := range services {
		modID := h.FindModuleID(svc.ContainerID)
		modName := h.GetModuleName(modID)
		if modName != s.Service.Module || svc.Name != s.Service.Name {
			continue
		}

		// Build set of roles to remove
		toRemove := make(map[string]bool)
		for _, role := range s.Roles {
			toRemove[role.Module+"."+role.Name] = true
		}

		// Filter out removed roles
		var remaining []string
		var removed []string
		for _, r := range svc.AllowedRoles {
			if toRemove[r] {
				removed = append(removed, r)
			} else {
				remaining = append(remaining, r)
			}
		}

		if err := ctx.Backend.UpdatePublishedRestServiceRoles(svc.ID, remaining); err != nil {
			return mdlerrors.NewBackend("update published rest service access", err)
		}

		if len(removed) == 0 {
			fmt.Fprintf(ctx.Output, "None of the specified roles had access on published rest service %s.%s\n", modName, svc.Name)
		} else {
			fmt.Fprintf(ctx.Output, "Revoked access on published rest service %s.%s from %s\n", modName, svc.Name, strings.Join(removed, ", "))
		}
		return nil
	}

	return mdlerrors.NewNotFound("published rest service", s.Service.Module+"."+s.Service.Name)
}

// execUpdateSecurity handles UPDATE SECURITY [IN Module].
func execUpdateSecurity(ctx *ExecContext, s *ast.UpdateSecurityStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	modules, err := getModulesFromCache(ctx)
	if err != nil {
		return err
	}

	totalModified := 0
	for _, mod := range modules {
		if s.Module != "" && mod.Name != s.Module {
			continue
		}

		dm, err := ctx.Backend.GetDomainModel(mod.ID)
		if err != nil {
			continue // module may not have a domain model
		}

		count, err := ctx.Backend.ReconcileMemberAccesses(dm.ID, mod.Name)
		if err != nil {
			return mdlerrors.NewBackend(fmt.Sprintf("reconcile security for module %s", mod.Name), err)
		}
		if count > 0 {
			fmt.Fprintf(ctx.Output, "Reconciled %d access rule(s) in module %s\n", count, mod.Name)
			totalModified += count
		}
	}

	if totalModified == 0 {
		fmt.Fprintf(ctx.Output, "All entity access rules are up to date\n")
	}

	return nil
}

// Executor method wrappers — delegate to free functions for callers that
// still use the Executor receiver (e.g. executor_query.go).

// inheritedAssociations returns the associations declared on entityQN's
// ancestors, qualified against the module that declares each.
//
// It walks the generalization chain the same way EntityMembersFor does, and
// stops at the same place: System.User's own members are Mendix's, and a user
// entity must not carry access entries for them.
func inheritedAssociations(ctx *ExecContext, entityQN string) []namedAssociation {
	if ctx == nil || ctx.Backend == nil {
		return nil
	}
	dms, err := ctx.Backend.ListDomainModels()
	if err != nil {
		return nil
	}
	var out []namedAssociation
	seen := map[string]bool{}
	claimed := map[string]bool{}

	current := entityQN
	for depth := 0; current != ""; depth++ {
		if seen[current] {
			break // cycle guard, as in EntityMembersFor
		}
		seen[current] = true

		ent, ok := findEntityByQN(ctx.Backend, current)
		if !ok {
			break
		}
		parent := ent.GeneralizationRef
		if parent == "" || strings.EqualFold(parent, userEntityBase) {
			break
		}
		ancestor, ok := findEntityByQN(ctx.Backend, parent)
		if !ok {
			break
		}
		ancestorModule := qualifiedModuleOf(parent)
		for _, dm := range dms {
			// Find the domain model that DECLARES the ancestor by looking for the
			// entity itself, rather than by matching module names: the module-name
			// lookup goes through the hierarchy cache and returns "" often enough
			// that filtering on it silently collected nothing (which is how the
			// first cut of this still produced CE0066).
			if !domainModelHasEntity(dm, ancestor.ID) {
				continue
			}
			collect := func(name string, parentID, childID model.ID, owner domainmodel.AssociationOwner) {
				ownedThere := parentID == ancestor.ID ||
					(owner == domainmodel.AssociationOwnerBoth && childID == ancestor.ID)
				if !ownedThere || claimed[name] {
					return
				}
				claimed[name] = true
				out = append(out, namedAssociation{Name: name, Ref: ancestorModule + "." + name})
			}
			for _, a := range dm.Associations {
				collect(a.Name, a.ParentID, a.ChildID, a.Owner)
			}
			for _, ca := range dm.CrossAssociations {
				// A cross-module association names its remote end by qualified name,
				// so the Both-owner case is matched on ChildRef rather than an ID.
				childID := model.ID("")
				if ca.Owner == domainmodel.AssociationOwnerBoth && ca.ChildRef == parent {
					childID = ancestor.ID
				}
				collect(ca.Name, ca.ParentID, childID, ca.Owner)
			}
		}
		current = parent
	}
	return out
}

// qualifiedModuleOf is the module part of "Module.Entity".
func qualifiedModuleOf(qn string) string {
	if i := strings.Index(qn, "."); i > 0 {
		return qn[:i]
	}
	return ""
}

// domainModelHasEntity reports whether a domain model declares the given entity.
func domainModelHasEntity(dm *domainmodel.DomainModel, id model.ID) bool {
	if dm == nil {
		return false
	}
	for _, e := range dm.Entities {
		if e != nil && e.ID == id {
			return true
		}
	}
	return false
}
