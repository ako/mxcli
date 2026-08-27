// SPDX-License-Identifier: Apache-2.0

package types

import "strings"

// webProfileKinds are the Navigation$NavigationProfile kinds a WEB profile can
// take. Mendix's set is fixed and closed — a navigation profile is not a
// user-named thing — which is why creating one takes a kind rather than a name.
//
// Each device kind has an offline twin. An offline profile is NOT a different
// document: measured on Studio Pro 11.14, a freshly added PhoneOffline stores
// exactly the same keys as a Phone, differing only in Kind and Name. What
// changes is what the platform then demands of every page the profile reaches —
// see the CE6206 warning MDL-OFFLINE01 raises.
//
// The native profiles (Navigation$NativeNavigationProfile, a different $Type)
// and the Hybrid kinds are deliberately absent: no reference document was
// available for either, and a profile assembled from a guess is the failure mode
// that builds clean and then will not open in Studio Pro.
var webProfileKinds = map[string]string{
	"responsive":        "Responsive",
	"phone":             "Phone",
	"tablet":            "Tablet",
	"responsiveoffline": "ResponsiveOffline",
	"phoneoffline":      "PhoneOffline",
	"tabletoffline":     "TabletOffline",
}

// CanonicalProfileKind maps a user-written profile name to the Kind Mendix
// stores, reporting whether it is one mxcli can create.
//
// It lives here rather than beside the writer because both sides need it and
// they must not know about each other: the executor decides whether a missing
// profile is creatable, and the backend builds it (ADR-0002 — the executor
// speaks to the backend interface, never to an implementation).
func CanonicalProfileKind(name string) (string, bool) {
	k, ok := webProfileKinds[strings.ToLower(strings.TrimSpace(name))]
	return k, ok
}

// IsOfflineProfileKind reports whether a canonical kind is an offline profile.
//
// It is a suffix test rather than a second table because that is what Mendix's
// own enum is: every offline kind is its online twin's name plus "Offline". A
// list here would be a second place to forget a kind.
func IsOfflineProfileKind(kind string) bool {
	return strings.HasSuffix(kind, "Offline")
}

// WebProfileKindNames lists the creatable kinds for error messages, online kinds
// first so the common case reads first.
func WebProfileKindNames() []string {
	return []string{"Responsive", "Phone", "Tablet", "ResponsiveOffline", "PhoneOffline", "TabletOffline"}
}
