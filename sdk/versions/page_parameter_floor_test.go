// SPDX-License-Identifier: Apache-2.0

package versions

import "testing"

// The page-parameter family's version floors, measured against the Mendix Model
// SDK's own StructureVersionInfo records (mendixmodelsdk 4.115.0, src/gen/pages.js):
//
//	Pages$PageParameter / Page.parameters      introduced 9.4.0
//	Pages$PageSettings.parameterMappings       introduced 9.7.0
//	Pages$LocalVariable                        introduced 10.17.0
//	Pages$LocalVariable.defaultValue           introduced 10.20.0
//	Pages$PageParameter.isRequired             introduced 11.5.0
//	Pages$PageParameter.defaultValue           introduced 11.5.0
//
// The registry previously put all three features at 11.0.0, which refused
// `CREATE PAGE ... (Params: ...)` on every Mendix 10 project (mendixlabs/mxcli#1121).
// That figure matched the illustrative sample output in
// docs/11-proposals/PROPOSAL_version_aware_agent_support.md and nothing else.
//
// Each row has its floor and the version just below it, so a regression that
// re-raises a floor fails here rather than passing vacuously.
func TestPageParameterFamilyFloors(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	tests := []struct {
		area, name string
		version    SemVer
		want       bool
	}{
		// Page parameters: 9.4.0.
		{"pages", "page_parameters", SemVer{9, 3, 0}, false},
		{"pages", "page_parameters", SemVer{9, 4, 0}, true},
		{"pages", "page_parameters", SemVer{10, 0, 0}, true},
		{"pages", "page_parameters", SemVer{10, 24, 25}, true}, // the reported project
		{"pages", "page_parameters", SemVer{11, 0, 0}, true},

		// Passing an argument to one: 9.7.0. Without this a parameterised page
		// is unreachable, which is the other half of #1121.
		{"microflows", "show_page_with_params", SemVer{9, 6, 0}, false},
		{"microflows", "show_page_with_params", SemVer{9, 7, 0}, true},
		{"microflows", "show_page_with_params", SemVer{10, 24, 25}, true},

		// Page variables: the element is 10.17.0 but its DefaultValue is 10.20.0,
		// and MDL always writes a default expression, so the floor is 10.20.0.
		{"pages", "page_variables", SemVer{10, 19, 0}, false},
		{"pages", "page_variables", SemVer{10, 20, 0}, true},
		{"pages", "page_variables", SemVer{11, 0, 0}, true},
	}
	for _, tt := range tests {
		if got := r.IsAvailable(tt.area, tt.name, tt.version); got != tt.want {
			t.Errorf("IsAvailable(%s, %s, %v) = %v, want %v",
				tt.area, tt.name, tt.version, got, tt.want)
		}
	}
}

// A feature a 10.x project already has must not be advertised as something an
// upgrade to 11 would unlock. page_parameters was the headline row of that list.
func TestPageParametersNotAnUpgradeOpportunity(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	for _, f := range r.FeaturesAddedSince(SemVer{10, 24, 0}) {
		if f.Area == "pages" && f.Name == "page_parameters" {
			t.Errorf("page_parameters reported as added since 10.24 (min_version %v); "+
				"Mendix has had it since 9.4.0", f.MinVersion)
		}
	}
}
