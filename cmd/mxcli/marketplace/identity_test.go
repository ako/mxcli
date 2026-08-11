// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"encoding/hex"
	"testing"
)

// TestCaptureIdentities_RecordsTheDatabaseKey is the assertion the whole update
// path rests on.
//
// The runtime keys entities and attributes on the model's GUID — measured
// against a live PostgreSQL, see PROPOSAL §8, where mendixsystem$entity.id for
// Administration.Account is b16e49ea-91df-4caa-aed8-6ba4c4e133c5. The value
// captured here must be exactly that, byte-for-byte in stored (.NET) order,
// because a module update that carries anything else destroys the table's data
// on the next deploy.
func TestCaptureIdentities_RecordsTheDatabaseKey(t *testing.T) {
	ids, err := CaptureIdentities(copyFixture(t), "Administration")
	if err != nil {
		t.Fatalf("CaptureIdentities: %v", err)
	}

	// Stored .NET byte order for b16e49ea-91df-4caa-aed8-6ba4c4e133c5: the first
	// three groups are little-endian, the rest is as-is.
	const accountGUID = "ea496eb1df91aa4caed86ba4c4e133c5"

	got, ok := ids["Account"]
	if !ok {
		t.Fatalf("entity Account has no recorded identity; captured: %v", ids.Paths())
	}
	if hex.EncodeToString(got) != accountGUID {
		t.Errorf("Account GUID = %s, want %s (the id the runtime keys the table on)",
			hex.EncodeToString(got), accountGUID)
	}
}

// TestCaptureIdentities_KeysByPathNotName — names repeat. Two entities each
// having an attribute of the same name is ordinary, so a name-keyed map would
// silently collapse them and transplant one entity's identity onto another's
// column. The path is what keeps them apart.
func TestCaptureIdentities_KeysByPathNotName(t *testing.T) {
	ids, err := CaptureIdentities(copyFixture(t), "Administration")
	if err != nil {
		t.Fatalf("CaptureIdentities: %v", err)
	}

	// Attributes are recorded under their entity.
	if _, ok := ids["Account/FullName"]; !ok {
		t.Errorf("expected Account/FullName to be recorded; captured: %v", ids.Paths())
	}
	// The bare attribute name must not be a key of its own.
	if _, ok := ids["FullName"]; ok {
		t.Error("attributes must be keyed under their entity, not by bare name")
	}

	// Every recorded GUID must be a full 16 bytes: a truncated identity is worse
	// than none, because it would be transplanted and silently wrong.
	for _, p := range ids.Paths() {
		if len(ids[p]) != 16 {
			t.Errorf("%s: GUID is %d bytes, want 16", p, len(ids[p]))
		}
	}
}

// TestCaptureIdentities_CoversTheWholeModule checks the capture reaches
// documents nested in folders, not just those sitting directly under the module.
// In a real marketplace module almost everything is foldered — reading one level
// is the defect that made DESCRIBE miss 15 of 16 microflows (#759).
func TestCaptureIdentities_CoversTheWholeModule(t *testing.T) {
	ids, err := CaptureIdentities(copyFixture(t), "Administration")
	if err != nil {
		t.Fatalf("CaptureIdentities: %v", err)
	}
	// The fixture's Administration yields 9: two entities, six attributes and one
	// association. That is the same count §4 measured Studio Pro transplanting
	// ("Elements carrying a GUID: 9, GUID preserved: 9") on a different project at
	// a different Mendix version — so the walk is finding the set Mendix itself
	// treats as identity-bearing, not merely a plausible-looking subset.
	//
	// Asserted as a floor rather than equality: a fixture gaining an attribute
	// should not fail this test, while a walk that stopped at the domain model's
	// direct children would come in far below.
	if len(ids) < 8 {
		t.Errorf("captured only %d identities (%v) — the walk is not reaching the whole module",
			len(ids), ids.Paths())
	}
	if _, ok := ids["AccountPasswordData"]; !ok {
		t.Errorf("second entity not captured; got %v", ids.Paths())
	}
}

// TestCaptureIdentities_RefusesAnUnknownModule — an update must not proceed on a
// module it could not read identities for, so this has to be an error rather
// than an empty map that a caller could mistake for "nothing to preserve".
func TestCaptureIdentities_RefusesAnUnknownModule(t *testing.T) {
	if _, err := CaptureIdentities(copyFixture(t), "NoSuchModule"); err == nil {
		t.Fatal("capturing identities for a module that does not exist must be an error")
	}
}
