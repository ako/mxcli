// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// ReconcileModuleSecurity brings one module's entity access rules back into sync
// with its domain model, and reports how many rules it had to change.
//
// This is the `UPDATE SECURITY <module>` statement's work, and Studio Pro's
// "Update security" button's, reached directly because a module that has just
// been transplanted is not the product of any MDL program.
//
// It is needed because a transplant is the one write path in mxcli that does not
// reconcile as it writes. Every other one does — the executor's finalize step
// after a program, `GRANT`, the association handlers — but `TransplantModule`
// copies raw units in verbatim and `RestoreRoleGrants` runs its statements one
// at a time, which never reaches the finalize step. So an access rule that does
// not cover every member of its entity arrives exactly as the package shipped
// it, and Mendix rejects the model with CE0066 "Entity access is out of date"
// (mendixlabs/mxcli#1085). The update itself wrote nothing wrong, which is why
// it had nothing to report and exited 0.
//
// A module whose rules are already complete is left alone: ReconcileMemberAccesses
// returns 0 and writes nothing. That matters more than it looks — reconciling
// unconditionally would rewrite a marketplace module's domain model on every
// install, `marketplace diff` would read the result back as a local edit, and a
// local edit is what makes the NEXT update refuse.
func ReconcileModuleSecurity(mprPath, moduleName string, newBackend func() backend.FullBackend) (int, error) {
	if newBackend == nil {
		return 0, fmt.Errorf("no backend factory")
	}
	b := newBackend()
	if err := b.Connect(mprPath); err != nil {
		return 0, fmt.Errorf("open %s: %w", mprPath, err)
	}
	defer b.Disconnect()

	mod, err := b.GetModuleByName(moduleName)
	if err != nil {
		return 0, fmt.Errorf("find module %s: %w", moduleName, err)
	}
	if mod == nil {
		return 0, fmt.Errorf("module %s not found", moduleName)
	}

	dm, err := b.GetDomainModel(mod.ID)
	if err != nil || dm == nil {
		// A module need not have a domain model — a theme or widget module has
		// none, and there is nothing to reconcile. Not an error.
		return 0, nil
	}

	count, err := b.ReconcileMemberAccesses(dm.ID, moduleName)
	if err != nil {
		return 0, err
	}
	if err := b.Commit(); err != nil {
		return count, fmt.Errorf("commit: %w", err)
	}
	return count, nil
}
