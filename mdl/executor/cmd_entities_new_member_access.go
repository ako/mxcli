// SPDX-License-Identifier: Apache-2.0

// Reporting for the access a newly added entity member does NOT get.
//
// Mendix gives a new member the rights named by its access rule's
// DefaultMemberAccessRights — that property is the rule's answer to "what should
// a member added later be allowed to do". MDL cannot set it directly: it is
// derived from the grant, `write *` → ReadWrite, `read *` → ReadOnly, and a
// grant written purely as a member list (`grant R on E (read (Subject))`) leaves
// it at None.
//
// So `alter entity … add attribute` on an entity carrying such a rule produces a
// model that is structurally complete — every rule gets a MemberAccess for the
// new member, and mxbuild reports 0 errors — in which the new attribute is
// nonetheless invisible to those roles. It renders blank in the UI for them and
// nothing in the toolchain says why (mendixlabs/mxcli#1067).
//
// The storage is not wrong; the silence is. This prints what the write implied.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// rolesBlindToNewMembers returns the module roles that will not see a member
// added to this entity now: the roles named ONLY by access rules whose default
// member rights are None. Sorted and de-duplicated.
//
// A role named by any rule with ReadOnly or ReadWrite defaults is excluded even
// when another of its rules is member-listed, because Mendix combines the access
// rights of every rule naming a role — the reference guide is explicit that
// several rules may name the same module role and that "all access rights of
// those rules are combined". Reporting such a role would be a false positive.
//
// Scope: this entity's own access rules. A specialization of this entity carries
// its own rules over the inherited member and could be blind too, but resolving
// the descendants means walking domain models this call does not hold — and a
// generalization in another module is not reachable at all. Widening the report
// there is worth doing separately; claiming it here without doing it would be
// worse than the stated limit.
func rolesBlindToNewMembers(entity *domainmodel.Entity) []string {
	if entity == nil {
		return nil
	}
	blind := map[string]bool{}
	sighted := map[string]bool{}
	for _, rule := range entity.AccessRules {
		if rule == nil {
			continue
		}
		// An empty default is None: Mendix's own documents leave the property off
		// when it carries the zero value, and reading empty as "sees new members"
		// would silence the warning on the very rules it exists for.
		seesNewMembers := rule.DefaultMemberAccessRights == domainmodel.MemberAccessRightsReadOnly ||
			rule.DefaultMemberAccessRights == domainmodel.MemberAccessRightsReadWrite
		for _, role := range rule.ModuleRoleNames {
			if role == "" {
				continue
			}
			if seesNewMembers {
				sighted[role] = true
			} else {
				blind[role] = true
			}
		}
	}

	var out []string
	for role := range blind {
		if !sighted[role] {
			out = append(out, role)
		}
	}
	sort.Strings(out)
	return out
}

// newMemberAccessWarning renders the notice for a member just added to
// entityQName, or "" when every role can see it.
//
// It names the remedy as a statement the reader can paste. GRANT is additive —
// it widens a rule and never narrows (narrowing is REVOKE's job) — so re-granting
// the one member is the minimal, non-destructive fix and leaves the rule's other
// members alone.
func newMemberAccessWarning(entityQName, memberName string, blindRoles []string) string {
	if len(blindRoles) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Warning: %s.%s is not readable by %s\n",
		entityQName, memberName, strings.Join(blindRoles, ", "))
	fmt.Fprintf(&b, "  Those access rules grant rights per member, so a member added later joins them\n")
	fmt.Fprintf(&b, "  with no access — the model is valid and builds clean, but the attribute renders\n")
	fmt.Fprintf(&b, "  blank for those roles. Widen a rule with:\n")
	for _, role := range blindRoles {
		fmt.Fprintf(&b, "    grant %s on %s (read (%s));\n", role, entityQName, memberName)
	}
	return b.String()
}
