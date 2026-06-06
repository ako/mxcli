// SPDX-License-Identifier: Apache-2.0

package enginecompare

import "testing"

// TestWriteParity_CreateEntityWithAttributes extends the write gate to an entity
// carrying typed attributes (string+length, integer, boolean, decimal, and a
// default), verifying the gen attribute/type/StoredValue conversion matches
// legacy (byte-faithful to Studio Pro) canonically.
func TestWriteParity_CreateEntityWithAttributes(t *testing.T) {
	const stmt = "CREATE PERSISTENT ENTITY MyFirstModule.AttrTest " +
		"( FullName: string(100), Count: integer, Active: boolean, Score: decimal, SortOrder: integer default 5 )"

	legProj := copyProject(t)
	if _, err := Run(Legacy, legProj, stmt); err != nil {
		t.Fatalf("legacy write: %v", err)
	}
	msdkProj := copyProject(t)
	if _, err := Run(ModelSDK, msdkProj, stmt); err != nil {
		t.Fatalf("modelsdk write: %v", err)
	}

	leg, err := EntityCanonBSON(legProj, "MyFirstModule", "AttrTest")
	if err != nil {
		t.Fatalf("legacy entity bson: %v", err)
	}
	msd, err := EntityCanonBSON(msdkProj, "MyFirstModule", "AttrTest")
	if err != nil {
		t.Fatalf("modelsdk entity bson: %v", err)
	}
	if leg != msd {
		t.Errorf("CreateEntity-with-attributes BSON divergence:\nlegacy:   %s\nmodelsdk: %s", leg, msd)
	}
}
