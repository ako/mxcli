package tunnelhub

import "testing"

func TestSafeReturnRejectsLookalikeDomains(t *testing.T) {
	cases := []struct {
		name, hubHost, cookieDomain, ret string
		wantSafe                         bool
	}{
		// Default wiring: buildHubAuth sets CookieDomain = "." + domain.
		{"dotted/own host", "hub.mxcli.org", ".mxcli.org", "https://hub.mxcli.org/", true},
		{"dotted/subdomain", "hub.mxcli.org", ".mxcli.org", "https://app.mxcli.org/", true},
		{"dotted/lookalike", "hub.mxcli.org", ".mxcli.org", "https://evil-mxcli.org/", false},
		{"dotted/foreign", "hub.mxcli.org", ".mxcli.org", "https://evil.com/", false},

		// Operator passed --cookie-domain without a leading dot.
		{"undotted/own host", "hub.mxcli.org", "mxcli.org", "https://hub.mxcli.org/", true},
		{"undotted/subdomain", "hub.mxcli.org", "mxcli.org", "https://app.mxcli.org/", true},
		{"undotted/LOOKALIKE", "hub.mxcli.org", "mxcli.org", "https://evil-mxcli.org/", false},
		{"undotted/SUFFIX", "hub.mxcli.org", "mxcli.org", "https://notmxcli.org/", false},

		// Scheme and shape.
		{"javascript", "hub.mxcli.org", ".mxcli.org", "javascript:alert(1)", false},
		{"protocol-relative", "hub.mxcli.org", ".mxcli.org", "//evil.com/", false},
		{"userinfo", "hub.mxcli.org", ".mxcli.org", "https://hub.mxcli.org@evil.com/", false},
		{"http downgrade", "hub.mxcli.org", ".mxcli.org", "http://hub.mxcli.org/", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := &AuthConfig{HubHost: c.hubHost, CookieDomain: c.cookieDomain}
			got := a.safeReturn(c.ret)
			if got != c.wantSafe {
				t.Errorf("safeReturn(%q) with CookieDomain=%q = %v, want %v",
					c.ret, c.cookieDomain, got, c.wantSafe)
			}
		})
	}
}
