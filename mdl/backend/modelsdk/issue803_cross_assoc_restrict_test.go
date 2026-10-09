// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#803, create half: CreateCrossAssociation (the path CREATE ASSOCIATION
// takes when the TO entity is in another module) wrote a restrict
// ("DeleteMeIfNoReferences") child side with a null ChildErrorMessage. That is
// the shape that stops the runtime starting (CapTrackV2 §1, see
// domainmodel_delete_message_test.go); #795 fixed it for the ALTER path only.
//
// Asserted on the stored unit after a reopen — the document the runtime reads.
// The keep case is the control: Studio Pro leaves its message null, and a fix
// that wrote a text for every behaviour would differ from it.
func TestIssue803_CreateCrossAssociationWritesRestrictMessage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		behaviour domainmodel.DeleteBehaviorType
		msg       string
		wantMsg   bool
	}{
		{"Restrict", domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences, "Still referenced", true},
		{"RestrictNoText", domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences, "", true},
		{"KeepControl", domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj := copyFixture(t)
			b := New()
			if err := b.Connect(proj); err != nil {
				t.Fatalf("connect: %v", err)
			}
			mod, err := b.GetModuleByName("MyFirstModule")
			if err != nil || mod == nil {
				t.Fatalf("GetModuleByName: %v", err)
			}
			dm, err := b.GetDomainModel(mod.ID)
			if err != nil {
				t.Fatalf("GetDomainModel: %v", err)
			}
			from := &domainmodel.Entity{Name: "Issue803From", Persistable: true}
			if err := b.CreateEntity(dm.ID, from); err != nil {
				t.Fatalf("CreateEntity: %v", err)
			}
			ca := &domainmodel.CrossModuleAssociation{
				Name:                "Issue803From_Remote",
				ParentID:            from.ID,
				ChildRef:            "Administration.Account",
				Type:                domainmodel.AssociationTypeReference,
				Owner:               domainmodel.AssociationOwnerDefault,
				ChildDeleteBehavior: &domainmodel.DeleteBehavior{Type: tc.behaviour, ErrorMessage: tc.msg},
			}
			if err := b.CreateCrossAssociation(dm.ID, ca); err != nil {
				t.Fatalf("CreateCrossAssociation: %v", err)
			}
			if err := b.Disconnect(); err != nil {
				t.Fatalf("disconnect: %v", err)
			}

			b2 := New()
			if err := b2.Connect(proj); err != nil {
				t.Fatalf("reconnect: %v", err)
			}
			t.Cleanup(func() { _ = b2.Disconnect() })
			gdm, err := b2.loadDomainModelGen(dm.ID)
			if err != nil {
				t.Fatalf("loadDomainModelGen: %v", err)
			}
			var db *genDm.AssociationDeleteBehavior
			for _, el := range gdm.CrossAssociationsItems() {
				if gca, ok := el.(*genDm.CrossAssociation); ok && gca.Name() == ca.Name {
					db, _ = gca.DeleteBehavior().(*genDm.AssociationDeleteBehavior)
				}
			}
			if db == nil {
				t.Fatal("stored cross-module association or its delete behaviour not found")
			}
			if got := db.ChildDeleteBehavior(); got != string(tc.behaviour) {
				t.Fatalf("child behaviour = %q, want %q", got, tc.behaviour)
			}
			msg := db.ChildErrorMessage()
			if !tc.wantMsg {
				if msg != nil {
					t.Errorf("%s wrote a ChildErrorMessage; Studio Pro leaves it null", tc.behaviour)
				}
				return
			}
			if msg == nil {
				t.Fatal("ChildErrorMessage is null on a restrict cross-module association — the runtime will not start (#803)")
			}
			if got := deleteErrorMessageFromGen(msg, ""); got != tc.msg {
				t.Errorf("ChildErrorMessage = %q, want %q", got, tc.msg)
			}
			if db.ParentErrorMessage() != nil {
				t.Error("ParentErrorMessage written; Studio Pro leaves the parent side null")
			}
		})
	}
}
