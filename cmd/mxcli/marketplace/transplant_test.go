// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"strings"
	"testing"
)

// TestTransplantModule_MovesTheWholeModuleAndPreservesIdentity is the shape of a
// real update, run end to end on the model: capture the identities, remove the
// module, copy a replacement in, transplant the identities back.
//
// The destination keeps its MPR v2 layout throughout, which is the whole reason
// this path exists rather than `mx module-import` — that command rewrites a v2
// project as v1 and refuses theme modules outright.
func TestTransplantModule_MovesTheWholeModuleAndPreservesIdentity(t *testing.T) {
	source := copyFixture(t) // stands in for the package's project
	target := copyFixture(t) // stands in for the user's project
	const module = "Administration"

	before, err := CaptureIdentities(target, module)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	// Make the source's identities differ, so a transplant that silently did
	// nothing would be indistinguishable from success.
	scrambled := Identities{}
	for p, g := range before {
		alt := append([]byte{}, g...)
		alt[0] ^= 0xFF
		scrambled[p] = alt
	}
	if _, _, err := ApplyIdentities(source, module, scrambled); err != nil {
		t.Fatalf("scramble source: %v", err)
	}

	execMDL(t, target, "drop module "+module+";")

	copied, err := TransplantModule(source, target, module)
	if err != nil {
		t.Fatalf("TransplantModule: %v", err)
	}
	if copied < 3 {
		t.Fatalf("copied only %d units; a module carries at least its own unit, a domain model and security", copied)
	}

	// The module is readable in the target, and carries the package's identities.
	moved, err := CaptureIdentities(target, module)
	if err != nil {
		t.Fatalf("capture after transplant: %v", err)
	}
	if len(moved) != len(before) {
		t.Errorf("transplanted module has %d identities, source had %d", len(moved), len(before))
	}
	if bytesEqual(moved["Account"], before["Account"]) {
		t.Fatal("the transplanted module already carries the target's old identity; " +
			"the restore below would prove nothing")
	}

	// Now the step that makes an update data-safe.
	applied, missing, err := ApplyIdentities(target, module, before)
	if err != nil {
		t.Fatalf("ApplyIdentities: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("nothing should be missing when the versions match; got %v", missing)
	}
	if applied != len(before) {
		t.Errorf("applied %d of %d identities", applied, len(before))
	}

	after, err := CaptureIdentities(target, module)
	if err != nil {
		t.Fatalf("capture after restore: %v", err)
	}
	for _, p := range before.Paths() {
		if !bytesEqual(after[p], before[p]) {
			t.Errorf("%s: identity not preserved across the update (%x vs %x)",
				p, after[p], before[p])
		}
	}
}

// TestTransplantModule_RefusesWhenTheModuleIsStillThere — copying on top of a
// live module leaves two of the same name, which is a corrupt model rather than
// an update. The caller has to remove it first, deliberately.
func TestTransplantModule_RefusesWhenTheModuleIsStillThere(t *testing.T) {
	source := copyFixture(t)
	target := copyFixture(t) // still has Administration

	_, err := TransplantModule(source, target, "Administration")
	if err == nil {
		t.Fatal("transplanting onto a module that is still present must be refused")
	}
	if !strings.Contains(err.Error(), "still present") {
		t.Errorf("the error should say why; got: %v", err)
	}
}

// TestTransplantModule_RefusesAnUnknownSourceModule keeps a typo from producing
// an empty, silent success.
func TestTransplantModule_RefusesAnUnknownSourceModule(t *testing.T) {
	if _, err := TransplantModule(copyFixture(t), copyFixture(t), "NoSuchModule"); err == nil {
		t.Fatal("a module that is not in the source must be an error")
	}
}
