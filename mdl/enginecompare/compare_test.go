// SPDX-License-Identifier: Apache-2.0

package enginecompare

import (
	"slices"
	"strings"
	"testing"
)

const fixture = "../../testdata/expr-checker/minimal.mpr"

// dropSystem filters out rows whose qualified name is in the System module —
// legacy injects the whole System module from hardcoded sdk/mpr/system_module.go,
// while the modelsdk engine reads only real project units. This is a known,
// tracked architectural difference, not a conversion error.
func dropSystem(row string) bool { return !strings.HasPrefix(row, "|System.") }

// TestReadParity runs each read query through both engines and asserts their
// normalized output matches. Cases with a knownGap are reported, not failed —
// they document where the engines legitimately differ today (and flag if they
// unexpectedly start matching, a cue to promote them to strict).
func TestReadParity(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		keep     func(string) bool
		knownGap string
	}{
		{name: "microflows", query: "SHOW MICROFLOWS"},
		{name: "nanoflows", query: "SHOW NANOFLOWS"},
		{name: "pages", query: "SHOW PAGES"},
		{name: "enumerations", query: "SHOW ENUMERATIONS"},
		{name: "constants", query: "SHOW CONSTANTS"},
		{name: "entities", query: "SHOW ENTITIES", keep: dropSystem},
		{
			name:     "modules",
			query:    "SHOW MODULES",
			knownGap: "module Source (FromAppStore/version) not yet converted; System aggregate counts injected by legacy",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			legacy, err := Run(Legacy, fixture, tc.query)
			if err != nil {
				t.Fatalf("legacy: %v", err)
			}
			modelsdk, err := Run(ModelSDK, fixture, tc.query)
			if err != nil {
				t.Fatalf("modelsdk: %v", err)
			}
			lr := NormalizeTable(legacy, tc.keep)
			mr := NormalizeTable(modelsdk, tc.keep)
			equal := slices.Equal(lr, mr)

			if tc.knownGap != "" {
				if equal {
					t.Logf("known-gap case now MATCHES — consider promoting %q to strict", tc.name)
				} else {
					t.Logf("known gap (expected divergence): %s", tc.knownGap)
				}
				return
			}
			if !equal {
				t.Errorf("read-parity divergence in %q (%d legacy rows, %d modelsdk rows):\n%s",
					tc.name, len(lr), len(mr), DiffRows(lr, mr))
			}
		})
	}
}
