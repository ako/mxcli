// SPDX-License-Identifier: Apache-2.0

package enginecompare

import (
	"os"
	"path/filepath"
	"testing"
)

func copyProject(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS("../../testdata/expr-checker")); err != nil {
		t.Fatalf("copy project: %v", err)
	}
	return filepath.Join(dst, "minimal.mpr")
}

// TestWriteParity_CreateEntity writes the same entity through both engines and
// compares the canonicalized BSON of the created entity — the Phase-2 write gate.
func TestWriteParity_CreateEntity(t *testing.T) {
	const stmt = "CREATE PERSISTENT ENTITY MyFirstModule.SliceTest"

	legProj := copyProject(t)
	if _, err := Run(Legacy, legProj, stmt); err != nil {
		t.Fatalf("legacy write: %v", err)
	}
	msdkProj := copyProject(t)
	if _, err := Run(ModelSDK, msdkProj, stmt); err != nil {
		t.Fatalf("modelsdk write: %v", err)
	}

	leg, err := EntityCanonBSON(legProj, "MyFirstModule", "SliceTest")
	if err != nil {
		t.Fatalf("legacy entity bson: %v", err)
	}
	msd, err := EntityCanonBSON(msdkProj, "MyFirstModule", "SliceTest")
	if err != nil {
		t.Fatalf("modelsdk entity bson: %v", err)
	}
	// Strict: with applyDefaults wired (codec.RegisterTypeDefaults for
	// DomainModels$EntityImpl emits GUID + the empty member arrays Studio Pro
	// produces), the modelsdk-written entity is canonically identical to legacy's
	// — which is itself byte-faithful to Studio Pro (verified vs real BSON in
	// mx-test-projects/test7-app).
	if leg != msd {
		t.Errorf("CreateEntity BSON divergence:\nlegacy:   %s\nmodelsdk: %s", leg, msd)
	}
}
