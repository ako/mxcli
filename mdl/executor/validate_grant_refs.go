// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// validateGrantReferences resolves what a security statement names — the
// entity, document or member it grants on, and the module and user roles it
// grants to — against the project and the statements before it.
//
// None of it was resolved at check time. `grant read * on entity System.Nope to
// M.R`, a grant to `M.NopeRole`, a grant on `M.NopeFlow` and a member list
// naming `NopeAttr` all printed "Check passed!" (measured on Evora Factory
// Management, 10.24.15). exec refuses every grant shape and both entity-revoke
// shapes — after the statements before it are written, since a script is not a
// transaction. The user-role and demo-user statements are worse: exec does not
// resolve their roles or user entity at all, and writes the dangling name into
// project security for MxBuild to report as CE1613 a build later.
//
// What check refuses is what exec refuses, plus the dangling writes; what exec
// treats as a no-op — revoking a document's access from a role that does not
// exist, dropping a module role a user role does not hold — passes here too, so
// that a cleanup script stays re-runnable after the role itself is gone.
//
// It walks the program in statement order, as exec does, so a role or entity
// created ABOVE the grant resolves and one created below it is reported with
// the reorder hint rather than as missing.
func validateGrantReferences(ctx *ExecContext, prog *ast.Program) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	g := &grantResolver{ctx: ctx, sc: newScriptContext(), whole: newScriptContext()}
	g.whole.collectDefinitions(prog)
	g.scriptModuleRoles, g.scriptUserRoles = map[string]bool{}, map[string]bool{}
	g.laterModuleRoles, g.laterUserRoles = map[string]bool{}, map[string]bool{}
	g.scriptServices, g.laterServices = map[string]bool{}, map[string]bool{}
	g.droppedModuleRoles, g.droppedUserRoles = map[string]bool{}, map[string]bool{}
	for _, stmt := range prog.Statements {
		g.recordLater(stmt)
	}

	var errs []error
	for i, stmt := range prog.Statements {
		if problems := g.check(stmt); len(problems) > 0 {
			errs = append(errs, mdlerrors.NewValidationf("statement %d: %s: %s",
				i+1, grantStatementLabel(stmt), strings.Join(problems, "; ")))
		}
		g.sc.collectSingle(stmt)
		g.record(stmt)
	}
	return errs
}

// grantResolver holds the project's names, read lazily — a script without a
// security statement reads no security at all — and what the script has
// created so far (sc and the script* sets) or creates anywhere (whole and the
// later* sets, for the reorder hint).
type grantResolver struct {
	ctx   *ExecContext
	sc    *scriptContext // statements before the current one
	whole *scriptContext // the whole program

	scriptModuleRoles, laterModuleRoles map[string]bool
	scriptUserRoles, laterUserRoles     map[string]bool // lower-cased
	scriptServices, laterServices       map[string]bool // "odata:M.S" / "rest:M.S"
	// namesRoles: some statement names a module role, so the auto-created
	// document role is worth tracking (it costs a module-security read).
	namesRoles bool
	// Roles a DROP above the statement removed, stored or not.
	droppedModuleRoles, droppedUserRoles map[string]bool

	moduleRoles   map[string]map[string]bool // module -> role names; nil value = unreadable
	userRoles     map[string]bool            // lower-cased; nil until read
	userRolesRead bool
	entities      map[string]bool
	docs          map[string]map[string]bool
}

func (g *grantResolver) recordLater(stmt ast.Statement) {
	switch stmt.(type) {
	case *ast.GrantEntityAccessStmt, *ast.RevokeEntityAccessStmt,
		*ast.GrantMicroflowAccessStmt, *ast.GrantNanoflowAccessStmt, *ast.GrantPageAccessStmt,
		*ast.GrantODataServiceAccessStmt, *ast.GrantPublishedRestServiceAccessStmt,
		*ast.CreateUserRoleStmt, *ast.AlterUserRoleStmt:
		g.namesRoles = true
	}
	switch s := stmt.(type) {
	case *ast.CreateModuleRoleStmt:
		g.laterModuleRoles[s.Name.String()] = true
	case *ast.CreateUserRoleStmt:
		g.laterUserRoles[strings.ToLower(s.Name)] = true
	case *ast.CreateODataServiceStmt:
		g.laterServices["odata:"+s.Name.String()] = true
	case *ast.CreatePublishedRestServiceStmt:
		g.laterServices["rest:"+s.Name.String()] = true
	}
}

// record notes what a statement creates once it has been checked, so the
// statements after it resolve against it. A DROP takes the role away again:
// exec would not find it either.
func (g *grantResolver) record(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.CreateModuleRoleStmt:
		g.scriptModuleRoles[s.Name.String()] = true
		delete(g.droppedModuleRoles, s.Name.String())
	case *ast.DropModuleRoleStmt:
		delete(g.scriptModuleRoles, s.Name.String())
		g.droppedModuleRoles[s.Name.String()] = true
	case *ast.CreateUserRoleStmt:
		g.scriptUserRoles[strings.ToLower(s.Name)] = true
		delete(g.droppedUserRoles, strings.ToLower(s.Name))
	case *ast.DropUserRoleStmt:
		delete(g.scriptUserRoles, strings.ToLower(s.Name))
		g.droppedUserRoles[strings.ToLower(s.Name)] = true
	case *ast.CreateODataServiceStmt:
		g.scriptServices["odata:"+s.Name.String()] = true
	case *ast.CreatePublishedRestServiceStmt:
		g.scriptServices["rest:"+s.Name.String()] = true
	case *ast.CreateMicroflowStmt:
		g.recordAutoDocumentRole(s.Name.Module)
	case *ast.CreateNanoflowStmt:
		g.recordAutoDocumentRole(s.Name.Module)
	case *ast.CreatePageStmtV3:
		g.recordAutoDocumentRole(s.Name.Module)
	}
}

// recordAutoDocumentRole mirrors defaultDocumentAccessRoles: creating a
// microflow, nanoflow or page in a module with no module roles at all creates
// <Module>.User and grants the document to it. A script that creates a module,
// its flows and then grants them to <Module>.User relies on exactly that
// (doctype-tests/02b), so the role exists from that statement on.
func (g *grantResolver) recordAutoDocumentRole(module string) {
	if module == "" || !g.namesRoles {
		return
	}
	auto := module + "." + autoDocumentRoleName
	if g.scriptModuleRoles[auto] {
		return
	}
	prefix := module + "."
	for name := range g.scriptModuleRoles {
		if strings.HasPrefix(name, prefix) {
			return // the script gave the module a role of its own
		}
	}
	stored, answered := g.rolesOfModule(module)
	if !answered {
		return
	}
	for name := range stored {
		if !g.droppedModuleRoles[prefix+name] {
			return
		}
	}
	g.scriptModuleRoles[auto] = true
	delete(g.droppedModuleRoles, auto)
}

// check returns the unresolved references of one statement, in the order the
// statement names them; nil for a statement this pass does not cover.
func (g *grantResolver) check(stmt ast.Statement) []string {
	var out []string
	add := func(msg string) {
		if msg != "" {
			out = append(out, msg)
		}
	}
	// strictRoles: the statement's exec refuses an unknown role. A document
	// revoke does not — it reports that the role had no access — so neither
	// does this.
	roles := func(rs []ast.QualifiedName, strictRoles bool) {
		if !strictRoles {
			return
		}
		for _, r := range rs {
			add(g.moduleRoleProblem(r))
		}
	}
	switch s := stmt.(type) {
	case *ast.GrantEntityAccessStmt:
		entityOK := g.entityTargetProblem(s.Entity, &out)
		roles(s.Roles, true)
		if entityOK {
			add(g.memberProblem(s.Entity, s.Rights))
		}
	case *ast.RevokeEntityAccessStmt:
		g.entityTargetProblem(s.Entity, &out)
		roles(s.Roles, true)
	case *ast.GrantMicroflowAccessStmt:
		add(g.documentProblem("microflow", s.Microflow))
		roles(s.Roles, true)
	case *ast.RevokeMicroflowAccessStmt:
		add(g.documentProblem("microflow", s.Microflow))
		roles(s.Roles, false)
	case *ast.GrantNanoflowAccessStmt:
		add(g.documentProblem("nanoflow", s.Nanoflow))
		roles(s.Roles, true)
	case *ast.RevokeNanoflowAccessStmt:
		add(g.documentProblem("nanoflow", s.Nanoflow))
		roles(s.Roles, false)
	case *ast.GrantPageAccessStmt:
		add(g.documentProblem("page", s.Page))
		roles(s.Roles, true)
	case *ast.RevokePageAccessStmt:
		add(g.documentProblem("page", s.Page))
		roles(s.Roles, false)
	case *ast.GrantODataServiceAccessStmt:
		add(g.documentProblem("published OData service", s.Service))
		roles(s.Roles, true)
	case *ast.RevokeODataServiceAccessStmt:
		add(g.documentProblem("published OData service", s.Service))
		roles(s.Roles, false)
	case *ast.GrantPublishedRestServiceAccessStmt:
		add(g.documentProblem("published REST service", s.Service))
		roles(s.Roles, true)
	case *ast.RevokePublishedRestServiceAccessStmt:
		add(g.documentProblem("published REST service", s.Service))
		roles(s.Roles, false)
	case *ast.CreateUserRoleStmt:
		// exec stores these names without resolving them (CE1613 at build).
		roles(s.ModuleRoles, true)
		for _, ur := range s.ManageableRoles {
			if !strings.EqualFold(ur, s.Name) {
				add(g.userRoleProblem(ur))
			}
		}
	case *ast.AlterUserRoleStmt:
		add(g.userRoleProblem(s.Name))
		// ADD writes the names unresolved; DROP of a role the user role does
		// not hold is reported as unchanged by exec.
		roles(s.ModuleRoles, s.Add)
	case *ast.CreateDemoUserStmt:
		for _, ur := range s.UserRoles {
			add(g.userRoleProblem(ur))
		}
		if s.Entity != "" {
			if qn, ok := splitQualified(s.Entity); ok && !g.entityExists(qn) {
				add(g.missing("entity", qn.String(), g.whole.producesEntity(qn)) +
					" (the demo user's entity, normally a specialization of System.User)")
			}
		}
	case *ast.AlterProjectSecurityStmt:
		if s.GuestUserRole != "" {
			add(g.userRoleProblem(s.GuestUserRole))
		}
	}
	return out
}

// missing words one unresolved name, with the reorder hint when the script
// creates it further down — exec resolves in statement order, so that is the
// fix, not a rename.
func (g *grantResolver) missing(kind, name string, createdLater bool) string {
	msg := fmt.Sprintf("%s %s does not exist", kind, name)
	if createdLater {
		msg += " yet — it is created later in this script; move that create statement above this one"
	}
	return msg
}

// entityTargetProblem reports an entity grant's target, and whether the entity
// resolved (so the member list can be checked against it).
func (g *grantResolver) entityTargetProblem(qn ast.QualifiedName, out *[]string) bool {
	if qn.Module == "" || qn.Name == "" {
		return false
	}
	// exec refuses every System entity, existing or not: the System domain
	// model is not stored in the project, so there is no rule to add to.
	if strings.EqualFold(qn.Module, systemModuleName) {
		if !g.entityExists(qn) {
			*out = append(*out, g.missing("entity", qn.String(), false))
			return false
		}
		*out = append(*out, refuseSystemEntityGrant(qn.Module, qn.Name).Error())
		return false
	}
	if g.entityExists(qn) {
		return true
	}
	msg := g.missing("entity", qn.String(), g.whole.producesEntity(qn))
	if hint := entityNameHint(qn); hint != "" {
		msg += ". " + hint
	}
	*out = append(*out, msg)
	return false
}

func (g *grantResolver) entityExists(qn ast.QualifiedName) bool {
	if g.sc.producesEntity(qn) {
		return true
	}
	if g.entities == nil {
		g.entities = buildEntityQualifiedNames(g.ctx)
	}
	if len(g.entities) == 0 {
		return true // a backend that lists none cannot say one is missing
	}
	if strings.EqualFold(qn.Module, systemModuleName) && !g.listsModule(systemModuleName) {
		return true // nor can one that does not expose System
	}
	return g.entities[qn.String()]
}

func (g *grantResolver) listsModule(module string) bool {
	prefix := module + "."
	for name := range g.entities {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// documentProblem resolves a document-access grant's target.
func (g *grantResolver) documentProblem(kind string, qn ast.QualifiedName) string {
	if qn.Module == "" || qn.Name == "" {
		return ""
	}
	name := qn.String()
	var inScript, later bool
	switch kind {
	case "microflow":
		inScript, later = g.sc.microflows[name], g.whole.microflows[name]
	case "nanoflow":
		inScript, later = g.sc.nanoflows[name], g.whole.nanoflows[name]
	case "page":
		inScript, later = g.sc.pages[name], g.whole.pages[name]
	case "published OData service":
		inScript, later = g.scriptServices["odata:"+name], g.laterServices["odata:"+name]
	case "published REST service":
		inScript, later = g.scriptServices["rest:"+name], g.laterServices["rest:"+name]
	}
	if inScript {
		return ""
	}
	stored := g.documents(kind)
	if stored == nil || stored[name] {
		return ""
	}
	return g.missing(kind, name, later)
}

// documents lists the project's documents of a kind, matched exactly as the
// grant executors match them (module name and document name). nil means the
// backend could not answer, which is not evidence that anything is missing.
func (g *grantResolver) documents(kind string) map[string]bool {
	if g.docs == nil {
		g.docs = map[string]map[string]bool{}
	}
	if m, ok := g.docs[kind]; ok {
		return m
	}
	var m map[string]bool
	switch kind {
	case "microflow":
		m = buildMicroflowQualifiedNames(g.ctx)
	case "nanoflow":
		m = buildNanoflowQualifiedNames(g.ctx)
	case "page":
		m = buildPageQualifiedNames(g.ctx)
	case "published OData service", "published REST service":
		m = g.serviceNames(kind == "published OData service")
	}
	// An empty microflow or page listing is a backend that could not answer as
	// often as a project without any (cf. validateForwardDefRefs). Services and
	// nanoflows are legitimately absent from many projects, and their listings
	// return an error rather than nothing when they fail.
	if len(m) == 0 && (kind == "microflow" || kind == "page") {
		m = nil
	}
	g.docs[kind] = m
	return m
}

func (g *grantResolver) serviceNames(odata bool) map[string]bool {
	h, err := getHierarchy(g.ctx)
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	if odata {
		svcs, err := g.ctx.Backend.ListPublishedODataServices()
		if err != nil {
			return nil
		}
		for _, s := range svcs {
			out[h.GetModuleName(h.FindModuleID(s.ContainerID))+"."+s.Name] = true
		}
		return out
	}
	svcs, err := g.ctx.Backend.ListPublishedRestServices()
	if err != nil {
		return nil
	}
	for _, s := range svcs {
		out[h.GetModuleName(h.FindModuleID(s.ContainerID))+"."+s.Name] = true
	}
	return out
}

// moduleRoleProblem resolves a module role the way validateModuleRole does —
// exact module and role name — against the project and the script so far. An
// unqualified role is MDL-GRANT02's to report, so it is left alone here.
func (g *grantResolver) moduleRoleProblem(r ast.QualifiedName) string {
	if r.Module == "" || r.Name == "" {
		return ""
	}
	name := r.String()
	if g.scriptModuleRoles[name] {
		return ""
	}
	known, answered := g.rolesOfModule(r.Module)
	if !answered || (known[r.Name] && !g.droppedModuleRoles[name]) {
		return ""
	}
	// System's module roles (System.User, System.Administrator) are not stored
	// in a module security of the project's; a backend that does not list them
	// cannot say one is missing.
	if strings.EqualFold(r.Module, systemModuleName) && len(known) == 0 {
		return ""
	}
	msg := g.missing("module role", name, g.laterModuleRoles[name])
	if !g.laterModuleRoles[name] {
		var have []string
		for n := range known {
			if !g.droppedModuleRoles[r.Module+"."+n] {
				have = append(have, n)
			}
		}
		for n := range g.scriptModuleRoles {
			if rest, ok := strings.CutPrefix(n, r.Module+"."); ok && !known[rest] {
				have = append(have, rest)
			}
		}
		if len(have) > 0 {
			sort.Strings(have)
			msg += fmt.Sprintf(" (module %s has: %s)", r.Module, strings.Join(have, ", "))
		}
	}
	return msg
}

// rolesOfModule reads one module's roles — including the auto-created document
// role buildModuleRoleQualifiedNames leaves out, which a grant may name like
// any other. Read per module and only for the modules a statement names:
// reading every module's security costs seconds on a large project.
//
// answered is false when the backend could not say (security unreadable), which
// is not evidence that a role is missing. A module that does not exist answers
// with no roles, as validateModuleRole refuses it.
func (g *grantResolver) rolesOfModule(module string) (roles map[string]bool, answered bool) {
	if g.moduleRoles == nil {
		g.moduleRoles = map[string]map[string]bool{}
	}
	if m, ok := g.moduleRoles[module]; ok {
		return m, m != nil
	}
	roles = map[string]bool{}
	if mod, err := findModule(g.ctx, module); err == nil {
		ms, err := g.ctx.Backend.GetModuleSecurity(mod.ID)
		if err != nil {
			roles = nil
		} else if ms != nil {
			for _, mr := range ms.ModuleRoles {
				if mr != nil {
					roles[mr.Name] = true
				}
			}
		}
	} else if strings.EqualFold(module, systemModuleName) {
		roles = map[string]bool{} // System not exposed as a module: accepted above
	}
	g.moduleRoles[module] = roles
	return roles, roles != nil
}

// userRoleProblem resolves a user role — project-level, no module part.
// Matched case-insensitively, as the guest-role executor matches it; the
// stricter executors would refuse a case mismatch with their own message.
func (g *grantResolver) userRoleProblem(name string) string {
	if name == "" || g.scriptUserRoles[strings.ToLower(name)] {
		return ""
	}
	if !g.userRolesRead {
		g.userRolesRead = true
		if known, err := projectUserRoles(g.ctx); err == nil {
			g.userRoles = map[string]bool{}
			for _, n := range known {
				g.userRoles[strings.ToLower(n)] = true
			}
		}
	}
	if g.userRoles == nil || (g.userRoles[strings.ToLower(name)] && !g.droppedUserRoles[strings.ToLower(name)]) {
		return ""
	}
	msg := g.missing("user role", name, g.laterUserRoles[strings.ToLower(name)])
	if strings.Contains(name, ".") {
		msg += " (a user role is project-level and has no module part)"
	}
	return msg
}

// memberProblem resolves the members a GRANT names in READ (…) / WRITE (…)
// against the entity's grantable members, which execGrantEntityAccess refuses
// with "entity … has no member(s) …". Only a stored entity is judged, and not
// one the script alters: a member list built from a declaration or an altered
// entity would be a second, drifting copy of exec's.
func (g *grantResolver) memberProblem(qn ast.QualifiedName, rights []ast.EntityAccessRight) string {
	name := qn.String()
	if g.whole.entities[name] || g.whole.indirectEntities[name] || g.whole.alteredEntities[name] {
		return ""
	}
	var readMembers, writeMembers []string
	for _, r := range rights {
		switch r.Type {
		case ast.EntityAccessReadMembers:
			readMembers = append(readMembers, r.Members...)
		case ast.EntityAccessWriteMembers:
			writeMembers = append(writeMembers, r.Members...)
		}
	}
	if len(readMembers) == 0 && len(writeMembers) == 0 {
		return ""
	}
	members, ok := grantableMemberNames(g.ctx, qn)
	if !ok {
		return ""
	}
	// An association the script declares becomes a member exec will find.
	for assoc := range g.whole.associations {
		members[assoc] = true
	}
	unknown := unmatchedGrantMembers(readMembers, writeMembers, members)
	if len(unknown) == 0 {
		return ""
	}
	return fmt.Sprintf("entity %s has no member(s) %s; grant only names members of the entity or of an entity it inherits from",
		name, strings.Join(unknown, ", "))
}

// grantableMemberNames is the set of member names execGrantEntityAccess builds
// a MemberAccess for (or accepts, for the audit members): the entity's own and
// inherited attributes, its audit members, the associations it owns — FROM
// side, or either side of an `owner both` one, cross-module included — and the
// associations it inherits. false when the entity cannot be read.
func grantableMemberNames(ctx *ExecContext, qn ast.QualifiedName) (map[string]bool, bool) {
	module, err := findModule(ctx, qn.Module)
	if err != nil {
		return nil, false
	}
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil || dm == nil {
		return nil, false
	}
	entity := dm.FindEntityByName(qn.Name)
	if entity == nil {
		return nil, false
	}
	entityQN := module.Name + "." + qn.Name
	names := map[string]bool{}
	for _, mem := range EntityMembers(ctx, entityQN) {
		names[mem.Name] = true
	}
	for _, sys := range storedAuditMembers(entity) {
		names[sys] = true
	}
	for _, assoc := range dm.Associations {
		if assoc.ParentID == entity.ID ||
			(assoc.Owner == domainmodel.AssociationOwnerBoth && assoc.ChildID == entity.ID) {
			names[assoc.Name] = true
		}
	}
	for _, ca := range dm.CrossAssociations {
		if ca.ParentID == entity.ID {
			names[ca.Name] = true
		}
	}
	for _, other := range otherModuleBothOwnerAssociations(ctx, module.Name, entityQN) {
		names[other.Name] = true
	}
	for _, inh := range inheritedAssociations(ctx, entityQN) {
		names[inh.Name] = true
	}
	return names, true
}

// splitQualified parses "Module.Name" into a qualified name.
func splitQualified(s string) (ast.QualifiedName, bool) {
	i := strings.Index(s, ".")
	if i <= 0 || i == len(s)-1 {
		return ast.QualifiedName{}, false
	}
	return ast.QualifiedName{Module: s[:i], Name: s[i+1:]}, true
}

// grantStatementLabel names the statement in the error, in MDL's own words, so
// the reader can find it in a long security script without counting.
func grantStatementLabel(stmt ast.Statement) string {
	roleList := func(rs []ast.QualifiedName) string {
		names := make([]string, len(rs))
		for i, r := range rs {
			names[i] = r.String()
		}
		return strings.Join(names, ", ")
	}
	switch s := stmt.(type) {
	case *ast.GrantEntityAccessStmt:
		return fmt.Sprintf("grant on entity %s to %s", s.Entity.String(), roleList(s.Roles))
	case *ast.RevokeEntityAccessStmt:
		return fmt.Sprintf("revoke on entity %s from %s", s.Entity.String(), roleList(s.Roles))
	case *ast.GrantMicroflowAccessStmt:
		return fmt.Sprintf("grant execute on microflow %s to %s", s.Microflow.String(), roleList(s.Roles))
	case *ast.RevokeMicroflowAccessStmt:
		return fmt.Sprintf("revoke execute on microflow %s from %s", s.Microflow.String(), roleList(s.Roles))
	case *ast.GrantNanoflowAccessStmt:
		return fmt.Sprintf("grant execute on nanoflow %s to %s", s.Nanoflow.String(), roleList(s.Roles))
	case *ast.RevokeNanoflowAccessStmt:
		return fmt.Sprintf("revoke execute on nanoflow %s from %s", s.Nanoflow.String(), roleList(s.Roles))
	case *ast.GrantPageAccessStmt:
		return fmt.Sprintf("grant view on page %s to %s", s.Page.String(), roleList(s.Roles))
	case *ast.RevokePageAccessStmt:
		return fmt.Sprintf("revoke view on page %s from %s", s.Page.String(), roleList(s.Roles))
	case *ast.GrantODataServiceAccessStmt:
		return fmt.Sprintf("grant access on published odata service %s to %s", s.Service.String(), roleList(s.Roles))
	case *ast.RevokeODataServiceAccessStmt:
		return fmt.Sprintf("revoke access on published odata service %s from %s", s.Service.String(), roleList(s.Roles))
	case *ast.GrantPublishedRestServiceAccessStmt:
		return fmt.Sprintf("grant access on published rest service %s to %s", s.Service.String(), roleList(s.Roles))
	case *ast.RevokePublishedRestServiceAccessStmt:
		return fmt.Sprintf("revoke access on published rest service %s from %s", s.Service.String(), roleList(s.Roles))
	case *ast.CreateUserRoleStmt:
		return fmt.Sprintf("create user role %s", s.Name)
	case *ast.AlterUserRoleStmt:
		return fmt.Sprintf("alter user role %s", s.Name)
	case *ast.CreateDemoUserStmt:
		return fmt.Sprintf("create demo user '%s'", s.UserName)
	case *ast.AlterProjectSecurityStmt:
		return "alter app security"
	}
	return "statement"
}
