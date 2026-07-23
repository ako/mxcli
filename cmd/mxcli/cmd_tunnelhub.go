// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mendixlabs/mxcli/cmd/mxcli/tunnelhub"
	"github.com/spf13/cobra"
)

// tunnelHubCmd runs the multi-tenant ingress relay. It fronts many locally-running
// Mendix apps — across projects, solutions, branches, and worktrees — each at its
// own <subdomain>.<domain> over a single 443 connection, with a registration API
// and an admin overview at the hub host. Apps stay in their own (possibly
// egress-only) environments and reverse-tunnel out; nothing is pushed here.
var tunnelHubCmd = &cobra.Command{
	Use:   "tunnel-hub",
	Short: "Multi-tenant ingress relay: front many locally-running mxcli apps at per-preview subdomains",
	Long: `Run the static ingress relay that exposes locally-running Mendix apps
(started elsewhere with 'mxcli run --hub <this-url>') in a browser.

Each app self-registers and is served at its own subdomain
(<project>-<branch>.<domain>, or <prefix>-<project>-<branch> with --hub-prefix);
the hub host (hub.<domain>) serves the registration API, the admin overview, and
the chisel control connection. Everything rides one 443 connection, so apps in
egress-only environments (e.g. Claude Code on the web) can reverse-tunnel out.

You run your own hub — there is no hosted service. Stand it up on a host you
control (a small VPS with a domain).

DNS: point a wildcard '*.<domain>' A record (and 'hub.<domain>') at this host.
TLS is issued per subdomain via Let's Encrypt on demand (needs inbound 80+443).

Security: set --secret and keep the hub to people you trust — this version uses a
single shared secret and open registration, so anyone with it can register a
preview (per-tenant auth is a follow-up). Don't expose it to the public.

Example (on your own VPS; *.example.com -> this host, ports 80+443 open):
  mxcli tunnel-hub --domain example.com --secret alice:s3cret

Then, in each app's environment:
  mxcli run --hub https://hub.example.com --hub-secret alice:s3cret \
    --hub-solution CustomerPortal -p app.mpr
`,
	Run: func(cmd *cobra.Command, args []string) {
		domain, _ := cmd.Flags().GetString("domain")
		hubHost, _ := cmd.Flags().GetString("hub-host")
		secret, _ := cmd.Flags().GetString("secret")
		httpsPort, _ := cmd.Flags().GetInt("port")
		httpPort, _ := cmd.Flags().GetInt("http-port")
		certCache, _ := cmd.Flags().GetString("cert-cache")

		if domain == "" {
			fmt.Fprintln(os.Stderr, "Error: --domain is required (the wildcard base, e.g. example.com)")
			os.Exit(1)
		}
		if certCache == "" {
			home, _ := os.UserHomeDir()
			certCache = filepath.Join(home, ".mxcli", "hub-certs")
		}

		reg := tunnelhub.NewRegistry(tunnelhub.RegistryOptions{Domain: domain})
		srv, err := tunnelhub.NewServer(tunnelhub.ServerOptions{
			Domain:         domain,
			HubHost:        hubHost,
			Registry:       reg,
			TunnelAuth:     secret,
			RegisterSecret: secret,
			CertCacheDir:   certCache,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: configuring tunnel-hub: %v\n", err)
			os.Exit(1)
		}

		host := hubHost
		if host == "" {
			host = "hub." + domain
		}
		fmt.Printf("mxcli tunnel-hub: serving *.%s (control/admin at https://%s) on :%d\n", domain, host, httpsPort)
		fmt.Printf("  admin overview: https://%s/\n", host)

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if err := srv.Start(ctx, fmt.Sprintf(":%d", httpsPort), fmt.Sprintf(":%d", httpPort)); err != nil {
			fmt.Fprintf(os.Stderr, "Error: tunnel-hub: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	tunnelHubCmd.Flags().String("domain", "", "Wildcard base domain you control, e.g. example.com (previews served at <sub>.<domain>)")
	tunnelHubCmd.Flags().String("hub-host", "", "Control/admin host (default hub.<domain>)")
	tunnelHubCmd.Flags().String("secret", "", "Shared secret (\"user:pass\") apps present via --hub-secret; empty = open")
	tunnelHubCmd.Flags().Int("port", 443, "HTTPS port to listen on")
	tunnelHubCmd.Flags().Int("http-port", 80, "HTTP port for ACME challenges + http->https redirect")
	tunnelHubCmd.Flags().String("cert-cache", "", "Directory for Let's Encrypt certificates (default ~/.mxcli/hub-certs)")
	rootCmd.AddCommand(tunnelHubCmd)
}
