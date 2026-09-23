// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#554 — an access rule on an entity carrying AutoOwner or AutoChangedBy
// was written with a MemberAccess for the implicit System.owner / System.changedBy
// association, and mxbuild then reported CE0066 "Entity access is out of date"
// against the whole module — which `update security` could not repair, because it
// re-added the very entry that caused it and reported success in the same breath.
//
// The audit DATE members were already known to work the other way round (an entry
// is CE0066, no entry checks clean — issuetracker #20). These two were assumed to
// be the opposite case because they are associations rather than attributes, and
// Mendix really does add them implicitly. Measured on mxbuild 11.14.0, one entity,
// one rule, one variable at a time:
//
//	AutoOwner     + MemberAccess System.owner       CE0066
//	AutoOwner     + no entry                        0 errors
//	AutoChangedBy + MemberAccess System.changedBy   CE0066
//	AutoChangedBy + no entry                        0 errors
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

func TestAccessRule_NoMemberAccessForAuditAssociations(t *testing.T) {
	for _, tc := range []struct {
		name     string
		owner    bool
		changed  bool
		stale    string // an entry already in the stored rule, to be repaired away
		mustGone []string
	}{
		{name: "AutoOwner", owner: true, mustGone: []string{"System.owner"}},
		{name: "AutoChangedBy", changed: true, mustGone: []string{"System.changedBy"}},
		{name: "both", owner: true, changed: true, mustGone: []string{"System.owner", "System.changedBy"}},
		// The repair path: a project damaged by an older mxcli must come back
		// clean, since `update security` is the documented remedy for CE0066.
		// The flag is OFF here, so the entry is stale twice over.
		{name: "stale entry with no flag is repaired away", stale: "System.owner", mustGone: []string{"System.owner"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj := copyFixture(t)
			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}
			t.Cleanup(func() { _ = b.Disconnect() })

			mod, err := b.GetModuleByName("MyFirstModule")
			if err != nil || mod == nil {
				t.Fatalf("GetModuleByName: %v", err)
			}
			dm, err := b.GetDomainModel(mod.ID)
			if err != nil {
				t.Fatalf("GetDomainModel: %v", err)
			}

			ent := &domainmodel.Entity{
				Name:         "ZzFab",
				Persistable:  true,
				HasOwner:     tc.owner,
				HasChangedBy: tc.changed,
				Attributes: []*domainmodel.Attribute{
					{Name: "FabName", Type: &domainmodel.StringAttributeType{Length: 100}},
				},
			}
			if err := b.CreateEntity(dm.ID, ent); err != nil {
				t.Fatalf("CreateEntity: %v", err)
			}

			members := []types.EntityMemberAccess{{AttributeRef: "MyFirstModule.ZzFab.FabName", AccessRights: "ReadWrite"}}
			if tc.stale != "" {
				members = append(members, types.EntityMemberAccess{AssociationRef: tc.stale, AccessRights: "ReadWrite"})
			}
			if err := b.AddEntityAccessRule(backend.EntityAccessRuleParams{
				UnitID:              dm.ID,
				EntityName:          "ZzFab",
				RoleNames:           []string{"MyFirstModule.User"},
				AllowCreate:         true,
				AllowDelete:         true,
				DefaultMemberAccess: "ReadWrite",
				MemberAccesses:      members,
			}); err != nil {
				t.Fatalf("AddEntityAccessRule: %v", err)
			}

			// The reconcile `update security` runs, and which every write path runs
			// after a program. It must not re-add what the grant left out.
			if _, err := b.ReconcileMemberAccesses(dm.ID, "MyFirstModule"); err != nil {
				t.Fatalf("ReconcileMemberAccesses: %v", err)
			}

			got := memberRefsOf(t, b, mod.ID, "ZzFab")
			for _, ref := range tc.mustGone {
				for _, g := range got {
					if g == ref {
						t.Errorf("access rule carries a MemberAccess for %q — mxbuild reports the module as "+
							"CE0066 \"Entity access is out of date\" (members: %v)", ref, got)
					}
				}
			}
			// Control: the reconcile still covers a real member. Without this the
			// test passes against a build that writes no members at all.
			var sawAttr bool
			for _, g := range got {
				if g == "MyFirstModule.ZzFab.FabName" {
					sawAttr = true
				}
			}
			if !sawAttr {
				t.Errorf("the entity's own attribute lost its MemberAccess (members: %v)", got)
			}
		})
	}
}

// memberRefsOf returns the member references of the entity's first access rule,
// read back from storage rather than from the value that was written.
func memberRefsOf(t *testing.T, b *Backend, moduleID model.ID, entityName string) []string {
	t.Helper()
	dm, err := b.GetDomainModel(moduleID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	for _, e := range dm.Entities {
		if e.Name != entityName {
			continue
		}
		var out []string
		for _, r := range e.AccessRules {
			for _, m := range r.MemberAccesses {
				if m.AttributeName != "" {
					out = append(out, m.AttributeName)
				}
				if m.AssociationName != "" {
					out = append(out, m.AssociationName)
				}
			}
		}
		return out
	}
	t.Fatalf("entity %s not found after write", entityName)
	return nil
}
