// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// TestCarriedApplyEntityAccess pins the three-way rule, and the middle row is
// the bug: an unstated rewrite must PRESERVE, not default to false.
//
// "Apply entity access" makes a microflow run under the current user's access
// rules, so clearing it WIDENS what the microflow may read and write — with
// `mxcli check` quiet, mxbuild quiet, and the model valid either way. Measured
// across 342 microflows in 4 projects: every microflow storing true came back
// false, because both writers hardcoded it.
func TestCarriedApplyEntityAccess(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name   string
		stated *bool
		stored bool
		want   bool
	}{
		{"unstated rewrite preserves a stored true", nil, true, true},
		{"unstated rewrite preserves a stored false", nil, false, false},
		{"@applyentityaccess sets it", &yes, false, true},
		{"@applyentityaccess(false) clears it", &no, true, false},
		// A CREATE has nothing stored, so a fresh microflow gets Studio Pro's
		// default (326 of 342 in the corpus are false).
		{"fresh microflow defaults off", nil, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := carriedApplyEntityAccess(tc.stated, tc.stored); got != tc.want {
				t.Errorf("carriedApplyEntityAccess(%v, %v) = %v, want %v",
					tc.stated, tc.stored, got, tc.want)
			}
		})
	}
}
