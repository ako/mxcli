// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
)

// TestIssue605_MovePreservesOwnAccessRuleRefs guards the half of ako/mxcli#605
// that lives inside the moved document: a moved entity's OWN access rules kept
// the source module's prefix on every member they name.
//
// MoveEntity re-points the view source and each validation rule's attribute (see
// the rewrites just before the rebuild), and access rules were the missed sibling.
// The consequence is worse than a stale string, because entityToGen's
// syncMemberAccesses matches existing entries BY QUALIFIED NAME: the stale
// `Source.Entity.Attr` never equals the freshly built `Target.Entity.Attr`, so it
// appends the new one and keeps the old — the entity ends up with duplicated
// MemberAccess entries, half of them dangling. Measured on a blank 11.13 app, 9 of
// 33 CE1613s were "at Access rule of entity" for the entity that had just moved.
//
// DESCRIBE cannot show this: it renders members bare, so the only visible trace is
// each member appearing twice. The assertion is therefore on the stored qualified
// names.
func TestIssue605_MovePreservesOwnAccessRuleRefs(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}

	srcID, srcMod, entID := entityWithAccessRules(t, b)
	dstID, dstMod := otherDomainModel(t, b, srcID)
	if dstID == "" {
		t.Skip("fixture has no second loadable domain model to move into")
	}
	ent := entityByID(t, b, srcID, entID)

	// Precondition: the stored rules must actually name members under the source
	// module, or nothing here can fail.
	before := accessRuleMemberRefs(t, b, srcID, entID)
	if len(before) == 0 {
		t.Skipf("entity %s has no access-rule member references", ent.Name)
	}
	srcPrefix := srcMod + "."
	found := false
	for _, r := range before {
		if strings.HasPrefix(r, srcPrefix) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no member reference under %q to go stale; refs=%v", srcPrefix, before)
	}

	if _, err := b.MoveEntity(ent, srcID, dstID, srcMod, dstMod); err != nil {
		t.Fatalf("MoveEntity: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	after := accessRuleMemberRefs(t, b2, dstID, entID)

	// 1. Nothing may still point at the source module.
	for _, r := range after {
		if strings.HasPrefix(r, srcPrefix) {
			t.Errorf("access rule still references %s after the move to %s", r, dstMod)
		}
	}
	// 2. No member may be named twice — the duplication syncMemberAccesses
	//    introduces when the stale entry fails to match the rebuilt one.
	seen := map[string]int{}
	for _, r := range after {
		seen[r]++
	}
	for r, n := range seen {
		if n > 1 {
			t.Errorf("member %s appears %d times in the moved entity's access rules", r, n)
		}
	}
	// 3. And the rules must still cover what they covered — a "fix" that dropped
	//    every member would pass 1 and 2.
	if len(seen) < len(uniq(before)) {
		t.Errorf("member count shrank across the move: %d distinct before, %d after (before=%v after=%v)",
			len(uniq(before)), len(seen), before, after)
	}
}

// TestIssue605_MoveReportsAssociationRenamesInBothDirections pins the contract the
// project-wide sweep is driven from: MoveEntity must report, per converted
// association, the qualified name it had and the one it now has.
//
// The two directions differ, and a sweep that assumed either one would be wrong in
// the other — the parent moving takes the cross-association to the target module,
// the child moving leaves it where it was.
func TestIssue605_MoveReportsAssociationRenamesInBothDirections(t *testing.T) {
	for _, tc := range []struct {
		name      string
		moveChild bool
		// wantMoved says whether the association's qualified name should change.
		wantMoved bool
	}{
		{name: "ParentMoved_AssociationFollows", moveChild: false, wantMoved: true},
		{name: "ChildMoved_AssociationStays", moveChild: true, wantMoved: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj := copyFixture(t)
			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}
			t.Cleanup(func() { _ = b.Disconnect() })

			srcID, srcMod, assocName, _, childID, parentID := associationSubject(t, b)
			dstID, dstMod := otherDomainModel(t, b, srcID)
			if dstID == "" {
				t.Skip("fixture has no second loadable domain model to move into")
			}
			entID := parentID
			if tc.moveChild {
				entID = childID
			}
			ent := entityByID(t, b, srcID, entID)

			moved, err := b.MoveEntity(ent, srcID, dstID, srcMod, dstMod)
			if err != nil {
				t.Fatalf("MoveEntity: %v", err)
			}
			if len(moved) != 1 {
				t.Fatalf("expected 1 converted association, got %d: %+v", len(moved), moved)
			}
			m := moved[0]
			if m.Name != assocName {
				t.Errorf("reported name %q, want %q", m.Name, assocName)
			}
			if got, want := m.OldQualifiedName, srcMod+"."+assocName; got != want {
				t.Errorf("OldQualifiedName = %q, want %q", got, want)
			}
			wantNew := srcMod + "." + assocName
			if tc.wantMoved {
				wantNew = dstMod + "." + assocName
			}
			if got := m.NewQualifiedName; got != wantNew {
				t.Errorf("NewQualifiedName = %q, want %q", got, wantNew)
			}
			if m.Moved() != tc.wantMoved {
				t.Errorf("Moved() = %v, want %v", m.Moved(), tc.wantMoved)
			}
		})
	}
}

// entityWithAccessRules returns a loadable domain model and an entity in it whose
// access rules name at least one member.
func entityWithAccessRules(t *testing.T, b *Backend) (dmID model.ID, moduleName string, entID model.ID) {
	t.Helper()
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, d := range dms {
		if _, err := b.loadDomainModelGen(d.ID); err != nil {
			continue
		}
		mod, err := b.GetModule(d.ContainerID)
		if err != nil || mod == nil {
			continue
		}
		for _, e := range d.Entities {
			if len(accessRuleMemberRefs(t, b, d.ID, e.ID)) > 0 {
				return d.ID, mod.Name, e.ID
			}
		}
	}
	t.Fatal("no entity with access-rule member references in the fixture")
	return
}

// accessRuleMemberRefs returns every qualified name an entity's access rules name, read
// from the stored gen elements (attributes and associations alike).
func accessRuleMemberRefs(t *testing.T, b *Backend, dmID, entID model.ID) []string {
	t.Helper()
	gdm, err := b.loadDomainModelGen(dmID)
	if err != nil {
		t.Fatalf("loadDomainModelGen: %v", err)
	}
	ge := findGenEntity(gdm, entID)
	if ge == nil {
		return nil
	}
	var out []string
	for _, el := range ge.AccessRulesItems() {
		ar, ok := el.(*genDm.AccessRule)
		if !ok {
			continue
		}
		for _, mel := range ar.MemberAccessesItems() {
			ma, ok := mel.(*genDm.MemberAccess)
			if !ok {
				continue
			}
			if qn := ma.AttributeQualifiedName(); qn != "" {
				out = append(out, qn)
			}
			if qn := ma.AssociationQualifiedName(); qn != "" {
				out = append(out, qn)
			}
		}
	}
	return out
}

func uniq(in []string) map[string]bool {
	out := map[string]bool{}
	for _, s := range in {
		out[s] = true
	}
	return out
}
