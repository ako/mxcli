// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// `check --references` resolved the DOCUMENT and the ENTITY a statement names,
// and then stopped at the door. A member name inside one — the attribute on the
// left of a CREATE / CHANGE assignment — was never resolved against the entity
// it belongs to, so a typo passed check, passed exec, and surfaced at the far
// end of a build as CE1613 (mendixlabs/mxcli#1048):
//
//	CHANGE $Order ("IsArchived" = true);   -- no such attribute
//	  mxcli check --references             -> exit 0
//	  mxcli exec                           -> "Created microflow"
//	  mx check                             -> [CE1613] "The selected attribute
//	                                          'Bench.Order.IsArchived' no longer exists."
//
// exec does resolve the name — resolveAttributeInEntityHierarchy, in the flow
// builder — but when resolution FAILS it falls back to writing
// `<entityQN>.<memberName>` anyway. That fabricated identifier is what mxbuild
// rejects. This check reports the same failure at the point it is cheap to fix.
//
// # Why this cannot simply mirror exec's resolver
//
// exec's resolver answers a two-valued question (resolved / did not resolve)
// because it has a fallback either way: it writes the fabricated name and lets
// the build complain. A CHECK has no such luxury — "did not resolve" conflates a
// real typo with a lookup that could not be PERFORMED, and reporting the second
// is a false error that blocks a script which would have worked. So the walk
// below has three outcomes and reports only the middle one.
//
// What that guard is and is not for, measured rather than assumed:
//
//   - INHERITANCE is not the case. The walk follows GeneralizationRef through
//     the backend, and the System module answers normally that way:
//     System.FileDocument loads with its 6 attributes, so `CHANGE $File (Name =
//     …)` on an entity extending it resolves, while a name on no link of the
//     chain is correctly reported. (`update security` fails on System for an
//     unrelated reason — it loads the domain-model UNIT BY ID out of
//     mprcontents, where System has no file. Different mechanism, and not this
//     one; mendixlabs/mxcli#1047.)
//
//   - A BACKEND THAT CANNOT ANSWER is the case. mxcli has more than one
//     (MPR, MCP/PED, mock), the interface permits an error from every lookup,
//     and a check that turns "I could not look" into "your attribute is
//     missing" fails every script against such a backend. That is what
//     memberUnknown covers, and TestResolveMemberOnEntity_SilentWhenTheModelCannotBeRead
//     is the control: flip that branch to memberMissing and it reports.
type memberResolution int

const (
	// memberFound: the name is an attribute of the entity or of one of its
	// generalizations.
	memberFound memberResolution = iota
	// memberMissing: the whole chain was walked to its root and the name is not
	// there. Only this is reported.
	memberMissing
	// memberUnknown: a link in the chain could not be loaded, so the question
	// was never answered. Silent.
	memberUnknown
)

// resolveMemberOnEntity walks entityQN and its generalizations looking for a
// member called memberName.
//
// Matching is case-sensitive, as Mendix's own is: mxbuild rejects `orderno` for
// `OrderNo` with the same CE1613, so accepting it here would let through the
// error this exists to catch.
func resolveMemberOnEntity(ctx *ExecContext, entityQN, memberName string) memberResolution {
	b, ok := ctx.Backend.(entityLookupBackend)
	if !ok || entityQN == "" || memberName == "" {
		return memberUnknown
	}
	seen := map[string]bool{}
	for currentQN := entityQN; currentQN != ""; {
		if seen[currentQN] {
			// A generalization cycle is not a model mxcli should reason about.
			return memberUnknown
		}
		seen[currentQN] = true

		parts := strings.SplitN(currentQN, ".", 2)
		if len(parts) != 2 {
			return memberUnknown
		}
		mod, err := b.GetModuleByName(parts[0])
		if err != nil || mod == nil {
			return memberUnknown
		}
		dm, err := b.GetDomainModel(mod.ID)
		if err != nil || dm == nil {
			// The System module lands here. Silence, not a report.
			return memberUnknown
		}
		entity := dm.FindEntityByName(parts[1])
		if entity == nil {
			return memberUnknown
		}
		for _, attr := range entity.Attributes {
			if attr != nil && attr.Name == memberName {
				return memberFound
			}
		}
		// An association is a legal member of a CREATE / CHANGE too, and it is
		// named on the entity that OWNS it rather than on either end, so it is
		// checked against the module's whole association list rather than the
		// entity's attributes.
		if associationNamedInModule(dm, memberName) {
			return memberFound
		}
		currentQN = entity.GeneralizationRef
	}
	return memberMissing
}

// associationNamedInModule reports whether the domain model has an association
// of this bare name. A qualified member is handled by the caller; this covers
// the unqualified spelling, which resolves in the entity's own module.
func associationNamedInModule(dm *domainmodel.DomainModel, name string) bool {
	for _, a := range dm.Associations {
		if a != nil && a.Name == name {
			return true
		}
	}
	for _, a := range dm.CrossAssociations {
		if a != nil && a.Name == name {
			return true
		}
	}
	return false
}

// associationTargetFrom returns the entity at the other end of an association
// traversed from fromEntityQN.
//
// ParentID is the FROM entity (the foreign-key owner) and ChildID the TO entity
// — Mendix's inverted naming, per CLAUDE.md — and a retrieve may traverse from
// either end, so both directions resolve.
//
// A start entity that matches neither end returns false rather than guessing.
// That happens when the starting variable is a SPECIALISATION of the end, which
// this deliberately does not chase: the cost of being wrong is a false error on
// a working script, and the cost of being silent is one unchecked member.
func associationTargetFrom(ctx *ExecContext, assocQN, fromEntityQN string) (string, bool) {
	if assocQN == "" || fromEntityQN == "" {
		return "", false
	}
	b, ok := ctx.Backend.(entityLookupBackend)
	if !ok {
		return "", false
	}
	parts := strings.SplitN(assocQN, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	mod, err := b.GetModuleByName(parts[0])
	if err != nil || mod == nil {
		return "", false
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return "", false
	}
	for _, a := range dm.Associations {
		if a == nil || a.Name != parts[1] {
			continue
		}
		from := entityQNByID(ctx, a.ParentID)
		to := entityQNByID(ctx, a.ChildID)
		switch fromEntityQN {
		case from:
			return to, to != ""
		case to:
			return from, from != ""
		}
		return "", false
	}
	return "", false
}

// validateMemberReferences reports CREATE / CHANGE member names that do not
// exist on the entity they are assigned to.
//
// Runs in the --references pass: it needs the project, and the whole point is to
// answer a question `mxcli check` without -p cannot.
func validateMemberReferences(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []string {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	authored := authoredMembers(prog)
	var out []string
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			out = append(out, checkFlowMembers(ctx, sc, authored,
				s.Name.String(), s.Parameters, s.Body)...)
		case *ast.CreateNanoflowStmt:
			out = append(out, checkFlowMembers(ctx, sc, authored,
				s.Name.String(), s.Parameters, s.Body)...)
		}
	}
	return out
}

// checkFlowMembers walks one flow body, tracking which entity each variable
// holds so a CHANGE can be resolved at all.
func checkFlowMembers(ctx *ExecContext, sc *scriptContext, authored map[string]bool,
	flowQN string, params []ast.MicroflowParam, body []ast.MicroflowStatement) []string {

	vars := map[string]string{}
	for _, p := range params {
		if p.Name == "" {
			continue
		}
		// A bare qualified name parses as TypeEnumeration with EnumRef set (see
		// CLAUDE.md), so both spellings are recorded rather than guessed
		// between. A name that is really an enumeration resolves no entity and
		// is skipped, so the wrong guess costs nothing.
		switch {
		case p.Type.EntityRef != nil:
			vars[p.Name] = p.Type.EntityRef.String()
		case p.Type.Kind == ast.TypeEnumeration && p.Type.EnumRef != nil:
			vars[p.Name] = p.Type.EnumRef.String()
		}
	}

	var out []string
	var walk func(stmts []ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, st := range stmts {
			switch s := st.(type) {
			case *ast.CreateObjectStmt:
				if s.Variable != "" {
					vars[s.Variable] = s.EntityType.String()
				}
				out = append(out, checkMembers(ctx, sc, authored, flowQN,
					fmt.Sprintf("create %s", s.EntityType.String()),
					s.EntityType.String(), s.Changes)...)
			case *ast.ChangeObjectStmt:
				entityQN := vars[s.Variable]
				out = append(out, checkMembers(ctx, sc, authored, flowQN,
					fmt.Sprintf("change $%s", s.Variable), entityQN, s.Changes)...)
			case *ast.RetrieveStmt:
				// Source is the ENTITY for a database retrieve and the
				// ASSOCIATION for an association retrieve (StartVariable set).
				// Both are modelled: an association retrieve feeding a LOOP is
				// the ordinary bulk-update shape, and leaving it untyped made
				// the check silent on the commonest place a member is named.
				switch {
				case s.Variable == "" || s.Source.Name == "":
				case s.StartVariable == "":
					vars[s.Variable] = s.Source.String()
				default:
					if from, ok := vars[strings.TrimPrefix(s.StartVariable, "$")]; ok {
						if to, ok := associationTargetFrom(ctx, s.Source.String(), from); ok {
							vars[s.Variable] = to
						}
					}
				}
			case *ast.IfStmt:
				walk(s.ThenBody)
				walk(s.ElseBody)
			case *ast.WhileStmt:
				walk(s.Body)
			case *ast.LoopStmt:
				// The iterator holds the list's element type; without this a
				// CHANGE on the loop variable is unresolvable and silent.
				if s.LoopVariable != "" && s.ListVariable != "" {
					if qn, ok := vars[s.ListVariable]; ok {
						vars[s.LoopVariable] = qn
					}
				}
				walk(s.Body)
			}
			if eh := stmtErrorHandling(st); eh != nil && len(eh.Body) > 0 {
				walk(eh.Body)
			}
		}
	}
	walk(body)
	return out
}

// checkMembers resolves each member of one CREATE / CHANGE.
func checkMembers(ctx *ExecContext, sc *scriptContext, authored map[string]bool,
	flowQN, where, entityQN string, changes []ast.ChangeItem) []string {

	// No entity established (an untyped variable, an enumeration parameter, a
	// retrieve this walk did not model) — nothing to resolve against.
	if entityQN == "" || !strings.Contains(entityQN, ".") {
		return nil
	}
	// NOTE: there is deliberately no "skip entities the script creates" guard.
	// It looks necessary and is not, and having it cost the check most of its
	// reach: a self-contained script defines its own entities, so the guard
	// skipped the whole file — including the example written to demonstrate
	// this very check, which passed while exercising nothing.
	//
	// Two mechanisms already cover the case without the loss. An attribute the
	// script DECLARES is in `authored`. An entity that is not in the project at
	// all resolves to memberUnknown (FindEntityByName returns nil), which is
	// silent. What is left over — an entity the project has, and a member
	// neither it nor the script declares — is exactly the defect.

	var out []string
	for _, ch := range changes {
		name := strings.Trim(ch.Attribute, `"`)
		if name == "" {
			continue
		}
		// A qualified member is a different question with a different answer,
		// and exec already refuses the one-qualifier form that cannot be an
		// attribute (FINDINGS #51). Left alone here rather than second-guessed.
		if strings.Contains(name, ".") {
			continue
		}
		if authored[strings.ToLower(entityQN+"."+name)] {
			continue
		}
		if resolveMemberOnEntity(ctx, entityQN, name) != memberMissing {
			continue
		}
		out = append(out, fmt.Sprintf(
			"%s: %s has no member %q (in %s)%s — mxbuild reports this as CE1613 "+
				"\"The selected attribute '%s.%s' no longer exists\"",
			flowQN, entityQN, name, where, nearMembers(ctx, entityQN), entityQN, name))
	}
	return out
}

// authoredMembers collects every attribute the SCRIPT itself declares, so a
// script that adds an attribute and then assigns it is not reported.
//
// Without this the check fires on the ordinary shape of a migration script —
// add the column, then populate it — which is the change most likely to be
// written against an entity that does not yet have the member.
func authoredMembers(prog *ast.Program) map[string]bool {
	out := map[string]bool{}
	add := func(entityQN, attr string) {
		if entityQN != "" && attr != "" {
			out[strings.ToLower(entityQN+"."+strings.Trim(attr, `"`))] = true
		}
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateEntityStmt:
			for _, a := range s.Attributes {
				add(s.Name.String(), a.Name)
			}
		case *ast.AlterEntityStmt:
			if s.Operation == ast.AlterEntityAddAttribute && s.Attribute != nil {
				add(s.Name.String(), s.Attribute.Name)
			}
			// A rename makes the NEW name legal for the rest of the script.
			if s.Operation == ast.AlterEntityRenameAttribute {
				add(s.Name.String(), s.NewName)
			}
		}
	}
	return out
}

// nearMembers lists what the entity does have, capped so a wide entity does not
// bury the error. A typo is cheap to fix only when you can see what is there.
func nearMembers(ctx *ExecContext, entityQN string) string {
	const max = 10
	b, ok := ctx.Backend.(entityLookupBackend)
	if !ok {
		return ""
	}
	parts := strings.SplitN(entityQN, ".", 2)
	if len(parts) != 2 {
		return ""
	}
	mod, err := b.GetModuleByName(parts[0])
	if err != nil || mod == nil {
		return ""
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return ""
	}
	entity := dm.FindEntityByName(parts[1])
	if entity == nil {
		return ""
	}
	var names []string
	for _, a := range entity.Attributes {
		if a != nil {
			names = append(names, a.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	if len(names) > max {
		return fmt.Sprintf(" — it has %s and %d more",
			strings.Join(names[:max], ", "), len(names)-max)
	}
	return fmt.Sprintf(" — it has %s", strings.Join(names, ", "))
}
