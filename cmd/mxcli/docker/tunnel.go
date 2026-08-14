// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"golang.org/x/net/http/httpproxy"
)

// DefaultHubBackendPort is the port an mxcli tunnel-hub proxies public requests
// to (its tunnel server's --backend), and the reverse port the client tunnels
// into. The two must agree; both default to 9000.
const DefaultHubBackendPort = 9000

// ErrTunnelUnsupported is what every tunnel entry point returns on a build that
// ships without the tunnel (everything but Linux — see tunnel_stub.go and
// ADR-0009). It is a single value so the early flag check in `mxcli run` and the
// run-time failure say exactly the same thing.
var ErrTunnelUnsupported = errors.New(
	"the browser preview tunnel (--hub) is only available in Linux builds of mxcli\n\n" +
		"--hub reverse-tunnels the running app out to a tunnel-hub, and that tunnel ships\n" +
		"only in the Linux build, because that is the only place it runs: inside the\n" +
		"container. Windows and macOS builds leave it out deliberately.\n\n" +
		"Run mxcli inside the project's devcontainer (or any Linux container) to use --hub.\n" +
		"Everything else about `mxcli run --local` works here unchanged.\n\n" +
		"See https://mendixlabs.github.io/mxcli/tools/run-local.html")

// TunnelOptions configures an outbound reverse tunnel from a locally running app
// to an mxcli tunnel-hub, so the app is reachable in a browser at the hub's
// public URL. The app never leaves this machine — only live HTTP flows through
// the tunnel.
type TunnelOptions struct {
	// HubURL is the tunnel-hub base URL, e.g. https://hub.example.com. The
	// control connection dials it over 443.
	HubURL string
	// LocalPort is the local app port to expose (e.g. 8080).
	LocalPort int
	// RemotePort is the hub's reverse port, which the hub proxies public traffic
	// to (must match the hub's --backend port). Default 9000.
	RemotePort int
	// Secret is the shared tunnel auth ("user:pass"), matching the hub's --secret.
	// Optional but recommended.
	Secret string
	// Proxy is the outbound HTTP CONNECT proxy the control connection dials
	// through. In a Claude Code web session egress is proxy-only, so this must be
	// set; it defaults from HTTPS_PROXY/https_proxy in the environment. The tunnel
	// client does not read the proxy env itself, so we pass it explicitly.
	Proxy string
	// PublicURL is the browser-facing URL the app is served at (an assigned
	// subdomain on a multi-tenant hub). Defaults to HubURL when empty.
	PublicURL string
	// Stdout receives progress messages (default os.Stdout).
	Stdout io.Writer
}

// tunnelConn is the platform half of the tunnel seam. Only the Linux build has
// an implementation; keeping it to one interface with one method is what stops
// build tags spreading past these three files.
type tunnelConn interface {
	// Close tears the tunnel down.
	Close()
}

// Tunnel is a running reverse tunnel to a hub.
type Tunnel struct {
	conn      tunnelConn
	publicURL string
}

// TunnelSupported reports whether this build can open a hub tunnel. Callers use
// it to reject --hub during flag validation, rather than booting a whole app and
// failing at the last step.
func TunnelSupported() bool { return tunnelSupported }

func (o *TunnelOptions) applyDefaults() {
	if o.RemotePort == 0 {
		o.RemotePort = DefaultHubBackendPort
	}
	if o.Proxy == "" {
		o.Proxy = proxyForURL(o.HubURL)
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
}

// proxyForURL resolves the outbound HTTP proxy for hubURL from the standard
// proxy environment (HTTPS_PROXY etc.), honouring NO_PROXY — so an external hub
// goes through the egress proxy while a loopback or allow-listed hub connects
// directly. The tunnel client does not consult the proxy env itself, so we do it
// here.
func proxyForURL(hubURL string) string {
	u, err := url.Parse(hubURL)
	if err != nil || u.Host == "" {
		return ""
	}
	p, err := httpproxy.FromEnvironment().ProxyFunc()(u)
	if err != nil || p == nil {
		return ""
	}
	return p.String()
}

// StartTunnel opens the reverse tunnel and returns once the client has started
// connecting (it retries in the background until the process exits). Call Stop to
// tear it down. On non-Linux builds it returns ErrTunnelUnsupported.
func StartTunnel(o TunnelOptions) (*Tunnel, error) {
	o.applyDefaults()
	if o.HubURL == "" {
		return nil, fmt.Errorf("hub URL is required")
	}
	if o.LocalPort == 0 {
		return nil, fmt.Errorf("local app port is required")
	}

	conn, err := startTunnel(o)
	if err != nil {
		return nil, err
	}

	public := o.PublicURL
	if public == "" {
		public = o.HubURL
	}
	t := &Tunnel{conn: conn, publicURL: strings.TrimRight(public, "/")}
	via := ""
	if o.Proxy != "" {
		via = " (via proxy)"
	}
	fmt.Fprintf(o.Stdout, "Tunnel: exposing local :%d at %s%s\n", o.LocalPort, t.publicURL, via)
	return t, nil
}

// PublicURL is the browser-reachable URL the app is served at through the hub.
func (t *Tunnel) PublicURL() string { return t.publicURL }

// Stop tears down the tunnel.
func (t *Tunnel) Stop() {
	if t == nil {
		return
	}
	if t.conn != nil {
		t.conn.Close()
	}
}
