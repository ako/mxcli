// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1067 (c): `alter entity … add attribute` reported only
// "Added attribute 'X'", and said nothing about the roles that could not see it.
//
// The reporter read the result as the attribute being "added only to access
// rules with the widest member lists, silently skipped on narrower rules". It is
// not skipped anywhere — measured on an 11.13.0 project, the raw BSON carries
// exactly one MemberAccess per rule for the new attribute, and mxbuild reports 0
// errors. What differs is the RIGHTS: a new member joins each rule at that
// rule's DefaultMemberAccessRights, and a grant written as a member list
// (`grant R on E (read (Subject))`) leaves that default at None.
//
// So the discriminator is the rule's default, not the width of its member list.
// The control that settles it: a rule granted `read *, write (Subject)` — a
// NARROWER rule than `read *, write *` — still sees a newly added attribute,
// because `read *` sets its default to ReadOnly.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func ruleWithDefault(def domainmodel.MemberAccessRights, roles ...string) *domainmodel.AccessRule {
	return &domainmodel.AccessRule{
		ModuleRoleNames:           roles,
		DefaultMemberAccessRights: def,
	}
}

func TestRolesBlindToNewMembers(t *testing.T) {
	cases := []struct {
		name  string
		rules []*domainmodel.AccessRule
		want  []string
	}{
		{
			name:  "no access rules at all",
			rules: nil,
			want:  nil,
		},
		{
			// `grant R on E (create, delete, read *, write *)`
			name:  "ReadWrite default sees new members",
			rules: []*domainmodel.AccessRule{ruleWithDefault(domainmodel.MemberAccessRightsReadWrite, "Sales.Wide")},
			want:  nil,
		},
		{
			// `grant R on E (read *, write (Subject))` — the control. A narrower
			// rule than the one above, and still not blind.
			name:  "ReadOnly default sees new members",
			rules: []*domainmodel.AccessRule{ruleWithDefault(domainmodel.MemberAccessRightsReadOnly, "Sales.Mid")},
			want:  nil,
		},
		{
			// `grant R on E (read (Subject))`
			name:  "None default is blind",
			rules: []*domainmodel.AccessRule{ruleWithDefault(domainmodel.MemberAccessRightsNone, "Sales.Narrow")},
			want:  []string{"Sales.Narrow"},
		},
		{
			// An access rule Studio Pro wrote may leave the property empty rather
			// than spelling "None". Treating empty as "not blind" would make the
			// warning silent on exactly the rules it exists for.
			name:  "empty default is None",
			rules: []*domainmodel.AccessRule{ruleWithDefault("", "Sales.Blank")},
			want:  []string{"Sales.Blank"},
		},
		{
			name: "only the blind rules are named",
			rules: []*domainmodel.AccessRule{
				ruleWithDefault(domainmodel.MemberAccessRightsReadWrite, "Sales.Wide"),
				ruleWithDefault(domainmodel.MemberAccessRightsNone, "Sales.Narrow"),
				ruleWithDefault(domainmodel.MemberAccessRightsReadOnly, "Sales.Mid"),
			},
			want: []string{"Sales.Narrow"},
		},
		{
			// One rule may name several roles, and several rules may name the
			// same role. The report is a set, sorted, so it is stable.
			name: "roles are de-duplicated and sorted",
			rules: []*domainmodel.AccessRule{
				ruleWithDefault(domainmodel.MemberAccessRightsNone, "Sales.Zeta", "Sales.Alpha"),
				ruleWithDefault(domainmodel.MemberAccessRightsNone, "Sales.Alpha"),
			},
			want: []string{"Sales.Alpha", "Sales.Zeta"},
		},
		{
			// A role covered by ANY rule that sees new members is not blind, even
			// if another of its rules is member-listed. Mendix combines the access
			// rights of every rule naming a role, so reporting it would be wrong.
			name: "a role with a second, sighted rule is not blind",
			rules: []*domainmodel.AccessRule{
				ruleWithDefault(domainmodel.MemberAccessRightsNone, "Sales.Both"),
				ruleWithDefault(domainmodel.MemberAccessRightsReadOnly, "Sales.Both"),
			},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rolesBlindToNewMembers(&domainmodel.Entity{AccessRules: tc.rules})
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// The warning has to be actionable: it must name the attribute, the roles, and
// the statement that widens the rule. A warning that only says "some roles
// cannot see this" costs the reader the same investigation it was meant to save.
func TestNewMemberAccessWarning_IsActionable(t *testing.T) {
	got := newMemberAccessWarning("Sales.Order", "Priority", []string{"Sales.Narrow", "Sales.Other"})
	for _, want := range []string{
		"Priority",
		"Sales.Narrow",
		"Sales.Other",
		"grant Sales.Narrow on Sales.Order (read (Priority))",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("warning must contain %q, got:\n%s", want, got)
		}
	}
}

// No blind roles means no output. A warning printed on every ADD ATTRIBUTE is a
// warning nobody reads.
func TestNewMemberAccessWarning_SilentWhenNothingIsBlind(t *testing.T) {
	if got := newMemberAccessWarning("Sales.Order", "Priority", nil); got != "" {
		t.Errorf("want empty, got: %q", got)
	}
}
