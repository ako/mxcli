// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// The engine matrix decides how much the gate actually covers, so its selection
// is worth testing directly — the alternative is discovering in a CI log that a
// run "passed" over nothing.

func testMatrix() []gateEngine {
	return []gateEngine{
		{"modelsdk", func() backend.FullBackend { return nil }},
		{"legacy", func() backend.FullBackend { return nil }},
	}
}

func TestSelectGateEngines(t *testing.T) {
	for _, tc := range []struct {
		name    string
		spec    string
		want    string
		unknown string
	}{
		// The default has to be the FULL matrix: every workflow that wants less
		// says so, and a file that forgets loses minutes rather than coverage.
		{"empty is every engine", "", "modelsdk, legacy", ""},
		{"whitespace is every engine", "   ", "modelsdk, legacy", ""},
		{"all is every engine", "all", "modelsdk, legacy", ""},
		{"single engine", "modelsdk", "modelsdk", ""},
		{"the other engine", "legacy", "legacy", ""},
		{"comma separated", "modelsdk,legacy", "modelsdk, legacy", ""},
		{"space separated", "modelsdk legacy", "modelsdk, legacy", ""},
		// Order follows the matrix, not the spec, so a subset runs in the same
		// sequence as the whole and logs line up between runs.
		{"order follows the matrix", "legacy,modelsdk", "modelsdk, legacy", ""},
		{"case and padding are tolerated", " MODELSDK , legacy ", "modelsdk, legacy", ""},
		{"all wins over a narrower name", "legacy,all", "modelsdk, legacy", ""},
		// The one that matters: a typo must be reported, never silently
		// selecting nothing.
		{"a typo is reported", "modelsdkk", "", "modelsdkk"},
		{"a typo beside a real name is reported", "modelsdk,legcy", "modelsdk", "legcy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, unknown := selectGateEngines(tc.spec, testMatrix())
			if name := gateEngineNames(got); name != tc.want {
				t.Errorf("selected = %q, want %q", name, tc.want)
			}
			if u := strings.Join(unknown, ", "); u != tc.unknown {
				t.Errorf("unknown = %q, want %q", u, tc.unknown)
			}
		})
	}
}

// TestGateEnginesIsNeverSilentlyEmpty is the property TestMain's fatal check
// rests on: a spec that selects no engine must be distinguishable from one that
// selects every engine. Without this, `MXCLI_TEST_ENGINES=modelsdkk` would run
// zero scripts and report the gate green.
func TestGateEnginesIsNeverSilentlyEmpty(t *testing.T) {
	selected, unknown := selectGateEngines("nosuchengine", testMatrix())
	if len(selected) != 0 {
		t.Fatalf("selected %d engines for an unknown name, want 0", len(selected))
	}
	if len(unknown) == 0 {
		t.Fatal("an unknown engine name produced no report — the gate would run nothing and pass")
	}
}

// TestGateEnginesMatchesTheProcessEnv confirms the wiring: the live matrix is
// the one selectGateEngines produced, not a separately maintained list.
func TestGateEnginesMatchesTheProcessEnv(t *testing.T) {
	if len(unknownGateEngines) != 0 {
		t.Fatalf("this run has unknown engine names %v — TestMain should have refused to start", unknownGateEngines)
	}
	if len(gateEngines) == 0 {
		t.Fatal("the live engine matrix is empty")
	}
	for _, eng := range gateEngines {
		if eng.factory == nil {
			t.Errorf("engine %q has no backend factory", eng.name)
		}
	}
}
