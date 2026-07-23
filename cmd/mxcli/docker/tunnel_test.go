// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	chserver "github.com/jpillora/chisel/server"
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

// StartTunnel must reverse-tunnel a local port out to a hub so requests to the
// hub's reverse port reach the local app. This exercises the real embedded
// chisel client + server end to end, in-process (no external binary).
func TestTunnelRoundTrip(t *testing.T) {
	// The "local app" the tunnel exposes.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "hello from %s", r.Host)
	}))
	defer backend.Close()
	backendPort := mustPort(t, backend.URL)

	// The hub: a chisel reverse server on a free port.
	hubPort := freePort(t)
	srv, err := chserver.NewServer(&chserver.Config{Reverse: true})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := srv.Start("127.0.0.1", strconv.Itoa(hubPort)); err != nil {
		t.Fatalf("hub Start: %v", err)
	}
	defer srv.Close()

	// The reverse port the hub will open and forward to the backend.
	remotePort := freePort(t)

	tun, err := StartTunnel(TunnelOptions{
		HubURL:     "http://127.0.0.1:" + strconv.Itoa(hubPort),
		LocalPort:  backendPort,
		RemotePort: remotePort,
		Proxy:      "", // loopback: no proxy
		Stdout:     io.Discard,
	})
	if err != nil {
		t.Fatalf("StartTunnel: %v", err)
	}
	defer tun.Stop()

	if tun.PublicURL() != "http://127.0.0.1:"+strconv.Itoa(hubPort) {
		t.Errorf("PublicURL = %q", tun.PublicURL())
	}

	// The client connects + opens the reverse listener asynchronously; poll it.
	reverseURL := fmt.Sprintf("http://127.0.0.1:%d/", remotePort)
	body := pollGet(t, reverseURL, 10*time.Second)
	if body == "" {
		t.Fatalf("no response through tunnel at %s", reverseURL)
	}
	// The request reached the backend through the tunnel.
	if want := "hello from "; len(body) < len(want) || body[:len(want)] != want {
		t.Errorf("through-tunnel body = %q, want prefix %q", body, want)
	}
}

func mustPort(t *testing.T, rawURL string) int {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %q: %v", rawURL, err)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("port of %q: %v", rawURL, err)
	}
	return p
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func pollGet(t *testing.T, url string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return string(b)
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return ""
}
