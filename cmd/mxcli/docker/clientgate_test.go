// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientBundlePresent(t *testing.T) {
	dir := t.TempDir()
	if clientBundlePresent(dir) {
		t.Fatal("empty deploy dir should not report a bundle")
	}
	distDir := filepath.Join(dir, "web", "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.js"), []byte("//"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !clientBundlePresent(dir) {
		t.Fatal("should report the bundle present once web/dist/index.js exists")
	}
}

func TestClientBundleServed(t *testing.T) {
	code := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dist/index.js" {
			w.WriteHeader(code)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	appURL := srv.URL + "/"

	code = http.StatusOK
	if !clientBundleServed(appURL) {
		t.Fatal("a 200 on /dist/index.js should count as served")
	}
	code = http.StatusNotFound
	if clientBundleServed(appURL) {
		t.Fatal("a 404 on /dist/index.js must not count as served (the <noscript> shell case)")
	}
}

// TestEnsureClientServed_NoRecoveryWhenServed: when the bundle is present and
// served, ensureClientServed must not attempt a re-bundle. We prove it by passing
// a bogus mxbuild path — if recovery ran, BuildWebClient would fail and surface.
func TestEnsureClientServed_NoRecoveryWhenServed(t *testing.T) {
	dir := t.TempDir()
	distDir := filepath.Join(dir, "web", "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.js"), []byte("//"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dist/index.js" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if err := ensureClientServed(dir, srv.URL+"/", "/nonexistent/mxbuild", io.Discard); err != nil {
		t.Fatalf("a present+served bundle should need no recovery, got: %v", err)
	}
}

// TestEnsureClientServed_RecoversWhenNotServed: when /dist/index.js is not served,
// ensureClientServed must take the recovery branch (the synchronous one-shot
// bundle). Here that bundle fails cleanly (temp dir has no rollup.config.mjs),
// which both proves the branch ran and that the failure is reported.
func TestEnsureClientServed_RecoversWhenNotServed(t *testing.T) {
	old := clientProbeWindow
	clientProbeWindow = 150 * time.Millisecond
	defer func() { clientProbeWindow = old }()

	dir := t.TempDir() // no web/dist, no web/rollup.config.mjs
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // never served
	}))
	defer srv.Close()

	err := ensureClientServed(dir, srv.URL+"/", "/nonexistent/mxbuild", io.Discard)
	if err == nil {
		t.Fatal("expected recovery to run and fail (no rollup.config.mjs)")
	}
	if !strings.Contains(err.Error(), "re-bundle") {
		t.Fatalf("expected a re-bundle error (recovery branch), got: %v", err)
	}
}
