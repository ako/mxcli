// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	chclient "github.com/jpillora/chisel/client"
	"golang.org/x/net/http/httpproxy"
)

// DefaultHubBackendPort is the port an mxcli tunnel-hub proxies public requests
// to (its chisel server's --backend), and the reverse port the client tunnels
// into. The two must agree; both default to 9000.
const DefaultHubBackendPort = 9000

// TunnelOptions configures an outbound chisel reverse tunnel from a locally
// running app to an mxcli tunnel-hub, so the app is reachable in a browser at
// the hub's public URL. The app never leaves this machine — only live HTTP flows
// through the tunnel.
type TunnelOptions struct {
	// HubURL is the tunnel-hub base URL, e.g. https://hub.mxcli.org. The chisel
	// control connection dials it over 443.
	HubURL string
	// LocalPort is the local app port to expose (e.g. 8080).
	LocalPort int
	// RemotePort is the hub's chisel-server reverse port, which the hub proxies
	// public traffic to (must match the hub's --backend port). Default 9000.
	RemotePort int
	// Secret is the shared chisel auth ("user:pass"), matching the hub's --secret.
	// Optional but recommended.
	Secret string
	// Proxy is the outbound HTTP CONNECT proxy the control connection dials
	// through. In a Claude Code web session egress is proxy-only, so this must be
	// set; it defaults from HTTPS_PROXY/https_proxy in the environment. chisel does
	// not read the proxy env itself, so we pass it explicitly.
	Proxy string
	// PublicURL is the browser-facing URL the app is served at (an assigned
	// subdomain on a multi-tenant hub). Defaults to HubURL when empty.
	PublicURL string
	// Stdout receives progress messages (default os.Stdout).
	Stdout io.Writer
}

// Tunnel is a running reverse tunnel to a hub.
type Tunnel struct {
	client    *chclient.Client
	cancel    context.CancelFunc
	publicURL string
}

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
// directly. chisel does not consult the proxy env itself, so we do it here.
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

// StartTunnel opens the reverse tunnel and returns once the chisel client has
// started connecting (it retries in the background until the process exits).
// Call Stop to tear it down.
func StartTunnel(o TunnelOptions) (*Tunnel, error) {
	o.applyDefaults()
	if o.HubURL == "" {
		return nil, fmt.Errorf("hub URL is required")
	}
	if o.LocalPort == 0 {
		return nil, fmt.Errorf("local app port is required")
	}

	// R:<remote>:127.0.0.1:<local> — the hub's chisel server listens on <remote>
	// and forwards to this app's <local> port (the app binds 127.0.0.1).
	remote := fmt.Sprintf("R:%d:127.0.0.1:%d", o.RemotePort, o.LocalPort)
	cfg := &chclient.Config{
		Server:           o.HubURL,
		Proxy:            o.Proxy,
		Auth:             o.Secret,
		Remotes:          []string{remote},
		KeepAlive:        25 * time.Second,
		MaxRetryCount:    -1, // retry forever: survive a hub restart or network blip
		MaxRetryInterval: 30 * time.Second,
	}
	c, err := chclient.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("configuring tunnel client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := c.Start(ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("starting tunnel: %w", err)
	}

	public := o.PublicURL
	if public == "" {
		public = o.HubURL
	}
	t := &Tunnel{client: c, cancel: cancel, publicURL: strings.TrimRight(public, "/")}
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
	if t.cancel != nil {
		t.cancel()
	}
	if t.client != nil {
		_ = t.client.Close()
	}
}
