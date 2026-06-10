// SPDX-License-Identifier: Apache-2.0

package widgetobj

import "testing"

// TestCanonicalSelectionValue locks in the PascalCase normalisation of
// selection-enum values. MDL accepts any case (`selection: single`); the
// BSON writer must store `Single` (or `Multi` / `None`) so the gallery's
// embedded WidgetType matches what Studio Pro 11.9 expects. Lowercase
// values contribute to CE0463 widget-definition drift.
func TestCanonicalSelectionValue(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"single", "Single"},
		{"Single", "Single"},
		{"SINGLE", "Single"},
		{"multi", "Multi"},
		{"multiple", "Multi"},
		{"Multi", "Multi"},
		{"none", "None"},
		{"None", "None"},
		{"", ""},
		// Unknown values pass through unchanged — defer to runtime.
		{"weird", "weird"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := canonicalSelectionValue(tc.in); got != tc.want {
				t.Errorf("canonicalSelectionValue(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
