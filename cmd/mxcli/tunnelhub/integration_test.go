// SPDX-License-Identifier: Apache-2.0

package tunnelhub

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	chclient "github.com/jpillora/chisel/client"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func mustPort(t *testing.T, rawURL string) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(rawURL[len("http://"):])
	if err != nil {
		t.Fatalf("split %q: %v", rawURL, err)
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("port %q: %v", portStr, err)
	}
	return p
}

// End-to-end, in-process: a client reverse-tunnels a backend to the hub's
// embedded chisel server, then a request to that backend's subdomain routes
// through the tunnel and returns the backend's response. Exercises registration
// -> subdomain routing -> chisel reverse tunnel -> app.
func TestFront_ProxiesThroughTunnel(t *testing.T) {
	// The "app" being previewed. It echoes the Host it sees, so we can assert the
	// front preserves the public host rather than the loopback backend host.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "TUNNELED host=%s", r.Host)
	}))
	defer backend.Close()
	backendPort := mustPort(t, backend.URL)

	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	reg := NewRegistry(RegistryOptions{Domain: "mxcli.org", PortBase: freePort(t), PortCount: 20, Now: clk.now})
	chiselPort := freePort(t)
	srv, err := NewServer(ServerOptions{
		Domain:       "mxcli.org",
		Registry:     reg,
		ChiselAddr:   "127.0.0.1:" + strconv.Itoa(chiselPort),
		CertCacheDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	// Start just the embedded chisel server (Start() would also bind TLS).
	if err := srv.chisel.Start("127.0.0.1", strconv.Itoa(chiselPort)); err != nil {
		t.Fatalf("chisel start: %v", err)
	}
	defer srv.chisel.Close()

	// Register the preview -> assigned reverse port.
	b, err := reg.Register(RegisterRequest{Project: "App", Branch: "main", AppPort: backendPort})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// The client tunnels the backend to the assigned reverse port.
	client, err := chclient.NewClient(&chclient.Config{
		Server:  "http://127.0.0.1:" + strconv.Itoa(chiselPort),
		Remotes: []string{fmt.Sprintf("R:%d:127.0.0.1:%d", b.ReversePort, backendPort)},
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("client start: %v", err)
	}
	defer client.Close()

	// A request to app.mxcli.org must route through the tunnel to the backend.
	deadline := time.Now().Add(10 * time.Second)
	var body string
	for time.Now().Before(deadline) {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req("app.mxcli.org", "/"))
		if rec.Code == http.StatusOK {
			body = rec.Body.String()
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if body == "" {
		t.Fatal("no successful response through the tunnel within timeout")
	}
	if want := "TUNNELED host=app.mxcli.org"; body != want {
		t.Errorf("through-tunnel body = %q, want %q (public Host must be preserved)", body, want)
	}
}
