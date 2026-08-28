// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// userEntityBase is the Mendix entity whose specializations are "user entities".
// Its members are managed by the platform (login, blocking, password), so they do
// NOT belong in a specializing entity's access rule — see EntityMembers.
const userEntityBase = "System.User"

// EntityMember is one member of an entity's access surface: an attribute or an
// association, together with the qualified reference Mendix stores for it.
type EntityMember struct {
	Name string // bare member name, as written in GRANT
	// Ref is the reference stored in MemberAccess, qualified against the entity
	// that DECLARES the member — which is an ancestor for an inherited one.
	Ref          string
	Inherited    bool
	IsCalculated bool
}

// EntityMembers returns every member of an entity's access surface: its own
// attributes plus those inherited through the generalization chain, each carrying
// the reference Mendix expects in a MemberAccess entry.
//
// Two rules here are load-bearing, both established against `mx check` rather than
// inferred (mendixlabs/mxcli#758, #765):
//
//  1. An inherited member's reference is qualified against the entity that
//     DECLARES it, not the entity carrying the rule. Writing the child's name
//     produces CE1613 "The selected attribute ... no longer exists"; writing the
//     declaring entity's name validates clean. This is the same rule the
//     change-object writer needs (#451).
//
//  2. Members inherited from System.User are excluded. Mendix manages the platform
//     members of a user entity, and listing them turns a clean rule into CE0066 —
//     verified both on Mendix's own Administration.Account and on a fresh
//     specialization. Every other ancestor's members are REQUIRED: omitting the
//     six System.FileDocument members from a specializing entity's rule is CE0066
//     until they are all present.
//
// Ancestors that cannot be resolved (module not in the project) stop the walk; the
// members found so far are returned rather than nothing, so a partial model still
// produces a usable rule.
func EntityMembers(ctx *ExecContext, entityQN string) []EntityMember {
	if ctx == nil {
		return nil
	}
	return EntityMembersFor(ctx.Backend, entityQN)
}

// entityLookupBackend is the slice of the backend the generalization walk needs:
// resolve a module by name, then read its domain model.
type entityLookupBackend interface {
	backend.ModuleBackend
	backend.DomainModelBackend
}

// EntityMembersFor is EntityMembers against a backend directly, for callers that
// hold one without an ExecContext (the mapping builders).
func EntityMembersFor(b entityLookupBackend, entityQN string) []EntityMember {
	var out []EntityMember
	seen := map[string]bool{}    // cycle guard
	claimed := map[string]bool{} // a child's member shadows the ancestor's

	for currentQN, depth := entityQN, 0; currentQN != ""; depth++ {
		if seen[currentQN] {
			break
		}
		seen[currentQN] = true

		// Stop before collecting System.User's own members: its specializations are
		// user entities, whose platform members Mendix owns.
		if depth > 0 && strings.EqualFold(currentQN, userEntityBase) {
			break
		}

		entity, ok := findEntityByQN(b, currentQN)
		if !ok {
			break
		}

		for _, attr := range entity.Attributes {
			if attr == nil || claimed[attr.Name] {
				continue
			}
			claimed[attr.Name] = true
			out = append(out, EntityMember{
				Name:         attr.Name,
				Ref:          currentQN + "." + attr.Name,
				Inherited:    depth > 0,
				IsCalculated: attr.Value != nil && attr.Value.Type == "CalculatedValue",
			})
		}
		currentQN = entity.GeneralizationRef
	}
	return out
}

// findEntityByQN resolves a qualified entity name through the backend. The module
// is resolved by name rather than scanning every domain model, so a same-named
// entity in another module cannot be picked up by accident.
func findEntityByQN(b entityLookupBackend, entityQN string) (*domainmodel.Entity, bool) {
	if b == nil {
		return nil, false
	}
	parts := strings.SplitN(entityQN, ".", 2)
	if len(parts) != 2 {
		return nil, false
	}
	mod, err := b.GetModuleByName(parts[0])
	if err != nil || mod == nil {
		return nil, false
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		return nil, false
	}
	entity := dm.FindEntityByName(parts[1])
	if entity == nil {
		return nil, false
	}
	return entity, true
}

// unmatchedGrantMembers returns the members named in a GRANT that matched no
// attribute or association of the entity, in a stable order.
func unmatchedGrantMembers(readMembers, writeMembers []string, granted map[string]bool) []string {
	var unknown []string
	seen := map[string]bool{}
	for _, list := range [][]string{readMembers, writeMembers} {
		for _, name := range list {
			if name == "" || granted[name] || seen[name] {
				continue
			}
			seen[name] = true
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// ResolveMemberRef returns the reference Mendix stores for a member of an entity,
// qualified against the entity that DECLARES it — which is an ancestor when the
// member is inherited. Reports false when the entity has no such member.
//
// Qualifying an inherited member against the entity that merely uses it produces
// CE1613 "The selected attribute ... no longer exists": in an access rule
// (mendixlabs/mxcli#758) and equally in an import/export mapping, where Studio Pro
// additionally shows the field as unmapped (#703).
func ResolveMemberRef(b entityLookupBackend, entityQN, memberName string) (string, bool) {
	if entityQN == "" || memberName == "" {
		return "", false
	}
	for _, mem := range EntityMembersFor(b, entityQN) {
		if mem.Name == memberName {
			return mem.Ref, true
		}
	}
	return "", false
}

// DeclaringMemberRef returns the reference Mendix stores for an attribute of an
// entity, qualified against the entity that DECLARES it — "M.Base.Title" for an
// attribute Child inherits from Base. Reports false when nothing in the chain
// declares it, or when the chain cannot be walked.
//
// It differs from ResolveMemberRef in one way that matters: the walk does NOT
// stop at System.User. That stop exists because a user entity's platform members
// must not appear in its ACCESS RULE (listing them is CE0066), which says nothing
// about referring to one from a microflow — a validation feedback on an inherited
// System.User attribute is ordinary and wants "System.User.Name". Reusing the
// access-rule walk here would fall back to the child's name, which is the bug it
// would be there to fix (upstream #974).
func DeclaringMemberRef(b entityLookupBackend, entityQN, memberName string) (string, bool) {
	if b == nil || entityQN == "" || memberName == "" {
		return "", false
	}
	seen := map[string]bool{}
	for currentQN := entityQN; currentQN != ""; {
		if seen[currentQN] {
			return "", false // a cycle a corrupt model could contain
		}
		seen[currentQN] = true
		entity, ok := findEntityByQN(b, currentQN)
		if !ok {
			return "", false
		}
		for _, attr := range entity.Attributes {
			if attr != nil && attr.Name == memberName {
				return currentQN + "." + memberName, true
			}
		}
		currentQN = entity.GeneralizationRef
	}
	return "", false
}

// ResolveMemberType returns the data type of an entity's member, following the
// generalization chain. Returns "" when the member cannot be resolved.
func ResolveMemberType(b entityLookupBackend, entityQN, memberName string) string {
	if entityQN == "" || memberName == "" {
		return ""
	}
	seen := map[string]bool{}
	for currentQN := entityQN; currentQN != ""; {
		if seen[currentQN] {
			return ""
		}
		seen[currentQN] = true
		entity, ok := findEntityByQN(b, currentQN)
		if !ok {
			return ""
		}
		for _, attr := range entity.Attributes {
			if attr != nil && attr.Name == memberName && attr.Type != nil {
				return attr.Type.GetTypeName()
			}
		}
		currentQN = entity.GeneralizationRef
	}
	return ""
}
