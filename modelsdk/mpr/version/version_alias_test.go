// SPDX-License-Identifier: Apache-2.0

package version_test

// ProjectVersion must be an ALIAS of types.ProjectVersion, not a struct that
// happens to match it field for field.
//
// The distinction is invisible in an error message and that is the whole point:
// two identically-named types from different packages both print as
// "version.ProjectVersion", so a mismatch reads as a tautology. The same shape
// cost a session once already, on widget BSON, where a delegation handed back a
// v2 bson.D to a caller asserting the v1 one and the failure read
// "widget type is bson.D, want bson.D".
//
// The deleted sdk/mpr/version aliased the canonical type; this copy declared its
// own, which is what made the two engines' version values non-interchangeable.

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/mpr/version"
)

// Compile-time: only an alias satisfies this. A duplicate struct — even one
// whose fields match exactly — fails to build here.
var _ *types.ProjectVersion = (*version.ProjectVersion)(nil)

func TestProjectVersionIsTheCanonicalType(t *testing.T) {
	// Assignable in both directions without conversion, which is what callers
	// crossing the mdl/ ↔ modelsdk/ boundary actually need.
	var fromTypes *types.ProjectVersion = version.DefaultVersion()
	if fromTypes == nil {
		t.Fatal("DefaultVersion returned nil")
	}
	var back *version.ProjectVersion = fromTypes
	if back.ProductVersion != fromTypes.ProductVersion {
		t.Fatalf("round trip changed the value: %q vs %q", back.ProductVersion, fromTypes.ProductVersion)
	}
}

// The behaviour the alias must not change. These ran against the local struct
// before the alias and must give the same answers after it, or the two
// declarations were not equivalent after all.
func TestProjectVersionBehaviourIsUnchanged(t *testing.T) {
	v := &version.ProjectVersion{MajorVersion: 11, MinorVersion: 6, PatchVersion: 2, FormatVersion: 2, ProductVersion: "11.6.2"}

	for _, c := range []struct {
		name string
		got  bool
		want bool
	}{
		{"IsAtLeast lower major", v.IsAtLeast(10, 0), true},
		{"IsAtLeast same major lower minor", v.IsAtLeast(11, 5), true},
		{"IsAtLeast same major same minor", v.IsAtLeast(11, 6), true},
		{"IsAtLeast same major higher minor", v.IsAtLeast(11, 7), false},
		{"IsAtLeast higher major", v.IsAtLeast(12, 0), false},
		{"IsAtLeastFull same patch", v.IsAtLeastFull(11, 6, 2), true},
		{"IsAtLeastFull higher patch", v.IsAtLeastFull(11, 6, 3), false},
		{"IsAtLeastFull lower minor", v.IsAtLeastFull(11, 5, 9), true},
		{"IsMPRv2", v.IsMPRv2(), true},
	} {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if v.String() != "11.6.2" {
		t.Errorf("String() = %q, want 11.6.2", v.String())
	}
	if v1 := (&version.ProjectVersion{FormatVersion: 1}); v1.IsMPRv2() {
		t.Error("FormatVersion 1 reported as MPRv2")
	}
}
