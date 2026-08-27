// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

func TestCanonicalProfileKind(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		// Mendix's fixed web set, however the author cased it.
		{"Phone", "Phone", true},
		{"phone", "Phone", true},
		{"  Tablet  ", "Tablet", true},
		{"RESPONSIVE", "Responsive", true},

		// An invented name is NOT creatable. This is the point of a closed set:
		// creating "Phone" adds the profile the platform routes to, while
		// creating "Mobile" would add one that can never route — the runtime
		// dispatches on User-Agent to Mendix's own kinds.
		{"Mobile", "", false},
		{"Desktop", "", false},
		{"", "", false},

		// Offline and native kinds are real Mendix values but are deliberately
		// not creatable here: no reference document was available for either, and
		// a guessed profile builds clean and will not open in Studio Pro.
		{"PhoneOffline", "", false},
		{"TabletOffline", "", false},
	}
	for _, tc := range tests {
		got, ok := CanonicalProfileKind(tc.in)
		if ok != tc.ok {
			t.Errorf("CanonicalProfileKind(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("CanonicalProfileKind(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWebProfileKindNamesMatchTheTable(t *testing.T) {
	// The list is what error messages offer the user, so it must not drift from
	// what is actually creatable.
	for _, n := range WebProfileKindNames() {
		if _, ok := CanonicalProfileKind(n); !ok {
			t.Errorf("%q is offered in WebProfileKindNames but is not creatable", n)
		}
	}
	if len(WebProfileKindNames()) != len(webProfileKinds) {
		t.Errorf("WebProfileKindNames has %d entries, table has %d",
			len(WebProfileKindNames()), len(webProfileKinds))
	}
}
