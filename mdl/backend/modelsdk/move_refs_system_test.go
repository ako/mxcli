// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "testing"

// TestUpdateRefsForMovedEntity_SkipsSystemDomainModel is a regression test for
// the MOVE ENTITY / MOVE ENUMERATION reference-update warnings ("Could not
// update OQL queries" / "Could not update enumeration references": load domain
// model 00000000-0000-0000-0000-000000000002: ... no such file or directory).
//
// ListDomainModels injects the virtual System-module domain model (its unit is
// not stored in mprcontents). The reference-update sweeps re-load every listed
// domain model from disk, so they must skip the System DM instead of failing on
// its missing unit. Both methods should return cleanly even when nothing matches.
func TestUpdateRefsForMovedEntity_SkipsSystemDomainModel(t *testing.T) {
	proj := copyFixture(t)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	// Names that match nothing in the fixture: the sweep still walks every domain
	// model (including the virtual System one) and must not error on it.
	if err := b.UpdateEnumerationRefsInAllDomainModels("NoSuch.Enum", "Other.Enum"); err != nil {
		t.Fatalf("UpdateEnumerationRefsInAllDomainModels errored on System DM: %v", err)
	}
	if _, err := b.UpdateOqlQueriesForMovedEntity("NoSuch.Entity", "Other.Entity"); err != nil {
		t.Fatalf("UpdateOqlQueriesForMovedEntity errored on System DM: %v", err)
	}
}
