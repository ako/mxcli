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

		// Each device kind has an offline twin. Measured over PED on Studio Pro
		// 11.14: a freshly created PhoneOffline stores the same keys as a Phone,
		// so it is the same builder rather than a guessed second shape.
		{"PhoneOffline", "PhoneOffline", true},
		{"tabletoffline", "TabletOffline", true},
		{"ResponsiveOffline", "ResponsiveOffline", true},

		// The Hybrid and Native kinds are real Mendix values but remain
		// uncreatable: no reference document was available for either, and a
		// guessed profile builds clean and will not open in Studio Pro.
		{"HybridPhone", "", false},
		{"NativePhone", "", false},
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

func TestIsOfflineProfileKind(t *testing.T) {
	// Creating an offline profile is what makes CE6206 apply to every page it
	// reaches, so the executor has to be able to tell the two apart. Mendix's own
	// enum names each offline kind after its online twin, which is why this is a
	// suffix test and not a second table to forget a kind in.
	for _, tc := range []struct {
		kind    string
		offline bool
	}{
		{"Responsive", false},
		{"Phone", false},
		{"Tablet", false},
		{"ResponsiveOffline", true},
		{"PhoneOffline", true},
		{"TabletOffline", true},
	} {
		if got := IsOfflineProfileKind(tc.kind); got != tc.offline {
			t.Errorf("IsOfflineProfileKind(%q) = %v, want %v", tc.kind, got, tc.offline)
		}
	}
}

func TestEveryOnlineKindHasAnOfflineTwin(t *testing.T) {
	// The pairing is Mendix's, not mxcli's: adding an online kind without its
	// offline twin would silently make one of the six uncreatable.
	for _, k := range WebProfileKindNames() {
		if IsOfflineProfileKind(k) {
			continue
		}
		if _, ok := CanonicalProfileKind(k + "Offline"); !ok {
			t.Errorf("%s is creatable but %sOffline is not", k, k)
		}
	}
}
