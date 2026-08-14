// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"testing"
)

// proxyForURL must route an external hub through the egress proxy but connect to
// a loopback / NO_PROXY hub directly — otherwise a local hub (or an allow-listed
// one) would be forced through a proxy that refuses it.
func TestProxyForURL(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:33451")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:33451")
	t.Setenv("NO_PROXY", "127.0.0.1,localhost,.internal.example")

	cases := []struct {
		url  string
		want string
	}{
		{"https://hub.example.com", "http://127.0.0.1:33451"},
		{"http://127.0.0.1:9500", ""},          // loopback → NO_PROXY
		{"https://relay.internal.example", ""}, // suffix match in NO_PROXY
		{"", ""},                               // no URL
		{"://bad", ""},                         // unparseable
	}
	for _, c := range cases {
		if got := proxyForURL(c.url); got != c.want {
			t.Errorf("proxyForURL(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}
