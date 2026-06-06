// SPDX-License-Identifier: Apache-2.0

package enginecompare

import "testing"

// TestWriteParity_CreateAssociation creates two entities and an association
// between them via each engine and compares the canonicalized association BSON
// (entity-id pointers are masked, so structure + enum strings + delete behavior
// are what must match).
func TestWriteParity_CreateAssociation(t *testing.T) {
	const script = "CREATE PERSISTENT ENTITY MyFirstModule.AsA;" +
		"CREATE PERSISTENT ENTITY MyFirstModule.AsB;" +
		"CREATE ASSOCIATION MyFirstModule.AsA_AsB FROM MyFirstModule.AsA TO MyFirstModule.AsB;"

	legProj := copyProject(t)
	if _, err := Run(Legacy, legProj, script); err != nil {
		t.Fatalf("legacy write: %v", err)
	}
	msdkProj := copyProject(t)
	if _, err := Run(ModelSDK, msdkProj, script); err != nil {
		t.Fatalf("modelsdk write: %v", err)
	}

	leg, err := AssociationCanonBSON(legProj, "MyFirstModule", "AsA_AsB")
	if err != nil {
		t.Fatalf("legacy assoc bson: %v", err)
	}
	msd, err := AssociationCanonBSON(msdkProj, "MyFirstModule", "AsA_AsB")
	if err != nil {
		t.Fatalf("modelsdk assoc bson: %v", err)
	}
	if leg != msd {
		t.Errorf("CreateAssociation BSON divergence:\nlegacy:   %s\nmodelsdk: %s", leg, msd)
	}
}
