// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance §4: `grant <Role> on System.User (...)` failed with a
// filesystem error naming a UUID and a .mxunit path, which reads like a corrupt
// project. It is not — the System module's domain model is not a stored unit in
// ANY Mendix project.
package executor

import (
	"strings"
	"testing"
)

func TestSystemEntityGrantIsRefusedWithAnExplanation(t *testing.T) {
	err := refuseSystemEntityGrant("System", "User")
	if err == nil {
		t.Fatal("granting on a System entity was not refused")
	}
	msg := err.Error()

	// The failure mode being replaced: a UUID and a path to a file that was never
	// going to exist.
	for _, leak := range []string{".mxunit", "00000000-0000", "no such file"} {
		if strings.Contains(msg, leak) {
			t.Errorf("message still leaks storage detail %q:\n%s", leak, msg)
		}
	}
	// It has to say this is normal, or the reader goes looking for the corruption.
	if !strings.Contains(msg, "every Mendix project") {
		t.Errorf("message does not say the limitation is universal:\n%s", msg)
	}
	// And name the way out the maintenance app actually took.
	if !strings.Contains(msg, "Administration.Account") {
		t.Errorf("message offers no alternative:\n%s", msg)
	}
}

func TestOrdinaryModuleGrantIsNotRefused(t *testing.T) {
	// The control. A guard keyed on the module name must not catch anything else —
	// including a module whose name merely contains "System".
	for _, mod := range []string{"MyFirstModule", "Administration", "SystemHelpers"} {
		if err := refuseSystemEntityGrant(mod, "Thing"); err != nil {
			t.Errorf("%s.Thing was refused: %v", mod, err)
		}
	}
	// Case is not significance: Mendix's module is System however it is written.
	if err := refuseSystemEntityGrant("system", "User"); err == nil {
		t.Error("lowercase system.User was not refused")
	}
}
