// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/marketplace"
)

// An association another module owns, reaching into the module being replaced,
// is carried across by the update with its identity (mendixlabs/mxcli#1502's
// follow-up). It is not an edit to the module and the update no longer discards
// it, so it must not make an unedited module refuse to update.
func TestGateOnLocalEdits_LetsAKeptAssociationThrough(t *testing.T) {
	drift := &marketplace.Report{
		Module: "Administration",
		Findings: []marketplace.Finding{
			{Key: marketplace.ElementKey{Type: "ENTITY", Name: "Account"}, Verdict: marketplace.Unchanged},
		},
		Dependents: []marketplace.Element{{
			Key: marketplace.ElementKey{Type: "ASSOCIATION", Name: "MyFirstModule.Config_Account"},
			MDL: "create or modify association MyFirstModule.Config_Account\nfrom MyFirstModule.Config to Administration.Account;",
		}},
	}
	var out bytes.Buffer
	if err := gateOnLocalEdits(&out, drift, false, ""); err != nil {
		t.Fatalf("an unedited module with an association into it must update without --force: %v", err)
	}
}

// The summary names what happened to each one: kept is a count, lost names the
// association and the entity the new version removed.
func TestReportUpdate_NamesKeptAndLostAssociations(t *testing.T) {
	var out bytes.Buffer
	reportUpdate(&out, &marketplace.UpdateResult{
		Module: "Administration", FromVersion: "4.3.2", ToVersion: "4.5.0",
		DependentsKept: []string{"MyFirstModule.Config_Account"},
		DependentsLost: []string{"MyFirstModule.Audit_Legacy (Administration.Legacy is not in the new version)"},
	})
	got := out.String()
	for _, want := range []string{
		"1 association(s) from other modules into Administration kept",
		"MyFirstModule.Audit_Legacy (Administration.Legacy is not in the new version)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q:\n%s", want, got)
		}
	}
}
