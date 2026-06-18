// SPDX-License-Identifier: Apache-2.0

package mcp

import "testing"

func TestDialFor(t *testing.T) {
	cases := []struct {
		name       string
		host       string
		dockerable bool
		want       string
	}{
		{"localhost in container → docker gateway", "localhost:7784", true, "host.docker.internal:7784"},
		{"localhost on host → localhost (Issue 8)", "localhost:7784", false, "localhost:7784"},
		{"127.0.0.1 on host → unchanged", "127.0.0.1:7782", false, "127.0.0.1:7782"},
		{"explicit host always passthrough", "myhost:9000", true, "myhost:9000"},
		{"localhost no port in container", "localhost", true, "host.docker.internal:80"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dialFor(c.host, c.dockerable); got != c.want {
				t.Errorf("dialFor(%q, %v) = %q, want %q", c.host, c.dockerable, got, c.want)
			}
		})
	}
}
