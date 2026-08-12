// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"strings"
	"testing"
)

// TestRoleGrants_SurviveADropAndTransplant is the whole point of this pair.
//
// A module's role grants live in the *project's* security document, not in the
// module, so dropping the module takes them with it and putting the module back
// does not return them. The app still builds; users just quietly lose access.
// Measured on a blank 11.12.1 project: dropping Administration left Administrator
// with 2 module roles instead of 3, and User with 3 instead of 4.
func TestRoleGrants_SurviveADropAndTransplant(t *testing.T) {
	source := copyFixture(t)
	target := copyFixture(t)
	const module = "Administration"

	before, err := CaptureRoleGrants(target, module)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if len(before) == 0 {
		t.Skip("the fixture grants none of this module's roles; nothing to preserve")
	}

	execMDL(t, target, "drop module "+module+";")

	// The grants are gone — this is the loss being repaired, and asserting it
	// keeps the restore below from passing vacuously.
	gone, err := CaptureRoleGrants(target, module)
	if err != nil {
		t.Fatalf("capture after drop: %v", err)
	}
	if len(gone) != 0 {
		t.Fatalf("expected the drop to remove the grants; %d user role(s) still hold them", len(gone))
	}

	if _, err := TransplantModule(source, target, module); err != nil {
		t.Fatalf("TransplantModule: %v", err)
	}
	// Still gone after the module is back: the module returning does not restore
	// grants that live outside it.
	if after, _ := CaptureRoleGrants(target, module); len(after) != 0 {
		t.Fatalf("transplant unexpectedly restored grants; the restore step would be untested")
	}

	restored, dropped, err := RestoreRoleGrants(target, module, before, testBackend)
	if err != nil {
		t.Fatalf("RestoreRoleGrants: %v", err)
	}
	if len(dropped) != 0 {
		t.Errorf("no role should be missing when the versions match; got %v", dropped)
	}

	after, err := CaptureRoleGrants(target, module)
	if err != nil {
		t.Fatalf("capture after restore: %v", err)
	}
	for _, ur := range before.UserRoles() {
		want, got := strings.Join(before[ur], ","), strings.Join(after[ur], ",")
		if want != got {
			t.Errorf("%s: grants = [%s], want [%s]", ur, got, want)
		}
	}
	if restored == 0 {
		t.Error("restored count should be non-zero")
	}
}

// TestRestoreRoleGrants_ReportsARoleTheNewVersionRemoved — someone had that
// access and now cannot. Silently skipping it would hide a real permission
// change behind a successful-looking update.
func TestRestoreRoleGrants_ReportsARoleTheNewVersionRemoved(t *testing.T) {
	mpr := copyFixture(t)
	const module = "Administration"

	grants, err := CaptureRoleGrants(mpr, module)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if len(grants) == 0 {
		t.Skip("fixture grants none of this module's roles")
	}
	first := grants.UserRoles()[0]
	grants[first] = append(grants[first], "RoleRemovedInTheNewVersion")

	_, dropped, err := RestoreRoleGrants(mpr, module, grants, testBackend)
	if err != nil {
		t.Fatalf("RestoreRoleGrants: %v", err)
	}
	if len(dropped) != 1 || !strings.Contains(dropped[0], "RoleRemovedInTheNewVersion") {
		t.Errorf("dropped = %v, want the one role the module no longer defines", dropped)
	}
}

// TestCaptureRoleGrants_IgnoresOtherModules — an update touches one module, so
// restoring a grant it never removed would be a write nobody asked for.
func TestCaptureRoleGrants_IgnoresOtherModules(t *testing.T) {
	grants, err := CaptureRoleGrants(copyFixture(t), "Administration")
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	for ur, roles := range grants {
		for _, r := range roles {
			if strings.Contains(r, ".") {
				t.Errorf("%s: %q is qualified, so a role from another module leaked in", ur, r)
			}
		}
	}
}
