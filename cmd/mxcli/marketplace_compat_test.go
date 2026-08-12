// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/marketplace"
)

// The shape measured on 2026-08-12: the latest release of every agent-stack
// module required Mendix 11.12.2, five days after that patch shipped, while the
// project under test was on 11.12.1. `install` with no --version resolves to the
// latest, so this is the default path, not an edge case.
func agentStackVersions() []marketplace.Version {
	return []marketplace.Version{
		{VersionNumber: "7.2.0", MinSupportedMendixVersion: "11.12.2"},
		{VersionNumber: "7.1.1", MinSupportedMendixVersion: "11.12.1"},
		{VersionNumber: "7.0.0", MinSupportedMendixVersion: "11.12.0"},
		{VersionNumber: "6.2.1", MinSupportedMendixVersion: "10.24.13"},
	}
}

func TestCheckMendixCompatibility_RefusesAndNamesTheVersionToUse(t *testing.T) {
	all := agentStackVersions()
	err := checkMendixCompatibility(&all[0], all, "11.12.1", "GenAI Commons")
	if err == nil {
		t.Fatal("a version requiring 11.12.2 was accepted for an 11.12.1 project; " +
			"without this the refusal surfaces as 'mx module-import' exit 117, three layers down")
	}
	msg := err.Error()
	for _, want := range []string{"11.12.2", "11.12.1", "--version 7.1.1"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error must contain %q so the user can act on it, got:\n%s", want, msg)
		}
	}
}

// The negative case: a check that refuses everything would also pass the test
// above. A compatible version must install.
func TestCheckMendixCompatibility_AllowsACompatibleVersion(t *testing.T) {
	all := agentStackVersions()
	for _, i := range []int{1, 2, 3} {
		if err := checkMendixCompatibility(&all[i], all, "11.12.1", "GenAI Commons"); err != nil {
			t.Errorf("version %s (min %s) refused on an 11.12.1 project: %v",
				all[i].VersionNumber, all[i].MinSupportedMendixVersion, err)
		}
	}
}

// A check that cannot evaluate must not block. Both an unknown project version
// and an unpublished minimum fall through, matching how checkFeature treats a
// backend that reports no version.
func TestCheckMendixCompatibility_SkipsWhenItCannotEvaluate(t *testing.T) {
	all := agentStackVersions()
	if err := checkMendixCompatibility(&all[0], all, "", "GenAI Commons"); err != nil {
		t.Errorf("refused with an unknown project version: %v", err)
	}
	noMin := marketplace.Version{VersionNumber: "9.9.9"}
	if err := checkMendixCompatibility(&noMin, all, "11.12.1", "GenAI Commons"); err != nil {
		t.Errorf("refused a version publishing no minimum: %v", err)
	}
	if err := checkMendixCompatibility(nil, all, "11.12.1", "GenAI Commons"); err != nil {
		t.Errorf("refused a nil version: %v", err)
	}
}

// When nothing is compatible the hint must not point at a version that does not
// exist — an empty suggestion is worse than none.
func TestCheckMendixCompatibility_NoCompatibleVersionSaysSo(t *testing.T) {
	all := []marketplace.Version{
		{VersionNumber: "2.0.0", MinSupportedMendixVersion: "11.12.2"},
		{VersionNumber: "1.0.0", MinSupportedMendixVersion: "11.12.2"},
	}
	err := checkMendixCompatibility(&all[0], all, "10.24.0", "Agent Editor")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if strings.Contains(err.Error(), "--version") {
		t.Errorf("suggested a --version when none is compatible:\n%s", err)
	}
	if !strings.Contains(err.Error(), "upgrade the project") {
		t.Errorf("expected the message to name the only remaining option, got:\n%s", err)
	}
}

func TestNewestCompatibleVersion_PrefersPublicationOrder(t *testing.T) {
	all := agentStackVersions()
	if got := newestCompatibleVersion(all, "11.12.1"); got != "7.1.1" {
		t.Errorf("newestCompatibleVersion = %q, want 7.1.1", got)
	}
	if got := newestCompatibleVersion(all, "10.24.13"); got != "6.2.1" {
		t.Errorf("newestCompatibleVersion = %q, want 6.2.1", got)
	}
	if got := newestCompatibleVersion(all, "9.0.0"); got != "" {
		t.Errorf("newestCompatibleVersion = %q, want an empty result", got)
	}
}
