// SPDX-License-Identifier: Apache-2.0

package tunnelhub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*Server, *Registry) {
	t.Helper()
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	reg := newTestRegistry(clk)
	srv, err := NewServer(ServerOptions{
		Domain:       "example.com",
		Registry:     reg,
		CertCacheDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv, reg
}

func req(host, target string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://"+host+target, nil)
	r.Host = host
	return r
}

func TestFront_HubHostServesAdmin(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req("hub.example.com", "/"))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "mxcli tunnel-hub") {
		t.Errorf("admin page: code=%d, body has hub title=%v", rec.Code, strings.Contains(rec.Body.String(), "mxcli tunnel-hub"))
	}
}

func TestFront_HubHostAPI(t *testing.T) {
	srv, reg := newTestServer(t)
	reg.Register(RegisterRequest{Project: "App", Branch: "main"})
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req("hub.example.com", "/api/backends"))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"project":"App"`) {
		t.Errorf("api/backends: code=%d body=%s", rec.Code, rec.Body)
	}
}

func TestFront_UnknownSubdomain(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req("ghost.example.com", "/"))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "No preview") {
		t.Errorf("unknown subdomain: code=%d body=%s", rec.Code, rec.Body)
	}
}

func TestFront_RegisteredButOffline(t *testing.T) {
	srv, reg := newTestServer(t)
	b, _ := reg.Register(RegisterRequest{Project: "App", Branch: "main"}) // subdomain "app"

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req("app.example.com", "/"))
	// Registered but its reverse port has no tunnel -> the proxy error handler
	// returns the offline page.
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "offline") {
		t.Errorf("offline preview: code=%d body=%s", rec.Code, rec.Body)
	}
	// The request still counted as usage.
	if v, ok := reg.LookupSubdomain(b.Subdomain); !ok || v.LastUsedAt.IsZero() {
		t.Error("a request to a preview should update LastUsedAt")
	}
}

func TestSubOf(t *testing.T) {
	srv, _ := newTestServer(t)
	cases := []struct {
		host string
		sub  string
		ok   bool
	}{
		{"app.example.com", "app", true},
		{"my-app-feat.example.com", "my-app-feat", true},
		{"hub.example.com", "hub", true}, // subOf is purely structural; routing handles hub separately
		{"a.b.example.com", "", false},   // multi-label not allowed
		{"example.com", "", false},
		{"evil.com", "", false},
	}
	for _, c := range cases {
		sub, ok := srv.subOf(c.host)
		if sub != c.sub || ok != c.ok {
			t.Errorf("subOf(%q) = (%q,%v), want (%q,%v)", c.host, sub, ok, c.sub, c.ok)
		}
	}
}

func TestStripPort(t *testing.T) {
	for in, want := range map[string]string{
		"app.example.com":      "app.example.com",
		"app.example.com:443":  "app.example.com",
		"hub.example.com:8443": "hub.example.com",
	} {
		if got := stripPort(in); got != want {
			t.Errorf("stripPort(%q) = %q, want %q", in, got, want)
		}
	}
}
