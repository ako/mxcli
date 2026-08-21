// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
)

// No-op elision (ADR-0008 decision 1) is type-agnostic: it compares canonical
// forms of raw BSON and knows nothing about document types, so a new one is
// covered the day it is added. Identity preservation is not. `canon.identityFields`
// is hand-maintained, and it cannot currently be generated — Mendix's
// `IsIdentifier` flag lives in the modeler assemblies, not in the reflection data
// `generated/metamodel` is built from.
//
// So the failure mode for a *future* document type is silent: the type gets an
// identity-bearing property, nobody adds a row, and elision quietly stops working
// for it. No error, no failing test, just churn coming back.
//
// This guard closes the most likely path to that. A property registered as a
// codec FreshGUIDField is one the encoder mints anew on every write — by
// construction, a property that makes a document differ from itself. Every one of
// them has to be a deliberate decision: identity (carry it) or content (let it
// churn). Registering one without recording that decision fails here.
//
// It is a guard, not a proof. It cannot catch an identity property that the
// encoder does *not* mint fresh — for that, establish the property's status the
// way `StableId` was established (ADR-0008, "How this was established") before
// adding the document type.

// churnIsIntended lists FreshGUIDFields deliberately treated as content rather
// than identity. Each entry needs a reason that says what reads the value and why
// re-minting it is harmless — "it looked random" is not one.
var churnIsIntended = map[string][]string{}

// The guard's blind spot, which is how Workflows$*.PersistentId got in.
//
// It sees only properties minted through the codec's FreshGUIDFields
// registration. A property minted by hand in a writer is invisible to it —
// PersistentId was emitted by addFreshPersistentID (modelsdk) and a literal
// idToBsonBinary(generateUUID()) (legacy), so no registration existed and
// nothing complained while every workflow write churned it (issue #949).
//
// It is now carried by canon.CarryPersistentIDs rather than registered here,
// because it lives on nested elements: identityFields/CarryIdentity only reach
// top-level properties of the document root, and PersistentId sits on activities
// and outcomes arbitrarily deep in the flow tree.
//
// So the rule this guard encodes still holds — a fresh GUID needs a recorded
// decision — but "registered with the codec" is narrower than "minted fresh".
// When adding a writer that mints an identity by hand, either register it so
// this guard can see it, or carry it in canon and say so here.

func TestFreshGUIDFieldsHaveAnIdentityDecision(t *testing.T) {
	registered := codec.FreshGUIDRegistrations()
	if len(registered) == 0 {
		t.Fatal("no FreshGUIDFields registered at all — either the registration moved out of " +
			"this package's init, in which case this guard no longer sees it, or the mechanism " +
			"is gone and this test should go with it")
	}

	types := make([]string, 0, len(registered))
	for typeName := range registered {
		types = append(types, typeName)
	}
	sort.Strings(types)

	for _, typeName := range types {
		carried := map[string]bool{}
		for _, f := range canon.IdentityFields(typeName) {
			carried[f] = true
		}
		waived := map[string]bool{}
		for _, f := range churnIsIntended[typeName] {
			waived[f] = true
		}
		for _, field := range registered[typeName] {
			if carried[field] || waived[field] {
				continue
			}
			t.Errorf("%s.%s is minted fresh on every write but no identity decision is recorded.\n"+
				"  A document carrying it always differs from itself, so no-op elision cannot work for %s.\n"+
				"  Establish what Mendix uses the property for (ADR-0008, \"How this was established\"), then either\n"+
				"    - add it to identityFields in modelsdk/canon/identity.go, so the stored value is carried, or\n"+
				"    - add it to churnIsIntended here with a reason naming what reads the value.",
				typeName, field, typeName)
		}
	}
}

// The reverse direction: an identity row for a type nobody writes is dead weight
// that reads as coverage. This does not require a FreshGUIDField — a property can
// be identity without the encoder minting it — so it only reports types that no
// write path in this backend produces at all.
func TestIdentityFieldsAreNotDeadRows(t *testing.T) {
	for _, typeName := range []string{"Microflows$Microflow"} {
		if len(canon.IdentityFields(typeName)) == 0 {
			t.Errorf("identityFields lost its %s row; microflow StableId preservation is not wired", typeName)
		}
	}
}
