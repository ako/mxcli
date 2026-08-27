// SPDX-License-Identifier: Apache-2.0

package types

import "strings"

// webProfileKinds are the Navigation$NavigationProfile kinds a WEB profile can
// take. Mendix's set is fixed and closed — a navigation profile is not a
// user-named thing — which is why creating one takes a kind rather than a name.
//
// The Offline kinds (ResponsiveOffline, PhoneOffline, TabletOffline) and the
// native profiles (Navigation$NativeNavigationProfile, a different $Type) are
// deliberately absent: no reference document was available for either, and a
// profile assembled from a guess is the failure mode that builds clean and then
// will not open in Studio Pro.
var webProfileKinds = map[string]string{
	"responsive": "Responsive",
	"phone":      "Phone",
	"tablet":     "Tablet",
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

// WebProfileKindNames lists the creatable kinds for error messages, in the order
// Studio Pro's own Add Navigation Profile dialog offers them.
func WebProfileKindNames() []string { return []string{"Responsive", "Phone", "Tablet"} }
