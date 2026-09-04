// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// CapTrackV2 FINDINGS §1 — an association with PREVENT stopped the runtime
// STARTING, which is worse than a build error and names nothing about the model:
//
//	ERROR - M2EE: An error occurred while initializing the Runtime: None.get
//	  at …SchemeFactory$.…$setDeleteBehavior(SchemeFactory.scala:515)
//
// mxcli wrote ChildDeleteBehavior "DeleteMeIfNoReferences" with a null
// ChildErrorMessage. Studio Pro writes a Texts$Text there — the field the
// association dialog only reveals once that behaviour is selected, which is why
// it went unnoticed. Measured on a Studio Pro reference (ako/TestApp,
// Mappings.Order_Customer, Mendix 11):
//
//	"ChildDeleteBehavior": "DeleteMeIfNoReferences",
//	"ChildErrorMessage": { "$Type": "Texts$Text",
//	                       "Items": [ 3, { "$Type": "Texts$Translation",
//	                                       "LanguageCode": "en_US",
//	                                       "Text": "Aha here it is" } ] },
//	"ParentErrorMessage": null
//
// Two things that reference pins down and no amount of reasoning would have:
// the item collection's typed-array marker is **3**, and the OTHER side stays
// null because that side is still "keep". A census of 47,789 units across 122
// projects found 423 delete behaviours and NOT ONE using this behaviour — so
// there was no reference anywhere until one was made, which is exactly why the
// null shipped.

func assocWithBehaviour(t *testing.T, behaviour domainmodel.DeleteBehaviorType, msg string) map[string]any {
	t.Helper()
	a := &domainmodel.Association{
		Name:                "Order_Customer",
		ChildDeleteBehavior: &domainmodel.DeleteBehavior{Type: behaviour, ErrorMessage: msg},
	}
	gen := assocToGen(a)
	if gen == nil {
		t.Fatal("assocToGen returned nil")
	}
	dbEl := gen.DeleteBehavior()
	if dbEl == nil {
		t.Fatal("no DeleteBehavior on the generated association")
	}
	db, ok := dbEl.(*genDm.AssociationDeleteBehavior)
	if !ok {
		t.Fatalf("DeleteBehavior is %T", dbEl)
	}
	return map[string]any{
		"child":     db.ChildDeleteBehavior(),
		"childMsg":  db.ChildErrorMessage(),
		"parent":    db.ParentDeleteBehavior(),
		"parentMsg": db.ParentErrorMessage(),
	}
}

// The reported case: RESTRICT must carry a message element, not null.
func TestDeleteBehaviour_RestrictWritesTheErrorMessage(t *testing.T) {
	got := assocWithBehaviour(t, domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences,
		"A customer with orders cannot be deleted")

	if got["child"] != "DeleteMeIfNoReferences" {
		t.Fatalf("child behaviour = %v", got["child"])
	}
	if got["childMsg"] == nil {
		t.Fatal("ChildErrorMessage is nil — this is the document that stops the runtime starting")
	}
}

// CONTROL 1: the other two behaviours keep a null message, which is what the
// reference's two untouched associations show. Writing a text element for every
// association would differ from Studio Pro on the 423 behaviours that exist in
// the wild and none of which have one.
func TestDeleteBehaviour_OtherBehavioursStayNull(t *testing.T) {
	for _, b := range []domainmodel.DeleteBehaviorType{
		domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences,
		domainmodel.DeleteBehaviorTypeDeleteMeAndReferences,
	} {
		got := assocWithBehaviour(t, b, "")
		if got["childMsg"] != nil {
			t.Errorf("%s wrote a ChildErrorMessage; Studio Pro leaves it null", b)
		}
	}
}

// CONTROL 2: the PARENT side is untouched. MDL only ever sets the child side,
// and the reference confirms ParentErrorMessage stays null even when the child
// carries a message.
func TestDeleteBehaviour_ParentSideUntouched(t *testing.T) {
	got := assocWithBehaviour(t, domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences, "blocked")
	if got["parent"] != "DeleteMeButKeepReferences" {
		t.Errorf("parent behaviour = %v, want the default", got["parent"])
	}
}

// A message on a behaviour that has no use for one is not written. Mendix only
// reads it for the restrict case, and Studio Pro cannot even author one there.
func TestDeleteBehaviour_MessageIgnoredOnOtherBehaviours(t *testing.T) {
	got := assocWithBehaviour(t, domainmodel.DeleteBehaviorTypeDeleteMeAndReferences, "not applicable")
	if got["childMsg"] != nil {
		t.Error("a message was written for CASCADE, which Studio Pro cannot author")
	}
}
